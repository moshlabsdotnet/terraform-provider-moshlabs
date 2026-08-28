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
	Account     types.String `tfsdk:"account"`
	Environment types.String `tfsdk:"environment"`
	Service     types.String `tfsdk:"service"`
	Metadata    types.Map    `tfsdk:"metadata"`
}

type contextDataSourceModel struct {
	Init        *contextInitModel `tfsdk:"init"`
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
		Description: "Resolves the account/environment/service context for the calling scope: cascades metadata and builds the labels map, exactly as modules/system-context does, but as a single data source read instead of nested module calls.",
		Attributes: map[string]schema.Attribute{
			"init": schema.SingleNestedAttribute{
				Description: "The already-resolved context of the parent scope (e.g. the output of a `moshlabs_context` data source one level up). Fields set directly (account/environment/service/metadata) take precedence over the matching init field.",
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
			"account": schema.StringAttribute{
				Description: "Account name. Required at some level of the init chain — this is the top of the hierarchy.",
				Optional:    true,
				Computed:    true,
			},
			"environment": schema.StringAttribute{
				Description: "Environment name. Leave unset to resolve an account-scoped context.",
				Optional:    true,
				Computed:    true,
			},
			"service": schema.StringAttribute{
				Description: "Service name. Requires environment to be set (directly or via init) — a service always belongs to an environment.",
				Optional:    true,
				Computed:    true,
			},
			"metadata": schema.DynamicAttribute{
				Description: "Metadata for this scope. Accepts any shape (matching system-context's permissive `any`-typed metadata argument) — only the keys relevant to the resolved scope are kept (account: ecosystem/platform/cloud/region; environment: +region; service: +version); everything else is silently dropped, same as the HCL module's per-scope object() type coercion did. The resolved value is always a flat string map.",
				Optional:    true,
				Computed:    true,
			},
			"scope": schema.StringAttribute{
				Description: "The deepest resolved scope: \"account\", \"environment\", or \"service\".",
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
		Account:     stringPtrFromTF(config.Account),
		Environment: stringPtrFromTF(config.Environment),
		Service:     stringPtrFromTF(config.Service),
	}

	input.Metadata = scalarsFromDynamic(config.Metadata)

	if config.Init != nil {
		input.Init.Account = stringPtrFromTF(config.Init.Account)
		input.Init.Environment = stringPtrFromTF(config.Init.Environment)
		input.Init.Service = stringPtrFromTF(config.Init.Service)

		input.Init.Metadata = stringMapFromTF(config.Init.Metadata)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := computeContext(input)
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve context", err.Error())
		return
	}

	config.Account = types.StringValue(result.Account)
	config.Environment = stringPtrToTF(result.Environment)
	config.Service = stringPtrToTF(result.Service)
	config.Scope = types.StringValue(result.Scope)

	metadataMapVal, diags := types.MapValueFrom(ctx, types.StringType, result.Metadata)
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

// stringMapFromTF decodes a types.Map(String) into a plain Go map, treating
// null elements as absent rather than erroring. This matters for init.metadata
// specifically: state written by the old HCL module can contain explicit
// nulls for optional(string) attributes that were never set (e.g. `region`)
// — object() type constraints fill unset optional attributes with null
// rather than omitting the key, unlike a plain map. Go's string type can't
// represent that null, so a strict ElementsAs conversion fails here; every
// consumer downstream already treats "absent" and "null" the same way
// (coalesceStringPtr, filterKeys), so dropping the key changes nothing
// observable.
func stringMapFromTF(m types.Map) map[string]string {
	if m.IsNull() || m.IsUnknown() {
		return nil
	}
	out := map[string]string{}
	for k, v := range m.Elements() {
		sv, ok := v.(types.String)
		if !ok || sv.IsNull() || sv.IsUnknown() {
			continue
		}
		out[k] = sv.ValueString()
	}
	return out
}

// scalarsFromDynamic extracts a best-effort map[string]string out of a
// Dynamic value of arbitrary shape (object or map), mirroring the
// permissiveness of system-context's `variable "metadata" { type = any }`.
// Non-scalar entries (nested objects, lists, etc.) are silently dropped
// rather than erroring — the per-scope allowlist in computeContext decides
// which keys matter, and irrelevant keys (e.g. system-service passing its
// whole var.config, which includes a nested "variables" map) were never
// inspected by the original HCL module either, since its object() type
// coercion only ever looked at the specific attributes it declared.
func scalarsFromDynamic(d types.Dynamic) map[string]string {
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

	out := map[string]string{}
	for k, v := range elements {
		if s, ok := scalarAttrValueToString(v); ok {
			out[k] = s
		}
	}
	return out
}

func scalarAttrValueToString(v attr.Value) (string, bool) {
	switch val := v.(type) {
	case types.String:
		if val.IsNull() || val.IsUnknown() {
			return "", false
		}
		return val.ValueString(), true
	case types.Bool:
		if val.IsNull() || val.IsUnknown() {
			return "", false
		}
		return strconv.FormatBool(val.ValueBool()), true
	case types.Number:
		if val.IsNull() || val.IsUnknown() {
			return "", false
		}
		return val.ValueBigFloat().Text('f', -1), true
	default:
		return "", false
	}
}
