// Copyright 2026 Mosh Labs
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func dynamicObject(t *testing.T, attrs map[string]attr.Value, attrTypes map[string]attr.Type) types.Dynamic {
	t.Helper()
	obj, diags := types.ObjectValue(attrTypes, attrs)
	if diags.HasError() {
		t.Fatalf("building test object: %v", diags)
	}
	return types.DynamicValue(obj)
}

// ---------------------------------------------------------------------
// StateResource.render
// ---------------------------------------------------------------------

func TestStateResourceRender_UsesGivenLineageAndSerial(t *testing.T) {
	r := &StateResource{terraformVersion: "1.9.8"}
	plan := stateResourceModel{
		Name:  types.StringValue("platform"),
		Value: dynamicObject(t, map[string]attr.Value{"account": types.StringValue("moshlabs-dev")}, map[string]attr.Type{"account": types.StringType}),
	}

	result, err := r.render(context.Background(), plan, plan, "fixed-lineage", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Lineage.ValueString() != "fixed-lineage" {
		t.Fatalf("lineage: got %q, want %q", result.Lineage.ValueString(), "fixed-lineage")
	}
	if result.Serial.ValueInt64() != 5 {
		t.Fatalf("serial: got %d, want 5", result.Serial.ValueInt64())
	}
}

func TestStateResourceRender_DefaultsTerraformVersionToConfiguredValue(t *testing.T) {
	// Mirrors what provider.Configure passes down from the real calling
	// Terraform's ConfigureRequest.TerraformVersion — the whole point of
	// threading it through, instead of hardcoding a guess.
	r := &StateResource{terraformVersion: "1.9.8"}
	plan := stateResourceModel{
		Name:  types.StringValue("platform"),
		Value: dynamicObject(t, map[string]attr.Value{"account": types.StringValue("x")}, map[string]attr.Type{"account": types.StringType}),
	}

	result, err := r.render(context.Background(), plan, plan, "l", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TerraformVersion.ValueString() != "1.9.8" {
		t.Fatalf("terraform_version: got %q, want %q", result.TerraformVersion.ValueString(), "1.9.8")
	}
}

func TestStateResourceRender_ExplicitTerraformVersionOverridesConfigured(t *testing.T) {
	// terraform_version's "was this explicitly set" check reads from
	// config, not plan — see the comment on Update() for why (UseStateForUnknown
	// makes plan values indistinguishable from a carried-forward prior state).
	r := &StateResource{terraformVersion: "1.9.8"}
	plan := stateResourceModel{
		Name:  types.StringValue("platform"),
		Value: dynamicObject(t, map[string]attr.Value{"account": types.StringValue("x")}, map[string]attr.Type{"account": types.StringType}),
	}
	config := plan
	config.TerraformVersion = types.StringValue("1.0.0")

	result, err := r.render(context.Background(), plan, config, "l", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TerraformVersion.ValueString() != "1.0.0" {
		t.Fatalf("terraform_version: got %q, want the explicit override %q", result.TerraformVersion.ValueString(), "1.0.0")
	}
}

func TestStateResourceRender_FallsBackWhenNeverConfigured(t *testing.T) {
	// Configure() not called (e.g. constructing the resource directly, as
	// in this test) leaves terraformVersion empty — must not surface an
	// empty terraform_version in the rendered document.
	r := &StateResource{}
	plan := stateResourceModel{
		Name:  types.StringValue("platform"),
		Value: dynamicObject(t, map[string]attr.Value{"account": types.StringValue("x")}, map[string]attr.Type{"account": types.StringType}),
	}

	result, err := r.render(context.Background(), plan, plan, "l", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TerraformVersion.ValueString() == "" {
		t.Fatal("terraform_version: got empty string, want a non-empty fallback")
	}
}

func TestStateResourceRender_DefaultsSensitiveToTrue(t *testing.T) {
	r := &StateResource{terraformVersion: "1.9.8"}
	plan := stateResourceModel{
		Name:  types.StringValue("platform"),
		Value: dynamicObject(t, map[string]attr.Value{"account": types.StringValue("x")}, map[string]attr.Type{"account": types.StringType}),
	}

	result, err := r.render(context.Background(), plan, plan, "l", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Sensitive.ValueBool() {
		t.Fatal("sensitive: got false, want true (the default)")
	}
}

func TestStateResourceRender_RejectsNullValue(t *testing.T) {
	r := &StateResource{terraformVersion: "1.9.8"}
	plan := stateResourceModel{
		Name:  types.StringValue("platform"),
		Value: types.DynamicNull(),
	}

	if _, err := r.render(context.Background(), plan, plan, "l", 1); err == nil {
		t.Fatal("expected an error for a null value, got nil")
	}
}

// ---------------------------------------------------------------------
// stringOrDefault
// ---------------------------------------------------------------------

func TestStringOrDefault(t *testing.T) {
	tests := []struct {
		name string
		v    types.String
		def  string
		want string
	}{
		{"null falls back", types.StringNull(), "fallback", "fallback"},
		{"unknown falls back", types.StringUnknown(), "fallback", "fallback"},
		{"empty string falls back", types.StringValue(""), "fallback", "fallback"},
		{"explicit value wins", types.StringValue("explicit"), "fallback", "explicit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stringOrDefault(tt.v, tt.def)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
