// Copyright 2026 Mosh Labs
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

var _ provider.Provider = &MoshlabsProvider{}

// MoshlabsProvider replaces modules/system-context: instead of a Terraform
// module fanning out into nested account/environment/service module calls
// (67+ call sites, each adding several graph vertices — see ARCHITECTURE.md),
// the account -> environment -> service cascade is computed once in Go and
// exposed as a single data source read.
type MoshlabsProvider struct {
	version string
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &MoshlabsProvider{version: version}
	}
}

func (p *MoshlabsProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "moshlabs"
	resp.Version = p.version
}

func (p *MoshlabsProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Mosh Labs system-context primitives (account/environment/service scoping) as native Terraform data sources instead of nested modules.",
	}
}

// Configure passes the actual calling Terraform's version down to resources
// (moshlabs_state uses it to default terraform_version to the real running
// version, rather than a guessed constant).
func (p *MoshlabsProvider) Configure(_ context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	resp.ResourceData = req.TerraformVersion
}

func (p *MoshlabsProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewContextDataSource,
		NewResourceDataSource,
	}
}

func (p *MoshlabsProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewStateResource,
	}
}
