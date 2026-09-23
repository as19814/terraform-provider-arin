package arin

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func publicationTestReply(pdus string) string {
	return `<msg xmlns="` + rpkiPublicationNamespace + `" version="4" type="reply">` + pdus + `</msg>`
}

func TestRPKIPublicationListParsing(t *testing.T) {
	hash := strings.Repeat("AB", 32)
	item := `<list uri="rsync://repo.example/module/a.cer" hash="` + hash + `"/>`
	for _, body := range []string{publicationTestReply(""), publicationTestReply(item)} {
		got, rejection, err := parseRPKIPublicationList([]byte(body))
		if err != nil || rejection != nil || got == nil {
			t.Fatalf("valid inventory rejected: %v", err)
		}
		if strings.Contains(body, "a.cer") && (len(got) != 1 || got[0].SHA256 != strings.ToLower(hash) || got[0].URI != "rsync://repo.example/module/a.cer") {
			t.Fatalf("wrong objects: %#v", got)
		}
	}
	rejectionXML := `<report_error error_code="permission_failure"><error_text>secret remote detail</error_text><failed_pdu><list/></failed_pdu></report_error>`
	got, rejection, err := parseRPKIPublicationList([]byte(publicationTestReply(rejectionXML)))
	if err != nil || got != nil || rejection == nil || len(rejection.Codes) != 1 || rejection.Codes[0] != "permission_failure" || strings.Contains(rejection.Error(), "secret") {
		t.Fatal("incorrect protocol rejection")
	}
	for _, code := range []string{"xml_error", "bad_cms_signature", "object_already_present", "no_object_present", "no_object_matching_hash", "consistency_problem", "other_error"} {
		_, rejection, err := parseRPKIPublicationList([]byte(publicationTestReply(`<report_error error_code="` + code + `"/>`)))
		if err != nil || rejection == nil {
			t.Fatalf("valid error code rejected: %s", code)
		}
	}
	cases := map[string]string{
		"version":              strings.Replace(publicationTestReply(item), `version="4"`, `version="3"`, 1),
		"type":                 strings.Replace(publicationTestReply(item), `type="reply"`, `type="query"`, 1),
		"namespace":            strings.ReplaceAll(publicationTestReply(item), rpkiPublicationNamespace, "urn:foreign"),
		"duplicate_attribute":  strings.Replace(publicationTestReply(item), `type="reply"`, `type="reply" type="reply"`, 1),
		"unknown_attribute":    strings.Replace(publicationTestReply(item), `type="reply"`, `type="reply" extra="x"`, 1),
		"duplicate_object":     publicationTestReply(item + item),
		"foreign_child":        publicationTestReply(strings.Replace(item, "<list", `<list xmlns="urn:foreign"`, 1)),
		"missing_hash":         publicationTestReply(`<list uri="rsync://repo.example/module/a.cer"/>`),
		"bad_hash":             publicationTestReply(strings.Replace(item, hash, strings.Repeat("x", 64), 1)),
		"short_hash":           publicationTestReply(strings.Replace(item, hash, "ab", 1)),
		"http_uri":             publicationTestReply(strings.Replace(item, "rsync:", "https:", 1)),
		"uri_query":            publicationTestReply(strings.Replace(item, "a.cer", "a.cer?token=secret", 1)),
		"uri_credentials":      publicationTestReply(strings.Replace(item, "repo.example", "user@repo.example", 1)),
		"nested_object":        publicationTestReply(strings.Replace(item, "/>", "><list/></list>", 1)),
		"non_xml_whitespace":   publicationTestReply("\u00a0" + item),
		"text":                 publicationTestReply("unexpected" + item),
		"success":              publicationTestReply(`<success/>`),
		"mixed":                publicationTestReply(item + rejectionXML),
		"mixed_reverse":        publicationTestReply(rejectionXML + item),
		"unknown_error":        publicationTestReply(`<report_error error_code="secret"/>`),
		"wrong_tag":            publicationTestReply(`<report_error error_code="xml_error" tag="another-request"/>`),
		"wrong_failed_pdu":     publicationTestReply(strings.Replace(rejectionXML, "<list/>", `<withdraw tag="x"/>`, 1)),
		"failed_list_attrs":    publicationTestReply(strings.Replace(rejectionXML, "<list/>", `<list tag="x"/>`, 1)),
		"duplicate_error_text": publicationTestReply(strings.Replace(rejectionXML, "<failed_pdu>", "<error_text>duplicate</error_text><failed_pdu>", 1)),
		"error_order":          publicationTestReply(`<report_error error_code="xml_error"><failed_pdu><list/></failed_pdu><error_text>late</error_text></report_error>`),
		"oversized":            strings.Repeat(" ", (4<<20)+1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			got, rejection, err := parseRPKIPublicationList([]byte(body))
			if err == nil || got != nil || rejection != nil {
				t.Fatal("malformed reply accepted")
			}
		})
	}
}

func TestRPKIPublicationListExchange(t *testing.T) {
	for _, mode := range []string{"inventory", "empty", "rejected", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			local, _ := cmsSigningFixture(t)
			remote, _ := cmsSigningFixture(t)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				verified, err := verifyRPKICMS(raw, rpkiCMSTrust{Anchor: local.Anchor, Now: cmsTrustNow()})
				if err != nil {
					t.Error(err)
					return
				}
				root, err := parseXML(verified.Content)
				if err != nil {
					t.Error(err)
					return
				}
				a, err := publicationAttrs(root, "msg", "version", "type")
				if err != nil || a["version"] != "4" || a["type"] != "query" || len(root.Children) != 1 || !publicationEmpty(root.Children[0], "list") {
					t.Error("invalid list query")
					return
				}
				pdu := ""
				switch mode {
				case "inventory":
					pdu = `<list uri="rsync://repo.example/module/test.cer" hash="` + strings.Repeat("12", 32) + `"/>`
				case "rejected":
					pdu = `<report_error error_code="permission_failure"><error_text>private detail</error_text></report_error>`
				case "malformed":
					pdu = `<success/>`
				}
				signed, err := signRPKICMS([]byte(publicationTestReply(pdu)), remote, cmsTrustNow(), time.Time{})
				if err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "application/rpki-publication")
				_, _ = w.Write(signed)
			}))
			defer server.Close()
			config := rpkiHTTPExchange{Endpoint: server.URL, MediaType: "application/rpki-publication", Directory: privateExchangeDir(t), PeerScope: "publisher", Identity: local, PeerAnchor: remote.Anchor, Clock: cmsTrustNow}
			client := rpkiPublicationClient{Exchange: config}
			for i := 0; i < 2; i++ {
				got, err := client.List(context.Background())
				switch mode {
				case "inventory":
					if err != nil || len(got) != 1 {
						t.Fatalf("inventory failed: %v", err)
					}
				case "empty":
					if err != nil || got == nil || len(got) != 0 {
						t.Fatalf("empty inventory failed: %v", err)
					}
				case "rejected":
					var rejection *rpkiPublicationError
					if !errors.As(err, &rejection) || got != nil || strings.Contains(err.Error(), "private") {
						t.Fatalf("wrong rejection: %v", err)
					}
				case "malformed":
					if err == nil || got != nil {
						t.Fatal("malformed accepted")
					}
				}
			}
			peer, _ := config.peerID()
			lease, err := openRPKIExchange(config.Directory, peer)
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Close()
			state, err := lease.State()
			if err != nil {
				t.Fatal(err)
			}
			if mode == "malformed" {
				if calls.Load() != 1 || state.Pending == nil || !state.LastReceived.IsZero() {
					t.Fatal("malformed response did not block retry")
				}
			} else if calls.Load() != 2 || state.Pending != nil || !state.LastReceived.Equal(cmsTrustNow()) {
				t.Fatal("valid reply did not complete exchange")
			}
		})
	}
}
