package arin

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func routeFixture(prefix, origin, extra string) string {
	return fmt.Sprintf(`<route xmlns="http://www.arin.net/regrws/core/v1"><prefix>%s</prefix><originAS>%s</originAS><orgHandle>EXAMPLE-1</orgHandle><netHandle>NET-EXAMPLE-1</netHandle><source>ARIN</source><description><line number="0">Example route</line></description>%s</route>`, prefix, origin, extra)
}
func TestIRRRouteIdentity(t *testing.T) {
	for _, id := range []string{"192.0.2.0/24,AS64496", "2001:db8::/32,AS4294967295"} {
		if err := ValidateIRRRouteID(id); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"192.0.2.1/24,AS64496", "2001:0db8::/32,AS64496", "::ffff:192.0.2.0/120,AS64496", "192.0.2.0/24,AS0", "192.0.2.0/24,AS01", "192.0.2.0/24,as64496", "192.0.2.0/24,AS4294967296", "../../bad,AS64496"} {
		if err := ValidateIRRRouteID(id); err == nil {
			t.Fatalf("accepted %s", id)
		}
	}
	r, err := decodeIRRRoute([]byte(routeFixture("192.000.002.000/24", "AS64496", "")), "192.0.2.0/24,AS64496")
	if err != nil || r.Prefix != "192.0.2.0/24" {
		t.Fatalf("address normalization: %v", err)
	}
	for _, body := range []string{`<route/>`, routeFixture("192.0.3.0/24", "AS64496", ""), routeFixture("192.0.2.0/24", "AS64497", ""), routeFixture("192.0.2.0/24", "AS64496", "<futureField/>"), routeFixture("192.0.2.0/24", "AS64496", `<memberOf><routeSetRef name="AS-INVALID"/></memberOf>`)} {
		if _, err := decodeIRRRoute([]byte(body), "192.0.2.0/24,AS64496"); err == nil {
			t.Fatal("accepted unsupported response")
		}
	}
}
func TestIRRRoutePayload(t *testing.T) {
	r := IRRRoute{Prefix: "2001:db8::/48", OriginAS: "AS64496", OrgHandle: "EXAMPLE-1", Description: []string{"<example> & peers"}, POCs: []IRRPOC{{"TECH-1", "T"}}, AutoLinkedROAHandle: "must-not-send"}
	b, err := r.marshal()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, forbidden := range []string{"pocLinks", "netHandle", "creationDate", "autoLinkedRoaHandle", "<remarks"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("unexpected field %s", forbidden)
		}
	}
	if !strings.Contains(s, "&lt;example&gt; &amp; peers") {
		t.Fatal("text was not escaped")
	}
}
func TestIRRRouteMutationGuards(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		extra  string
	}{
		{"linked", 200, "<autoLinkedRoaHandle>roa1</autoLinkedRoaHandle>"},
		{"forbidden", 403, ""},
		{"server error", 500, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writes := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					writes++
					t.Error("unexpected mutation")
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, routeFixture("192.0.2.0/24", "AS64496", tc.extra))
			}))
			defer s.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: s.URL})
			input := IRRRoute{Prefix: "192.0.2.0/24", OriginAS: "AS64496", OrgHandle: "EXAMPLE-1", Description: []string{"Example"}}
			if _, err := c.CreateIRRRoute(context.Background(), input); err == nil {
				t.Error("create accepted existing/unreadable route")
			}
			if _, err := c.UpdateIRRRoute(context.Background(), input); err == nil {
				t.Error("update accepted linked/unreadable route")
			}
			if err := c.DeleteIRRRoute(context.Background(), input.ID()); err == nil {
				t.Error("delete accepted linked/unreadable route")
			}
			if writes != 0 {
				t.Fatal("mutated protected route")
			}
		})
	}
}
func TestIRRRouteFailedWritesNotRetried(t *testing.T) {
	for _, method := range []string{"POST", "PUT", "DELETE"} {
		for _, status := range []int{202, 307, 403, 409, 429, 500} {
			t.Run(fmt.Sprintf("%s/%d", method, status), func(t *testing.T) {
				writes := 0
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == "GET" {
						if method == "POST" {
							w.WriteHeader(404)
						} else {
							fmt.Fprint(w, routeFixture("192.0.2.0/24", "AS64496", ""))
						}
						return
					}
					if r.Method != method {
						t.Error("unexpected method")
					}
					writes++
					w.Header().Set("Location", "/redirect")
					w.WriteHeader(status)
				}))
				defer s.Close()
				c, _ := New(Config{APIKey: "test-key", BaseURL: s.URL})
				input := IRRRoute{Prefix: "192.0.2.0/24", OriginAS: "AS64496", OrgHandle: "EXAMPLE-1", Description: []string{"Example"}}
				var err error
				switch method {
				case "POST":
					_, err = c.CreateIRRRoute(context.Background(), input)
				case "PUT":
					_, err = c.UpdateIRRRoute(context.Background(), input)
				case "DELETE":
					err = c.DeleteIRRRoute(context.Background(), input.ID())
				}
				if err == nil || writes != 1 {
					t.Fatalf("err=%v writes=%d", err, writes)
				}
			})
		}
	}
}

func TestIRRRouteMembership(t *testing.T) {
	input := IRRRoute{Prefix: "192.0.2.0/24", OriginAS: "AS64496", OrgHandle: "EXAMPLE-1", Description: []string{"Example"}, MemberOf: []string{"RS-EXAMPLE", "AS64496:RS-SECOND"}}
	body, err := input.marshal()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	// Verify the nested reference attributes, not just an echo of the body.
	var payload struct {
		Refs []struct {
			Name string `xml:"name,attr"`
		} `xml:"memberOf>routeSetRef"`
	}
	if err := xml.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	for _, ref := range payload.Refs {
		names = append(names, ref.Name)
	}
	if !reflect.DeepEqual(names, input.MemberOf) {
		t.Fatal("incorrect membership payload")
	}
	decoded, err := decodeIRRRoute([]byte(routeFixture(input.Prefix, input.OriginAS, `<memberOf><routeSetRef name="RS-EXAMPLE"/><routeSetRef name="AS64496:RS-SECOND"/></memberOf>`)), input.ID())
	if err != nil || !reflect.DeepEqual(decoded.MemberOf, []string{"AS64496:RS-SECOND", "RS-EXAMPLE"}) {
		t.Fatalf("membership round trip: %v", err)
	}
	for _, extra := range []string{`<memberOf name="RS-EXAMPLE"/>`, `<memberOf><routeSetRef/></memberOf>`, `<memberOf><routeSetRef name="RS-EXAMPLE" extra="lost"/></memberOf>`, `<memberOf><futureField/></memberOf>`} {
		if _, err := decodeIRRRoute([]byte(routeFixture(input.Prefix, input.OriginAS, extra)), input.ID()); err == nil {
			t.Fatal("accepted lossy membership representation")
		}
	}
	input.MemberOf = []string{"AS-BAD"}
	if _, err := input.marshal(); err == nil {
		t.Fatal("accepted invalid route set")
	}
}
