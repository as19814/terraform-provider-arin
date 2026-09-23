package arin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func rpslFixture(kind, name string) string {
	origin := ""
	if kind == "route" || kind == "route6" {
		origin = "origin: AS64496\n"
	}
	return kind + ": " + name + "\n" + origin + "descr: Example\nmnt-by: MNT-EXAMPLE-1\nsource: ARIN\n"
}
func TestRPSLParser(t *testing.T) {
	for kind, name := range map[string]string{"route": "192.0.2.0/24", "route6": "2001:db8::/48", "as-set": "AS-EXAMPLE", "route-set": "AS64496:RS-EXAMPLE", "aut-num": "AS64496"} {
		t.Run(kind, func(t *testing.T) {
			raw := rpslFixture(kind, name) + "remarks: preserve unknown attributes\nx-example: opaque\n+ continued value\n"
			obj, err := ParseRPSL(raw)
			if err != nil || obj.Text != raw || obj.Key.Kind != kind || obj.Key.Name != name || obj.OrgHandle != "EXAMPLE-1" {
				t.Fatalf("incorrect parsed object: %v", err)
			}
			if _, err := obj.Key.path(); err != nil {
				t.Fatal(err)
			}
		})
	}
	good := rpslFixture("as-set", "AS-EXAMPLE")
	for _, raw := range []string{"", "<asSet/>", good + "\n" + good, good + "source: ARIN\n", good + "route-set: RS-OTHER\n", good + "mnt-by: MNT-OTHER\n", strings.Replace(good, "source: ARIN", "source: OTHER", 1), strings.Replace(good, "MNT-EXAMPLE-1", "MNT-EXAMPLE-1, MNT-OTHER", 1), good + "remarks: bad\x00\n", " continuation\n" + good, rpslFixture("route", "2001:db8::/48"), rpslFixture("route6", "192.0.2.0/24")} {
		if _, err := ParseRPSL(raw); err == nil {
			t.Fatal("accepted invalid or ambiguous RPSL")
		}
	}
}
func TestRPSLClientLifecycle(t *testing.T) {
	for kind, name := range map[string]string{"route": "192.0.2.0/24", "route6": "2001:db8::/48", "as-set": "AS-EXAMPLE", "route-set": "RS-EXAMPLE", "aut-num": "AS64496"} {
		t.Run(kind, func(t *testing.T) {
			raw := rpslFixture(kind, name)
			obj, _ := ParseRPSL(raw)
			path, _ := obj.Key.path()
			stored := ""
			writes := map[string]int{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "ApiKey test-key" || r.Header.Get("Accept") != rpslMediaType || r.Header.Get("Content-Type") != rpslMediaType {
					t.Error("incorrect RPSL headers")
					w.WriteHeader(400)
					return
				}
				expected := path
				if r.Method == "POST" && (kind == "as-set" || kind == "route-set") {
					expected = "/rest/irr/" + kind + "?orgHandle=EXAMPLE-1"
				}
				if r.URL.RequestURI() != expected {
					t.Errorf("unexpected endpoint %s", r.URL.RequestURI())
				}
				switch r.Method {
				case "GET":
					if stored == "" {
						w.WriteHeader(404)
						return
					}
					fmt.Fprint(w, stored)
				case "POST", "PUT":
					writes[r.Method]++
					b, _ := io.ReadAll(r.Body)
					stored = string(b)
					fmt.Fprint(w, stored)
				case "DELETE":
					writes[r.Method]++
					stored = ""
					w.WriteHeader(204)
				}
			}))
			defer server.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
			ctx := context.Background()
			if _, err := c.CreateRPSL(ctx, raw); err != nil {
				t.Fatal(err)
			}
			if _, err := c.CreateRPSL(ctx, raw); err == nil {
				t.Fatal("existing object overwritten")
			}
			if _, err := c.UpdateRPSL(ctx, strings.Replace(raw, "MNT-EXAMPLE-1", "MNT-OTHER", 1)); err == nil {
				t.Fatal("organization changed")
			}
			updated := raw + "remarks: changed\n"
			if got, err := c.UpdateRPSL(ctx, updated); err != nil || got.Text != updated {
				t.Fatalf("update: %v", err)
			}
			if err := c.DeleteRPSL(ctx, obj.Key, "OTHER"); err == nil {
				t.Fatal("deleted another organization's object")
			}
			if err := c.DeleteRPSL(ctx, obj.Key, obj.OrgHandle); err != nil {
				t.Fatal(err)
			}
			if err := c.DeleteRPSL(ctx, obj.Key, obj.OrgHandle); err != nil {
				t.Fatal(err)
			}
			if writes["POST"] != 1 || writes["PUT"] != 1 || writes["DELETE"] != 1 {
				t.Fatalf("unexpected writes: %v", writes)
			}
		})
	}
}
func TestRPSLReadFailuresPreventMutation(t *testing.T) {
	for _, status := range []int{400, 403, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Error("read failure allowed a mutation")
				}
				w.WriteHeader(status)
			}))
			defer server.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
			raw := rpslFixture("as-set", "AS-EXAMPLE")
			obj, _ := ParseRPSL(raw)
			if _, err := c.CreateRPSL(context.Background(), raw); err == nil {
				t.Fatal("creation accepted unreadable object")
			}
			if _, err := c.UpdateRPSL(context.Background(), raw); err == nil {
				t.Fatal("update accepted unreadable object")
			}
			if err := c.DeleteRPSL(context.Background(), obj.Key, obj.OrgHandle); err == nil {
				t.Fatal("deletion accepted unreadable object")
			}
		})
	}
}
func TestRPSLUncertainWritesNotRetried(t *testing.T) {
	for _, status := range []int{202, 307, 403, 409, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					w.WriteHeader(404)
					return
				}
				writes++
				w.Header().Set("Location", "/redirect")
				w.WriteHeader(status)
			}))
			defer server.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
			if _, err := c.CreateRPSL(context.Background(), rpslFixture("as-set", "AS-EXAMPLE")); err == nil || writes != 1 {
				t.Fatalf("uncertain write accepted or retried: %v (%d)", err, writes)
			}
		})
	}
}

func TestRPSLWriteResponseIdentity(t *testing.T) {
	raw := rpslFixture("as-set", "AS-EXAMPLE")
	for _, body := range []string{"<asSet/>", strings.Replace(raw, "AS-EXAMPLE", "AS-OTHER", 1), strings.Replace(raw, "MNT-EXAMPLE-1", "MNT-OTHER", 1)} {
		t.Run(fmt.Sprint(len(body)), func(t *testing.T) {
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					w.WriteHeader(404)
					return
				}
				writes++
				fmt.Fprint(w, body)
			}))
			defer server.Close()
			c, _ := New(Config{APIKey: "test-key", BaseURL: server.URL})
			if _, err := c.CreateRPSL(context.Background(), raw); err == nil || writes != 1 {
				t.Fatal("accepted an unverifiable write or repeated it")
			}
		})
	}
}

func TestRPSLComparison(t *testing.T) {
	raw := rpslFixture("aut-num", "AS64496") + "import: from AS64497\n  accept ANY\nremarks: keep  internal spacing\n"
	equivalent := "aut-num:    AS64496\nsource: ARIN\nmnt-by: MNT-EXAMPLE-1\ndescr: Example\nimport: from AS64497 accept ANY\nremarks: keep  internal spacing\ncreated: 2026-01-01\nlast-modified: 2026-09-23\n"
	if !EqualRPSL(raw, equivalent) {
		t.Fatal("formatting or timestamps caused drift")
	}
	for _, changed := range []string{strings.Replace(equivalent, "ANY", "AS64498", 1), strings.Replace(equivalent, "keep  internal", "keep internal", 1), equivalent + "remarks: extra\n", equivalent + "x-policy: opaque\n"} {
		if EqualRPSL(raw, changed) {
			t.Fatal("policy change was ignored")
		}
	}
	if EqualRPSL(raw+"remarks: second\n", strings.Replace(raw, "remarks: keep  internal spacing", "remarks: second\nremarks: keep  internal spacing", 1)) {
		t.Fatal("repeated attribute order was ignored")
	}
}
func TestRPSLImportIdentity(t *testing.T) {
	for _, id := range []string{"as-set/AS-EXAMPLE", "route-set/RS-EXAMPLE", "aut-num/AS64496", "route/192.0.2.0/24,AS64496", "route6/2001:db8::/48,AS64496"} {
		key, err := ParseRPSLID(id)
		if err != nil || key.ID() != id {
			t.Fatalf("identity round trip: %v", err)
		}
	}
	for _, id := range []string{"AS-EXAMPLE", "as-set/AS-EXAMPLE,", "route/2001:db8::/48,AS64496", "route/192.0.2.0/24", "as-set/AS-EXAMPLE/extra"} {
		if _, err := ParseRPSLID(id); err == nil {
			t.Fatal("accepted invalid import identity")
		}
	}
}
