package arin

import (
	"context"
	"encoding/xml"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
)

// These payload probes run only inside the journaled disposable RPKI fixture.
// They never change an organization, POC record or network registration.
func auditOTERouteFields(t *testing.T, ctx context.Context, c *Client, route *IRRRoute) {
	t.Helper()
	if !handlePattern.MatchString(route.NetHandle) {
		t.Fatal("invalid baseline network handle")
	}
	if len(route.POCs) < 2 {
		t.Fatal("route field audit requires at least two baseline POCs")
	}
	input := *route
	if input.AutoLinkedROAHandle != "" {
		remarks, err := LinkedRouteUserRemarks(input)
		if err != nil {
			t.Fatal(err)
		}
		input.Remarks = remarks
	}
	body, err := input.marshal()
	if err != nil {
		t.Fatal(err)
	}
	path, err := routePath(route.ID())
	if err != nil {
		t.Fatal(err)
	}
	pocXML := func(pocs []IRRPOC) string {
		type ref struct {
			Handle   string `xml:"handle,attr"`
			Function string `xml:"function,attr"`
		}
		payload := struct {
			XMLName xml.Name `xml:"pocLinks"`
			Refs    []ref    `xml:"pocLinkRef"`
		}{}
		for _, p := range pocs {
			payload.Refs = append(payload.Refs, ref{p.Handle, p.Function})
		}
		b, err := xml.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	probes := []struct{ name, extra string }{
		{"echo-pocs", pocXML(route.POCs)},
		{"empty-pocs", pocXML(nil)},
		{"echo-net-handle", "<netHandle>" + route.NetHandle + "</netHandle>"},
		{"empty-net-handle", "<netHandle></netHandle>"},
		{"nonexistent-net-handle", "<netHandle>NET-TF-NOT-A-REAL-NET</netHandle>"},
	}
	if len(route.POCs) > 1 {
		probes = append(probes, struct{ name, extra string }{"subset-pocs", pocXML(route.POCs[:1])})
	}
	samePOCs := func(a, b []IRRPOC) bool {
		a = slices.Clone(a)
		b = slices.Clone(b)
		less := func(a, b IRRPOC) int {
			return strings.Compare(a.Handle+":"+a.Function+":"+a.Description, b.Handle+":"+b.Function+":"+b.Description)
		}
		slices.SortFunc(a, less)
		slices.SortFunc(b, less)
		return slices.Equal(a, b)
	}
	for _, probe := range probes {
		payload := strings.Replace(string(body), "</route>", probe.extra+"</route>", 1)
		response, writeErr := c.request(ctx, http.MethodPut, c.baseURL, path, "application/xml", true, []byte(payload))
		if writeErr != nil {
			var apiErr *APIError
			if !errors.As(writeErr, &apiErr) {
				t.Fatal(writeErr)
			}
			if probe.name != "subset-pocs" || apiErr.StatusCode != 400 || !strings.Contains(apiErr.Message, "system generated value") {
				t.Fatalf("unexpected field rejection for %s: %v", probe.name, writeErr)
			}
			t.Logf("route field probe %s (linked=%t): HTTP %d, %s", probe.name, route.AutoLinkedROAHandle != "", apiErr.StatusCode, apiErr.Message)
		} else {
			if probe.name == "subset-pocs" || response.StatusCode != 200 {
				t.Fatalf("unexpected field update response for %s: HTTP %d", probe.name, response.StatusCode)
			}
			t.Logf("route field probe %s (linked=%t): HTTP %d", probe.name, route.AutoLinkedROAHandle != "", response.StatusCode)
		}
		after, err := c.GetIRRRoute(ctx, route.ID())
		if err != nil {
			t.Fatal(err)
		}

		if after.ID() != route.ID() || !slices.Equal(after.MemberOf, route.MemberOf) || !samePOCs(after.POCs, route.POCs) || after.NetHandle != route.NetHandle || after.OrgHandle != route.OrgHandle || after.AutoLinkedROAHandle != route.AutoLinkedROAHandle || !slices.Equal(after.Description, route.Description) || !slices.Equal(after.Remarks, route.Remarks) {
			t.Fatalf("probe %s changed route fields (POCs same=%t, net same=%t); writable field support needs reconciliation", probe.name, samePOCs(after.POCs, route.POCs), after.NetHandle == route.NetHandle)
		}
	}
	if route.AutoLinkedROAHandle != "" {
		return
	}
	// Recreate only the known disposable route so create-time fields receive
	// direct evidence, rather than inheriting assumptions from PUT behavior.
	for _, probe := range probes {
		if probe.name == "echo-net-handle" || probe.name == "empty-net-handle" {
			continue
		}
		if err := c.DeleteIRRRoute(ctx, route.ID()); err != nil {
			t.Fatal(err)
		}
		if _, err := c.GetIRRRoute(ctx, route.ID()); !IsNotFound(err) {
			t.Fatalf("disposable route deletion not confirmed: %v", err)
		}
		payload := strings.Replace(string(body), "</route>", probe.extra+"</route>", 1)
		response, writeErr := c.request(ctx, http.MethodPost, c.baseURL, path, "application/xml", true, []byte(payload))
		if writeErr != nil {
			var apiErr *APIError
			if !errors.As(writeErr, &apiErr) || apiErr.StatusCode != 400 || probe.name != "subset-pocs" || !strings.Contains(apiErr.Message, "system generated value") {
				t.Fatalf("unexpected create field response for %s: %v", probe.name, writeErr)
			}
			t.Logf("route create field probe %s: HTTP %d, %s", probe.name, apiErr.StatusCode, apiErr.Message)
			if _, err := c.GetIRRRoute(ctx, route.ID()); !IsNotFound(err) {
				t.Fatalf("rejected create left a route: %v", err)
			}
			if _, err := c.CreateIRRRoute(ctx, input); err != nil {
				t.Fatal(err)
			}
		} else {
			if probe.name == "subset-pocs" || (response.StatusCode != 200 && response.StatusCode != 201) {
				t.Fatalf("unconfirmed create field response: HTTP %d", response.StatusCode)
			}
			t.Logf("route create field probe %s: HTTP %d", probe.name, response.StatusCode)
		}
		actual, err := c.GetIRRRoute(ctx, route.ID())
		if err != nil {
			t.Fatal(err)
		}
		if actual.ID() != route.ID() || !slices.Equal(actual.MemberOf, route.MemberOf) || actual.OrgHandle != route.OrgHandle || actual.NetHandle != route.NetHandle || !samePOCs(actual.POCs, route.POCs) || !slices.Equal(actual.Description, route.Description) || !slices.Equal(actual.Remarks, route.Remarks) || actual.AutoLinkedROAHandle != "" {
			t.Fatalf("create probe %s changed supposedly generated fields", probe.name)
		}
	}
}
