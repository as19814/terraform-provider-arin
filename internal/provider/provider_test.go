package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestConfigure(t *testing.T) {
	for _, tc := range []struct {
		name            string
		key, base       types.String
		timeout         types.Int64
		envKey, envBase string
		wantError       bool
	}{
		{"environment", types.StringNull(), types.StringNull(), types.Int64Null(), "test-key", "", false},
		{"explicit overrides env", types.StringValue("key"), types.StringValue("https://reg.ote.arin.net"), types.Int64Value(10), "", "invalid", false},
		{"public access without key", types.StringNull(), types.StringNull(), types.Int64Null(), "", "", false},
		{"explicit empty key", types.StringValue(""), types.StringNull(), types.Int64Null(), "env-key", "", true},
		{"unknown key", types.StringUnknown(), types.StringNull(), types.Int64Null(), "env-key", "", true},
		{"unknown origin", types.StringValue("key"), types.StringUnknown(), types.Int64Null(), "", "", true},
		{"unknown timeout", types.StringValue("key"), types.StringNull(), types.Int64Unknown(), "", "", true},
		{"invalid origin", types.StringValue("key"), types.StringValue("http://example.net"), types.Int64Null(), "", "", true},
		{"empty origin", types.StringValue("key"), types.StringValue(""), types.Int64Null(), "", "", true},
		{"zero timeout", types.StringValue("key"), types.StringNull(), types.Int64Value(0), "", "", true},
		{"large timeout", types.StringValue("key"), types.StringNull(), types.Int64Value(301), "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ARIN_RDAP_BASE_URL", "")
			t.Setenv("ARIN_API_KEY", tc.envKey)
			t.Setenv("ARIN_BASE_URL", tc.envBase)
			ctx := context.Background()
			p := New("test")()
			var schemaResp provider.SchemaResponse
			p.Schema(ctx, provider.SchemaRequest{}, &schemaResp)
			state := tfsdk.State{Schema: schemaResp.Schema}
			diags := state.Set(ctx, providerModel{APIKey: tc.key, BaseURL: tc.base, TimeoutSeconds: tc.timeout})
			if diags.HasError() {
				t.Fatal(diags)
			}
			var resp provider.ConfigureResponse
			p.Configure(ctx, provider.ConfigureRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: state.Raw}, TerraformVersion: "test"}, &resp)
			if resp.Diagnostics.HasError() != tc.wantError {
				t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
			}
			if !tc.wantError && (resp.DataSourceData == nil || resp.ResourceData == nil) {
				t.Fatal("missing configured client")
			}
		})
	}
}

func TestConfigureRDAPOrigin(t *testing.T) {
	for _, tc := range []struct {
		name      string
		origin    types.String
		env       string
		wantError bool
	}{
		{"default", types.StringNull(), "", false},
		{"environment", types.StringNull(), "https://rdap.ote.arin.net", false},
		{"explicit overrides environment", types.StringValue("https://rdap.arin.net"), "invalid", false},
		{"invalid environment", types.StringNull(), "http://example.net", true},
		{"unknown", types.StringUnknown(), "", true},
		{"empty", types.StringValue(""), "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ARIN_API_KEY", "")
			t.Setenv("ARIN_BASE_URL", "")
			t.Setenv("ARIN_RDAP_BASE_URL", tc.env)
			ctx := context.Background()
			p := New("test")()
			var schemaResp provider.SchemaResponse
			p.Schema(ctx, provider.SchemaRequest{}, &schemaResp)
			state := tfsdk.State{Schema: schemaResp.Schema}
			diags := state.Set(ctx, providerModel{APIKey: types.StringNull(), BaseURL: types.StringNull(), RDAPBaseURL: tc.origin, TimeoutSeconds: types.Int64Null()})
			if diags.HasError() {
				t.Fatal(diags)
			}
			var resp provider.ConfigureResponse
			p.Configure(ctx, provider.ConfigureRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: state.Raw}}, &resp)
			if resp.Diagnostics.HasError() != tc.wantError {
				t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
			}
		})
	}
}
