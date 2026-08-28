// Copyright 2026 Mosh Labs
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &ResourceDataSource{}

func NewResourceDataSource() datasource.DataSource {
	return &ResourceDataSource{}
}

type ResourceDataSource struct{}

// resourceContextModel mirrors the `context` object system-resource accepts.
// It's structurally identical to moshlabs_context's resolved output, but
// system-resource (and so this data source) only ever reads
// account/environment/service — metadata is accepted so the whole output of
// a moshlabs_context data source can be passed straight through (extra
// attributes like labels/scope get dropped automatically), but it's never
// used in the naming logic.
type resourceContextModel struct {
	Account     types.String `tfsdk:"account"`
	Environment types.String `tfsdk:"environment"`
	Service     types.String `tfsdk:"service"`
	Metadata    types.Map    `tfsdk:"metadata"`
}

type resourceDataSourceModel struct {
	Context   *resourceContextModel `tfsdk:"context"`
	Name      types.String          `tfsdk:"name"`
	Delimiter types.String          `tfsdk:"delimiter"`
	Scoped    types.Bool            `tfsdk:"scoped"`
	RootScope types.String          `tfsdk:"root_scope"`
	Result    types.String          `tfsdk:"result"`
}

func (d *ResourceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_resource"
}

func (d *ResourceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Builds a consistently-delimited resource name from a context (account/environment/service), matching modules/system-resource's naming convention: {account?}-{environment?}-{service?}-{name}, where how much of the hierarchy is included is controlled by root_scope.",
		Attributes: map[string]schema.Attribute{
			"context": schema.SingleNestedAttribute{
				Description: "Typically the whole output of a moshlabs_context data source (or its `init`-shaped equivalent). Only account/environment/service are used.",
				Optional:    true,
				Attributes: map[string]schema.Attribute{
					"account":     schema.StringAttribute{Optional: true},
					"environment": schema.StringAttribute{Optional: true},
					"service":     schema.StringAttribute{Optional: true},
					"metadata": schema.MapAttribute{
						ElementType: types.StringType,
						Optional:    true,
					},
				},
			},
			"name": schema.StringAttribute{
				Description: "The resource-specific part of the name. Dots and underscores are replaced with dashes. Optional as long as scoped is not false.",
				Optional:    true,
			},
			"delimiter": schema.StringAttribute{
				Description: "Separator joining the scope prefix and name. Defaults to \"-\".",
				Optional:    true,
			},
			"scoped": schema.BoolAttribute{
				Description: "Whether to prepend the account/environment/service prefix. Defaults to true. Must be false only if name is set.",
				Optional:    true,
			},
			"root_scope": schema.StringAttribute{
				Description: "How much of the hierarchy to include in the prefix: \"account\" includes account+environment+service, \"environment\" (default) includes environment+service, \"service\" includes only service.",
				Optional:    true,
			},
			"result": schema.StringAttribute{
				Description: "The computed name.",
				Computed:    true,
			},
		},
	}
}

func (d *ResourceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config resourceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := resourceNameInput{
		Name:      stringPtrFromTF(config.Name),
		Delimiter: stringPtrFromTF(config.Delimiter),
		RootScope: stringPtrFromTF(config.RootScope),
	}
	if !config.Scoped.IsNull() && !config.Scoped.IsUnknown() {
		v := config.Scoped.ValueBool()
		input.Scoped = &v
	}
	if config.Context != nil {
		input.Account = stringPtrFromTF(config.Context.Account)
		input.Environment = stringPtrFromTF(config.Context.Environment)
		input.Service = stringPtrFromTF(config.Context.Service)
	}

	result, err := computeResourceName(input)
	if err != nil {
		resp.Diagnostics.AddError("Unable to compute resource name", err.Error())
		return
	}

	config.Result = types.StringValue(result)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
