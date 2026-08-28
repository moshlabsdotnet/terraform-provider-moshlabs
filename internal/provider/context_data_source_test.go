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
	got := stringMapFromTF(m)
	want := map[string]string{"ecosystem": "production", "platform": "core"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestStringMapFromTF_NullElementIsDroppedNotErrored(t *testing.T) {
	// Regression test: state written by the old HCL system-context module
	// can contain an explicit null for an unset optional(string) attribute
	// (e.g. "region") — object() type constraints fill unset optional
	// attributes with null rather than omitting the key. A real plan against
	// suture-dev/k8s's account remote state hit exactly this: init.metadata
	// came through with region = null and the old strict ElementsAs-based
	// decode failed with "Received null value, however the target type
	// cannot handle null values". The null element must be dropped, not
	// error, since every downstream consumer already treats "absent" and
	// "null" identically.
	m := types.MapValueMust(types.StringType, map[string]attr.Value{
		"ecosystem": types.StringValue("production"),
		"region":    types.StringNull(),
	})
	got := stringMapFromTF(m)
	want := map[string]string{"ecosystem": "production"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v (region should be dropped, not present as \"\" or cause an error)", got, want)
	}
	if _, ok := got["region"]; ok {
		t.Fatalf("got %#v, want \"region\" key entirely absent", got)
	}
}

func TestStringMapFromTF_AllNullYieldsEmptyMapNotNil(t *testing.T) {
	m := types.MapValueMust(types.StringType, map[string]attr.Value{
		"region": types.StringNull(),
	})
	got := stringMapFromTF(m)
	if got == nil {
		t.Fatal("got nil, want a non-nil empty map (the Map itself was known, just its only element was null)")
	}
	if len(got) != 0 {
		t.Fatalf("got %#v, want empty", got)
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
	got := scalarsFromDynamic(types.DynamicValue(obj))
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
	got := scalarsFromDynamic(types.DynamicValue(obj))
	want := map[string]string{"version": "1.2.3", "replicas": "3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v (variables must be dropped, not error)", got, want)
	}
}

func TestScalarsFromDynamic_NullAttributeIsDropped(t *testing.T) {
	obj := types.ObjectValueMust(
		map[string]attr.Type{"region": types.StringType},
		map[string]attr.Value{"region": types.StringNull()},
	)
	got := scalarsFromDynamic(types.DynamicValue(obj))
	if _, ok := got["region"]; ok {
		t.Fatalf("got %#v, want \"region\" absent (null attribute)", got)
	}
}

func TestScalarsFromDynamic_MapUnderlyingType(t *testing.T) {
	m := types.MapValueMust(types.StringType, map[string]attr.Value{"region": types.StringValue("us-east1")})
	got := scalarsFromDynamic(types.DynamicValue(m))
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
	got := scalarsFromDynamic(types.DynamicValue(obj))
	want := map[string]string{"enabled": "true"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
