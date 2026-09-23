package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSourceWithConfigure = &registrationDataSource{}

type registrationDataSource struct {
	spec   arin.ReadSpec
	client *arin.Client
}

func newRegistrationDataSource(spec arin.ReadSpec) datasource.DataSource {
	return &registrationDataSource{spec: spec}
}
func (d *registrationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + d.spec.Name
}
func (d *registrationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attributes := map[string]schema.Attribute{}
	for _, field := range outputFields(d.spec) {
		attributes[field.Name] = fieldSchema(field, d.spec.Sensitive)
	}
	for _, in := range d.spec.Inputs {
		description := in.Description
		if in.Default != "" {
			description += " Defaults to `" + in.Default + "`."
		}
		switch in.Kind {
		case "bool":
			attributes[in.Name] = schema.BoolAttribute{Required: in.Default == "", Optional: in.Default != "", Sensitive: d.spec.Sensitive, MarkdownDescription: description}
		case "asn", "id":
			attributes[in.Name] = schema.Int64Attribute{Required: in.Default == "", Optional: in.Default != "", Sensitive: d.spec.Sensitive, MarkdownDescription: description}
		default:
			attributes[in.Name] = schema.StringAttribute{Required: in.Default == "", Optional: in.Default != "", Sensitive: d.spec.Sensitive, MarkdownDescription: description}
		}
	}
	resp.Schema = schema.Schema{MarkdownDescription: d.spec.Description, Attributes: attributes}
}
func (d *registrationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*arin.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider client", "The provider did not supply an ARIN API client.")
		return
	}
	d.client = client
}
func (d *registrationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	params := map[string]string{}
	for _, in := range d.spec.Inputs {
		value := in.Default
		switch in.Kind {
		case "bool":
			var v types.Bool
			resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(in.Name), &v)...)
			if v.IsUnknown() {
				resp.Diagnostics.AddAttributeError(path.Root(in.Name), "Unknown input", "This value must be known before reading ARIN.")
				return
			}
			if !v.IsNull() {
				value = strconv.FormatBool(v.ValueBool())
			}
		case "asn", "id":
			var v types.Int64
			resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(in.Name), &v)...)
			if v.IsUnknown() {
				resp.Diagnostics.AddAttributeError(path.Root(in.Name), "Unknown input", "This value must be known before reading ARIN.")
				return
			}
			if !v.IsNull() {
				value = strconv.FormatInt(v.ValueInt64(), 10)
			}
		default:
			var v types.String
			resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root(in.Name), &v)...)
			if v.IsUnknown() {
				resp.Diagnostics.AddAttributeError(path.Root(in.Name), "Unknown input", "This value must be known before reading ARIN.")
				return
			}
			if !v.IsNull() {
				value = v.ValueString()
			}
		}
		params[in.Name] = value
	}
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Missing ARIN client", "Configure the provider before reading ARIN.")
		return
	}
	values, err := d.client.ReadRegistration(ctx, d.spec, params)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read ARIN "+d.spec.Name, err.Error())
		return
	}
	if d.spec.Name == "irr_route" || d.spec.Name == "irr_aut_num" {
		field := "origin_as"
		if d.spec.Name == "irr_aut_num" {
			field = "as_number"
		}
		actual, ok := values[field].(string)
		if !ok || strings.TrimPrefix(strings.ToUpper(actual), "AS") != params["asn"] {
			resp.Diagnostics.AddError("Mismatched ARIN object", "The response ASN does not match the requested ASN.")
			return
		}
	}
	resp.State.Raw = req.Config.Raw
	for _, field := range outputFields(d.spec) {
		// Preserve configured identities, but never silently associate a mismatched record.
		if requested, exists := params[field.Name]; exists {
			actual := values[field.Name]
			if (actual == nil && field.Required) || (actual != nil && !strings.EqualFold(strings.TrimSuffix(fmt.Sprint(actual), "."), strings.TrimSuffix(requested, "."))) {
				resp.Diagnostics.AddError("Mismatched ARIN object", "The response identity does not match the requested "+field.Name+".")
			}
			continue
		}
		value, err := fieldValue(field, values[field.Name])
		if err != nil {
			resp.Diagnostics.AddError("Invalid ARIN data", err.Error())
			continue
		}
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(field.Name), value)...)
	}
}

func outputFields(spec arin.ReadSpec) []arin.Field {
	if spec.Collection {
		description := "Records returned by ARIN, ordered deterministically. An empty collection is an empty list; authorization and missing-object errors remain errors."
		if spec.Name == "rdap_entities" {
			description = "Entity records returned by ARIN, ordered by handle. No matches, including a structured RDAP 404, produce an empty list. Other failures remain errors."
		}
		return []arin.Field{{Name: spec.Output, Kind: arin.ObjectsKind, Fields: spec.Fields, Description: description}}
	}
	return spec.Fields
}
func fieldSchema(f arin.Field, sensitive bool) schema.Attribute {
	sensitive = sensitive || f.Sensitive
	switch f.Kind {
	case arin.IntKind:
		return schema.Int64Attribute{Computed: true, Sensitive: sensitive, MarkdownDescription: f.Description}
	case arin.BoolKind:
		return schema.BoolAttribute{Computed: true, Sensitive: sensitive, MarkdownDescription: f.Description}
	case arin.StringsKind, arin.IntsKind:
		element := attr.Type(types.StringType)
		if f.Kind == arin.IntsKind {
			element = types.Int64Type
		}
		return schema.ListAttribute{Computed: true, Sensitive: sensitive, ElementType: element, MarkdownDescription: f.Description}
	case arin.ObjectsKind:
		children := map[string]schema.Attribute{}
		for _, child := range f.Fields {
			children[child.Name] = fieldSchema(child, sensitive)
		}
		return schema.ListNestedAttribute{Computed: true, Sensitive: sensitive, MarkdownDescription: f.Description, NestedObject: schema.NestedAttributeObject{Attributes: children}}
	default:
		return schema.StringAttribute{Computed: true, Sensitive: sensitive, MarkdownDescription: f.Description}
	}
}
func fieldType(f arin.Field) attr.Type {
	switch f.Kind {
	case arin.IntKind:
		return types.Int64Type
	case arin.BoolKind:
		return types.BoolType
	case arin.StringsKind:
		return types.ListType{ElemType: types.StringType}
	case arin.IntsKind:
		return types.ListType{ElemType: types.Int64Type}
	case arin.ObjectsKind:
		return types.ListType{ElemType: recordType(f.Fields)}
	default:
		return types.StringType
	}
}
func recordType(fields []arin.Field) types.ObjectType {
	attrs := map[string]attr.Type{}
	for _, f := range fields {
		attrs[f.Name] = fieldType(f)
	}
	return types.ObjectType{AttrTypes: attrs}
}
func fieldValue(f arin.Field, value any) (attr.Value, error) {
	if value == nil {
		switch f.Kind {
		case arin.IntKind:
			return types.Int64Null(), nil
		case arin.BoolKind:
			return types.BoolNull(), nil
		case arin.StringsKind, arin.IntsKind, arin.ObjectsKind:
			return types.ListNull(fieldType(f).(types.ListType).ElemType), nil
		default:
			return types.StringNull(), nil
		}
	}
	switch f.Kind {
	case arin.StringKind:
		v, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("invalid string field %s", f.Name)
		}
		return types.StringValue(v), nil
	case arin.IntKind:
		v, ok := value.(int64)
		if !ok {
			return nil, fmt.Errorf("invalid integer field %s", f.Name)
		}
		return types.Int64Value(v), nil
	case arin.BoolKind:
		v, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("invalid boolean field %s", f.Name)
		}
		return types.BoolValue(v), nil
	default:
		list, ok := value.([]any)
		if !ok {
			return nil, fmt.Errorf("invalid list field %s", f.Name)
		}
		elements := make([]attr.Value, 0, len(list))
		for _, item := range list {
			var element attr.Value
			if f.Kind == arin.ObjectsKind {
				record, ok := item.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid object field %s", f.Name)
				}
				attributes := map[string]attr.Value{}
				for _, child := range f.Fields {
					v, err := fieldValue(child, record[child.Name])
					if err != nil {
						return nil, err
					}
					attributes[child.Name] = v
				}
				v, diags := types.ObjectValue(recordType(f.Fields).AttrTypes, attributes)
				if diags.HasError() {
					return nil, fmt.Errorf("invalid object schema for %s", f.Name)
				}
				element = v
			} else {
				kind := arin.StringKind
				if f.Kind == arin.IntsKind {
					kind = arin.IntKind
				}
				v, err := fieldValue(arin.Field{Name: f.Name, Kind: kind}, item)
				if err != nil {
					return nil, err
				}
				element = v
			}
			elements = append(elements, element)
		}
		result, diags := types.ListValue(fieldType(f).(types.ListType).ElemType, elements)
		if diags.HasError() {
			return nil, fmt.Errorf("invalid list schema for %s", f.Name)
		}
		return result, nil
	}
}
