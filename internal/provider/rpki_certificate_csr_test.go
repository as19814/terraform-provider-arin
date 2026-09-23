package provider

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"testing"
)

func certificateCSRFixture(t *testing.T, key *rsa.PrivateKey, name string) string {
	t.Helper()
	sia, err := asn1.Marshal([]struct {
		Method   asn1.ObjectIdentifier
		Location asn1.RawValue
	}{
		{asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 48, 5}, asn1.RawValue{Class: 2, Tag: 6, Bytes: []byte("rsync://repo.example/module/child/")}},
		{asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 48, 10}, asn1.RawValue{Class: 2, Tag: 6, Bytes: []byte("rsync://repo.example/module/child/manifest.mft")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: name}, ExtraExtensions: []pkix.Extension{
		{Id: asn1.ObjectIdentifier{2, 5, 29, 19}, Critical: true, Value: []byte{0x30, 3, 1, 1, 0xff}},
		{Id: asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 11}, Value: sia},
	}}, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}))
}

func TestCertificateUnknownCSRPlan(t *testing.T) {
	ctx := context.Background()
	r := &rpkiCertificateResource{}
	var schema resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schema)
	state := tfsdk.State{Schema: schema.Schema}
	m := rpkiCertificateModel{CSR: types.StringValue("prior"), Notifications: types.ListNull(types.StringType)}
	if d := state.Set(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	plan := tfsdk.Plan{Schema: schema.Schema, Raw: state.Raw}
	if d := plan.SetAttribute(ctx, path.Root("csr_pem"), types.StringUnknown()); d.HasError() {
		t.Fatal(d)
	}
	response := resource.ModifyPlanResponse{Plan: plan}
	r.ModifyPlan(ctx, resource.ModifyPlanRequest{State: state, Plan: plan}, &response)
	if !response.Diagnostics.HasError() || len(response.RequiresReplace) != 0 {
		t.Fatal("unknown update guessed whether to revoke")
	}
	state.Raw = tftypes.NewValue(state.Raw.Type(), nil)
	response = resource.ModifyPlanResponse{Plan: plan}
	r.ModifyPlan(ctx, resource.ModifyPlanRequest{State: state, Plan: plan}, &response)
	if response.Diagnostics.HasError() {
		t.Fatal("unknown initial CSR should defer to apply")
	}
}
