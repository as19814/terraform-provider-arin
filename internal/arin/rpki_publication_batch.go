package arin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"strconv"
	"strings"
)

// DER is opaque to publication: the caller supplies signed objects and an
// updated manifest in the same batch. OldSHA256 is absent only for creation.
type rpkiPublicationChange struct {
	URI, OldSHA256 string
	DER            []byte
	Withdraw       bool
}

var errRPKIPublicationBatch = errors.New("invalid RPKI publication batch")

func (c rpkiPublicationClient) Apply(ctx context.Context, changes []rpkiPublicationChange) error {
	if c.Exchange.MediaType != "application/rpki-publication" {
		return errRPKIPublicationBatch
	}
	request, err := buildRPKIPublicationBatch(changes)
	if err != nil {
		return err
	}
	var rejected *rpkiPublicationError
	_, err = c.Exchange.exchange(ctx, "publication-batch", request, func(query, reply []byte) error {
		var err error
		rejected, err = validateRPKIPublicationBatch(query, reply)
		return err
	})
	if err != nil {
		return err
	}
	if rejected != nil {
		return rejected
	}
	return nil
}

func buildRPKIPublicationBatch(changes []rpkiPublicationChange) ([]byte, error) {
	if len(changes) == 0 || len(changes) > 10000 {
		return nil, errRPKIPublicationBatch
	}
	var b bytes.Buffer
	b.WriteString(`<msg xmlns="` + rpkiPublicationNamespace + `" version="4" type="query">`)
	enc := xml.NewEncoder(&b)
	prefix := rand.Text()
	seen := make(map[string]bool)
	for i, c := range changes {
		if !publicationURI(c.URI) || seen[c.URI] || len(c.DER) > 3<<20 {
			return nil, errRPKIPublicationBatch
		}
		seen[c.URI] = true
		if c.OldSHA256 != "" {
			if _, err := hex.DecodeString(c.OldSHA256); err != nil || len(c.OldSHA256) != 64 {
				return nil, errRPKIPublicationBatch
			}
		}
		name := "publish"
		if c.Withdraw {
			if c.OldSHA256 == "" || len(c.DER) != 0 {
				return nil, errRPKIPublicationBatch
			}
			name = "withdraw"
		} else if len(c.DER) == 0 {
			return nil, errRPKIPublicationBatch
		}
		start := xml.StartElement{Name: xml.Name{Local: name}, Attr: []xml.Attr{
			{Name: xml.Name{Local: "tag"}, Value: prefix + "-" + strconv.Itoa(i)},
			{Name: xml.Name{Local: "uri"}, Value: c.URI},
		}}
		if c.OldSHA256 != "" {
			start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: "hash"}, Value: strings.ToLower(c.OldSHA256)})
		}
		if err := enc.EncodeElement(base64.StdEncoding.EncodeToString(c.DER), start); err != nil {
			return nil, errRPKIPublicationBatch
		}
		if b.Len() > 4<<20 {
			return nil, errRPKIPublicationBatch
		}
	}
	if err := enc.Flush(); err != nil {
		return nil, errRPKIPublicationBatch
	}
	b.WriteString(`</msg>`)
	if b.Len() > 4<<20 {
		return nil, errRPKIPublicationBatch
	}
	return b.Bytes(), nil
}

func validateRPKIPublicationBatch(query, reply []byte) (*rpkiPublicationError, error) {
	if len(reply) > 4<<20 {
		return nil, errRPKIPublicationReply
	}
	request, err := parseXML(query)
	if err != nil {
		return nil, errRPKIPublicationReply
	}
	expected := make(map[string]*xmlNode)
	indexes := make(map[string]int)
	for i, pdu := range request.Children {
		attrs, err := publicationAttrs(pdu, pdu.Name.Local, "tag", "uri", "hash")
		if err != nil {
			return nil, errRPKIPublicationReply
		}
		expected[attrs["tag"]] = pdu
		indexes[attrs["tag"]] = i
	}
	root, err := parseXML(reply)
	if err != nil {
		return nil, errRPKIPublicationReply
	}
	a, err := publicationAttrs(root, "msg", "type", "version")
	if err != nil || a["type"] != "reply" || a["version"] != "4" || strings.Trim(root.Text, " \t\r\n") != "" {
		return nil, errRPKIPublicationReply
	}
	if len(root.Children) == 1 && publicationEmpty(root.Children[0], "success") {
		return nil, nil
	}
	if len(root.Children) == 0 {
		return nil, errRPKIPublicationReply
	}
	var codes []string
	var operationIndexes []int
	for _, pdu := range root.Children {
		code, err := publicationError(pdu, func(tag string, failed *xmlNode) bool {
			want := expected[tag]
			// Untagged generic errors are permitted, but may not echo a mutation.
			if want == nil {
				return tag == "" && failed == nil
			}
			if failed == nil {
				return true
			}
			gotAttrs, err := publicationAttrs(failed, want.Name.Local, "tag", "uri", "hash")
			wantAttrs, _ := publicationAttrs(want, want.Name.Local, "tag", "uri", "hash")
			if err != nil || len(failed.Children) != 0 || len(gotAttrs) != len(wantAttrs) {
				return false
			}
			for k, v := range wantAttrs {
				if gotAttrs[k] != v {
					return false
				}
			}
			if want.Name.Local == "withdraw" {
				return strings.Trim(failed.Text, " \t\r\n") == ""
			}
			got, err := base64.StdEncoding.DecodeString(strings.Map(func(r rune) rune {
				if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
					return -1
				}
				return r
			}, failed.Text))
			return err == nil && base64.StdEncoding.EncodeToString(got) == want.Text
		})
		if err != nil {
			return nil, err
		}
		codes = append(codes, code)
		attrs, _ := publicationAttrs(pdu, "report_error", "error_code", "tag")
		index, ok := indexes[attrs["tag"]]
		if !ok {
			index = -1
		}
		operationIndexes = append(operationIndexes, index)
	}
	return &rpkiPublicationError{Codes: codes, OperationIndexes: operationIndexes}, nil
}
