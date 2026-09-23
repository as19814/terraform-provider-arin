package arin

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"
)

// This client-level test establishes ticket and payload behavior before the
// Terraform assignment resource relies on it. All writes target disposable OT&E records.
func TestOTENetAssignmentClientLifecycle(t *testing.T) {
	for _, family := range []string{"v4", "v6"} {
		t.Run(family, func(t *testing.T) { testOTENetAssignmentClientLifecycle(t, family) })
	}
}
func testOTENetAssignmentClientLifecycle(t *testing.T, family string) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires explicit OT&E write opt-in")
	}
	key := os.Getenv("ARIN_OTE_API_KEY")
	org := os.Getenv("ARIN_TEST_ORG_HANDLE")
	if key == "" || org == "" {
		t.Fatal("requires ARIN_OTE_API_KEY and ARIN_TEST_ORG_HANDLE")
	}
	c, err := New(Config{APIKey: key, BaseURL: OTEURL, RDAPBaseURL: RDAPOTEURL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	networks, err := c.ListOrganizationNetworks(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	var parent, prefix string
	for _, network := range networks {
		if network.IPVersion != family || !strings.Contains(strings.ToLower(network.Type), "allocation") {
			continue
		}
		detail, err := c.GetRegisteredNet(ctx, network.Handle)
		if err != nil {
			t.Fatal(err)
		}
		if detail.OrgHandle != org {
			continue
		}
		for _, block := range detail.Blocks {
			if block.Type != "DA" && block.Type != "A" {
				continue
			}
			bits := 64
			if family == "v4" {
				bits = 32
			}
			if block.CIDRLength > bits {
				continue
			}
			start := netip.MustParseAddr(block.StartAddress)
			end := netip.MustParseAddr(block.EndAddress)
			low := new(big.Int).SetBytes(start.AsSlice())
			high := new(big.Int).SetBytes(end.AsSlice())
			width := new(big.Int).Add(new(big.Int).Sub(high, low), big.NewInt(1))
			for attempt := 0; attempt < 10; attempt++ {
				offset, err := rand.Int(rand.Reader, width)
				if err != nil {
					t.Fatal(err)
				}
				raw := new(big.Int).Add(low, offset).FillBytes(make([]byte, start.BitLen()/8))
				address, _ := netip.AddrFromSlice(raw)
				candidate := netip.PrefixFrom(address, bits).Masked()
				var current map[string]any
				for _, spec := range RegistrationReads() {
					if spec.Name == "parent_net" {
						current, err = c.ReadRegistration(ctx, spec, map[string]string{"start_address": candidate.Addr().String(), "end_address": prefixEnd(candidate).String()})
						break
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				if current["handle"] == network.Handle && current["org_handle"] == org {
					parent = network.Handle
					prefix = candidate.String()
					break
				}
			}
			if prefix != "" {
				break
			}
		}
		if prefix != "" {
			break
		}
	}
	if prefix == "" {
		t.Fatalf("no unassigned %s candidate under an owned allocation", family)
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("TERRAFORM-OTE-%X", random[:])
	customer, err := c.CreateCustomer(ctx, parent, Customer{Name: name, CountryCode: "US", Subdivision: "VA", PostalCode: "20151", City: "Chantilly", StreetAddress: []string{"123 Test Street"}, Private: true})
	if err != nil {
		t.Fatal(err)
	}
	handle := ""
	recipientCustomer, recipientOrg, expectedName := customer.Handle, "", name
	// Cleanup can recover a created NET even when its write response was lost.
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if handle == "" {
			p := netip.MustParsePrefix(prefix)
			for _, spec := range RegistrationReads() {
				if spec.Name != "most_specific_net" {
					continue
				}
				v, e := c.ReadRegistration(cleanup, spec, map[string]string{"start_address": p.Addr().String(), "end_address": prefixEnd(p).String()})
				if e == nil && netString(v, "customer_handle") == recipientCustomer && netString(v, "org_handle") == recipientOrg && v["name"] == expectedName && v["parent_net_handle"] == parent {
					handle, _ = v["handle"].(string)
				}
				if e != nil && !IsNotFound(e) {
					t.Errorf("cleanup lookup: %v", e)
				}
			}
		}
		if handle != "" {
			result, e := c.DeleteNetAssignment(cleanup, handle)
			if e != nil {
				t.Errorf("NET cleanup %s: %v", handle, e)
				return
			}
			if result.TicketNumber != "" {
				t.Logf("Cleanup ticket: %s (%s)", result.TicketNumber, result.TicketStatus)
			}
			if _, e = c.GetRegisteredNet(cleanup, handle); !IsNotFound(e) {
				t.Errorf("NET deletion not confirmed for %s: %v", handle, e)
				return
			}
		}
		if e := c.DeleteCustomer(cleanup, customer.Handle); e != nil {
			t.Errorf("customer cleanup %s: %v", customer.Handle, e)
		}
		if _, e := c.GetCustomer(cleanup, customer.Handle); !IsNotFound(e) {
			t.Errorf("customer deletion unconfirmed: %v", e)
		}
	})
	assignment := NetAssignment{ParentNetHandle: parent, Name: name, CustomerHandle: customer.Handle, Prefixes: []string{prefix}, Comments: []string{"Disposable OT&E network"}}
	result, err := c.CreateNetAssignment(ctx, assignment)
	if result != nil && result.Net != nil {
		handle = result.Net.Handle
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Create ticket: %s (%s), network: %s", result.TicketNumber, result.TicketStatus, handle)
	if handle == "" {
		t.Fatalf("pending ticket requires reconciliation: %s", result.TicketNumber)
	}
	n, err := c.GetRegisteredNet(ctx, handle)
	if err != nil {
		t.Fatal(err)
	}
	if n.CustomerHandle != customer.Handle || n.ParentNetHandle != parent || n.Name != name {
		t.Fatal("created NET identity mismatch")
	}
	expectedName = name + "-UPDATED"
	updated, err := c.UpdateRegisteredNet(ctx, handle, expectedName, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != name+"-UPDATED" || len(updated.Comments) != 0 || len(updated.OriginASNs) != 0 {
		t.Fatal("NET metadata change or clearing failed")
	}
	deleted, err := c.DeleteNetAssignment(ctx, handle)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Delete ticket: %s (%s)", deleted.TicketNumber, deleted.TicketStatus)
	if _, err := c.GetRegisteredNet(ctx, handle); !IsNotFound(err) {
		t.Fatalf("deleted NET remains: %v", err)
	}
	// Reuse the now-free range to exercise organization recipients. These are
	// disposable child registrations, not modifications to the parent allocation.
	for _, reallocate := range []bool{false, true} {
		mode := "DETAILED"
		if reallocate {
			mode = "REALLOC"
		}
		handle = ""
		recipientCustomer, recipientOrg, expectedName = "", org, name+"-"+mode
		assignment.CustomerHandle = ""
		assignment.OrgHandle = org
		assignment.Name = expectedName
		assignment.Reallocate = reallocate
		result, err := c.CreateNetAssignment(ctx, assignment)
		if result != nil && result.Net != nil {
			handle = result.Net.Handle
		}
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		if handle == "" {
			t.Fatalf("%s pending ticket: %s", mode, result.TicketNumber)
		}
		t.Logf("%s network: %s", mode, handle)
		record, err := c.GetRegisteredNet(ctx, handle)
		if err != nil || record.OrgHandle != org {
			t.Fatalf("%s read: %v", mode, err)
		}
		if !reallocate {
			auditOTEDownstreamRoutes(t, ctx, c, org, parent, handle, prefix, name)
		}
		if _, err := c.DeleteNetAssignment(ctx, handle); err != nil {
			t.Fatalf("%s delete: %v", mode, err)
		}
		if _, err := c.GetRegisteredNet(ctx, handle); !IsNotFound(err) {
			t.Fatalf("%s deletion unconfirmed: %v", mode, err)
		}
	}
}
