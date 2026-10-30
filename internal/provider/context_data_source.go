// Copyright 2026 Mosh Labs
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &ContextDataSource{}

func NewContextDataSource() datasource.DataSource {
	return &ContextDataSource{}
}

type ContextDataSource struct{}

// contextInitModel mirrors the `init` object system-context accepts: the
// already-resolved context of the parent scope (e.g. account's output),
// passed down so this level doesn't have to re-derive it from scratch.
type contextInitModel struct {
	Platform    types.String `tfsdk:"platform"`
	Account     types.String `tfsdk:"account"`
	Environment types.String `tfsdk:"environment"`
	Service     types.String `tfsdk:"service"`
	Metadata    types.Map    `tfsdk:"metadata"`
}

type contextDataSourceModel struct {
	Init        *contextInitModel `tfsdk:"init"`
	Platform    types.String      `tfsdk:"platform"`
	Account     types.String      `tfsdk:"account"`
	Environment types.String      `tfsdk:"environment"`
	Service     types.String      `tfsdk:"service"`
	Metadata    types.Dynamic     `tfsdk:"metadata"`
	Scope       types.String      `tfsdk:"scope"`
	Labels      types.Map         `tfsdk:"labels"`
}

func (d *ContextDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_context"
}

func (d *ContextDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Resolves the platform/account/environment/service context for the calling scope: cascades metadata and builds the labels map, exactly as modules/system-context does, but as a single data source read instead of nested module calls.",
		Attributes: map[string]schema.Attribute{
			"init": schema.SingleNestedAttribute{
				Description: "The already-resolved context of the parent scope (e.g. the output of a `moshlabs_context` data source one level up). Fields set directly (platform/account/environment/service/metadata) take precedence over the matching init field.",
				Optional:    true,
				Attributes: map[string]schema.Attribute{
					"platform":    schema.StringAttribute{Optional: true},
					"account":     schema.StringAttribute{Optional: true},
					"environment": schema.StringAttribute{Optional: true},
					"service":     schema.StringAttribute{Optional: true},
					"metadata": schema.MapAttribute{
						ElementType: types.StringType,
						Optional:    true,
					},
				},
			},
			"platform": schema.StringAttribute{
				Description: "Platform name. Required, directly or via init — this is the top of the hierarchy.",
				Optional:    true,
				Computed:    true,
			},
			"account": schema.StringAttribute{
				Description: "Account name. Leave unset to resolve a platform-scoped context.",
				Optional:    true,
				Computed:    true,
			},
			"environment": schema.StringAttribute{
				Description: "Environment name. Requires account to be set (directly or via init). Leave unset to resolve an account-scoped context.",
				Optional:    true,
				Computed:    true,
			},
			"service": schema.StringAttribute{
				Description: "Service name. Requires environment to be set (directly or via init) — a service always belongs to an environment.",
				Optional:    true,
				Computed:    true,
			},
			"metadata": schema.DynamicAttribute{
				Description: "Metadata for this scope, merged over init.metadata. Accepts any shape (matching system-context's permissive `any`-typed metadata argument) — only the keys declared by each resolved scope are kept (platform: organization/status/control_plane, all required; account: +ecosystem (required)/cloud/region; environment: +region; service: +version); everything else is silently dropped, same as the HCL module's per-scope object() type coercion. The resolved value is a flat string map in which declared-but-unset keys are present as null.",
				Optional:    true,
				Computed:    true,
			},
			"scope": schema.StringAttribute{
				Description: "The deepest resolved scope: \"platform\", \"account\", \"environment\", or \"service\".",
				Computed:    true,
			},
			"labels": schema.MapAttribute{
				Description: "Cloud-resource-label-safe key/values accumulated across the scope cascade (lowercased, underscores->dashes in keys, dots->dashes in values).",
				ElementType: types.StringType,
				Computed:    true,
			},
		},
	}
}

func (d *ContextDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config contextDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	input := contextInput{
		Platform:    stringPtrFromTF(config.Platform),
		Account:     stringPtrFromTF(config.Account),
		Environment: stringPtrFromTF(config.Environment),
		Service:     stringPtrFromTF(config.Service),
	}

	input.Metadata = scalarsFromDynamic(config.Metadata)

	if config.Init != nil {
		input.Init.Platform = stringPtrFromTF(config.Init.Platform)
		input.Init.Account = stringPtrFromTF(config.Init.Account)
		input.Init.Environment = stringPtrFromTF(config.Init.Environment)
		input.Init.Service = stringPtrFromTF(config.Init.Service)

		input.Init.Metadata = stringMapFromTF(config.Init.Metadata)
	}

	result, err := computeContext(input)
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve context", err.Error())
		return
	}

	config.Platform = stringPtrToTF(result.Platform)
	config.Account = stringPtrToTF(result.Account)
	config.Environment = stringPtrToTF(result.Environment)
	config.Service = stringPtrToTF(result.Service)
	config.Scope = types.StringValue(result.Scope)

	metadataMapVal, diags := types.MapValueFrom(ctx, types.StringType, stringMapToTF(result.Metadata))
	resp.Diagnostics.Append(diags...)
	config.Metadata = types.DynamicValue(metadataMapVal)

	labelsVal, diags := types.MapValueFrom(ctx, types.StringType, result.Labels)
	resp.Diagnostics.Append(diags...)
	config.Labels = labelsVal

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

func stringPtrFromTF(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

func stringPtrToTF(v *string) types.String {
	if v == nil {
		return types.StringNull()
	}
	return types.StringValue(*v)
}

// stringMapFromTF decodes a types.Map(String) into a Go map, keeping null
// elements as nil. init.metadata routinely carries explicit nulls — the HCL
// module's object() coercion fills unset optional attributes (e.g. `region`)
// with null rather than omitting the key — and the distinction matters: a
// required attribute (e.g. control_plane) that is present-but-null passes the
// HCL type check, while an absent one fails it.
func stringMapFromTF(m types.Map) map[string]*string {
	if m.IsNull() || m.IsUnknown() {
		return nil
	}
	out := map[string]*string{}
	for k, v := range m.Elements() {
		sv, ok := v.(types.String)
		if !ok || sv.IsUnknown() {
			continue
		}
		out[k] = stringPtrFromTF(sv)
	}
	return out
}

// stringMapToTF converts a nil-able string map into framework values, so
// declared-but-unset metadata keys surface as null elements.
func stringMapToTF(m map[string]*string) map[string]types.String {
	out := make(map[string]types.String, len(m))
	for k, v := range m {
		out[k] = stringPtrToTF(v)
	}
	return out
}

// scalarsFromDynamic extracts a best-effort string map out of a Dynamic
// value of arbitrary shape (object or map), mirroring the permissiveness of
// system-context's `variable "metadata" { type = any }`. Null entries are
// kept as nil (an explicit null overrides init.metadata in HCL merge()).
// Non-scalar entries (nested objects, lists, etc.) are silently dropped
// rather than erroring — the per-scope attribute lists in computeContext
// decide which keys matter, and irrelevant keys (e.g. system-service passing
// its whole var.config, which includes a nested "variables" map) were never
// inspected by the original HCL module either, since its object() type
// coercion only ever looked at the specific attributes it declared.
func scalarsFromDynamic(d types.Dynamic) map[string]*string {
	if d.IsNull() || d.IsUnknown() || d.IsUnderlyingValueNull() || d.IsUnderlyingValueUnknown() {
		return nil
	}

	var elements map[string]attr.Value
	switch v := d.UnderlyingValue().(type) {
	case types.Object:
		elements = v.Attributes()
	case types.Map:
		elements = v.Elements()
	default:
		return nil
	}

	out := map[string]*string{}
	for k, v := range elements {
		if s, ok := scalarAttrValueToString(v); ok {
			out[k] = s
		}
	}
	return out
}

// scalarAttrValueToString returns (nil, true) for a null scalar, and
// (_, false) for unknowns and non-scalars, which are skipped.
func scalarAttrValueToString(v attr.Value) (*string, bool) {
	if v.IsUnknown() {
		return nil, false
	}
	switch val := v.(type) {
	case types.String:
		if val.IsNull() {
			return nil, true
		}
		return strptr(val.ValueString()), true
	case types.Bool:
		if val.IsNull() {
			return nil, true
		}
		return strptr(strconv.FormatBool(val.ValueBool())), true
	case types.Number:
		if val.IsNull() {
			return nil, true
		}
		return strptr(val.ValueBigFloat().Text('f', -1)), true
	case types.Dynamic:
		// A literal `null` in an object expression has no type of its own,
		// so it arrives as a null Dynamic rather than a null String.
		if val.IsNull() || val.IsUnderlyingValueNull() {
			return nil, true
		}
		return scalarAttrValueToString(val.UnderlyingValue())
	default:
		return nil, false
	}
}
