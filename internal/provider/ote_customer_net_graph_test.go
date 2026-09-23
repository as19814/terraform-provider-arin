package provider

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type customerNetReceipt struct {
	Org, Parent, Prefix, Name, Pending string
	Customers, Nets                    []string
}
type customerNetOTETransport struct {
	mu        sync.Mutex
	path      string
	receipt   customerNetReceipt
	transport http.RoundTripper
}

func (g *customerNetOTETransport) save() error {
	data, err := json.MarshalIndent(g.receipt, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(g.path+".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(g.path+".tmp", g.path)
}

type graphNetXML struct {
	Handle   string                    `xml:"handle"`
	Name     string                    `xml:"netName"`
	Parent   string                    `xml:"parentNetHandle"`
	Customer string                    `xml:"customerHandle"`
	Blocks   []arin.RegisteredNetBlock `xml:"netBlocks>netBlock"`
}

func (g *customerNetOTETransport) RoundTrip(req *http.Request) (*http.Response, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if req.URL.Scheme != "https" || req.URL.Host != "reg.ote.arin.net" || req.URL.RawQuery != "" {
		return nil, errors.New("unexpected customer/NET sandbox origin or query")
	}
	if req.Method == "GET" {
		return g.transport.RoundTrip(req)
	}
	if g.receipt.Pending != "" {
		return nil, errors.New("an uncertain graph mutation requires reconciliation before more writes")
	}
	path := req.URL.Path
	createCustomer := req.Method == "POST" && path == "/rest/net/"+g.receipt.Parent+"/customer"
	createNet := req.Method == "PUT" && path == "/rest/net/"+g.receipt.Parent+"/reassign"
	knownCustomer := slices.Contains(g.receipt.Customers, strings.TrimPrefix(path, "/rest/customer/")) && strings.HasPrefix(path, "/rest/customer/")
	knownNet := slices.Contains(g.receipt.Nets, strings.TrimPrefix(path, "/rest/net/")) && strings.HasPrefix(path, "/rest/net/")
	if !createCustomer && !createNet && !((req.Method == "PUT" || req.Method == "DELETE") && (knownCustomer || knownNet)) {
		return nil, errors.New("refusing mutation outside disposable customer/NET graph")
	}
	if req.Body != nil {
		body, err := io.ReadAll(io.LimitReader(req.Body, 4<<20))
		if err != nil {
			return nil, err
		}
		req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(body))
		if createCustomer || (knownCustomer && req.Method == "PUT") {
			var customer struct {
				Name string `xml:"customerName"`
			}
			if xml.Unmarshal(body, &customer) != nil || customer.Name != g.receipt.Name {
				return nil, errors.New("unexpected disposable customer identity")
			}
		}
		if createNet || (knownNet && req.Method == "PUT") {
			var net graphNetXML
			prefix := netip.MustParsePrefix(g.receipt.Prefix)
			if xml.Unmarshal(body, &net) != nil || net.Name != g.receipt.Name || net.Parent != g.receipt.Parent || !slices.Contains(g.receipt.Customers, net.Customer) || len(net.Blocks) != 1 {
				return nil, errors.New("unexpected disposable NET identity")
			}
			start, e1 := netip.ParseAddr(net.Blocks[0].StartAddress)
			end, e2 := netip.ParseAddr(net.Blocks[0].EndAddress)
			if e1 != nil || e2 != nil || start != prefix.Addr() || end != otePrefixEnd(prefix) || net.Blocks[0].CIDRLength != prefix.Bits() {
				return nil, errors.New("refusing NET mutation outside reserved test range")
			}
		}
	}
	g.receipt.Pending = req.Method + " " + path
	if err := g.save(); err != nil {
		return nil, err
	}
	response, err := g.transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	response.Body.Close()
	response.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil || len(body) > 4<<20 {
		return nil, errors.New("could not record graph mutation response")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if response.StatusCode == 404 && req.Method == "DELETE" {
			g.receipt.Pending = ""
			if err := g.save(); err != nil {
				return nil, err
			}
		}
		return response, nil
	}
	if createCustomer {
		var customer struct {
			Handle string `xml:"handle"`
			Name   string `xml:"customerName"`
		}
		if xml.Unmarshal(body, &customer) != nil || customer.Name != g.receipt.Name || arin.ValidateCustomerContext(g.receipt.Parent, customer.Handle) != nil || customer.Handle == "" {
			return nil, errors.New("customer creation needs manual receipt reconciliation")
		}
		g.receipt.Customers = append(g.receipt.Customers, customer.Handle)
	}
	if createNet {
		var envelope struct {
			Net graphNetXML `xml:"net"`
		}
		if xml.Unmarshal(body, &envelope) != nil || envelope.Net.Name != g.receipt.Name || !strings.HasPrefix(envelope.Net.Handle, "NET") || !slices.Contains(g.receipt.Customers, envelope.Net.Customer) {
			return nil, errors.New("NET creation needs manual receipt reconciliation")
		}
		g.receipt.Nets = append(g.receipt.Nets, envelope.Net.Handle)
	}
	var ticketed struct {
		Ticket *struct{} `xml:"ticket"`
	}
	_ = xml.Unmarshal(body, &ticketed)
	if ticketed.Ticket != nil {
		return nil, errors.New("asynchronous graph mutation requires manual reconciliation")
	}
	g.receipt.Pending = ""
	if err := g.save(); err != nil {
		return nil, err
	}
	return response, nil
}

type customerNetOTEProvider struct {
	*ARINProvider
	client *arin.Client
}

func (p *customerNetOTEProvider) Configure(_ context.Context, _ frameworkprovider.ConfigureRequest, resp *frameworkprovider.ConfigureResponse) {
	resp.ResourceData = p.client
	resp.DataSourceData = p.client
}

func TestOTECustomerNetGraphLifecycle(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires explicit OT&E write opt-in")
	}
	key, org := os.Getenv("ARIN_OTE_API_KEY"), os.Getenv("ARIN_TEST_ORG_HANDLE")
	if key == "" || org == "" {
		t.Fatal("requires sandbox key and organization handle")
	}
	discovery, err := arin.New(arin.Config{APIKey: key, BaseURL: arin.OTEURL, RDAPBaseURL: arin.RDAPOTEURL})
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range []string{"v4", "v6"} {
		t.Run(family, func(t *testing.T) {
			cache, err := os.UserCacheDir()
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256([]byte(org))
			path := filepath.Join(cache, "terraform-provider-arin", fmt.Sprintf("ote-customer-net-%x-%s.json", hash[:8], family))
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(path); err == nil {
				t.Fatalf("existing graph receipt must be reconciled first: %s", path)
			} else if !os.IsNotExist(err) {
				t.Fatal(err)
			}
			parent, prefix := oteNetCandidate(t, discovery, org, family)
			var random [8]byte
			if _, err := rand.Read(random[:]); err != nil {
				t.Fatal(err)
			}
			g := &customerNetOTETransport{path: path, receipt: customerNetReceipt{Org: org, Parent: parent, Prefix: prefix, Name: fmt.Sprintf("TERRAFORM-GRAPH-%X", random[:])}, transport: http.DefaultTransport}
			f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
			if err := g.save(); err != nil {
				t.Fatal(err)
			}
			client, err := arin.New(arin.Config{APIKey: key, BaseURL: arin.OTEURL, HTTPClient: &http.Client{Transport: g}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if g.receipt.Pending != "" {
					t.Errorf("uncertain graph mutation retained in %s", path)
					return
				}
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				for _, handle := range g.receipt.Nets {
					n, err := client.GetRegisteredNet(ctx, handle)
					if arin.IsNotFound(err) {
						continue
					}
					if err != nil || n.Name != g.receipt.Name || n.ParentNetHandle != parent || !slices.Contains(g.receipt.Customers, n.CustomerHandle) {
						t.Errorf("cannot verify disposable NET %s for cleanup: %v", handle, err)
						return
					}
					if _, err := client.DeleteNetAssignment(ctx, handle); err != nil {
						t.Errorf("graph NET cleanup: %v", err)
						return
					}
					if _, err := client.GetRegisteredNet(ctx, handle); !arin.IsNotFound(err) {
						t.Errorf("graph NET deletion unconfirmed: %v", err)
						return
					}
				}
				for _, handle := range g.receipt.Customers {
					c, err := client.GetCustomer(ctx, handle)
					if arin.IsNotFound(err) {
						continue
					}
					if err != nil || c.Name != g.receipt.Name || c.ParentOrgHandle != org {
						t.Errorf("cannot verify disposable customer %s for cleanup: %v", handle, err)
						return
					}
					if err := client.DeleteCustomer(ctx, handle); err != nil {
						t.Errorf("graph customer cleanup: %v", err)
						return
					}
					if _, err := client.GetCustomer(ctx, handle); !arin.IsNotFound(err) {
						t.Errorf("graph customer deletion unconfirmed: %v", err)
						return
					}
				}
				if err := os.Remove(path); err != nil {
					t.Error(err)
				}
			})
			p := &customerNetOTEProvider{ARINProvider: &ARINProvider{version: "ote-test"}, client: client}
			config := func(generation int, updated bool) string {
				return customerNetGraphConfig(parent, prefix, g.receipt.Name, generation, updated)
			}
			capture := func(s *terraform.State) error {
				customer, net := s.RootModule().Resources["arin_customer.recipient"], s.RootModule().Resources["arin_net.assignment"]
				if customer == nil || net == nil || customer.Primary == nil || net.Primary == nil {
					return errors.New("missing graph state")
				}
				if net.Primary.Attributes["customer_handle"] != customer.Primary.ID {
					return errors.New("NET recipient does not match managed customer")
				}
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				n, err := client.GetRegisteredNet(ctx, net.Primary.ID)
				if err != nil {
					return err
				}
				if n.CustomerHandle != customer.Primary.ID {
					return errors.New("native NET customer relationship mismatch")
				}
				t.Logf("disposable %s graph: customer %s, NET %s", family, customer.Primary.ID, net.Primary.ID)
				return nil
			}
			resource.Test(t, resource.TestCase{ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){"arin": providerserver.NewProtocol6WithError(p)}, CheckDestroy: func(_ *terraform.State) error {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				for _, h := range g.receipt.Nets {
					if _, err := client.GetRegisteredNet(ctx, h); !arin.IsNotFound(err) {
						return fmt.Errorf("NET %s deletion unconfirmed: %v", h, err)
					}
				}
				for _, h := range g.receipt.Customers {
					if _, err := client.GetCustomer(ctx, h); !arin.IsNotFound(err) {
						return fmt.Errorf("customer %s deletion unconfirmed: %v", h, err)
					}
				}
				return nil
			}, Steps: []resource.TestStep{
				{Config: config(1, false), Check: capture},
				{Config: config(1, true), Check: resource.ComposeAggregateTestCheckFunc(capture, resource.TestCheckResourceAttr("arin_customer.recipient", "street_address.#", "2"), resource.TestCheckResourceAttr("arin_net.assignment", "comments.#", "0"))},
				{ResourceName: "arin_customer.recipient", ImportState: true, ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return parent + "/" + s.RootModule().Resources["arin_customer.recipient"].Primary.ID, nil
				}, ImportStateVerify: true},
				{ResourceName: "arin_net.assignment", ImportState: true, ImportStateVerify: true},
				{Config: config(1, true), PlanOnly: true},
				{Config: config(2, true), Check: capture},
				{Config: config(2, true), PlanOnly: true},
			}})
			if len(g.receipt.Customers) != 2 || len(g.receipt.Nets) != 2 {
				t.Error("expected exactly two customer and NET creations across replacement")
			}
		})
	}
}

type graphRoundTripFunc func(*http.Request) (*http.Response, error)

func (f graphRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCustomerNetGraphReceiptGuard(t *testing.T) {
	for _, tc := range []struct {
		name, origin, body string
		uncertain          bool
	}{
		{"completed", "https://reg.ote.arin.net", `<customer><handle>C123</handle><customerName>TEST-GRAPH</customerName></customer>`, false},
		{"lost_response", "https://reg.ote.arin.net", "", true},
		{"production", "https://reg.arin.net", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			g := &customerNetOTETransport{path: filepath.Join(t.TempDir(), "receipt.json"), receipt: customerNetReceipt{Name: "TEST-GRAPH", Parent: "NET-192-0-2-0-1", Prefix: "192.0.2.0/32"}}
			g.transport = graphRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				data, err := os.ReadFile(g.path)
				if err != nil {
					t.Fatal(err)
				}
				var receipt customerNetReceipt
				if json.Unmarshal(data, &receipt) != nil || receipt.Pending == "" {
					t.Fatal("mutation sent without durable pending receipt")
				}
				if tc.uncertain {
					return nil, errors.New("simulated lost response")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})
			req, _ := http.NewRequest("POST", tc.origin+"/rest/net/NET-192-0-2-0-1/customer", strings.NewReader(`<customer><customerName>TEST-GRAPH</customerName></customer>`))
			response, err := g.RoundTrip(req)
			if response != nil {
				response.Body.Close()
			}
			if tc.name == "production" {
				if err == nil || calls != 0 {
					t.Fatal("production mutation escaped guard")
				}
				return
			}
			data, e := os.ReadFile(g.path)
			if e != nil {
				t.Fatal(e)
			}
			var stored customerNetReceipt
			if e := json.Unmarshal(data, &stored); e != nil {
				t.Fatal(e)
			}
			if tc.uncertain {
				if err == nil || stored.Pending == "" {
					t.Fatal("uncertain mutation was cleared")
				}
				if _, err := g.RoundTrip(req); err == nil || calls != 1 {
					t.Fatal("uncertain creation was retried")
				}
			} else if err != nil || stored.Pending != "" || len(stored.Customers) != 1 || stored.Customers[0] != "C123" {
				t.Fatalf("lost completed creation receipt: %v", err)
			}
		})
	}
}
