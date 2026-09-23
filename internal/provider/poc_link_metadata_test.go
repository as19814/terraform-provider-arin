package provider

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func fakePOCDescriptions(body string) string {
	return regexp.MustCompile(`<pocLinkRef\b[^>]*>`).ReplaceAllStringFunc(body, func(tag string) string {
		tag = regexp.MustCompile(` description="[^"]*"`).ReplaceAllString(tag, "")
		for role, description := range map[string]string{"AD": "Admin", "T": "Tech", "AB": "Abuse", "N": "NOC", "R": "Routing", "D": "DNS"} {
			if strings.Contains(tag, `function="`+role+`"`) {
				return strings.Replace(tag, "<pocLinkRef", `<pocLinkRef description="`+description+`"`, 1)
			}
		}
		return tag
	})
}

func TestPOCLinkMetadataRecovery(t *testing.T) {
	ctx := context.Background()
	links := func(handle, role string, description types.String) types.Set {
		value, d := types.SetValueFrom(ctx, irrPOCType, []irrPOCModel{{Handle: types.StringValue(handle), Function: types.StringValue(role), Description: description}})
		if d.HasError() {
			t.Fatal(d)
		}
		return value
	}
	desired := links("TECH-ARIN", "T", types.StringUnknown())
	actual := links("TECH-ARIN", "T", types.StringValue("Tech"))
	pending := pendingPOCLinks(desired)
	value, err := pending.ToTerraformValue(ctx)
	if err != nil || !value.IsFullyKnown() {
		t.Fatal("pending receipt contains unknown metadata")
	}
	if !pocLinksWritableEqual(pending, actual) {
		t.Fatal("server label prevented association recovery")
	}
	if pocLinksWritableEqual(pending, links("OTHER-ARIN", "T", types.StringValue("Tech"))) || pocLinksWritableEqual(pending, links("TECH-ARIN", "N", types.StringValue("NOC"))) {
		t.Fatal("changed association accepted as the requested state")
	}
	if !pendingPOCLinks(actual).Equal(actual) {
		t.Fatal("pending receipt changed known metadata")
	}
}
