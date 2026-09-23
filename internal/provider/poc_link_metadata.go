package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func mapPOCLinkDescriptions(links types.Set, mapValue func(attr.Value) attr.Value) types.Set {
	if links.IsNull() || links.IsUnknown() {
		return links
	}
	values := make([]attr.Value, 0, len(links.Elements()))
	for _, value := range links.Elements() {
		object := value.(types.Object)
		if object.IsNull() || object.IsUnknown() {
			values = append(values, object)
			continue
		}
		fields := object.Attributes()
		fields["description"] = mapValue(fields["description"])
		values = append(values, types.ObjectValueMust(irrPOCType.AttrTypes, fields))
	}
	return types.SetValueMust(irrPOCType, values)
}

// POC role descriptions are server metadata, not desired association identity.
func pocLinksWritableEqual(a, b types.Set) bool {
	strip := func(attr.Value) attr.Value { return types.StringNull() }
	return mapPOCLinkDescriptions(a, strip).Equal(mapPOCLinkDescriptions(b, strip))
}

// A pending organization receipt must contain no unknown computed descriptions.
// The labels are populated by a confirmed organization read during recovery.
func pendingPOCLinks(links types.Set) types.Set {
	return mapPOCLinkDescriptions(links, func(value attr.Value) attr.Value {
		if value.IsNull() || value.IsUnknown() {
			return types.StringValue("")
		}
		return value
	})
}

func unknownPOCLinkDescriptions(links types.Set) types.Set {
	return mapPOCLinkDescriptions(links, func(attr.Value) attr.Value { return types.StringUnknown() })
}
