package arin

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

type rpkiOTETransport struct{ shape string }

func (t *rpkiOTETransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil || req.Method != "POST" {
		return resp, err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	root, err := parseXML(body)
	if err == nil {
		paths := map[string]bool{}
		var walk func(*xmlNode, string)
		walk = func(n *xmlNode, p string) {
			p += "/" + n.Name.Local + "{" + n.Name.Space + "}"
			paths[p] = true
			for _, child := range n.Children {
				walk(child, p)
			}
		}
		walk(root, "")
		list := []string{}
		for p := range paths {
			list = append(list, p)
		}
		slices.Sort(list)
		t.shape = strings.Join(list, ", ")
	}
	return resp, nil
}
func rpkiOTEPrefixes(t *testing.T, ctx context.Context, c *Client, org string) []ROAResource {
	t.Helper()
	networks, err := c.ListOrganizationNetworks(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	out := []ROAResource{}
	for _, family := range []string{"v4", "v6"} {
		found := ""
		for _, network := range networks {
			if network.IPVersion != family || !strings.Contains(strings.ToLower(network.Type), "allocation") {
				continue
			}
			n, err := c.GetRegisteredNet(ctx, network.Handle)
			if err != nil {
				t.Fatal(err)
			}
			if n.OrgHandle != org {
				continue
			}
			for _, block := range n.Blocks {
				if block.Type != "DA" && block.Type != "A" {
					continue
				}
				start, end := netip.MustParseAddr(block.StartAddress), netip.MustParseAddr(block.EndAddress)
				low, high := new(big.Int).SetBytes(start.AsSlice()), new(big.Int).SetBytes(end.AsSlice())
				width := new(big.Int).Add(new(big.Int).Sub(high, low), big.NewInt(1))
				for attempt := 0; attempt < 10; attempt++ {
					offset, err := rand.Int(rand.Reader, width)
					if err != nil {
						t.Fatal(err)
					}
					address, _ := netip.AddrFromSlice(new(big.Int).Add(low, offset).FillBytes(make([]byte, start.BitLen()/8)))
					for _, spec := range RegistrationReads() {
						if spec.Name != "parent_net" {
							continue
						}
						parent, err := c.ReadRegistration(ctx, spec, map[string]string{"start_address": address.String(), "end_address": address.String()})
						if err != nil {
							t.Fatal(err)
						}
						if parent["handle"] == network.Handle && parent["org_handle"] == org {
							found = netip.PrefixFrom(address, address.BitLen()).String()
						}
						break
					}
					if found != "" {
						break
					}
				}
				if found != "" {
					break
				}
			}
			if found != "" {
				break
			}
		}
		if found == "" {
			t.Fatalf("no owned free %s host prefix for OT&E ROA", family)
		}
		out = append(out, ROAResource{Prefix: found})
	}
	return out
}
func canonicalROAs(in []ROA) []ROA {
	out := slices.Clone(in)
	for i := range out {
		out[i].Resources = slices.Clone(out[i].Resources)
		slices.SortFunc(out[i].Resources, func(a, b ROAResource) int { return strings.Compare(a.Prefix, b.Prefix) })
	}
	slices.SortFunc(out, func(a, b ROA) int { return strings.Compare(a.Handle, b.Handle) })
	return out
}
func canonicalASPAs(in []ASPA) []ASPA {
	out := slices.Clone(in)
	for i := range out {
		out[i].ProviderASNs = slices.Clone(out[i].ProviderASNs)
		slices.Sort(out[i].ProviderASNs)
	}
	slices.SortFunc(out, func(a, b ASPA) int {
		if a.CustomerASN < b.CustomerASN {
			return -1
		}
		if a.CustomerASN > b.CustomerASN {
			return 1
		}
		return 0
	})
	return out
}
func TestOTERPKIClientLifecycle(t *testing.T) {
	if os.Getenv("ARIN_OTE_WRITE_TESTS") != "1" || os.Getenv("TF_ACC") != "1" {
		t.Skip("requires explicit OT&E write opt-in")
	}
	key, org := os.Getenv("ARIN_OTE_API_KEY"), os.Getenv("ARIN_TEST_ORG_HANDLE")
	if key == "" || org == "" {
		t.Fatal("requires sandbox key and organization handle")
	}
	trace := &rpkiOTETransport{}
	c, err := New(Config{APIKey: key, BaseURL: OTEURL, RDAPBaseURL: RDAPOTEURL, HTTPClient: &http.Client{Transport: trace}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	beforeROAs, err := c.ListROAs(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	beforeASPAs, err := c.ListASPAs(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	if len(beforeASPAs) == 0 {
		t.Fatal("requires an existing sandbox ASPA to snapshot and restore")
	}
	resources := rpkiOTEPrefixes(t, ctx, c, org)
	nonce := make([]byte, 5)
	if _, err = rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("tf-ote-rpki-%x", nonce)
	for _, r := range beforeROAs {
		if r.Name == name {
			t.Fatal("disposable ROA name already exists")
		}
	}
	original := beforeASPAs[0]
	changed := ASPA{CustomerASN: original.CustomerASN, ProviderASNs: slices.DeleteFunc(slices.Clone(original.ProviderASNs), func(asn int64) bool { return asn == 0 })}
	for _, candidate := range []int64{13335, 15169, 3356, 1299} {
		if candidate == changed.CustomerASN || slices.Contains(changed.ProviderASNs, candidate) {
			continue
		}
		verified := false
		for _, spec := range PublicReads() {
			if spec.Name == "asn" {
				if _, err := c.ReadRegistration(ctx, spec, map[string]string{"asn": strconv.FormatInt(candidate, 10)}); err == nil {
					verified = true
				}
				break
			}
		}
		if verified {
			changed.ProviderASNs = append(changed.ProviderASNs, candidate)
			break
		}
	}
	if ASPAEqual(original, changed) {
		t.Fatal("no disposable provider ASN available")
	}
	request := ROARequest{Name: name, ASN: original.CustomerASN, Resources: resources}
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cache, "terraform-provider-arin")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(org))
	snapshot := filepath.Join(dir, fmt.Sprintf("ote-rpki-%x.json", hash[:8]))
	data, err := json.Marshal(struct {
		Org         string
		ROAs        []ROA
		ASPAs       []ASPA
		Request     ROARequest
		ChangedASPA ASPA
	}{org, beforeROAs, beforeASPAs, request, changed})
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(snapshot, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err = f.Sync(); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		roas, err := c.ListROAs(ctx, org)
		if err != nil {
			t.Errorf("cannot read cleanup inventory; retain %s: %v", snapshot, err)
			return
		}
		cleanup := RPKITransaction{}
		for _, r := range roas {
			if r.Name == name {
				cleanup.DeleteROAs = append(cleanup.DeleteROAs, ROADelete{Handle: r.Handle})
			}
		}
		aspas, err := c.ListASPAs(ctx, org)
		if err != nil {
			t.Errorf("cannot read cleanup ASPAs; retain %s: %v", snapshot, err)
			return
		}
		restored := false
		present := false
		for _, a := range aspas {
			if a.CustomerASN == original.CustomerASN {
				present = true
				restored = ASPAEqual(a, original)
			}
		}
		if !restored {
			if present {
				cleanup.DeleteASPAs = []int64{original.CustomerASN}
			}
			cleanup.AddASPAs = []ASPA{original}
		}
		if len(cleanup.DeleteROAs)+len(cleanup.AddASPAs) > 0 {
			if _, err = c.ApplyRPKITransaction(ctx, org, cleanup); err != nil {
				t.Logf("cleanup response needs inventory verification: %v; XML shape: %s", err, trace.shape)
			}
		}
		roas, err = c.ListROAs(ctx, org)
		if err != nil {
			t.Error(err)
			return
		}
		aspas, err = c.ListASPAs(ctx, org)
		if err != nil {
			t.Error(err)
			return
		}
		if !reflect.DeepEqual(canonicalROAs(roas), canonicalROAs(beforeROAs)) || !reflect.DeepEqual(canonicalASPAs(aspas), canonicalASPAs(beforeASPAs)) {
			t.Errorf("original RPKI inventory not restored; retain %s", snapshot)
			return
		}
		if err = os.Remove(snapshot); err != nil {
			t.Error(err)
		}
	})
	_, rejected := c.ApplyRPKITransaction(ctx, org, RPKITransaction{AddROAs: []ROARequest{request}, DeleteASPAs: []int64{original.CustomerASN}, AddASPAs: []ASPA{{CustomerASN: original.CustomerASN, ProviderASNs: []int64{64496}}}})
	var apiErr *APIError
	if !errors.As(rejected, &apiErr) || apiErr.StatusCode != 400 || !strings.Contains(apiErr.Message, "reserved ASN") {
		t.Fatalf("unexpected reserved-provider result: %v", rejected)
	}
	rejectedROAs, err := c.ListROAs(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	rejectedASPAs, err := c.ListASPAs(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(canonicalROAs(rejectedROAs), canonicalROAs(beforeROAs)) || !reflect.DeepEqual(canonicalASPAs(rejectedASPAs), canonicalASPAs(beforeASPAs)) {
		t.Fatal("rejected transaction changed the inventory before cleanup")
	}
	t.Log("reserved-provider transaction rejected without changing either inventory")
	result, err := c.ApplyRPKITransaction(ctx, org, RPKITransaction{AddROAs: []ROARequest{request}, DeleteASPAs: []int64{original.CustomerASN}, AddASPAs: []ASPA{changed}})
	if err != nil {
		t.Fatalf("combined creation/replacement failed: %v; XML shape: %s", err, trace.shape)
	}
	if len(result.ROAs) != 1 || len(result.ASPAs) != 1 || result.ROAs[0].NotValidBefore == "" {
		t.Fatal("incomplete combined result")
	}
	t.Log("combined IPv4/IPv6 ROA creation and ASPA replacement passed")
	next := request
	next.Resources = []ROAResource{resources[0]}
	if _, err = c.ApplyRPKITransaction(ctx, org, RPKITransaction{DeleteROAs: []ROADelete{{Handle: result.ROAs[0].Handle}}, AddROAs: []ROARequest{next}}); err != nil {
		t.Fatalf("ROA replacement failed: %v; XML shape: %s", err, trace.shape)
	}
	t.Log("atomic ROA replacement passed; cleanup will restore the original inventories")
}
