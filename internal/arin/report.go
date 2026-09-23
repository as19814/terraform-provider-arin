package arin

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

const (
	ReportAssociations = "associations"
	ReportReassignment = "reassignment"
	ReportWhoWasASN    = "who_was_asn"
	ReportWhoWasNet    = "who_was_net"
)

// ReportNotSubmittedError identifies a local precondition failure before any
// HTTP request, so Terraform must not retain an uncertain submission receipt.
type ReportNotSubmittedError struct{ Reason string }

func (e *ReportNotSubmittedError) Error() string { return e.Reason }

// ReportRequest is a ticket-creating operation even though its wire method is GET.
// Target is empty for associations, a NET handle for reassignment, a decimal ASN
// for who_was_asn, and an IP address (not a CIDR) for who_was_net.
type ReportRequest struct{ Type, Target string }

func (r ReportRequest) Validate() error { _, err := r.path(); return err }
func (r ReportRequest) path() (string, error) {
	switch r.Type {
	case ReportAssociations:
		if r.Target != "" {
			return "", errors.New("associations reports do not accept a target")
		}
		return "/rest/report/associations", nil
	case ReportReassignment:
		if !handlePattern.MatchString(r.Target) || !strings.HasPrefix(r.Target, "NET") || strings.ToUpper(r.Target) != r.Target {
			return "", errors.New("reassignment report target must be an uppercase NET handle")
		}
		return "/rest/report/reassignment/" + url.PathEscape(r.Target), nil
	case ReportWhoWasASN:
		asn, err := strconv.ParseUint(r.Target, 10, 32)
		if err != nil || asn == 0 || strconv.FormatUint(asn, 10) != r.Target {
			return "", errors.New("WhoWas ASN target must be a canonical decimal ASN between 1 and 4294967295")
		}
		return "/rest/report/whoWas/asn/" + r.Target, nil
	case ReportWhoWasNet:
		address, err := netip.ParseAddr(r.Target)
		if err != nil || address.Is4In6() || address.Zone() != "" || address.String() != r.Target {
			return "", errors.New("WhoWas NET target must be a canonical IPv4 or IPv6 address, not a prefix or NET handle")
		}
		return "/rest/report/whoWas/net/" + url.PathEscape(r.Target), nil
	default:
		return "", errors.New("report type must be associations, reassignment, who_was_asn or who_was_net")
	}
}
func (r ReportRequest) MatchesTicket(t Ticket) bool {
	switch r.Type {
	case ReportAssociations:
		return t.Type == "ASSOCIATIONS_REPORT"
	case ReportReassignment:
		return t.Type == "REASSIGNMENT_REPORT" || t.Type == "USER_REASSIGNMENT_REPORT"
	case ReportWhoWasASN, ReportWhoWasNet:
		return t.Type == "WHOWAS_REPORT"
	default:
		return false
	}
}

// RequestReport submits once and never polls or resubmits. A non-nil ticket must
// be retained even when accompanying validation fails. An error with no ticket
// can still follow an accepted request, and requires manual reconciliation.
func (c *Client) RequestReport(ctx context.Context, r ReportRequest) (*Ticket, error) {
	path, err := r.path()
	if err != nil {
		return nil, &ReportNotSubmittedError{Reason: err.Error()}
	}
	if c.apiKey == "" {
		return nil, &ReportNotSubmittedError{Reason: "api_key or ARIN_API_KEY is required for report requests"}
	}
	response, err := c.requestReport(ctx, path)
	if err != nil {
		return nil, err
	}
	ticket, err := decodeTicket(response.Body, "")
	if err != nil {
		return ticket, err
	}
	if response.StatusCode != 200 && response.StatusCode != 201 && response.StatusCode != 202 {
		return ticket, errors.New("ARIN did not confirm report submission")
	}
	if !r.MatchesTicket(*ticket) {
		return ticket, fmt.Errorf("ARIN returned an unexpected report ticket type %q", ticket.Type)
	}
	return ticket, nil
}
