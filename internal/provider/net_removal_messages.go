package provider

import (
	"context"
	"encoding/base64"
	"maps"
	"slices"

	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var netRemovalMessageType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"subject": types.StringType, "text": types.ListType{ElemType: types.StringType},
	"category": types.StringType, "attachments": types.MapType{ElemType: types.StringType},
}}

type netRemovalMessageModel struct {
	Subject     types.String `tfsdk:"subject"`
	Text        types.List   `tfsdk:"text"`
	Category    types.String `tfsdk:"category"`
	Attachments types.Map    `tfsdk:"attachments"`
}

func netRemovalMessagesSchema() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{Optional: true, Sensitive: true,
		MarkdownDescription: "Messages submitted only when destroying this NET, using ARIN's remove endpoint. Omit or use an empty list for ordinary DELETE. Updating this local policy sends no message. Messages and base64 attachment data remain in Terraform state. Imports have no removal messages.",
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"subject":     schema.StringAttribute{Optional: true, MarkdownDescription: "Message subject on one line."},
			"text":        schema.ListAttribute{Optional: true, ElementType: types.StringType, MarkdownDescription: "Ordered message text lines, including blank lines."},
			"category":    schema.StringAttribute{Optional: true, MarkdownDescription: "NONE or JUSTIFICATION. Omission sends NONE."},
			"attachments": schema.MapAttribute{Optional: true, ElementType: types.StringType, Sensitive: true, MarkdownDescription: "Map of filenames to base64-encoded attachment contents. Filenames must not contain paths."},
		}},
	}
}

func netRemovalMessages(ctx context.Context, values types.List) ([]arin.RegistrationMessage, diag.Diagnostics) {
	var d diag.Diagnostics
	if values.IsNull() {
		return nil, d
	}
	var models []netRemovalMessageModel
	d.Append(values.ElementsAs(ctx, &models, false)...)
	if d.HasError() {
		return nil, d
	}
	messages := make([]arin.RegistrationMessage, 0, len(models))
	for _, model := range models {
		message := arin.RegistrationMessage{Subject: model.Subject.ValueString(), Category: model.Category.ValueString()}
		d.Append(model.Text.ElementsAs(ctx, &message.Text, false)...)
		attachments := map[string]string{}
		d.Append(model.Attachments.ElementsAs(ctx, &attachments, false)...)
		if d.HasError() {
			return nil, d
		}
		// Sort filenames to make the payload stable despite Terraform map ordering.
		for _, filename := range slices.Sorted(maps.Keys(attachments)) {
			data, err := base64.StdEncoding.Strict().DecodeString(attachments[filename])
			if err != nil {
				d.AddError("Invalid NET removal attachment", "Attachment contents must be valid base64.")
				return nil, d
			}
			message.Attachments = append(message.Attachments, arin.RegistrationAttachment{Filename: filename, Data: data})
		}
		messages = append(messages, message)
	}
	if err := arin.ValidateRegistrationMessages(messages); err != nil {
		d.AddError("Invalid NET removal messages", err.Error())
	}
	return messages, d
}

func netStateWithRemoval(ctx context.Context, n *arin.RegisteredNet, messages types.List) (netModel, diag.Diagnostics) {
	out, d := netState(ctx, n)
	out.RemovalMessages = messages
	return out, d
}
