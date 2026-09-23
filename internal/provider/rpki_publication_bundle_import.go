package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type publicationBundleImport struct {
	ID                string            `json:"id,omitempty"`
	Endpoint          string            `json:"endpoint"`
	Publisher         string            `json:"publisher_handle"`
	Journal           string            `json:"journal_directory"`
	KeyFile           string            `json:"signing_key_file"`
	Certificate       string            `json:"signing_certificate_pem"`
	Anchor            string            `json:"signing_ca_pem"`
	Intermediates     *string           `json:"signing_intermediates_pem,omitempty"`
	CRLs              string            `json:"signing_crls_pem"`
	Peer              string            `json:"peer_ca_pem"`
	PeerIntermediates *string           `json:"peer_intermediates_pem,omitempty"`
	Objects           map[string]string `json:"objects"`
}

var errPublicationBundleImport = errors.New("expected an absolute path to a private regular JSON file (at most 16 MiB) containing publication configuration and nonempty exact object content")

func readPublicationBundleImport(filename string) (publicationBundleImport, error) {
	var manifest publicationBundleImport
	if !filepath.IsAbs(filename) {
		return manifest, errPublicationBundleImport
	}
	info, err := os.Lstat(filename)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16<<20 {
		return manifest, errPublicationBundleImport
	}
	f, err := os.Open(filename)
	if err != nil {
		return manifest, errPublicationBundleImport
	}
	actual, statErr := f.Stat()
	raw, readErr := io.ReadAll(io.LimitReader(f, (16<<20)+1))
	closeErr := f.Close()
	if statErr != nil || !os.SameFile(info, actual) || readErr != nil || closeErr != nil || len(raw) > 16<<20 {
		return manifest, errPublicationBundleImport
	}
	if bundleImportJSON(json.NewDecoder(bytes.NewReader(raw)), 0) != nil {
		return manifest, errPublicationBundleImport
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return manifest, errPublicationBundleImport
	}
	for name := range fields {
		if name != strings.ToLower(name) {
			return manifest, errPublicationBundleImport
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&manifest) != nil || d.Decode(new(any)) != io.EOF {
		return publicationBundleImport{}, errPublicationBundleImport
	}
	for _, required := range []string{manifest.Endpoint, manifest.Publisher, manifest.Journal, manifest.KeyFile, manifest.Certificate, manifest.Anchor, manifest.CRLs, manifest.Peer} {
		if required == "" {
			return publicationBundleImport{}, errPublicationBundleImport
		}
	}
	if len(manifest.Objects) == 0 || (manifest.ID != "" && !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(manifest.ID)) {
		return publicationBundleImport{}, errPublicationBundleImport
	}
	if _, _, err := arin.DecodeRPKIPublicationObjects(manifest.Objects); err != nil {
		return publicationBundleImport{}, errPublicationBundleImport
	}
	return manifest, nil
}
func (r *rpkiPublicationBundleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	manifest, err := readPublicationBundleImport(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid publication bundle import manifest", err.Error())
		return
	}
	optional := func(value *string) types.String {
		if value == nil {
			return types.StringNull()
		}
		return types.StringValue(*value)
	}
	m := rpkiPublicationBundleModel{Endpoint: types.StringValue(manifest.Endpoint), Publisher: types.StringValue(manifest.Publisher), Journal: types.StringValue(manifest.Journal), KeyFile: types.StringValue(manifest.KeyFile), Certificate: types.StringValue(manifest.Certificate), Anchor: types.StringValue(manifest.Anchor), Intermediates: optional(manifest.Intermediates), CRLs: types.StringValue(manifest.CRLs), Peer: types.StringValue(manifest.Peer), PeerIntermediates: optional(manifest.PeerIntermediates)}
	_, hashes, err := arin.DecodeRPKIPublicationObjects(manifest.Objects)
	if err != nil {
		resp.Diagnostics.AddError("Invalid publication bundle import objects", err.Error())
		return
	}
	inventory, err := r.read(ctx, m.config())
	if err != nil {
		resp.Diagnostics.AddError("Could not verify publication bundle import", err.Error())
		return
	}
	observed := map[string]string{}
	for _, object := range inventory.Objects {
		observed[object.URI] = object.SHA256
	}
	for uri, hash := range hashes {
		if observed[uri] != hash {
			resp.Diagnostics.AddError("Publication bundle import mismatch", "Every specified object must exist with exactly the supplied content hash. No objects were adopted or changed.")
			return
		}
	}
	objects, diags := types.MapValueFrom(ctx, types.StringType, manifest.Objects)
	m.Objects = objects
	resp.Diagnostics.Append(diags...)
	m.Hashes, diags = types.MapValueFrom(ctx, types.StringType, hashes)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	m.ID = types.StringValue(manifest.ID)
	if manifest.ID == "" {
		m.assignID(manifest.Objects)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
