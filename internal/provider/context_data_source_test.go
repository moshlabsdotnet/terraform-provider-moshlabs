// Copyright 2026 Mosh Labs
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ---------------------------------------------------------------------
// stringMapFromTF
// ---------------------------------------------------------------------

func TestStringMapFromTF_NullMap(t *testing.T) {
	got := stringMapFromTF(types.MapNull(types.StringType))
	if got != nil {
		t.Fatalf("got %#v, want nil", got)
	}
}

func TestStringMapFromTF_UnknownMap(t *testing.T) {
	got := stringMapFromTF(types.MapUnknown(types.StringType))
	if got != nil {
		t.Fatalf("got %#v, want nil", got)
	}
}

func TestStringMapFromTF_AllValuesPresent(t *testing.T) {
	m := types.MapValueMust(types.StringType, map[string]attr.Value{
		"ecosystem": types.StringValue("production"),
		"platform":  types.StringValue("core"),
	})
	got := derefMap(stringMapFromTF(m))
	want := map[string]string{"ecosystem": "production", "platform": "core"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestStringMapFromTF_NullElementIsKeptAsNil(t *testing.T) {
	// State written by the HCL system-context module contains explicit
	// nulls for unset optional(string) attributes (e.g. "region") — object()
	// type constraints fill them with null rather than omitting the key. A
	// strict ElementsAs-based decode fails on those ("Received null value,
	// however the target type cannot handle null values"), so they must
	// decode without error — and stay present, since a present-but-null
	// required attribute (e.g. control_plane) passes the HCL type check
	// where an absent one fails it.
	m := types.MapValueMust(types.StringType, map[string]attr.Value{
		"ecosystem": types.StringValue("production"),
		"region":    types.StringNull(),
	})
	got := stringMapFromTF(m)
	v, ok := got["region"]
	if !ok || v != nil {
		t.Fatalf("got %#v, want \"region\" present with a nil value", derefMap(got))
	}
	if derefPtr(got["ecosystem"]) != "production" {
		t.Fatalf("got %#v, want ecosystem=production", derefMap(got))
	}
}

func TestStringMapFromTF_EmptyMap(t *testing.T) {
	m := types.MapValueMust(types.StringType, map[string]attr.Value{})
	got := stringMapFromTF(m)
	if got == nil || len(got) != 0 {
		t.Fatalf("got %#v, want non-nil empty map", got)
	}
}

// ---------------------------------------------------------------------
// scalarsFromDynamic
// ---------------------------------------------------------------------

func TestScalarsFromDynamic_Null(t *testing.T) {
	got := scalarsFromDynamic(types.DynamicNull())
	if got != nil {
		t.Fatalf("got %#v, want nil", got)
	}
}

func TestScalarsFromDynamic_Unknown(t *testing.T) {
	got := scalarsFromDynamic(types.DynamicUnknown())
	if got != nil {
		t.Fatalf("got %#v, want nil", got)
	}
}

func TestScalarsFromDynamic_ObjectWithScalarsOnly(t *testing.T) {
	obj := types.ObjectValueMust(
		map[string]attr.Type{"ecosystem": types.StringType, "platform": types.StringType},
		map[string]attr.Value{"ecosystem": types.StringValue("production"), "platform": types.StringValue("core")},
	)
	got := derefMap(scalarsFromDynamic(types.DynamicValue(obj)))
	want := map[string]string{"ecosystem": "production", "platform": "core"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestScalarsFromDynamic_NonScalarKeysAreDroppedNotErrored(t *testing.T) {
	// Mirrors system-service passing its whole var.config (version=string,
	// replicas=number, release_timeout=number, variables=map(string)) as
	// metadata: "variables" is a nested map, not a scalar, and must be
	// silently ignored rather than causing a conversion error, since it's
	// never a key any scope's allowlist cares about anyway.
	variables := types.MapValueMust(types.StringType, map[string]attr.Value{"FOO": types.StringValue("bar")})
	obj := types.ObjectValueMust(
		map[string]attr.Type{
			"version":   types.StringType,
			"replicas":  types.NumberType,
			"variables": types.MapType{ElemType: types.StringType},
		},
		map[string]attr.Value{
			"version":   types.StringValue("1.2.3"),
			"replicas":  types.NumberValue(big.NewFloat(3)),
			"variables": variables,
		},
	)
	got := derefMap(scalarsFromDynamic(types.DynamicValue(obj)))
	want := map[string]string{"version": "1.2.3", "replicas": "3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v (variables must be dropped, not error)", got, want)
	}
}

func TestScalarsFromDynamic_NullAttributeIsKeptAsNil(t *testing.T) {
	// An explicit null overrides init.metadata in HCL merge(), so it must
	// survive decoding rather than being dropped.
	obj := types.ObjectValueMust(
		map[string]attr.Type{"region": types.StringType},
		map[string]attr.Value{"region": types.StringNull()},
	)
	got := scalarsFromDynamic(types.DynamicValue(obj))
	if v, ok := got["region"]; !ok || v != nil {
		t.Fatalf("got %#v, want \"region\" present with a nil value", derefMap(got))
	}
}

func TestScalarsFromDynamic_UntypedNullIsKeptAsNil(t *testing.T) {
	// `metadata = { region = null }` — the literal null has no type, so the
	// object attribute is a null Dynamic, not a null String.
	obj := types.ObjectValueMust(
		map[string]attr.Type{"region": types.DynamicType},
		map[string]attr.Value{"region": types.DynamicNull()},
	)
	got := scalarsFromDynamic(types.DynamicValue(obj))
	if v, ok := got["region"]; !ok || v != nil {
		t.Fatalf("got %#v, want \"region\" present with a nil value", derefMap(got))
	}
}

func TestScalarsFromDynamic_MapUnderlyingType(t *testing.T) {
	m := types.MapValueMust(types.StringType, map[string]attr.Value{"region": types.StringValue("us-east1")})
	got := derefMap(scalarsFromDynamic(types.DynamicValue(m)))
	want := map[string]string{"region": "us-east1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestScalarsFromDynamic_BoolValue(t *testing.T) {
	obj := types.ObjectValueMust(
		map[string]attr.Type{"enabled": types.BoolType},
		map[string]attr.Value{"enabled": types.BoolValue(true)},
	)
	got := derefMap(scalarsFromDynamic(types.DynamicValue(obj)))
	want := map[string]string{"enabled": "true"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
