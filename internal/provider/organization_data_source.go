package provider

import (
	"github.com/as19814/terraform-provider-arin/internal/arin"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

// NewOrganizationDataSource preserves the original constructor and schema names
// while extending organization reads with the shared registration field model.
func NewOrganizationDataSource() datasource.DataSource {
	for _, spec := range arin.RegistrationReads() {
		if spec.Name == "org" {
			return newRegistrationDataSource(spec)
		}
	}
	panic("organization read specification is missing")
}
