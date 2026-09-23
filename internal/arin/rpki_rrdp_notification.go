package arin

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"io"
	"math/big"
	"net/url"
	"slices"
	"strings"
)

type rrdpFileReference struct {
	URI  string
	Hash [sha256.Size]byte
}
type rrdpDeltaReference struct {
	Serial string
	File   rrdpFileReference
}
type rrdpNotification struct {
	Session, Serial string
	Snapshot        rrdpFileReference
	Deltas          []rrdpDeltaReference // Ascending serial order, independent of XML order.
}

func rrdpXMLDecoder(body []byte) (*xml.Decoder, error) {
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
	return decoder, nil
}

func parseRRDPNotification(body []byte) (*rrdpNotification, error) {
	if len(body) == 0 || len(body) > 4<<20 {
		return nil, errRPKIRRDP
	}
	decoder, err := rrdpXMLDecoder(body)
	if err != nil {
		return nil, err
	}
	out := &rrdpNotification{}
	depth := 0
	started, ended, snapshot := false, false, false
	seen := map[string]bool{}
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
				a, err := rrdpAttributes(t, "notification", "version", "session_id", "serial")
				if err != nil || a["version"] != "1" || !rrdpSession.MatchString(a["session_id"]) || !rrdpSerial.MatchString(a["serial"]) {
					return nil, errRPKIRRDP
				}
				out.Session, out.Serial = a["session_id"], a["serial"]
			case 1:
				if t.Name.Local == "snapshot" {
					if snapshot {
						return nil, errRPKIRRDP
					}
					a, err := rrdpAttributes(t, "snapshot", "uri", "hash")
					if err != nil {
						return nil, err
					}
					out.Snapshot, err = parseRRDPReference(a)
					if err != nil {
						return nil, err
					}
					snapshot = true
				} else {
					if !snapshot || len(out.Deltas) >= 10000 {
						return nil, errRPKIRRDP
					}
					a, err := rrdpAttributes(t, "delta", "uri", "hash", "serial")
					if err != nil || !rrdpSerial.MatchString(a["serial"]) || seen[a["serial"]] {
						return nil, errRPKIRRDP
					}
					ref, err := parseRRDPReference(a)
					if err != nil {
						return nil, err
					}
					seen[a["serial"]] = true
					out.Deltas = append(out.Deltas, rrdpDeltaReference{Serial: a["serial"], File: ref})
				}
			default:
				return nil, errRPKIRRDP
			}
			depth++
		case xml.EndElement:
			if depth == 1 {
				ended = true
			} else if depth != 2 {
				return nil, errRPKIRRDP
			}
			depth--
		case xml.CharData:
			if strings.Trim(string(t), " \t\r\n") != "" {
				return nil, errRPKIRRDP
			}
		case xml.Directive:
			return nil, errRPKIRRDP
		}
	}
	if !ended || depth != 0 || !snapshot {
		return nil, errRPKIRRDP
	}
	slices.SortFunc(out.Deltas, func(a, b rrdpDeltaReference) int {
		if len(a.Serial) < len(b.Serial) {
			return -1
		}
		if len(a.Serial) > len(b.Serial) {
			return 1
		}
		return strings.Compare(a.Serial, b.Serial)
	})
	if len(out.Deltas) > 0 {
		if out.Deltas[len(out.Deltas)-1].Serial != out.Serial {
			return nil, errRPKIRRDP
		}
		previous, _ := new(big.Int).SetString(out.Deltas[0].Serial, 10)
		for _, d := range out.Deltas[1:] {
			previous.Add(previous, big.NewInt(1))
			if previous.String() != d.Serial {
				return nil, errRPKIRRDP
			}
		}
	}
	return out, nil
}

func parseRRDPReference(a map[string]string) (rrdpFileReference, error) {
	var out rrdpFileReference
	s := a["uri"]
	if !validRRDPURL(s) {
		return out, errRPKIRRDP
	}
	hash, err := hex.DecodeString(a["hash"])
	if err != nil || len(hash) != sha256.Size {
		return out, errRPKIRRDP
	}
	out.URI = s
	copy(out.Hash[:], hash)
	return out, nil
}

func validRRDPURL(s string) bool {
	if !validRPKIProfileURI(s) {
		return false
	}
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Opaque == "" && !strings.Contains(s, "#")
}
