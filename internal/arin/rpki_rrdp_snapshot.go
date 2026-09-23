package arin

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"strings"
)

const rrdpNamespace = "http://www.ripe.net/rpki/rrdp"

var rrdpSession = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-4[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
var rrdpSerial = regexp.MustCompile(`^[1-9][0-9]{0,127}$`)
var errRPKIRRDP = errors.New("invalid or inconsistent RPKI repository data")

// parseRRDPSnapshot verifies the notification's exact file digest and identity.
// Returned objects are transport data, not authenticated RPKI objects. Callers
// still need manifest/path validation and persistent session/serial tracking.
func parseRRDPSnapshot(body []byte, session, serial string, digest [sha256.Size]byte) (map[string][]byte, error) {
	changes, err := parseRRDPObjectFile(body, session, serial, digest, false)
	if err != nil {
		return nil, err
	}
	objects := make(map[string][]byte, len(changes))
	for _, c := range changes {
		if _, exists := objects[c.URI]; exists {
			return nil, errRPKIRRDP
		}
		objects[c.URI] = c.Data
	}
	return objects, nil
}

type rrdpChange struct {
	URI      string
	OldHash  *[sha256.Size]byte
	Withdraw bool
	Data     []byte
}

func parseRRDPObjectFile(body []byte, session, serial string, digest [sha256.Size]byte, delta bool) ([]rrdpChange, error) {
	if len(body) == 0 || len(body) > 128<<20 || !rrdpSession.MatchString(session) || !rrdpSerial.MatchString(serial) || sha256.Sum256(body) != digest {
		return nil, errRPKIRRDP
	}
	var changes []rrdpChange
	err := streamRRDPObjectFile(bytes.NewReader(body), session, serial, digest, delta,
		rrdpStreamLimits{Encoded: 128 << 20, Decoded: 64 << 20, Objects: 10000},
		func(change rrdpChange) error { changes = append(changes, change); return nil })
	if err != nil {
		return nil, err
	}
	return changes, nil
}

// The visitor receives provisional objects. It must stage them privately and
// discard every change unless this function returns nil after EOF and digest
// verification. No consumer may use a partial repository for path validation.
func streamRRDPObjectFile(input io.Reader, session, serial string, digest [sha256.Size]byte, delta bool, limits rrdpStreamLimits, visit func(rrdpChange) error) error {
	if input == nil || visit == nil || !rrdpSession.MatchString(session) || !rrdpSerial.MatchString(serial) || !limits.valid() {
		return errRPKIRRDP
	}
	hash := sha256.New()
	bounded := &rrdpStreamReader{input: io.TeeReader(input, hash), remaining: limits.Encoded}
	decoder := xml.NewDecoder(bounded)
	decoder.CharsetReader = func(label string, input io.Reader) (io.Reader, error) {
		if strings.EqualFold(label, "us-ascii") {
			return input, nil
		}
		return nil, errRPKIRRDP
	}
	count := 0
	depth, total := 0, 0
	started, ended := false, false
	var change rrdpChange
	rootName := "snapshot"
	if delta {
		rootName = "delta"
	}
	var content strings.Builder
	for {
		bounded.tokenRemaining = 8 << 20
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errRPKIRRDP
		}
		switch t := token.(type) {
		case xml.StartElement:
			if ended {
				return errRPKIRRDP
			}
			switch depth {
			case 0:
				if started {
					return errRPKIRRDP
				}
				started = true
				a, err := rrdpAttributes(t, rootName, "version", "session_id", "serial")
				if err != nil || a["version"] != "1" || a["session_id"] != session || a["serial"] != serial {
					return errRPKIRRDP
				}
			case 1:
				required := []string{"uri"}
				withdraw := delta && t.Name.Local == "withdraw"
				if withdraw {
					required = append(required, "hash")
				} else if delta {
					for _, a := range t.Attr {
						if a.Name.Space == "" && a.Name.Local == "hash" {
							required = append(required, "hash")
							break
						}
					}
				}
				name := "publish"
				if withdraw {
					name = "withdraw"
				}
				a, err := rrdpAttributes(t, name, required...)
				if err != nil || count >= limits.Objects || !publicationURI(a["uri"]) || strings.HasSuffix(a["uri"], "/") {
					return errRPKIRRDP
				}
				change = rrdpChange{URI: a["uri"], Withdraw: withdraw}
				if value, present := a["hash"]; present {
					hash, err := hex.DecodeString(value)
					if err != nil || len(hash) != sha256.Size {
						return errRPKIRRDP
					}
					h := [sha256.Size]byte(hash)
					change.OldHash = &h
				}
				content.Reset()
			default:
				return errRPKIRRDP
			}
			depth++
		case xml.EndElement:
			if depth == 2 {
				if change.Withdraw {
					if content.Len() != 0 {
						return errRPKIRRDP
					}
				} else {
					encoded := content.String()
					if base64.StdEncoding.DecodedLen(len(encoded)) > (4<<20)+2 {
						return errRPKIRRDP
					}
					data, err := base64.StdEncoding.Strict().DecodeString(encoded)
					if err != nil || len(data) == 0 || len(data) > 4<<20 {
						return errRPKIRRDP
					}
					total += len(data)
					if int64(total) > limits.Decoded {
						return errRPKIRRDP
					}
					change.Data = data
				}
				if err := visit(change); err != nil {
					return err
				}
				count++
			} else if depth == 1 {
				ended = true
			} else {
				return errRPKIRRDP
			}
			depth--
		case xml.CharData:
			if depth == 2 {
				for _, b := range t {
					if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
						continue
					}
					if b > 127 || content.Len() >= base64.StdEncoding.EncodedLen(4<<20) {
						return errRPKIRRDP
					}
					content.WriteByte(b)
				}
			} else if strings.Trim(string(t), " \t\r\n") != "" {
				return errRPKIRRDP
			}
		case xml.Directive:
			return errRPKIRRDP
		}
	}
	if !started || !ended || depth != 0 || (delta && count == 0) {
		return errRPKIRRDP
	}
	if !bytes.Equal(hash.Sum(nil), digest[:]) {
		return errRPKIRRDP
	}
	return nil
}

func rrdpAttributes(e xml.StartElement, name string, required ...string) (map[string]string, error) {
	if e.Name.Space != rrdpNamespace || e.Name.Local != name {
		return nil, errRPKIRRDP
	}
	out := make(map[string]string, len(required))
	for _, a := range e.Attr {
		if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
			continue
		}
		if a.Name.Space != "" {
			return nil, errRPKIRRDP
		}
		allowed := false
		for _, key := range required {
			if a.Name.Local == key {
				allowed = true
				break
			}
		}
		if _, exists := out[a.Name.Local]; exists || !allowed {
			return nil, errRPKIRRDP
		}
		out[a.Name.Local] = a.Value
	}
	if len(out) != len(required) {
		return nil, errRPKIRRDP
	}
	return out, nil
}
