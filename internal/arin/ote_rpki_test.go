package arin

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
		if r.Name == name || r.Name == name+"-shared" {
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
	// Prove ownership of every potential auto-created route before any mutation.
	routeIDs := make([]string, 0, len(resources))
	for _, resource := range resources {
		id := fmt.Sprintf("%s,AS%d", resource.Prefix, original.CustomerASN)
		if _, err := c.GetIRRRoute(ctx, id); !IsNotFound(err) {
			t.Fatalf("disposable route must be absent before testing: %v", err)
		}
		routeIDs = append(routeIDs, id)
	}
	for _, resource := range resources {
		if _, err := c.request(ctx, http.MethodGet, c.baseURL, "/rest/irr/route/"+resource.Prefix+"/AS0", "application/xml", true, nil); !IsNotFound(err) {
			t.Fatalf("AS0 route must be absent before testing: %v", err)
		}
	}
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
		Names       []string
	}{org, beforeROAs, beforeASPAs, request, changed, []string{name, name + "-shared"}})
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
			if r.Name == name || r.Name == name+"-shared" {
				cleanup.DeleteROAs = append(cleanup.DeleteROAs, ROADelete{Handle: r.Handle, AutoLink: true})
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
		for _, id := range routeIDs {
			route, err := c.GetIRRRoute(ctx, id)
			if IsNotFound(err) {
				continue
			}
			if err != nil || route.AutoLinkedROAHandle != "" {
				t.Errorf("cannot safely remove disposable IRR route; retain %s: %v", snapshot, err)
				return
			}
			if err := c.DeleteIRRRoute(ctx, id); err != nil {
				t.Errorf("disposable IRR cleanup failed; retain %s: %v", snapshot, err)
				return
			}
			if _, err := c.GetIRRRoute(ctx, id); !IsNotFound(err) {
				t.Errorf("disposable IRR route remains; retain %s: %v", snapshot, err)
				return
			}
		}
		for _, resource := range resources {
			if _, err := c.request(ctx, http.MethodGet, c.baseURL, "/rest/irr/route/"+resource.Prefix+"/AS0", "application/xml", true, nil); !IsNotFound(err) {
				t.Errorf("AS0 route cleanup unconfirmed; retain %s: %v", snapshot, err)
				return
			}
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
	result, err = c.ApplyRPKITransaction(ctx, org, RPKITransaction{DeleteROAs: []ROADelete{{Handle: result.ROAs[0].Handle}}, AddROAs: []ROARequest{next}})
	if err != nil {
		t.Fatalf("ROA replacement failed: %v; XML shape: %s", err, trace.shape)
	}
	t.Log("atomic ROA replacement passed")
	// Authorize one level of more-specific prefixes, not just an explicit default.
	// Verify the entire expanded range still belongs to the original parent.
	next = request
	next.ASN = 0
	next.Resources = slices.Clone(resources)
	for i := range next.Resources {
		host := netip.MustParsePrefix(next.Resources[i].Prefix)
		expanded := netip.PrefixFrom(host.Addr(), host.Bits()-1).Masked()
		for _, spec := range RegistrationReads() {
			if spec.Name != "parent_net" {
				continue
			}
			parent, err := c.ReadRegistration(ctx, spec, map[string]string{"start_address": host.Addr().String(), "end_address": host.Addr().String()})
			if err != nil {
				t.Fatal(err)
			}
			wider, err := c.ReadRegistration(ctx, spec, map[string]string{"start_address": expanded.Addr().String(), "end_address": prefixEnd(expanded).String()})
			if err != nil || wider["org_handle"] != org || wider["handle"] != parent["handle"] {
				t.Fatalf("expanded test prefix is not owned by the original parent: %v", err)
			}
			break
		}
		length := int64(host.Bits())
		next.Resources[i].Prefix = expanded.String()
		next.Resources[i].MaxLength = &length
	}
	result, err = c.ApplyRPKITransaction(ctx, org, RPKITransaction{DeleteROAs: []ROADelete{{Handle: result.ROAs[0].Handle}}, AddROAs: []ROARequest{next}})
	if err != nil {
		t.Fatalf("AS0 with explicit maximum lengths failed: %v", err)
	}
	t.Log("IPv4/IPv6 AS0 ROA with explicit maximum lengths passed")
	for _, deleteLinked := range []bool{false, true} {
		next = request
		next.AutoLink = true
		transaction := RPKITransaction{AddROAs: []ROARequest{next}}
		if len(result.ROAs) > 0 {
			transaction.DeleteROAs = []ROADelete{{Handle: result.ROAs[0].Handle}}
		}
		result, err = c.ApplyRPKITransaction(ctx, org, transaction)
		if err != nil {
			t.Fatalf("auto-linked ROA creation failed: %v", err)
		}
		for _, id := range routeIDs {
			route, err := c.GetIRRRoute(ctx, id)
			if err != nil || route.AutoLinkedROAHandle != result.ROAs[0].Handle {
				t.Fatalf("IRR route does not link to the created ROA: %v", err)
			}
		}
		result, err = c.ApplyRPKITransaction(ctx, org, RPKITransaction{DeleteROAs: []ROADelete{{Handle: result.ROAs[0].Handle, AutoLink: deleteLinked}}})
		if err != nil {
			t.Fatalf("auto-linked ROA deletion failed: %v", err)
		}
		for _, id := range routeIDs {
			route, err := c.GetIRRRoute(ctx, id)
			if deleteLinked {
				if !IsNotFound(err) {
					t.Fatalf("autoLink=true did not remove its IRR route: %v", err)
				}
			} else {
				if err != nil || route.AutoLinkedROAHandle != "" {
					t.Fatalf("autoLink=false did not preserve an unlinked IRR route: %v", err)
				}
				if err = c.DeleteIRRRoute(ctx, id); err != nil {
					t.Fatal(err)
				}
			}
		}
		t.Logf("ROA delete autoLink=%t verified for IPv4 and IPv6 IRR routes", deleteLinked)
	}
	manualRoutes := map[string]*IRRRoute{}
	for _, resource := range resources {
		manual, err := c.CreateIRRRoute(ctx, IRRRoute{Prefix: resource.Prefix, OriginAS: fmt.Sprintf("AS%d", original.CustomerASN), OrgHandle: org, Description: []string{"Disposable manual route description"}, Remarks: []string{"Disposable manual route remark"}})
		if err != nil {
			t.Fatal(err)
		}
		manualRoutes[manual.ID()] = manual
	}
	linkedRequest := request
	linkedRequest.AutoLink = true
	firstLinked, err := c.ApplyRPKITransaction(ctx, org, RPKITransaction{AddROAs: []ROARequest{linkedRequest}})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range routeIDs {
		route, err := c.GetIRRRoute(ctx, id)
		if err != nil || route.AutoLinkedROAHandle != firstLinked.ROAs[0].Handle {
			t.Fatalf("manual route adoption failed: %v", err)
		}
		manual := manualRoutes[id]
		if !slices.Equal(route.Description, manual.Description) || !slices.Equal(route.POCs, manual.POCs) || route.NetHandle != manual.NetHandle || route.OrgHandle != manual.OrgHandle {
			t.Fatal("manual route adoption changed registration metadata")
		}
		if !slices.Contains(route.Remarks, manual.Remarks[0]) || len(route.Remarks) != len(manual.Remarks)+1 {
			t.Fatal("linked manual route lost its original remarks or did not gain the link annotation")
		}
	}
	beforeDuplicate, err := c.ListROAs(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	beforeDuplicateASPAs, err := c.ListASPAs(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	linkedRequest.Name = name + "-shared"
	_, rejected = c.ApplyRPKITransaction(ctx, org, RPKITransaction{AddROAs: []ROARequest{linkedRequest}})
	if !errors.As(rejected, &apiErr) || apiErr.StatusCode != 400 || !strings.Contains(apiErr.Message, "same origin AS") {
		t.Fatalf("unexpected duplicate-origin/prefix result: %v", rejected)
	}
	afterDuplicate, err := c.ListROAs(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	afterDuplicateASPAs, err := c.ListASPAs(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(canonicalROAs(beforeDuplicate), canonicalROAs(afterDuplicate)) || !reflect.DeepEqual(canonicalASPAs(beforeDuplicateASPAs), canonicalASPAs(afterDuplicateASPAs)) {
		t.Fatal("rejected duplicate ROA changed RPKI inventories")
	}
	for _, id := range routeIDs {
		route, err := c.GetIRRRoute(ctx, id)
		if err != nil || route.AutoLinkedROAHandle != firstLinked.ROAs[0].Handle {
			t.Fatalf("rejected duplicate ROA changed the original IRR link: %v", err)
		}
	}
	t.Log("duplicate origin/prefix ROA rejected without changing either inventory or existing IRR links")
	if _, err := c.ApplyRPKITransaction(ctx, org, RPKITransaction{DeleteROAs: []ROADelete{{Handle: firstLinked.ROAs[0].Handle, AutoLink: false}}}); err != nil {
		t.Fatal(err)
	}
	for _, id := range routeIDs {
		route, err := c.GetIRRRoute(ctx, id)
		if err != nil || route.AutoLinkedROAHandle != "" {
			t.Fatalf("manual route was not preserved and unlinked: %v", err)
		}
		manual := manualRoutes[id]
		if !slices.Equal(route.Description, manual.Description) || !slices.Equal(route.Remarks, manual.Remarks) || !slices.Equal(route.POCs, manual.POCs) || route.NetHandle != manual.NetHandle || route.OrgHandle != manual.OrgHandle {
			t.Fatal("unlinking did not restore original manual route metadata")
		}
		if err := c.DeleteIRRRoute(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	t.Log("manual IPv4/IPv6 route adoption preserved metadata and added a remark; unlinking removed only that annotation")
	// Deliberately bypass local validation to retain evidence of the API's
	// silent AS0 normalization. Never use this raw path in provider writes.
	as0 := roaRequestXML{Name: name, ASN: 0, AutoLink: true}
	for _, resource := range resources {
		p := netip.MustParsePrefix(resource.Prefix)
		as0.Resources = append(as0.Resources, roaResourceXML{Start: p.Addr().String(), CIDR: p.Bits()})
	}
	body, err := xml.Marshal(rpkiTransactionXML{AddROAs: &roaAddListXML{Items: []roaRequestXML{as0}}})
	if err != nil {
		t.Fatal(err)
	}
	native, err := c.request(ctx, http.MethodPost, c.baseURL, "/rest/rpki/"+org, "application/xml", true, body)
	if err != nil {
		t.Fatal(err)
	}
	if native.StatusCode != http.StatusOK && native.StatusCode != http.StatusCreated {
		t.Fatal("AS0 probe did not complete")
	}
	result, err = decodeRPKITransaction(native.Body)
	if err != nil || len(result.ROAs) != 1 {
		t.Fatalf("AS0 probe returned an incomplete transaction: %v", err)
	}
	next = request
	next.ASN = 0
	probeROAs, err := c.ListROAs(ctx, org)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range probeROAs {
		if a.Handle == result.ROAs[0].Handle {
			if !ROAMatchesRequest(a, next) {
				t.Fatal("AS0 did not normalize to an unlinked authorization")
			}
			found = true
		}
	}
	if !found {
		t.Fatal("accepted AS0 ROA missing from inventory")
	}
	for _, resource := range resources {
		if _, err := c.request(ctx, http.MethodGet, c.baseURL, "/rest/irr/route/"+resource.Prefix+"/AS0", "application/xml", true, nil); !IsNotFound(err) {
			t.Fatalf("unexpected AS0 IRR route: %v", err)
		}
	}
	t.Log("native AS0 autoLink=true accepted as an unlinked ROA with no IRR objects; local validation rejects this combination")
	t.Log("cleanup will verify restoration of both RPKI inventories and route absence")
}
