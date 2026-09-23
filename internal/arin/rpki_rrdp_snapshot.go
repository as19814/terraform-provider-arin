package arin

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"strings"
)

const rrdpNamespace = "http://www.ripe.net/rpki/rrdp"

var rrdpSession = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-4[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
var rrdpSerial = regexp.MustCompile(`^[1-9][0-9]{0,127}$`)
var errRPKIRRDP = errors.New("invalid or inconsistent RPKI repository snapshot")

// parseRRDPSnapshot verifies the notification's exact file digest and identity.
// Returned objects are transport data, not authenticated RPKI objects. Callers
// still need manifest/path validation and persistent session/serial tracking.
func parseRRDPSnapshot(body []byte, session, serial string, digest [sha256.Size]byte) (map[string][]byte, error) {
	if len(body) == 0 || len(body) > 128<<20 || !rrdpSession.MatchString(session) || !rrdpSerial.MatchString(serial) || sha256.Sum256(body) != digest {
		return nil, errRPKIRRDP
	}
	for _, b := range body {
		if b > 127 {
			return nil, errRPKIRRDP
		}
	}
	decoder := xml.NewDecoder(bytes.NewReader(body))
	decoder.CharsetReader = func(label string, input io.Reader) (io.Reader, error) {
		if strings.EqualFold(label, "us-ascii") {
			return input, nil
		}
		return nil, errRPKIRRDP
	}
	objects := make(map[string][]byte)
	depth, total := 0, 0
	started, ended := false, false
	var uri string
	var content strings.Builder
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errRPKIRRDP
		}
		switch t := token.(type) {
		case xml.StartElement:
			if ended {
				return nil, errRPKIRRDP
			}
			switch depth {
			case 0:
				if started {
					return nil, errRPKIRRDP
				}
				started = true
				a, err := rrdpAttributes(t, "snapshot", "version", "session_id", "serial")
				if err != nil || a["version"] != "1" || a["session_id"] != session || a["serial"] != serial {
					return nil, errRPKIRRDP
				}
			case 1:
				a, err := rrdpAttributes(t, "publish", "uri")
				if err != nil || len(objects) >= 10000 || !publicationURI(a["uri"]) || strings.HasSuffix(a["uri"], "/") {
					return nil, errRPKIRRDP
				}
				uri = a["uri"]
				if _, exists := objects[uri]; exists {
					return nil, errRPKIRRDP
				}
				content.Reset()
			default:
				return nil, errRPKIRRDP
			}
			depth++
		case xml.EndElement:
			if depth == 2 {
				encoded := content.String()
				if base64.StdEncoding.DecodedLen(len(encoded)) > (4<<20)+2 {
					return nil, errRPKIRRDP
				}
				data, err := base64.StdEncoding.Strict().DecodeString(encoded)
				if err != nil || len(data) == 0 || len(data) > 4<<20 {
					return nil, errRPKIRRDP
				}
				total += len(data)
				if total > 64<<20 {
					return nil, errRPKIRRDP
				}
				objects[uri] = data
			} else if depth == 1 {
				ended = true
			} else {
				return nil, errRPKIRRDP
			}
			depth--
		case xml.CharData:
			if depth == 2 {
				for _, b := range t {
					if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
						continue
					}
					if b > 127 || content.Len() >= base64.StdEncoding.EncodedLen(4<<20) {
						return nil, errRPKIRRDP
					}
					content.WriteByte(b)
				}
			} else if strings.Trim(string(t), " \t\r\n") != "" {
				return nil, errRPKIRRDP
			}
		case xml.Directive:
			return nil, errRPKIRRDP
		}
	}
	if !started || !ended || depth != 0 {
		return nil, errRPKIRRDP
	}
	return objects, nil
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
