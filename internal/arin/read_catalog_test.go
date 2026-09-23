package arin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

func readSpec(t *testing.T, name string) ReadSpec {
	t.Helper()
	for _, s := range RegistrationReads() {
		if s.Name == name {
			return s
		}
	}
	t.Fatal("missing read", name)
	return ReadSpec{}
}
func testParams(spec ReadSpec) map[string]string {
	p := map[string]string{}
	for _, in := range spec.Inputs {
		p[in.Name] = in.Example
		if in.Default != "" {
			p[in.Name] = in.Default
		}
	}
	if spec.Name == "roa" {
		p["handle"] = "abc123"
	}
	return p
}

func TestEveryRegistrationReadUsesGETAndDecodes(t *testing.T) {
	for _, spec := range RegistrationReads() {
		t.Run(spec.Name, func(t *testing.T) {
			root := spec.Root
			if root == "" {
				root = spec.Item
			}
			body := []byte{0, 1, 255, 3}
			if !spec.Binary {
				var err error
				body, err = os.ReadFile("testdata/" + root + ".xml")
				if err != nil {
					t.Fatal(err)
				}
			}
			if spec.Collection || spec.SelectInput != "" {
				body = []byte(`<collection xmlns="http://www.arin.net/regrws/core/v1">` + string(body) + `</collection>`)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Error("non-read method")
				}
				if r.Header.Get("Authorization") != "ApiKey secret" || r.Header.Get("Content-Type") != "application/xml" {
					t.Error("missing authentication or XML content type")
				}
				if r.URL.Query().Get("apikey") != "" || strings.Contains(r.URL.Path, "/report/") {
					t.Error("unsafe read endpoint")
				}
				_, _ = w.Write(body)
			}))
			defer server.Close()
			c, err := New(Config{APIKey: "secret", BaseURL: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			values, err := c.ReadRegistration(context.Background(), spec, testParams(spec))
			if err != nil {
				t.Fatal(err)
			}
			if len(values) == 0 {
				t.Fatal("no decoded output")
			}
		})
	}
}

func TestReadEndpointPaths(t *testing.T) {
	tests := map[string]string{
		"org": "/rest/org/FT-684", "net": "/rest/net/NET6-2602-F805-1", "parent_net": "/rest/net/parentNet/192.0.2.0/192.0.2.255", "most_specific_net": "/rest/net/mostSpecificNet/192.0.2.0/192.0.2.255", "nets_by_ip_range": "/rest/net/netsByIpRange/192.0.2.0/192.0.2.255",
		"delegation": "/rest/delegation/0.5.0.8.f.2.0.6.2.ip6.arpa.", "net_delegations": "/rest/net/NET6-2602-F805-1/delegations", "poc": "/rest/poc/TECH1482-ARIN", "customer": "/rest/customer/C00000001",
		"irr_route": "/rest/irr/route/192.0.2.0/24/AS64496", "irr_routes": "/rest/org/FT-684/routes", "net_routes": "/rest/net/NET6-2602-F805-1/routes?reassignments=false", "irr_aut_num": "/rest/irr/aut-num/AS64496", "irr_aut_nums": "/rest/org/FT-684/aut-nums", "irr_as_set": "/rest/irr/as-set/AS-FOUNDABILITY", "irr_as_sets": "/rest/org/FT-684/as-sets", "irr_route_set": "/rest/irr/route-set/RS-EXAMPLE", "irr_route_sets": "/rest/org/FT-684/route-sets",
		"roas": "/rest/roa/FT-684", "roa": "/rest/roa/FT-684", "aspas": "/rest/aspa/FT-684", "aspa": "/rest/aspa/FT-684",
		"ticket": "/rest/ticket/20260922-X1?msgRefs=true", "ticket_summary": "/rest/ticket/20260922-X1/summary", "tickets": "/rest/ticket;ticketType=ANY;ticketStatus=ANY_OPEN", "ticket_summaries": "/rest/ticket/summary;ticketType=ANY;ticketStatus=ANY_OPEN", "ticket_message": "/rest/ticket/20260922-X1/message/1", "ticket_attachment": "/rest/ticket/20260922-X1/message/1/attachment/1",
	}
	if len(tests) != len(RegistrationReads()) {
		t.Fatal("path test inventory is incomplete")
	}
	for name, want := range tests {
		s := readSpec(t, name)
		p := testParams(s)
		if err := s.Validate(p); err != nil {
			t.Fatal(err)
		}
		if got := s.Path(p); got != want {
			t.Errorf("%s: got %s, want %s", name, got, want)
		}
	}
	route := readSpec(t, "irr_route")
	p := testParams(route)
	p["prefix"] = "2001:db8::/32"
	if got := route.Path(p); got != "/rest/irr/route/2001:db8::/32/AS64496" {
		t.Errorf("incorrect IPv6 endpoint: %s", got)
	}
}

func TestTypedFieldsAndOrderedLines(t *testing.T) {
	for _, name := range []string{"net", "delegation", "roas", "aspas", "ticket", "ticket_message"} {
		s := readSpec(t, name)
		root := s.Root
		if root == "" {
			root = s.Item
		}
		body, err := os.ReadFile("testdata/" + root + ".xml")
		if err != nil {
			t.Fatal(err)
		}
		node, err := parseXML(body)
		if err != nil {
			t.Fatal(err)
		}
		record, err := decodeFields(node, s.Fields)
		if err != nil {
			t.Fatal(err)
		}
		switch name {
		case "net":
			if !reflect.DeepEqual(record["comments"], []any{"First line", "Last line"}) {
				t.Fatal("line ordering lost")
			}
			if record["ip_version"] != int64(4) {
				t.Fatal("IP version not typed")
			}
		case "delegation":
			ns := record["nameservers"].([]any)
			if ns[0].(map[string]any)["ttl"] != int64(86400) || ns[1].(map[string]any)["ttl"] != nil {
				t.Fatal("TTL missing-value semantics lost")
			}
		case "roas":
			rs := record["resources"].([]any)
			if len(rs) != 2 {
				t.Fatal("lost repeated ROA resources")
			}
			var nullMax bool
			for _, v := range rs {
				if v.(map[string]any)["max_length"] == nil {
					nullMax = true
				}
			}
			if !nullMax {
				t.Fatal("invented ROA maximum length")
			}
		case "aspas":
			if !reflect.DeepEqual(record["provider_asns"], []any{int64(64497), int64(64498)}) {
				t.Fatal("provider ASNs not decoded")
			}
		case "ticket":
			if record["shared"] != true || record["org_handle"] != "EXAMPLE-1" {
				t.Fatal("namespace-qualified fields missing")
			}
		case "ticket_message":
			if len(record["attachment_references"].([]any)) != 1 {
				t.Fatal("attachment reference missing")
			}
		}
	}
}

func TestRegistrationRejectsInvalidResponses(t *testing.T) {
	for _, body := range []string{`<collection/>`, `<collection xmlns="http://www.arin.net/regrws/core/v1"><unexpected/></collection>`, `<net xmlns="http://www.arin.net/regrws/core/v1"/>`, `<collection xmlns="http://www.arin.net/regrws/core/v1"><net><handle>N</handle><version>bad</version></net></collection>`, `<collection xmlns="http://www.arin.net/regrws/core/v1"/><extra/>`, `<!DOCTYPE x><collection/>`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		c, _ := New(Config{APIKey: "secret", BaseURL: server.URL})
		spec := readSpec(t, "nets_by_ip_range")
		if _, err := c.ReadRegistration(context.Background(), spec, testParams(spec)); err == nil {
			t.Errorf("accepted bad response: %s", body)
		}
		server.Close()
	}
}

func TestRegistrationInputValidation(t *testing.T) {
	for _, s := range RegistrationReads() {
		for _, input := range s.Inputs {
			p := testParams(s)
			p[input.Name] = "../bad?apikey=secret"
			if err := s.Validate(p); err == nil {
				t.Errorf("%s accepted unsafe %s", s.Name, input.Name)
			}
		}
	}
	s := readSpec(t, "nets_by_ip_range")
	p := testParams(s)
	p["end_address"] = "2001:db8::"
	if s.Validate(p) == nil {
		t.Fatal("mixed address families accepted")
	}
	p["end_address"] = "192.0.1.0"
	if s.Validate(p) == nil {
		t.Fatal("reversed range accepted")
	}
}

func TestRegistrationAddressNormalization(t *testing.T) {
	for input, want := range map[string]string{"192.000.002.000": "192.0.2.0", "008.009.010.011": "8.9.10.11", "2001:0DB8:0000:0000::": "2001:db8::"} {
		got, err := normalizeReadString("start_address", input)
		if err != nil || got != want {
			t.Fatalf("normalization failed for %s: %s %v", input, got, err)
		}
	}
	for input, want := range map[string]string{"192.000.002.000/24": "192.0.2.0/24", "2001:0DB8:0000:0000::/32": "2001:db8::/32"} {
		got, err := normalizeReadString("prefix", input)
		if err != nil || got != want {
			t.Fatalf("CIDR normalization failed: %s %v", got, err)
		}
	}
	for _, bad := range []string{"999.0.0.0", "-1.0.0.0", "192..2.0", "192.0.2.0.1", "fe80::1%eth0"} {
		if _, err := normalizeReadString("start_address", bad); err == nil {
			t.Fatal("invalid address accepted")
		}
	}
}

func TestAttachmentMetadata(t *testing.T) {
	s := readSpec(t, "ticket_attachment")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''report%20sample.txt")
		w.Header().Set("Content-Type", "application/octet-stream")
		fmt.Fprint(w, "abc")
	}))
	defer server.Close()
	c, _ := New(Config{APIKey: "secret", BaseURL: server.URL})
	result, err := c.ReadRegistration(context.Background(), s, testParams(s))
	if err != nil {
		t.Fatal(err)
	}
	if result["filename"] != "report sample.txt" || result["content_type"] != "application/octet-stream" || result["size_bytes"] != int64(3) || result["content_base64"] != "YWJj" || result["sha256"] != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("incorrect attachment metadata: %+v", result)
	}
}

func TestEmptyCollectionsAndMissingSelection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<collection xmlns="http://www.arin.net/regrws/core/v1"/>`)
	}))
	defer server.Close()
	c, _ := New(Config{APIKey: "secret", BaseURL: server.URL})
	for _, s := range RegistrationReads() {
		if !s.Collection && s.SelectInput == "" {
			continue
		}
		result, err := c.ReadRegistration(context.Background(), s, testParams(s))
		if s.SelectInput != "" {
			if !IsNotFound(err) {
				t.Errorf("%s did not fail on missing selection: %v", s.Name, err)
			}
			continue
		}
		if err != nil || len(result[s.Output].([]any)) != 0 {
			t.Fatalf("%s failed empty collection: %v", s.Name, err)
		}
	}
}

func TestTicketFlaggedField(t *testing.T) {
	for _, name := range []string{"ticket", "ticket_summary", "tickets", "ticket_summaries"} {
		spec := readSpec(t, name)
		for _, raw := range []string{"true", "false", "custom-server-value", ""} {
			body := `<ticket xmlns="http://www.arin.net/regrws/core/v1"><ticketNo>20260923-X1</ticketNo>`
			if raw != "" {
				body += "<flagged>" + raw + "</flagged>"
			}
			body += "</ticket>"
			node, err := parseXML([]byte(body))
			if err != nil {
				t.Fatal(err)
			}
			values, err := decodeFields(node, spec.Fields)
			if err != nil {
				t.Fatal(err)
			}
			if raw == "" {
				if values["flagged"] != nil {
					t.Fatalf("%s invented absent flagged value", name)
				}
			} else if values["flagged"] != raw {
				t.Fatalf("%s lost raw flagged field", name)
			}
		}
	}
}
