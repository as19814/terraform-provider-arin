package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type certificateImport struct {
	Endpoint          string   `json:"endpoint"`
	Child             string   `json:"child_handle"`
	Parent            string   `json:"parent_handle"`
	Journal           string   `json:"journal_directory"`
	KeyFile           string   `json:"signing_key_file"`
	Certificate       string   `json:"signing_certificate_pem"`
	Anchor            string   `json:"signing_ca_pem"`
	Intermediates     *string  `json:"signing_intermediates_pem"`
	CRLs              string   `json:"signing_crls_pem"`
	Peer              string   `json:"peer_ca_pem"`
	PeerIntermediates *string  `json:"peer_intermediates_pem"`
	Class             string   `json:"class_name"`
	CSR               string   `json:"csr_pem"`
	RequestedASN      *string  `json:"requested_asn"`
	RequestedIPv4     *string  `json:"requested_ipv4"`
	RequestedIPv6     *string  `json:"requested_ipv6"`
	ResourceAnchor    string   `json:"resource_anchor_pem"`
	Issuers           string   `json:"issuer_chain_pem"`
	Notifications     []string `json:"rrdp_notifications"`
	Cache             string   `json:"rrdp_cache_directory"`
	History           string   `json:"manifest_history_directory"`
}

var errCertificateImport = errors.New("expected an absolute path to a private regular JSON file (at most 16 MiB) containing certificate, BPKI and resource trust configuration")

func readCertificateImport(filename string) (certificateImport, error) {
	var manifest certificateImport
	fail := func() (certificateImport, error) { return certificateImport{}, errCertificateImport }
	if !filepath.IsAbs(filename) {
		return fail()
	}
	info, err := os.Lstat(filename)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16<<20 {
		return fail()
	}
	f, err := os.Open(filename)
	if err != nil {
		return fail()
	}
	actual, statErr := f.Stat()
	raw, readErr := io.ReadAll(io.LimitReader(f, (16<<20)+1))
	closeErr := f.Close()
	if statErr != nil || !os.SameFile(info, actual) || readErr != nil || closeErr != nil || len(raw) > 16<<20 {
		return fail()
	}
	if bundleImportJSON(json.NewDecoder(bytes.NewReader(raw)), 0) != nil {
		return fail()
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return fail()
	}
	for name := range fields {
		if name != strings.ToLower(name) {
			return fail()
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&manifest) != nil || d.Decode(new(any)) != io.EOF {
		return fail()
	}
	for _, required := range []string{manifest.Endpoint, manifest.Child, manifest.Parent, manifest.Journal, manifest.KeyFile, manifest.Certificate, manifest.Anchor, manifest.CRLs, manifest.Peer, manifest.Class, manifest.CSR, manifest.ResourceAnchor, manifest.Issuers, manifest.Cache, manifest.History} {
		if required == "" {
			return fail()
		}
	}
	if len(manifest.Notifications) == 0 || len(manifest.Notifications) > 31 {
		return fail()
	}
	for _, url := range manifest.Notifications {
		if url == "" {
			return fail()
		}
	}
	return manifest, nil
}

func (r *rpkiCertificateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	manifest, err := readCertificateImport(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid certificate import manifest", err.Error())
		return
	}
	optional := func(s *string) types.String {
		if s == nil {
			return types.StringNull()
		}
		return types.StringValue(*s)
	}
	m := rpkiCertificateModel{Endpoint: types.StringValue(manifest.Endpoint), Child: types.StringValue(manifest.Child), Parent: types.StringValue(manifest.Parent), Journal: types.StringValue(manifest.Journal), KeyFile: types.StringValue(manifest.KeyFile), Certificate: types.StringValue(manifest.Certificate), Anchor: types.StringValue(manifest.Anchor), Intermediates: optional(manifest.Intermediates), CRLs: types.StringValue(manifest.CRLs), Peer: types.StringValue(manifest.Peer), PeerIntermediates: optional(manifest.PeerIntermediates), Class: types.StringValue(manifest.Class), CSR: types.StringValue(manifest.CSR), RequestedASN: optional(manifest.RequestedASN), RequestedIPv4: optional(manifest.RequestedIPv4), RequestedIPv6: optional(manifest.RequestedIPv6), ResourceAnchor: types.StringValue(manifest.ResourceAnchor), Issuers: types.StringValue(manifest.Issuers), Cache: types.StringValue(manifest.Cache), History: types.StringValue(manifest.History)}
	notifications, diags := types.ListValueFrom(ctx, types.StringType, manifest.Notifications)
	m.Notifications = notifications
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	v, diags := m.validation(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	certificate, err := r.read(ctx, m.config(), m.input(), v)
	if err != nil {
		resp.Diagnostics.AddError("Could not verify certificate import", err.Error())
		return
	}
	if certificate == nil {
		resp.Diagnostics.AddError("Certificate import target missing", "The CSR key was not found in the specified resource class. Import does not issue certificates.")
		return
	}
	m.setCertificate(certificate)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
