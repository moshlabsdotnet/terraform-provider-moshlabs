// Copyright 2026 Mosh Labs
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"reflect"
	"strings"
	"testing"
)

func strptr(s string) *string { return &s }

// ---------------------------------------------------------------------
// coalesceString / coalesceStringPtr
// ---------------------------------------------------------------------

func TestCoalesceStringPtr(t *testing.T) {
	tests := []struct {
		name string
		vals []*string
		want *string
	}{
		{"no args", nil, nil},
		{"single nil", []*string{nil}, nil},
		{"all nil", []*string{nil, nil, nil}, nil},
		{"single value", []*string{strptr("a")}, strptr("a")},
		{"first wins", []*string{strptr("a"), strptr("b")}, strptr("a")},
		{"skips leading nil", []*string{nil, strptr("b")}, strptr("b")},
		{"skips leading empty string", []*string{strptr(""), strptr("b")}, strptr("b")},
		{"empty string is not a value", []*string{strptr("")}, nil},
		{"all empty strings", []*string{strptr(""), strptr("")}, nil},
		{"nil then empty then value", []*string{nil, strptr(""), strptr("c")}, strptr("c")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := coalesceStringPtr(tt.vals...)
			if (got == nil) != (tt.want == nil) {
				t.Fatalf("coalesceStringPtr(%v) = %v, want %v", derefAll(tt.vals), derefPtr(got), derefPtr(tt.want))
			}
			if got != nil && *got != *tt.want {
				t.Fatalf("coalesceStringPtr(%v) = %q, want %q", derefAll(tt.vals), *got, *tt.want)
			}
		})
	}
}

func TestCoalesceString(t *testing.T) {
	tests := []struct {
		name string
		vals []*string
		want string
	}{
		{"no args", nil, ""},
		{"all nil", []*string{nil, nil}, ""},
		{"all empty", []*string{strptr(""), strptr("")}, ""},
		{"single value", []*string{strptr("acme")}, "acme"},
		{"first non-empty wins", []*string{strptr(""), strptr("acme")}, "acme"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := coalesceString(tt.vals...); got != tt.want {
				t.Fatalf("coalesceString(%v) = %q, want %q", derefAll(tt.vals), got, tt.want)
			}
		})
	}
}

func derefPtr(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func derefAll(vals []*string) []string {
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i] = derefPtr(v)
	}
	return out
}

// ---------------------------------------------------------------------
// mergeStringMaps
// ---------------------------------------------------------------------

func TestMergeStringMaps(t *testing.T) {
	tests := []struct {
		name string
		in   []map[string]string
		want map[string]string
	}{
		{"no maps", nil, map[string]string{}},
		{"single nil map", []map[string]string{nil}, map[string]string{}},
		{"single empty map", []map[string]string{{}}, map[string]string{}},
		{
			"single populated map",
			[]map[string]string{{"a": "1"}},
			map[string]string{"a": "1"},
		},
		{
			"disjoint keys combine",
			[]map[string]string{{"a": "1"}, {"b": "2"}},
			map[string]string{"a": "1", "b": "2"},
		},
		{
			"later map overrides earlier on collision",
			[]map[string]string{{"a": "1"}, {"a": "2"}},
			map[string]string{"a": "2"},
		},
		{
			"nil maps interleaved with populated ones are skipped",
			[]map[string]string{{"a": "1"}, nil, {"b": "2"}},
			map[string]string{"a": "1", "b": "2"},
		},
		{
			"three-way override chain, last write wins",
			[]map[string]string{{"a": "1"}, {"a": "2"}, {"a": "3"}},
			map[string]string{"a": "3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeStringMaps(tt.in...)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("mergeStringMaps(%v) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------
// filterKeys
// ---------------------------------------------------------------------

func TestFilterKeys(t *testing.T) {
	tests := []struct {
		name    string
		m       map[string]string
		allowed []string
		want    map[string]string
	}{
		{"nil map, no allowed", nil, nil, map[string]string{}},
		{"empty map", map[string]string{}, []string{"a"}, map[string]string{}},
		{"no allowed keys drops everything", map[string]string{"a": "1"}, nil, map[string]string{}},
		{
			"only allowed keys pass",
			map[string]string{"a": "1", "b": "2", "c": "3"},
			[]string{"a", "c"},
			map[string]string{"a": "1", "c": "3"},
		},
		{
			"allowed key absent from map is simply not present in output",
			map[string]string{"a": "1"},
			[]string{"a", "z"},
			map[string]string{"a": "1"},
		},
		{
			"matching is case-sensitive and exact",
			map[string]string{"Scope": "account", "scope": "account"},
			[]string{"scope"},
			map[string]string{"scope": "account"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterKeys(tt.m, tt.allowed...)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("filterKeys(%#v, %v) = %#v, want %#v", tt.m, tt.allowed, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------
// normalizeLabels
// ---------------------------------------------------------------------

func TestNormalizeLabels(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]string
		want map[string]string
	}{
		{"nil map", nil, map[string]string{}},
		{"empty map", map[string]string{}, map[string]string{}},
		{
			"underscore in key becomes dash",
			map[string]string{"environment_id": "42"},
			map[string]string{"environment-id": "42"},
		},
		{
			"dot in value becomes dash",
			map[string]string{"platform": "some.dotted.value"},
			map[string]string{"platform": "some-dotted-value"},
		},
		{
			"multiple underscores and dots",
			map[string]string{"a_b_c": "x.y.z"},
			map[string]string{"a-b-c": "x-y-z"},
		},
		{
			"keys and values are lowercased",
			map[string]string{"Account": "ACME"},
			map[string]string{"account": "acme"},
		},
		{
			"empty value is dropped entirely",
			map[string]string{"account": "", "service": "api"},
			map[string]string{"service": "api"},
		},
		{
			"all empty values yields empty map, not nil",
			map[string]string{"account": ""},
			map[string]string{},
		},
		{
			"key with dot and value with underscore are untouched by the other's rule",
			map[string]string{"a.b": "x_y"},
			map[string]string{"a.b": "x_y"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeLabels(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("normalizeLabels(%#v) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------
// computeContext — account scope
// ---------------------------------------------------------------------

func TestComputeContext_AccountScope(t *testing.T) {
	got, err := computeContext(contextInput{
		Account: strptr("acme"),
		Metadata: map[string]string{
			"ecosystem": "production",
			"platform":  "core",
			"cloud":     "gcp",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Scope != "account" {
		t.Fatalf("Scope = %q, want %q", got.Scope, "account")
	}
	if got.Account != "acme" {
		t.Fatalf("Account = %q, want %q", got.Account, "acme")
	}
	if got.Environment != nil || got.Service != nil {
		t.Fatalf("Environment/Service should be nil at account scope, got %v/%v", got.Environment, got.Service)
	}
	if got.Metadata["scope"] != "account" {
		t.Fatalf("Metadata[scope] = %q, want %q", got.Metadata["scope"], "account")
	}
	if got.Metadata["cloud"] != "gcp" {
		t.Fatalf("Metadata[cloud] = %q, want %q (metadata not scoped should pass through unfiltered)", got.Metadata["cloud"], "gcp")
	}

	wantLabels := map[string]string{
		"account":   "acme",
		"scope":     "account",
		"platform":  "core",
		"ecosystem": "production",
		"terraform": "true",
	}
	if !reflect.DeepEqual(got.Labels, wantLabels) {
		t.Fatalf("Labels = %#v, want %#v (cloud must NOT leak into labels — only scope/platform/ecosystem are allowed at account scope)", got.Labels, wantLabels)
	}
	if _, ok := got.Labels["cloud"]; ok {
		t.Fatalf("cloud must not leak into labels at account scope: %#v", got.Labels)
	}
}

// baseMetadata is the minimal metadata every success-path test needs now
// that ecosystem/platform are required at account scope (modules/account
// has no `optional(...)` on either) — tests that aren't specifically
// exercising that requirement use this so the requirement itself doesn't
// need repeating everywhere.
func baseMetadata() map[string]string {
	return map[string]string{"ecosystem": "production", "platform": "core"}
}

func TestComputeContext_AccountScope_EmptyAccountDropsLabel(t *testing.T) {
	// Account unset entirely (nil, and no init) resolves to "" per coalesceString,
	// and normalizeLabels drops empty-valued entries — so "account" is absent
	// from Labels even though it's present (as "") in the Account field.
	got, err := computeContext(contextInput{Metadata: baseMetadata()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Scope != "account" {
		t.Fatalf("Scope = %q, want %q", got.Scope, "account")
	}
	if got.Account != "" {
		t.Fatalf("Account = %q, want empty string", got.Account)
	}
	if _, ok := got.Labels["account"]; ok {
		t.Fatalf("account label should be dropped when account is empty, got %#v", got.Labels)
	}
	wantLabels := map[string]string{"scope": "account", "terraform": "true", "ecosystem": "production", "platform": "core"}
	if !reflect.DeepEqual(got.Labels, wantLabels) {
		t.Fatalf("Labels = %#v, want %#v", got.Labels, wantLabels)
	}
}

func TestComputeContext_AccountScope_NilMetadataErrorsCleanly(t *testing.T) {
	// Nil metadata (no ecosystem/platform available from anywhere) must
	// produce a clean error, not a panic and not a silently-empty result.
	got, err := computeContext(contextInput{
		Account:  strptr("acme"),
		Metadata: nil,
		Init:     contextInit{Metadata: nil},
	})
	if err == nil {
		t.Fatalf("expected an error (ecosystem/platform required), got result %#v", got)
	}
}

func TestComputeContext_AccountScope_UserSuppliedScopeKeyIsOverridden(t *testing.T) {
	// The literal "scope" computed by computeContext must win over any
	// user-supplied metadata["scope"], since it's merged in last.
	got, err := computeContext(contextInput{
		Account:  strptr("acme"),
		Metadata: mergeStringMaps(baseMetadata(), map[string]string{"scope": "bogus"}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Metadata["scope"] != "account" {
		t.Fatalf("Metadata[scope] = %q, want %q (computed scope must override user-supplied value)", got.Metadata["scope"], "account")
	}
	if got.Labels["scope"] != "account" {
		t.Fatalf("Labels[scope] = %q, want %q", got.Labels["scope"], "account")
	}
}

func TestComputeContext_AccountScope_MetadataPreservesCasingAndDots(t *testing.T) {
	// Only Labels go through normalizeLabels; Metadata is untouched.
	got, err := computeContext(contextInput{
		Account:  strptr("acme"),
		Metadata: mergeStringMaps(baseMetadata(), map[string]string{"platform": "Some.Dotted.Value"}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Metadata["platform"] != "Some.Dotted.Value" {
		t.Fatalf("Metadata[platform] = %q, want unmodified %q", got.Metadata["platform"], "Some.Dotted.Value")
	}
}

func TestComputeContext_AccountScope_EmptyAccountPointerTreatedAsUnset(t *testing.T) {
	// A non-nil pointer to "" must behave identically to a nil pointer,
	// mirroring HCL coalesce()'s empty-string skipping.
	got, err := computeContext(contextInput{
		Account:  strptr(""),
		Init:     contextInit{Account: strptr("from-init")},
		Metadata: baseMetadata(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Account != "from-init" {
		t.Fatalf("Account = %q, want %q (empty direct value should fall through to init)", got.Account, "from-init")
	}
}

// ---------------------------------------------------------------------
// computeContext — per-scope metadata allowlist and required fields
// ---------------------------------------------------------------------

func TestComputeContext_AccountMetadata_EcosystemRequired(t *testing.T) {
	_, err := computeContext(contextInput{
		Account:  strptr("acme"),
		Metadata: map[string]string{"platform": "core"},
	})
	if err == nil {
		t.Fatal("expected an error when ecosystem is missing")
	}
	if !strings.Contains(err.Error(), "ecosystem") {
		t.Fatalf("error %q should mention ecosystem", err.Error())
	}
}

func TestComputeContext_AccountMetadata_PlatformRequired(t *testing.T) {
	_, err := computeContext(contextInput{
		Account:  strptr("acme"),
		Metadata: map[string]string{"ecosystem": "production"},
	})
	if err == nil {
		t.Fatal("expected an error when platform is missing")
	}
	if !strings.Contains(err.Error(), "platform") {
		t.Fatalf("error %q should mention platform", err.Error())
	}
}

func TestComputeContext_AccountMetadata_EcosystemMustBeValidEnumValue(t *testing.T) {
	_, err := computeContext(contextInput{
		Account:  strptr("acme"),
		Metadata: map[string]string{"ecosystem": "staging", "platform": "core"},
	})
	if err == nil {
		t.Fatal(`expected an error for ecosystem "staging"`)
	}
	if !strings.Contains(err.Error(), "staging") {
		t.Fatalf("error %q should name the invalid value", err.Error())
	}
}

func TestComputeContext_AccountMetadata_UnknownKeysDroppedFromMetadataNotJustLabels(t *testing.T) {
	// Mirrors system-service passing its whole var.config as metadata: keys
	// outside the scope's allowlist (e.g. "replicas", "release_timeout")
	// must not survive into context.metadata at all, not just be excluded
	// from labels.
	got, err := computeContext(contextInput{
		Account: strptr("acme"),
		Metadata: mergeStringMaps(baseMetadata(), map[string]string{
			"replicas":        "1",
			"release_timeout": "300",
		}),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := got.Metadata["replicas"]; ok {
		t.Fatalf("Metadata = %#v, want \"replicas\" dropped (not in account's metadata allowlist)", got.Metadata)
	}
	if _, ok := got.Metadata["release_timeout"]; ok {
		t.Fatalf("Metadata = %#v, want \"release_timeout\" dropped", got.Metadata)
	}
}

func TestComputeContext_EnvironmentMetadata_OnlyRegionCanBeIntroducedDirectly(t *testing.T) {
	// version is only in serviceMetadataKeys, not environmentMetadataKeys —
	// passed directly at environment scope it must be dropped, exactly like
	// account-scope keys (ecosystem/platform/cloud) can't be re-set here
	// either (they just carry forward from account via the merge cascade).
	got, err := computeContext(contextInput{
		Init:        contextInit{Account: strptr("acme"), Metadata: baseMetadata()},
		Environment: strptr("prod"),
		Metadata:    map[string]string{"version": "9.9.9", "region": "us-east1"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := got.Metadata["version"]; ok {
		t.Fatalf("Metadata = %#v, want \"version\" dropped at environment scope", got.Metadata)
	}
	if got.Metadata["region"] != "us-east1" {
		t.Fatalf("Metadata[region] = %q, want %q", got.Metadata["region"], "us-east1")
	}
}

// ---------------------------------------------------------------------
// computeContext — environment scope
// ---------------------------------------------------------------------

func TestComputeContext_EnvironmentScope_InheritsAccountLabels(t *testing.T) {
	got, err := computeContext(contextInput{
		Init: contextInit{
			Account: strptr("acme"),
			Metadata: map[string]string{
				"ecosystem": "production",
				"platform":  "core",
			},
		},
		Environment: strptr("prod"),
		Metadata: map[string]string{
			"environment_id": "42",
			"region":         "us-east1",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Scope != "environment" {
		t.Fatalf("Scope = %q, want %q", got.Scope, "environment")
	}
	if got.Account != "acme" || got.Environment == nil || *got.Environment != "prod" {
		t.Fatalf("Account/Environment = %q/%v, want acme/prod", got.Account, got.Environment)
	}
	if got.Service != nil {
		t.Fatalf("Service should be nil at environment scope, got %v", got.Service)
	}

	// platform/ecosystem must survive from the account-level label cascade
	// even though "environment" scope's own metadata filter doesn't list them.
	// environment_id is absent here on purpose: environmentMetadataKeys only
	// allows "region", so environment_id never reaches environmentMetadata in
	// the first place — matching the original HCL module's behavior exactly
	// (verified empirically against modules/system-context; its label filter
	// lists "environment_id" too, but that's dead code there for the same
	// reason).
	wantLabels := map[string]string{
		"account":     "acme",
		"environment": "prod",
		"scope":       "environment",
		"platform":    "core",
		"ecosystem":   "production",
		"terraform":   "true",
	}
	if !reflect.DeepEqual(got.Labels, wantLabels) {
		t.Fatalf("Labels = %#v, want %#v", got.Labels, wantLabels)
	}
	if _, ok := got.Labels["region"]; ok {
		t.Fatalf("region must not leak into labels at environment scope: %#v", got.Labels)
	}
	if _, ok := got.Labels["environment-id"]; ok {
		t.Fatalf("environment_id must not survive into labels (dropped upstream at the metadata-allowlist step): %#v", got.Labels)
	}
}

func TestComputeContext_EnvironmentScope_DirectMetadataOverridesInitMetadata(t *testing.T) {
	// region is the one key environment scope can actually introduce/override
	// directly (environment_id can't — see InheritsAccountLabels above).
	got, err := computeContext(contextInput{
		Init: contextInit{
			Account:  strptr("acme"),
			Metadata: mergeStringMaps(baseMetadata(), map[string]string{"region": "from-init"}),
		},
		Environment: strptr("prod"),
		Metadata:    map[string]string{"region": "from-direct"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Metadata["region"] != "from-direct" {
		t.Fatalf("Metadata[region] = %q, want direct metadata to win over init metadata", got.Metadata["region"])
	}
}

func TestComputeContext_EnvironmentScope_UserSuppliedScopeKeyIsOverridden(t *testing.T) {
	got, err := computeContext(contextInput{
		Init:        contextInit{Account: strptr("acme"), Metadata: baseMetadata()},
		Environment: strptr("prod"),
		Metadata:    map[string]string{"scope": "bogus"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Metadata["scope"] != "environment" {
		t.Fatalf("Metadata[scope] = %q, want %q", got.Metadata["scope"], "environment")
	}
}

func TestComputeContext_EnvironmentScope_EmptyAccountPointerTreatedAsUnset(t *testing.T) {
	got, err := computeContext(contextInput{
		Account:     strptr(""),
		Environment: strptr("prod"),
		Metadata:    baseMetadata(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Account != "" {
		t.Fatalf("Account = %q, want empty string", got.Account)
	}
	if _, ok := got.Labels["account"]; ok {
		t.Fatalf("empty account should not appear in labels: %#v", got.Labels)
	}
	if got.Labels["environment"] != "prod" {
		t.Fatalf("Labels[environment] = %q, want %q", got.Labels["environment"], "prod")
	}
}

// ---------------------------------------------------------------------
// computeContext — service scope
// ---------------------------------------------------------------------

func TestComputeContext_ServiceScope_FullCascade(t *testing.T) {
	got, err := computeContext(contextInput{
		Init: contextInit{
			Account:     strptr("acme"),
			Environment: strptr("prod"),
			Metadata: map[string]string{
				"ecosystem": "production",
				"platform":  "core",
			},
		},
		Service: strptr("api"),
		Metadata: map[string]string{
			"version": "1.2.3",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Scope != "service" {
		t.Fatalf("Scope = %q, want %q", got.Scope, "service")
	}
	if got.Account != "acme" || *got.Environment != "prod" || *got.Service != "api" {
		t.Fatalf("got %q/%v/%v, want acme/prod/api", got.Account, got.Environment, got.Service)
	}

	wantLabels := map[string]string{
		"account":     "acme",
		"environment": "prod",
		"service":     "api",
		"scope":       "service",
		"platform":    "core",
		"ecosystem":   "production",
		"terraform":   "true",
		"env":         "prod",
	}
	if !reflect.DeepEqual(got.Labels, wantLabels) {
		t.Fatalf("Labels = %#v, want %#v", got.Labels, wantLabels)
	}
	if got.Metadata["version"] != "1.2.3" {
		t.Fatalf("Metadata[version] = %q, want %q", got.Metadata["version"], "1.2.3")
	}
	if _, ok := got.Labels["version"]; ok {
		t.Fatalf("version must not leak into labels at service scope: %#v", got.Labels)
	}
}

func TestComputeContext_ServiceScope_UserSuppliedScopeKeyIsOverridden(t *testing.T) {
	got, err := computeContext(contextInput{
		Init:     contextInit{Account: strptr("acme"), Environment: strptr("prod"), Metadata: baseMetadata()},
		Service:  strptr("api"),
		Metadata: map[string]string{"scope": "bogus"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Metadata["scope"] != "service" {
		t.Fatalf("Metadata[scope] = %q, want %q", got.Metadata["scope"], "service")
	}
	if got.Labels["scope"] != "service" {
		t.Fatalf("Labels[scope] = %q, want %q", got.Labels["scope"], "service")
	}
}

func TestComputeContext_ServiceScope_EnvironmentFromDirectValue(t *testing.T) {
	// environment doesn't have to come from init — a direct value works too.
	got, err := computeContext(contextInput{
		Account:     strptr("acme"),
		Environment: strptr("prod"),
		Service:     strptr("api"),
		Metadata:    baseMetadata(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Environment == nil || *got.Environment != "prod" {
		t.Fatalf("Environment = %v, want prod", got.Environment)
	}
}

func TestComputeContext_ServiceScope_EmptyServicePointerTreatedAsUnset(t *testing.T) {
	// A non-nil pointer to "" for Service must behave as if Service were
	// never set at all — so no environment-required error, and scope stays
	// at whatever environment/account resolves to.
	got, err := computeContext(contextInput{
		Account:     strptr("acme"),
		Environment: strptr("prod"),
		Service:     strptr(""),
		Metadata:    baseMetadata(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Scope != "environment" {
		t.Fatalf("Scope = %q, want %q (empty service should be treated as unset)", got.Scope, "environment")
	}
	if got.Service != nil {
		t.Fatalf("Service = %v, want nil", got.Service)
	}
}

// ---------------------------------------------------------------------
// computeContext — cross-scope precedence
// ---------------------------------------------------------------------

func TestComputeContext_DirectValueOverridesInit(t *testing.T) {
	got, err := computeContext(contextInput{
		Init: contextInit{
			Account:  strptr("from-init"),
			Metadata: map[string]string{"ecosystem": "production", "platform": "from-init"},
		},
		Account:  strptr("from-direct"),
		Metadata: map[string]string{"platform": "from-direct"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Account != "from-direct" {
		t.Fatalf("Account = %q, want direct value to win over init", got.Account)
	}
	if got.Labels["platform"] != "from-direct" {
		t.Fatalf("Labels[platform] = %q, want direct metadata to win over init metadata", got.Labels["platform"])
	}
}

func TestComputeContext_AccountNameCasingPreservedInAccountFieldButNotInLabels(t *testing.T) {
	got, err := computeContext(contextInput{
		Account:  strptr("ACME"),
		Metadata: baseMetadata(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Account != "ACME" {
		t.Fatalf("Account = %q, want %q (raw output must preserve case)", got.Account, "ACME")
	}
	if got.Labels["account"] != "acme" {
		t.Fatalf("Labels[account] = %q, want %q (labels must be lowercased)", got.Labels["account"], "acme")
	}
}

func TestComputeContext_LabelNormalization(t *testing.T) {
	got, err := computeContext(contextInput{
		Account: strptr("acme"),
		Metadata: map[string]string{
			"ecosystem": "production",
			"platform":  "some.dotted.value",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Labels["platform"] != "some-dotted-value" {
		t.Fatalf("Labels[platform] = %q, want dots replaced with dashes", got.Labels["platform"])
	}
}

// ---------------------------------------------------------------------
// computeContext — errors
// ---------------------------------------------------------------------

func TestComputeContext_ServiceRequiresEnvironment(t *testing.T) {
	// Mirrors the HCL module's implicit requirement: module.service reads
	// module.environment[0], which doesn't exist unless environment is set —
	// there, an unset environment surfaces as a confusing "index out of
	// range" error deep in a submodule. Here it's a direct, named error.
	_, err := computeContext(contextInput{
		Account: strptr("acme"),
		Service: strptr("api"),
	})
	if err == nil {
		t.Fatal("expected an error when service is set without environment, got nil")
	}
}

func TestComputeContext_ServiceRequiresEnvironment_ErrorNamesTheService(t *testing.T) {
	_, err := computeContext(contextInput{
		Account: strptr("acme"),
		Service: strptr("api"),
	})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if got := err.Error(); !strings.Contains(got, "api") || !strings.Contains(got, "environment") {
		t.Fatalf("error %q should name the service and mention environment", got)
	}
}

func TestComputeContext_ServiceFromInit_RequiresEnvironment(t *testing.T) {
	// Service can be supplied via Init just as easily as directly — the
	// environment requirement must still be enforced in that path.
	_, err := computeContext(contextInput{
		Account: strptr("acme"),
		Init:    contextInit{Service: strptr("api")},
	})
	if err == nil {
		t.Fatal("expected an error when service (via init) is set without environment, got nil")
	}
}

func TestComputeContext_ServiceWithEmptyEnvironmentPointerStillErrors(t *testing.T) {
	// Environment explicitly set to a pointer-to-empty-string is the same as
	// unset, so this must still error.
	_, err := computeContext(contextInput{
		Account:     strptr("acme"),
		Environment: strptr(""),
		Service:     strptr("api"),
	})
	if err == nil {
		t.Fatal("expected an error when environment resolves to unset (empty string), got nil")
	}
}

func TestComputeContext_ErrorResultIsZeroValue(t *testing.T) {
	got, err := computeContext(contextInput{
		Account: strptr("acme"),
		Service: strptr("api"),
	})
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !reflect.DeepEqual(got, contextResult{}) {
		t.Fatalf("result on error = %#v, want zero value", got)
	}
}
