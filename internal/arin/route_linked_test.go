package arin

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLinkedRouteMetadataAndDeletion(t *testing.T) {
	for _, prefix := range []string{"192.0.2.0/24", "2001:db8::/48"} {
		t.Run(prefix, func(t *testing.T) {
			puts, deletes := 0, 0
			current := routeFixture(prefix, "AS64496", `<autoLinkedRoaHandle>roa1</autoLinkedRoaHandle><remarks><line number="0">`+LinkedRouteRemark+`</line></remarks>`)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case "GET":
					if current == "" {
						w.WriteHeader(404)
						return
					}
					fmt.Fprint(w, current)
				case "PUT":
					puts++
					body, _ := io.ReadAll(r.Body)
					if strings.Contains(string(body), LinkedRouteRemark) || strings.Contains(string(body), "autoLinkedRoaHandle") {
						t.Error("sent server-owned fields")
					}
					var payload routeXML
					if err := xml.Unmarshal(body, &payload); err != nil {
						t.Error(err)
					}
					if payload.Remarks == nil {
						payload.Remarks = &irrLinesXML{}
					}
					payload.Remarks.Lines = append(payload.Remarks.Lines, irrLine{Number: len(payload.Remarks.Lines), Text: LinkedRouteRemark})
					b, _ := xml.Marshal(payload)
					current = strings.Replace(string(b), "</route>", "<netHandle>NET-EXAMPLE-1</netHandle><autoLinkedRoaHandle>roa1</autoLinkedRoaHandle></route>", 1)
					fmt.Fprint(w, current)
				case "DELETE":
					deletes++
					current = ""
					w.WriteHeader(200)
				}
			}))
			defer server.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
			ctx := context.Background()
			want := IRRRoute{Prefix: prefix, OriginAS: "AS64496", OrgHandle: "EXAMPLE-1", Description: []string{"Updated"}, Remarks: []string{"User remark"}, MemberOf: []string{"RS-TEST"}}
			for _, remarks := range [][]string{{"User remark"}, nil} {
				want.Remarks = remarks
				actual, err := c.UpdateLinkedIRRRoute(ctx, want, "roa1")
				if err != nil {
					t.Fatal(err)
				}
				if err := verifyLinkedRoute(*actual, want, "roa1"); err != nil {
					t.Fatal(err)
				}
			}
			if err := c.DeleteLinkedIRRRoute(ctx, want.ID(), want.OrgHandle, "roa1"); err != nil {
				t.Fatal(err)
			}
			if err := c.DeleteLinkedIRRRoute(ctx, want.ID(), want.OrgHandle, "roa1"); err != nil {
				t.Fatal(err)
			}
			if puts != 2 || deletes != 1 {
				t.Fatalf("writes replayed: %d PUT, %d DELETE", puts, deletes)
			}
		})
	}
}
func TestLinkedRouteOwnershipGuards(t *testing.T) {
	for _, mode := range []string{"wrong-link", "unlinked", "wrong-org", "unrecognized-annotation", "reserved-remark", "empty-expected", "read-403"} {
		t.Run(mode, func(t *testing.T) {
			writes := 0
			link := "roa1"
			annotation := LinkedRouteRemark
			if mode == "wrong-link" {
				link = "roa2"
			}
			if mode == "unlinked" {
				link = ""
			}
			if mode == "unrecognized-annotation" {
				annotation = "Future annotation"
			}
			body := routeFixture("192.0.2.0/24", "AS64496", fmt.Sprintf(`<autoLinkedRoaHandle>%s</autoLinkedRoaHandle><remarks><line number="0">%s</line></remarks>`, link, annotation))
			if mode == "wrong-org" {
				body = strings.ReplaceAll(body, "EXAMPLE-1", "OTHER-1")
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					writes++
					t.Error("unexpected write")
				}
				if mode == "read-403" {
					w.WriteHeader(403)
					return
				}
				fmt.Fprint(w, body)
			}))
			defer server.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
			want := IRRRoute{Prefix: "192.0.2.0/24", OriginAS: "AS64496", OrgHandle: "EXAMPLE-1", Description: []string{"Updated"}}
			expected := "roa1"
			if mode == "reserved-remark" {
				want.Remarks = []string{LinkedRouteRemark}
			}
			if mode == "empty-expected" {
				expected = ""
			}
			if _, err := c.UpdateLinkedIRRRoute(context.Background(), want, expected); err == nil {
				t.Fatal("unsafe metadata update accepted")
			}
			if mode != "unrecognized-annotation" && mode != "reserved-remark" {
				if err := c.DeleteLinkedIRRRoute(context.Background(), want.ID(), want.OrgHandle, expected); err == nil {
					t.Fatal("unowned deletion accepted")
				}
			}
			if writes != 0 {
				t.Fatal("ownership guard sent a mutation")
			}
		})
	}
}
func TestLinkedRouteUncertainWrites(t *testing.T) {
	for _, mode := range []string{"lost-update", "malformed", "changed-link", "ignored-metadata", "verification-403", "lost-delete", "ignored-delete", "delete-404"} {
		t.Run(mode, func(t *testing.T) {
			writes := 0
			body := routeFixture("192.0.2.0/24", "AS64496", `<autoLinkedRoaHandle>roa1</autoLinkedRoaHandle><remarks><line number="0">`+LinkedRouteRemark+`</line></remarks>`)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					if writes > 0 && mode == "verification-403" {
						w.WriteHeader(403)
						return
					}
					fmt.Fprint(w, body)
					return
				}
				writes++
				switch mode {
				case "lost-update", "lost-delete":
					w.WriteHeader(500)
				case "malformed":
					fmt.Fprint(w, "<invalid>")
				case "changed-link":
					fmt.Fprint(w, strings.ReplaceAll(body, "roa1", "roa2"))
				case "ignored-metadata", "verification-403":
					fmt.Fprint(w, body)
				case "ignored-delete":
					w.WriteHeader(200)
				case "delete-404":
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
			want := IRRRoute{Prefix: "192.0.2.0/24", OriginAS: "AS64496", OrgHandle: "EXAMPLE-1", Description: []string{"Updated"}}
			if mode == "verification-403" {
				want.Description = []string{"Example route"}
			}
			var err error
			if strings.Contains(mode, "delete") {
				err = c.DeleteLinkedIRRRoute(context.Background(), want.ID(), want.OrgHandle, "roa1")
			} else {
				_, err = c.UpdateLinkedIRRRoute(context.Background(), want, "roa1")
			}
			if err == nil || writes != 1 {
				t.Fatal("unverified write accepted or replayed")
			}
		})
	}
}
