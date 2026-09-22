package arin

import "strings"

func field(name, path string, kind ValueKind) Field {
	return Field{Name: name, Path: path, Kind: kind, Description: strings.ReplaceAll(name, "_", " ") + " returned by ARIN."}
}
func text(name, path string) Field         { return field(name, path, StringKind) }
func integer(name, path string) Field      { return field(name, path, IntKind) }
func boolean(name, path string) Field      { return field(name, path, BoolKind) }
func required(f Field) Field               { f.Required = true; return f }
func secret(f Field) Field                 { f.Sensitive = true; return f }
func stringsField(name, path string) Field { return field(name, path, StringsKind) }
func lines(name, path string) Field {
	f := stringsField(name, path+"/line")
	f.Ordered = true
	f.Description = "Ordered lines of " + strings.ReplaceAll(name, "_", " ") + "."
	return f
}
func objects(name, path string, fields ...Field) Field {
	f := field(name, path, ObjectsKind)
	f.Fields = fields
	return f
}
func joinFields(groups ...[]Field) []Field {
	var result []Field
	for _, g := range groups {
		result = append(result, g...)
	}
	return result
}

var pocLinks = objects("poc_links", "pocLinks/pocLinkRef", required(text("handle", "@handle")), text("function", "@function"), text("description", "@description"))
var addressFields = []Field{
	lines("street_address", "streetAddress"), text("city", "city"), text("subdivision", "iso3166-2"), text("postal_code", "postalCode"), text("country_code", "iso3166-1/code2"), text("country_name", "iso3166-1/name"),
}
var netFields = []Field{
	required(text("handle", "handle")), text("name", "netName"), integer("ip_version", "version"), text("org_handle", "orgHandle"), text("customer_handle", "customerHandle"), text("parent_net_handle", "parentNetHandle"), text("registration_date", "registrationDate"), lines("comments", "comment"),
	stringsField("origin_asns", "originASes/originAS"), pocLinks,
	objects("net_blocks", "netBlocks/netBlock", text("type", "type"), text("description", "description"), required(text("start_address", "startAddress")), required(text("end_address", "endAddress")), integer("cidr_length", "cidrLength")),
}
var orgFields = joinFields([]Field{required(text("handle", "handle")), required(text("name", "orgName")), text("dba_name", "dbaName"), text("registration_date", "registrationDate"), text("rwhois_url", "orgUrl"), secret(text("tax_id", "taxId")), boolean("accept_reassignments", "acceptReassignments"), lines("comments", "comment"), pocLinks}, addressFields)
var customerFields = joinFields([]Field{required(text("handle", "handle")), text("name", "customerName"), text("parent_org_handle", "parentOrgHandle"), boolean("private_customer", "privateCustomer"), text("registration_date", "registrationDate"), lines("comments", "comment")}, addressFields)
var pocFields = joinFields([]Field{
	required(text("handle", "handle")), text("contact_type", "contactType"), text("company_name", "companyName"), text("first_name", "firstName"), text("middle_name", "middleName"), text("last_name", "lastName"), text("registration_date", "registrationDate"), lines("comments", "comment"), stringsField("emails", "emails/email"),
	objects("phones", "phones/phone", text("number", "number"), text("extension", "extension"), text("type", "type/code"), text("description", "type/description")),
}, addressFields)
var delegationFields = []Field{
	required(text("name", "name")),
	objects("nameservers", "nameservers/nameserver", required(text("name", ".")), integer("ttl", "@ttl")),
	objects("ds_records", "delegationKeys/delegationKey", integer("algorithm", "algorithm"), text("algorithm_name", "algorithm/@name"), text("digest", "digest"), integer("digest_type", "digestType"), text("digest_type_name", "digestType/@name"), integer("key_tag", "keyTag"), integer("ttl", "ttl")),
}
var irrCommon = []Field{text("org_handle", "orgHandle"), text("source", "source"), text("creation_date", "creationDate"), text("last_modified_date", "lastModifiedDate"), lines("description", "description"), lines("remarks", "remarks"), pocLinks}
var routeFields = joinFields(irrCommon, []Field{required(text("prefix", "prefix")), required(text("origin_as", "originAS")), text("net_handle", "netHandle"), text("auto_linked_roa_handle", "autoLinkedRoaHandle")})
var autnumFields = joinFields(irrCommon, []Field{required(text("as_number", "asNumber")), text("as_name", "asName"), lines("import_policy", "import"), lines("export_policy", "export"), lines("default_policy", "default"), lines("mp_import_policy", "mpImport"), lines("mp_export_policy", "mpExport"), lines("mp_default_policy", "mpDefault")})
var setFields = joinFields(irrCommon, []Field{required(text("name", "name")), stringsField("members", "members/member/@name"), stringsField("members_by_ref", "membersByRef/memberByRef/@name"), stringsField("mp_members", "mpMembers/mpMember/@name")})
var routeRefFields = []Field{required(text("prefix", "prefix")), required(text("origin_as", "originAS")), text("org_handle", "orgHandle"), text("entry_type", "@entry")}
var roaFields = []Field{
	required(text("handle", "roaHandle")), integer("asn", "asNumber"), text("name", "name"), text("not_valid_before", "notValidBefore"), text("not_valid_after", "notValidAfter"), boolean("auto_renewed", "autoRenewed"),
	objects("resources", "resources", required(text("start_address", "startAddress")), text("end_address", "endAddress"), integer("ip_version", "ipVersion"), integer("cidr_length", "cidrLength"), integer("max_length", "maxLength"), boolean("auto_linked", "autoLinked")),
}
var aspaFields = []Field{required(integer("customer_asn", "customerAsId")), field("provider_asns", "providerAsIds/providerAsId", IntsKind)}
var attachmentRefFields = []Field{text("attachment_id", "attachmentId"), text("filename", "attachmentFilename")}
var messageFields = []Field{
	integer("message_id", "messageId"), text("created_date", "createdDate"), text("subject", "subject"), lines("text", "text"), text("category", "category"),
	objects("attachment_references", "attachmentReferences/attachmentReference", attachmentRefFields...),
	objects("attachments", "attachments/attachment", text("filename", "filename"), secret(text("content_base64", "data"))),
}
var ticketFields = []Field{
	required(text("ticket_number", "ticketNo")), text("org_handle", "orgHandle"), boolean("shared", "shared"), text("created_date", "createdDate"), text("resolved_date", "resolvedDate"), text("closed_date", "closedDate"), text("updated_date", "updatedDate"), text("ticket_type", "webTicketType"), text("ticket_status", "webTicketStatus"), text("resolution", "webTicketResolution"),
	objects("messages", "messages/message", messageFields...),
	objects("message_references", "messageReferences/messageReference", integer("message_id", "messageId"), text("created_date", "createdDate"), text("subject", "subject"), objects("attachment_references", "attachmentReferences/attachmentReference", attachmentRefFields...)),
}
