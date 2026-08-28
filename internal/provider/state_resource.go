// Copyright 2026 Mosh Labs
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"fmt"

	uuid "github.com/hashicorp/go-uuid"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource               = &StateResource{}
	_ resource.ResourceWithConfigure  = &StateResource{}
	_ resource.ResourceWithModifyPlan = &StateResource{}
)

func NewStateResource() resource.Resource {
	return &StateResource{}
}

// StateResource backs modules/system-state. Unlike moshlabs_context/
// moshlabs_resource, this has to be a resource, not a data source: a real
// state file's lineage is assigned once and kept for the file's whole
// lifetime, and its serial increments by one on every write that actually
// changes something — both are lifecycle facts a stateless data source has
// nowhere to remember between reads. terraformVersion is threaded down from
// the provider's own Configure (see provider.go), populated from the actual
// calling Terraform's version.
type StateResource struct {
	terraformVersion string
}

type stateResourceModel struct {
	Name             types.String  `tfsdk:"name"`
	Value            types.Dynamic `tfsdk:"value"`
	Sensitive        types.Bool    `tfsdk:"sensitive"`
	TerraformVersion types.String  `tfsdk:"terraform_version"`
	Serial           types.Int64   `tfsdk:"serial"`
	Lineage          types.String  `tfsdk:"lineage"`
	JSON             types.String  `tfsdk:"json"`
}

func (r *StateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_state"
}

func (r *StateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	v, ok := req.ProviderData.(string)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected string terraform version, got %T. This is a provider bug.", req.ProviderData))
		return
	}
	r.terraformVersion = v
}

func (r *StateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Terraform state v4 JSON document holding a single output, shaped exactly like a real `terraform apply` would produce — backs modules/system-state. Unlike moshlabs_context/moshlabs_resource this is a resource, not a data source: lineage is a real randomly-generated UUID assigned once at create and kept stable for the life of this resource, and serial increments by one on every write where the rendered value actually changed — both matching real Terraform state semantics rather than being recomputed from scratch (or hardcoded) on every read. Meant to be written to disk (e.g. via a local_file resource) and read elsewhere with `terraform_remote_state { backend = \"local\" }`.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Description: "The output key the value is stored under, e.g. \"platform\".",
				Required:    true,
			},
			"value": schema.DynamicAttribute{
				Description: "The value to emit as the output's value. Accepts any shape. Every keyed collection (HCL object or map) is encoded as an \"object\" type and every ordered collection (list, set, or tuple) as a \"tuple\" type — the narrower distinction a real apply would have recorded (e.g. `map(string)` vs `object(...)`) can't be recovered from a value alone, but object/tuple are the more permissive of each pair, so a consumer can still do anything the narrower type would have allowed. A null is always recorded as type \"string\", regardless of the field's real type.",
				Required:    true,
			},
			"sensitive": schema.BoolAttribute{
				Description: "Whether the output is marked sensitive.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"terraform_version": schema.StringAttribute{
				Description: "terraform_version recorded in the document. Defaults to the actual Terraform version running this apply — the same value real Terraform would have recorded — so it only changes when this resource is actually re-applied, not on every plan (see ModifyPlan: only marked unknown for recomputation when name/value/sensitive are actually changing). Set explicitly to override (e.g. to pin it below whatever will eventually read the file back, since a state whose recorded version is newer than the reader's own is a hard read error).",
				Optional:    true,
				Computed:    true,
			},
			"serial": schema.Int64Attribute{
				Description: "State serial number recorded in the document. 1 on first create; increments by one on every subsequent apply where the rendered value actually changed (see ModifyPlan). Not normally set explicitly.",
				Optional:    true,
				Computed:    true,
			},
			"lineage": schema.StringAttribute{
				Description: "State lineage identifier. A real randomly-generated UUID assigned once at create and kept stable for the life of this resource, matching how a real state file's lineage behaves. Set explicitly to pin it instead (e.g. to match a real state file this resource is standing in for).",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"json": schema.StringAttribute{
				Description: "The rendered state document.",
				Computed:    true,
			},
		},
	}
}

// ModifyPlan marks serial/terraform_version/json unknown exactly when
// something worth recomputing them for is actually changing (name, value, or
// sensitive), and pins serial/terraform_version to their prior values
// otherwise. This exists because of a real inconsistency Terraform core
// caught: Optional+Computed attributes default to an unknown proposed value
// whenever their config is null, and Computed-only attributes (json) default
// to the prior state value regardless of what else is changing — neither
// default lines up with what serial/terraform_version/json actually need
// (stay pinned on a true no-op, recompute freely whenever Update() will
// actually run). Update()/render() must agree with whatever this method
// decides, or Terraform rejects the applied result as "inconsistent with
// plan".
func (r *StateResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return // create or delete: no prior state to diff against
	}

	var plan, state stateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	changed := !plan.Name.Equal(state.Name) ||
		!plan.Value.Equal(state.Value) ||
		!plan.Sensitive.Equal(state.Sensitive)

	if !changed {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("serial"), state.Serial)...)
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("terraform_version"), state.TerraformVersion)...)
		return
	}

	var config stateResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("json"), types.StringUnknown())...)
	if config.Serial.IsNull() {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("serial"), types.Int64Unknown())...)
	}
	if config.TerraformVersion.IsNull() {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("terraform_version"), types.StringUnknown())...)
	}
}

func (r *StateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan stateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	var config stateResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	lineage := stringOrDefault(config.Lineage, "")
	if lineage == "" {
		generated, err := uuid.GenerateUUID()
		if err != nil {
			resp.Diagnostics.AddError("Unable to generate lineage", err.Error())
			return
		}
		lineage = generated
	}

	result, err := r.render(ctx, plan, config, lineage, 1)
	if err != nil {
		resp.Diagnostics.AddError("Unable to render state document", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &result)...)
}

func (r *StateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Nothing external to refresh from — the rendered document is a pure
	// function of this resource's own attributes, same as e.g. random_uuid.
	var state stateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *StateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan stateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	// Deliberately read explicit-override checks (below) from req.Config,
	// not req.Plan: serial/lineage/terraform_version all carry a
	// UseStateForUnknown plan modifier, which means by the time req.Plan
	// reaches here, an attribute the user left unset has *already* been
	// resolved to the prior state's value — indistinguishable, on req.Plan
	// alone, from the user explicitly re-affirming that same value.
	// req.Config reflects the raw HCL the user wrote, untouched by plan
	// modifiers, so it's the only reliable way to ask "did they set this?"
	var config stateResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	var state stateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Lineage never changes once assigned unless explicitly overridden —
	// that's the entire point of lineage. Carry the prior value forward.
	lineage := stringOrDefault(config.Lineage, state.Lineage.ValueString())

	// This method only runs because something actually changed (Terraform
	// only calls Update when the plan differs from prior state), so bump
	// the serial — unless the caller is explicitly pinning one.
	serial := state.Serial.ValueInt64() + 1
	if !config.Serial.IsNull() && !config.Serial.IsUnknown() {
		serial = config.Serial.ValueInt64()
	}

	result, err := r.render(ctx, plan, config, lineage, serial)
	if err != nil {
		resp.Diagnostics.AddError("Unable to render state document", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &result)...)
}

func (r *StateResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// No external side effects to unwind — removing it from state is enough.
}

// render derives the fields not already pinned by the caller (sensitive,
// terraform_version) and produces the fully-populated model, including the
// marshaled document. lineage and serial are decided by the caller (Create
// vs Update disagree on what "unset" should mean for both). terraform_version
// is read from config, same reasoning as serial/lineage above.
func (r *StateResource) render(ctx context.Context, plan, config stateResourceModel, lineage string, serial int64) (stateResourceModel, error) {
	value, typ, err := dynamicValueAndType(ctx, plan.Value)
	if err != nil {
		return stateResourceModel{}, err
	}

	sensitive := true
	if !plan.Sensitive.IsNull() && !plan.Sensitive.IsUnknown() {
		sensitive = plan.Sensitive.ValueBool()
	}

	terraformVersion := stringOrDefault(config.TerraformVersion, r.terraformVersion)
	if terraformVersion == "" {
		// Configure wasn't called (e.g. a unit test constructing the
		// resource directly) — fall back to a conservative constant rather
		// than an empty terraform_version.
		terraformVersion = "1.5.0"
	}

	doc, err := renderStateJSON(plan.Name.ValueString(), value, typ, sensitive, terraformVersion, serial, lineage)
	if err != nil {
		return stateResourceModel{}, err
	}

	plan.Sensitive = types.BoolValue(sensitive)
	plan.TerraformVersion = types.StringValue(terraformVersion)
	plan.Serial = types.Int64Value(serial)
	plan.Lineage = types.StringValue(lineage)
	plan.JSON = types.StringValue(doc)
	return plan, nil
}

func stringOrDefault(v types.String, def string) string {
	if v.IsNull() || v.IsUnknown() || v.ValueString() == "" {
		return def
	}
	return v.ValueString()
}
