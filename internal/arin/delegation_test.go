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

func testDelegation() Delegation {
	ttl := int64(3600)
	return Delegation{Name: "2.0.192.in-addr.arpa.", Nameservers: []DelegationNameserver{{Name: "ns1.example.net", TTL: &ttl}, {Name: "ns2.example.net"}}, DSRecords: []DelegationDS{{Algorithm: 13, DigestType: 2, KeyTag: 12345, Digest: strings.Repeat("AB", 32), TTL: &ttl}}}
}
func TestDelegationPayloadAndRoundTrip(t *testing.T) {
	d := testDelegation()
	d.DSRecords[0].AlgorithmName = "response-only-algorithm"
	d.DSRecords[0].DigestTypeName = "response-only-digest"
	body, err := d.marshal()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "response-only") {
		t.Fatal("response metadata sent in delegation write")
	}
	root, err := parseXML(body)
	if err != nil {
		t.Fatal(err)
	}
	foundTTL := false
	for _, keys := range root.Children {
		if keys.Name.Local == "delegationKeys" {
			for _, key := range keys.Children {
				for _, field := range key.Children {
					if field.Name.Local == "ttl" {
						foundTTL = true
						if field.Name.Space != delegationTTLNamespace {
							t.Fatal("DS TTL must use ARIN TTL namespace")
						}
					}
				}
			}
		}
	}
	if !foundTTL || !strings.Contains(string(body), "<name>"+d.Name+"</name>") {
		t.Fatal("missing DS TTL or immutable zone name")
	}
	body = []byte(strings.ReplaceAll(string(body), "<algorithm>", `<algorithm name="ECDSAP256SHA256">`))
	body = []byte(strings.ReplaceAll(string(body), "<digestType>", `<digestType name="SHA-256">`))
	decoded, err := decodeDelegation(body, d.Name)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Nameservers) != 2 || decoded.Nameservers[0].TTL == nil || *decoded.Nameservers[0].TTL != 3600 || decoded.Nameservers[1].TTL != nil || len(decoded.DSRecords) != 1 || decoded.DSRecords[0].TTL == nil || *decoded.DSRecords[0].TTL != 3600 {
		t.Fatalf("bad round trip: %+v", decoded)
	}
	if decoded.DSRecords[0].AlgorithmName != "ECDSAP256SHA256" || decoded.DSRecords[0].DigestTypeName != "SHA-256" {
		t.Fatal("lost DS response metadata")
	}
	empty := Delegation{Name: d.Name}
	body, err = empty.marshal()
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"nameservers", "delegationKeys"} {
		if !strings.Contains(string(body), "<"+field+"></"+field+">") {
			t.Fatalf("missing explicit empty %s", field)
		}
	}
}
func TestDelegationValidation(t *testing.T) {
	for _, change := range []func(*Delegation){
		func(d *Delegation) { d.Name = "example.net." },
		func(d *Delegation) { d.Name = "2.0.192.in-addr.arpa" },
		func(d *Delegation) { d.Nameservers[0].Name = "NS1.EXAMPLE.NET" },
		func(d *Delegation) { d.Nameservers[0].Name = "ns1.example.net." },
		func(d *Delegation) { d.Nameservers[0].Name = "bad/../host" },
		func(d *Delegation) { n := int64(-1); d.Nameservers[0].TTL = &n },
		func(d *Delegation) { d.Nameservers = append(d.Nameservers, d.Nameservers[0]) },
		func(d *Delegation) { d.DSRecords[0].Digest = "ABCD" },
		func(d *Delegation) { d.DSRecords[0].Digest = strings.Repeat("ab", 32) },
		func(d *Delegation) { d.DSRecords[0].Algorithm = 0 },
		func(d *Delegation) { d.DSRecords[0].KeyTag = 65536 },
		func(d *Delegation) { d.DSRecords = append(d.DSRecords, d.DSRecords[0]) },
	} {
		d := testDelegation()
		change(&d)
		if d.Validate() == nil {
			t.Fatal("accepted invalid delegation")
		}
	}
}
func TestDelegationResponseValidation(t *testing.T) {
	d := testDelegation()
	body, _ := d.marshal()
	for _, changed := range []string{
		strings.Replace(string(body), "</delegation>", "<extension/></delegation>", 1),
		strings.Replace(string(body), "<digest>", "<future/><digest>", 1),
		strings.Replace(string(body), "<name>"+d.Name+"</name>", "<name>different.in-addr.arpa.</name>", 1),
		strings.Replace(string(body), "<keyTag>12345</keyTag>", "", 1),
	} {
		if _, err := decodeDelegation([]byte(changed), d.Name); err == nil {
			t.Fatal("accepted incomplete or unsupported response")
		}
	}
	canonicalized := strings.ReplaceAll(string(body), "ns1.example.net", "NS1.EXAMPLE.NET.")
	if _, err := decodeDelegation([]byte(canonicalized), d.Name); err != nil {
		t.Fatal(err)
	}
}
func TestDelegationFullAndSuboperationRequests(t *testing.T) {
	d := testDelegation()
	body, _ := d.marshal()
	writes := 0
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		if r.Header.Get("Authorization") != "ApiKey test-key" || r.Header.Get("Content-Type") != "application/xml" {
			t.Error("bad headers")
			w.WriteHeader(403)
			return
		}
		if r.Method != "GET" {
			writes++
		}
		if r.Method == "PUT" {
			b, _ := io.ReadAll(r.Body)
			var payload delegationXML
			if xml.Unmarshal(b, &payload) != nil || payload.Name != d.Name {
				t.Error("missing immutable name")
			}
			root, _ := parseXML(b)
			for _, ns := range root.Children {
				if ns.Name.Local == "nameservers" {
					for _, n := range ns.Children {
						for _, a := range n.Attrs {
							if a.Name.Local == "ttl" && a.Name.Space != delegationTTLNamespace {
								t.Error("wrong nameserver TTL namespace")
							}
						}
					}
				}
			}
		}
		if r.Method == "POST" && r.URL.Query().Get("ttl") != "3600" {
			t.Error("missing nameserver TTL query")
		}
		w.Write(body)
	}))
	defer server.Close()
	c, _ := New(Config{BaseURL: server.URL, APIKey: "test-key"})
	ctx := context.Background()
	if _, err := c.UpdateDelegation(ctx, d); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SetDelegationNameserver(ctx, d.Name, d.Nameservers[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DeleteDelegationNameserver(ctx, d.Name, d.Nameservers[0].Name); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DeleteDelegationNameservers(ctx, d.Name); err != nil {
		t.Fatal(err)
	}
	if writes != 4 {
		t.Fatalf("unexpected writes: %d", writes)
	}
	expected := []string{
		"GET /rest/delegation/" + d.Name,
		"PUT /rest/delegation/" + d.Name,
		"POST /rest/delegation/" + d.Name + "/nameserver/ns1.example.net?ttl=3600",
		"DELETE /rest/delegation/" + d.Name + "/nameserver/ns1.example.net",
		"DELETE /rest/delegation/" + d.Name + "/nameservers",
	}
	if fmt.Sprint(requests) != fmt.Sprint(expected) {
		t.Fatalf("wrong requests: %v", requests)
	}

}
func TestDelegationReadFailurePreventsFullWrite(t *testing.T) {
	for _, status := range []int{404, 403, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" {
					t.Error("wrote without verified zone")
				}
				w.WriteHeader(status)
			}))
			defer server.Close()
			c, _ := New(Config{BaseURL: server.URL, APIKey: "test-key"})
			if _, err := c.UpdateDelegation(context.Background(), testDelegation()); err == nil {
				t.Fatal("ignored failed preflight")
			}
			if calls != 1 {
				t.Fatal("retried request")
			}
		})
	}
}
func TestDelegationWriteNotRetried(t *testing.T) {
	d := testDelegation()
	body, _ := d.marshal()
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			w.Write(body)
			return
		}
		writes++
		w.WriteHeader(500)
	}))
	defer server.Close()
	c, _ := New(Config{BaseURL: server.URL, APIKey: "test-key"})
	if _, err := c.UpdateDelegation(context.Background(), d); err == nil {
		t.Fatal("ignored failed write")
	}
	if writes != 1 {
		t.Fatal("retried mutation")
	}
}
