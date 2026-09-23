package arin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
)

// LinkedRouteRemark is appended by ARIN while a route is managed by a ROA.
const LinkedRouteRemark = "This object is automatically managed by ARIN"

// LinkedRouteUserRemarks separates the server annotation from editable remarks.
// Require the observed trailing annotation rather than silently discarding text.
func LinkedRouteUserRemarks(r IRRRoute) ([]string, error) {
	if r.AutoLinkedROAHandle == "" {
		return nil, errors.New("route is not linked to a ROA")
	}
	if len(r.Remarks) == 0 || r.Remarks[len(r.Remarks)-1] != LinkedRouteRemark {
		return nil, errors.New("linked route has an unrecognized server remark annotation")
	}
	return slices.Clone(r.Remarks[:len(r.Remarks)-1]), nil
}

// UpdateLinkedIRRRoute owns only editable route metadata. It never changes a ROA
// or its links. Callers must supply the exact currently expected ROA identity.
// The desired remarks exclude ARIN's generated annotation.
func (c *Client) UpdateLinkedIRRRoute(ctx context.Context, want IRRRoute, expectedROA string) (*IRRRoute, error) {
	if !handlePattern.MatchString(expectedROA) {
		return nil, errors.New("expected ROA handle must be valid and nonempty")
	}
	if err := want.Validate(); err != nil {
		return nil, err
	}
	if slices.Contains(want.Remarks, LinkedRouteRemark) {
		return nil, errors.New("remarks must exclude ARIN's generated linked-route annotation")
	}
	unlock, err := lockOrganization(ctx, c.baseURL, want.OrgHandle)
	if err != nil {
		return nil, err
	}
	defer unlock()
	current, err := c.GetIRRRoute(ctx, want.ID())
	if err != nil {
		return nil, err
	}
	if current.AutoLinkedROAHandle != expectedROA || current.OrgHandle != want.OrgHandle {
		return nil, errors.New("route organization or ROA link differs from the expected owner")
	}
	if _, err := LinkedRouteUserRemarks(*current); err != nil {
		return nil, err
	}
	body, err := want.marshal()
	if err != nil {
		return nil, err
	}
	path, err := routePath(want.ID())
	if err != nil {
		return nil, err
	}
	response, err := c.request(ctx, http.MethodPut, c.baseURL, path, "application/xml", true, body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("ARIN did not confirm a completed linked-route update; refresh before retrying")
	}
	returned, err := decodeIRRRoute(response.Body, want.ID())
	if err != nil {
		return nil, fmt.Errorf("accepted linked-route update has an unverifiable response: %w", err)
	}
	if err := verifyLinkedRoute(*returned, want, expectedROA); err != nil {
		return returned, err
	}
	actual, err := c.GetIRRRoute(ctx, want.ID())
	if err != nil {
		return returned, err
	}
	if err := verifyLinkedRoute(*actual, want, expectedROA); err != nil {
		return actual, err
	}
	return actual, nil
}
func verifyLinkedRoute(actual, want IRRRoute, expectedROA string) error {
	if actual.ID() != want.ID() || actual.OrgHandle != want.OrgHandle || actual.AutoLinkedROAHandle != expectedROA {
		return errors.New("linked-route update changed identity or ROA ownership")
	}
	remarks, err := LinkedRouteUserRemarks(actual)
	if err != nil {
		return err
	}
	members, expectedMembers := slices.Clone(actual.MemberOf), slices.Clone(want.MemberOf)
	slices.Sort(members)
	slices.Sort(expectedMembers)
	if !slices.Equal(actual.Description, want.Description) || !slices.Equal(remarks, want.Remarks) || !slices.Equal(members, expectedMembers) {
		return errors.New("linked-route metadata update is not yet confirmed")
	}
	return nil
}

// DeleteLinkedIRRRoute is an explicit, independently scoped deletion. It removes
// the IRR object, not its ROA. A ROA resource configured to maintain this link may
// recreate it, so metadata-only Terraform ownership must not call this method.
func (c *Client) DeleteLinkedIRRRoute(ctx context.Context, id, org, expectedROA string) error {
	if !handlePattern.MatchString(org) || !handlePattern.MatchString(expectedROA) {
		return errors.New("organization and expected ROA handles are required")
	}
	path, err := routePath(id)
	if err != nil {
		return err
	}
	unlock, err := lockOrganization(ctx, c.baseURL, org)
	if err != nil {
		return err
	}
	defer unlock()
	current, err := c.GetIRRRoute(ctx, id)
	if IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if current.OrgHandle != org || current.AutoLinkedROAHandle != expectedROA {
		return errors.New("route organization or ROA link differs from the expected owner")
	}
	response, err := c.request(ctx, http.MethodDelete, c.baseURL, path, "application/xml", true, nil)
	if err != nil && !IsNotFound(err) {
		return err
	}
	if err == nil && response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNoContent {
		return errors.New("ARIN did not confirm completed linked-route deletion; refresh before retrying")
	}
	_, err = c.GetIRRRoute(ctx, id)
	if IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("linked IRR route remains after deletion")
}
