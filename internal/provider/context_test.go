// Copyright 2026 Mosh Labs
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"reflect"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------

// md builds a metadata map from alternating key/value pairs; a value of
// nullv yields an explicit null element.
const nullv = "\x00null"

func md(kv ...string) map[string]*string {
	out := map[string]*string{}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] == nullv {
			out[kv[i]] = nil
		} else {
			out[kv[i]] = strptr(kv[i+1])
		}
	}
	return out
}

func derefPtr(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func derefMap(m map[string]*string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = derefPtr(v)
	}
	return out
}

func samePtr(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func platformMetadata() map[string]*string {
	return md("organization", "o", "status", "ACTIVE", "control_plane", "c")
}

// ---------------------------------------------------------------------
// coalesceStringPtr
// ---------------------------------------------------------------------

func TestCoalesceStringPtr(t *testing.T) {
	tests := []struct {
		name string
		vals []*string
		want *string
	}{
		{"no args", nil, nil},
		{"all nil", []*string{nil, nil, nil}, nil},
		{"first wins", []*string{strptr("a"), strptr("b")}, strptr("a")},
		{"skips leading nil", []*string{nil, strptr("b")}, strptr("b")},
		{"skips leading empty string", []*string{strptr(""), strptr("b")}, strptr("b")},
		{"empty string is not a value", []*string{strptr("")}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := coalesceStringPtr(tt.vals...); !samePtr(got, tt.want) {
				t.Fatalf("coalesceStringPtr = %s, want %s", derefPtr(got), derefPtr(tt.want))
			}
		})
	}
}

// ---------------------------------------------------------------------
// mergeMetadata / filterKeys / coerceMetadata / normalizeLabels
// ---------------------------------------------------------------------

func TestMergeMetadata(t *testing.T) {
	tests := []struct {
		name string
		in   []map[string]*string
		want map[string]string
	}{
		{"no maps", nil, map[string]string{}},
		{"nil maps are skipped", []map[string]*string{md("a", "1"), nil, md("b", "2")}, map[string]string{"a": "1", "b": "2"}},
		{"later map overrides earlier", []map[string]*string{md("a", "1"), md("a", "2")}, map[string]string{"a": "2"}},
		{"explicit null overrides a value", []map[string]*string{md("a", "1"), md("a", nullv)}, map[string]string{"a": "<nil>"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := derefMap(mergeMetadata(tt.in...)); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("mergeMetadata = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestFilterKeys(t *testing.T) {
	got := derefMap(filterKeys(md("a", "1", "b", nullv, "c", "3", "Scope", "x"), "b", "c", "scope"))
	want := map[string]string{"b": "<nil>", "c": "3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterKeys = %#v, want %#v", got, want)
	}
}

func TestCoerceMetadata(t *testing.T) {
	attrs := []metadataAttr{{Name: "req", Required: true}, {Name: "opt"}}

	got, err := coerceMetadata("x", md("req", "1", "extra", "dropped"), attrs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := map[string]string{"req": "1", "opt": "<nil>"}; !reflect.DeepEqual(derefMap(got), want) {
		t.Fatalf("coerceMetadata = %#v, want %#v (unlisted dropped, unset optional present as null)", derefMap(got), want)
	}

	if _, err := coerceMetadata("x", md("req", nullv), attrs); err != nil {
		t.Fatalf("present-but-null required attribute must pass, got %v", err)
	}

	if _, err := coerceMetadata("x", md("opt", "1"), attrs); err == nil || !strings.Contains(err.Error(), `"req"`) {
		t.Fatalf("missing required attribute must error naming it, got %v", err)
	}
}

func TestNormalizeLabels(t *testing.T) {
	got := normalizeLabels(md(
		"environment_id", "42",
		"Platform", "Some.Dotted_Value",
		"a.b", "x_y",
		"dropped", nullv,
		"empty", "",
	))
	want := map[string]string{
		"environment-id": "42",
		"platform":       "some-dotted_value",
		"a.b":            "x_y",
		"empty":          "",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeLabels = %#v, want %#v", got, want)
	}
}

// ---------------------------------------------------------------------
// computeContext — parity with modules/system-context
//
// Every expectation below was captured by running the HCL module itself
// (terraform apply of a root calling modules/system-context with the same
// inputs) — results are copied from `terraform output -json` verbatim.
// ---------------------------------------------------------------------

type wantContext struct {
	scope       string
	platform    *string
	account     *string
	environment *string
	service     *string
	metadata    map[string]string // "<nil>" = explicit null
	labels      map[string]string
}

func checkContext(t *testing.T, got contextResult, want wantContext) {
	t.Helper()
	if got.Scope != want.scope {
		t.Errorf("Scope = %q, want %q", got.Scope, want.scope)
	}
	for _, f := range []struct {
		name      string
		got, want *string
	}{
		{"Platform", got.Platform, want.platform},
		{"Account", got.Account, want.account},
		{"Environment", got.Environment, want.environment},
		{"Service", got.Service, want.service},
	} {
		if !samePtr(f.got, f.want) {
			t.Errorf("%s = %s, want %s", f.name, derefPtr(f.got), derefPtr(f.want))
		}
	}
	if m := derefMap(got.Metadata); !reflect.DeepEqual(m, want.metadata) {
		t.Errorf("Metadata = %#v\n want %#v", m, want.metadata)
	}
	if !reflect.DeepEqual(got.Labels, want.labels) {
		t.Errorf("Labels = %#v\n want %#v", got.Labels, want.labels)
	}
}

// resultInit converts a resolved context into the init of the next call,
// the way `init = module.<parent>` does in HCL.
func resultInit(r contextResult) contextInit {
	return contextInit{
		Platform:    r.Platform,
		Account:     r.Account,
		Environment: r.Environment,
		Service:     r.Service,
		Metadata:    r.Metadata,
	}
}

func mustContext(t *testing.T, in contextInput) contextResult {
	t.Helper()
	got, err := computeContext(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return got
}

func TestComputeContext_Cascade(t *testing.T) {
	platform := mustContext(t, contextInput{
		Platform: strptr("Acme_Plat"),
		Metadata: md("organization", "Acme.com", "status", "ACTIVE", "control_plane", "core", "extra", "x", "ecosystem", "production"),
	})
	checkContext(t, platform, wantContext{
		scope:    "platform",
		platform: strptr("Acme_Plat"),
		metadata: map[string]string{"control_plane": "core", "organization": "Acme.com", "platform": "Acme_Plat", "scope": "platform", "status": "ACTIVE"},
		labels:   map[string]string{"platform": "acme_plat", "scope": "platform", "terraform": "true"},
	})

	account := mustContext(t, contextInput{
		Init:     resultInit(platform),
		Account:  strptr("Acct.One"),
		Metadata: md("ecosystem", "development", "cloud", "gcp", "platform", "ignored"),
	})
	checkContext(t, account, wantContext{
		scope:    "account",
		platform: strptr("Acme_Plat"),
		account:  strptr("Acct.One"),
		metadata: map[string]string{"cloud": "gcp", "control_plane": "core", "ecosystem": "development", "organization": "Acme.com", "platform": "Acme_Plat", "region": "<nil>", "scope": "account", "status": "ACTIVE"},
		labels:   map[string]string{"account": "acct-one", "ecosystem": "development", "platform": "acme_plat", "scope": "account", "terraform": "true"},
	})

	environment := mustContext(t, contextInput{
		Init:        resultInit(account),
		Environment: strptr("dev"),
		Metadata:    md("region", "us-east1", "version", "9"),
	})
	checkContext(t, environment, wantContext{
		scope:       "environment",
		platform:    strptr("Acme_Plat"),
		account:     strptr("Acct.One"),
		environment: strptr("dev"),
		metadata:    map[string]string{"cloud": "gcp", "control_plane": "core", "ecosystem": "development", "organization": "Acme.com", "platform": "Acme_Plat", "region": "us-east1", "scope": "environment", "status": "ACTIVE"},
		labels:      map[string]string{"account": "acct-one", "ecosystem": "development", "environment": "dev", "platform": "acme_plat", "scope": "environment", "terraform": "true"},
	})

	// nested/num/flag stand in for system-service passing its whole
	// var.config; scalarsFromDynamic has already dropped the nested value
	// and stringified the scalars by the time computeContext sees them.
	service := mustContext(t, contextInput{
		Init:     resultInit(environment),
		Service:  strptr("api_svc"),
		Metadata: md("version", "1.2.3", "num", "3", "flag", "true"),
	})
	checkContext(t, service, wantContext{
		scope:       "service",
		platform:    strptr("Acme_Plat"),
		account:     strptr("Acct.One"),
		environment: strptr("dev"),
		service:     strptr("api_svc"),
		metadata:    map[string]string{"cloud": "gcp", "control_plane": "core", "ecosystem": "development", "organization": "Acme.com", "platform": "Acme_Plat", "region": "us-east1", "scope": "service", "status": "ACTIVE", "version": "1.2.3"},
		labels:      map[string]string{"account": "acct-one", "ecosystem": "development", "env": "dev", "environment": "dev", "platform": "acme_plat", "scope": "service", "service": "api_svc", "terraform": "true"},
	})
}

func TestComputeContext_Parity(t *testing.T) {
	tests := []struct {
		name string
		in   contextInput
		want wantContext
	}{
		{
			name: "account scope, unset optionals present as null",
			in: contextInput{
				Init:     contextInit{Platform: strptr("Acme_Plat"), Metadata: md("organization", "Acme.com", "status", "ACTIVE", "control_plane", "core")},
				Account:  strptr("a2"),
				Metadata: md("ecosystem", "production"),
			},
			want: wantContext{
				scope:    "account",
				platform: strptr("Acme_Plat"),
				account:  strptr("a2"),
				metadata: map[string]string{"cloud": "<nil>", "control_plane": "core", "ecosystem": "production", "organization": "Acme.com", "platform": "Acme_Plat", "region": "<nil>", "scope": "account", "status": "ACTIVE"},
				labels:   map[string]string{"account": "a2", "ecosystem": "production", "platform": "acme_plat", "scope": "account", "terraform": "true"},
			},
		},
		{
			name: "service scope set directly in one call",
			in: contextInput{
				Platform:    strptr("p"),
				Account:     strptr("a"),
				Environment: strptr("e"),
				Service:     strptr("s"),
				Metadata:    mergeMetadata(platformMetadata(), md("ecosystem", "production", "region", "r")),
			},
			want: wantContext{
				scope:       "service",
				platform:    strptr("p"),
				account:     strptr("a"),
				environment: strptr("e"),
				service:     strptr("s"),
				metadata:    map[string]string{"cloud": "<nil>", "control_plane": "c", "ecosystem": "production", "organization": "o", "platform": "p", "region": "r", "scope": "service", "status": "ACTIVE", "version": "<nil>"},
				labels:      map[string]string{"account": "a", "ecosystem": "production", "env": "e", "environment": "e", "platform": "p", "scope": "service", "service": "s", "terraform": "true"},
			},
		},
		{
			name: "null required attribute via init is accepted",
			in: fromInit(contextInit{
				Platform: strptr("p"),
				Metadata: md("organization", "o", "status", "ACTIVE", "control_plane", nullv),
			}),
			want: wantContext{
				scope:    "platform",
				platform: strptr("p"),
				metadata: map[string]string{"control_plane": "<nil>", "organization": "o", "platform": "p", "scope": "platform", "status": "ACTIVE"},
				labels:   map[string]string{"platform": "p", "scope": "platform", "terraform": "true"},
			},
		},
		{
			name: "empty platform passes validation but resolves to null",
			in:   contextInput{Platform: strptr(""), Metadata: platformMetadata()},
			want: wantContext{
				scope:    "platform",
				metadata: map[string]string{"control_plane": "c", "organization": "o", "platform": "<nil>", "scope": "platform", "status": "ACTIVE"},
				labels:   map[string]string{"scope": "platform", "terraform": "true"},
			},
		},
		{
			name: "empty required attribute is still a value",
			in:   contextInput{Platform: strptr("p"), Metadata: md("organization", "", "status", "ACTIVE", "control_plane", "c")},
			want: wantContext{
				scope:    "platform",
				platform: strptr("p"),
				metadata: map[string]string{"control_plane": "c", "organization": "", "platform": "p", "scope": "platform", "status": "ACTIVE"},
				labels:   map[string]string{"platform": "p", "scope": "platform", "terraform": "true"},
			},
		},
		{
			name: "empty account resolves to platform scope",
			in:   contextInput{Platform: strptr("p"), Account: strptr(""), Metadata: platformMetadata()},
			want: wantContext{
				scope:    "platform",
				platform: strptr("p"),
				metadata: map[string]string{"control_plane": "c", "organization": "o", "platform": "p", "scope": "platform", "status": "ACTIVE"},
				labels:   map[string]string{"platform": "p", "scope": "platform", "terraform": "true"},
			},
		},
		{
			name: "empty metadata value is kept",
			in:   contextInput{Platform: strptr("p"), Account: strptr("a"), Metadata: mergeMetadata(platformMetadata(), md("ecosystem", "production", "region", ""))},
			want: wantContext{
				scope:    "account",
				platform: strptr("p"),
				account:  strptr("a"),
				metadata: map[string]string{"cloud": "<nil>", "control_plane": "c", "ecosystem": "production", "organization": "o", "platform": "p", "region": "", "scope": "account", "status": "ACTIVE"},
				labels:   map[string]string{"account": "a", "ecosystem": "production", "platform": "p", "scope": "account", "terraform": "true"},
			},
		},
		{
			name: "direct values override init",
			in: contextInput{
				Init: contextInit{
					Platform:    strptr("ip"),
					Account:     strptr("ia"),
					Environment: strptr("ie"),
					Metadata:    mergeMetadata(platformMetadata(), md("ecosystem", "production", "region", "ir")),
				},
				Environment: strptr("de"),
				Metadata:    md("region", "dr"),
			},
			want: wantContext{
				scope:       "environment",
				platform:    strptr("ip"),
				account:     strptr("ia"),
				environment: strptr("de"),
				metadata:    map[string]string{"cloud": "<nil>", "control_plane": "c", "ecosystem": "production", "organization": "o", "platform": "ip", "region": "dr", "scope": "environment", "status": "ACTIVE"},
				labels:      map[string]string{"account": "ia", "ecosystem": "production", "environment": "de", "platform": "ip", "scope": "environment", "terraform": "true"},
			},
		},
		{
			name: "explicit null metadata overrides init",
			in: contextInput{
				Init:     contextInit{Platform: strptr("p"), Account: strptr("a"), Metadata: mergeMetadata(platformMetadata(), md("ecosystem", "production", "region", "ir"))},
				Metadata: md("region", nullv),
			},
			want: wantContext{
				scope:    "account",
				platform: strptr("p"),
				account:  strptr("a"),
				metadata: map[string]string{"cloud": "<nil>", "control_plane": "c", "ecosystem": "production", "organization": "o", "platform": "p", "region": "<nil>", "scope": "account", "status": "ACTIVE"},
				labels:   map[string]string{"account": "a", "ecosystem": "production", "platform": "p", "scope": "account", "terraform": "true"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkContext(t, mustContext(t, tt.in), tt.want)
		})
	}
}

// fromInit builds an input whose metadata arrives only via init — the only
// way an explicit null reaches computeContext from real config.
func fromInit(init contextInit) contextInput { return contextInput{Init: init} }

func TestComputeContext_Errors(t *testing.T) {
	withAccount := func(extra ...string) map[string]*string {
		return mergeMetadata(platformMetadata(), md(extra...))
	}
	tests := []struct {
		name    string
		in      contextInput
		errPart string
	}{
		{"no platform", contextInput{Metadata: platformMetadata()}, "platform must be set"},
		{"missing control_plane", contextInput{Platform: strptr("p"), Metadata: md("organization", "o", "status", "ACTIVE")}, `"control_plane" is required`},
		{"status must be ACTIVE/INACTIVE", contextInput{Platform: strptr("p"), Metadata: md("organization", "o", "status", "active", "control_plane", "c")}, `"status" must be`},
		{"null status", fromInit(contextInit{Platform: strptr("p"), Metadata: md("organization", "o", "status", nullv, "control_plane", "c")}), `"status" must not be null`},
		{"environment without account", contextInput{Platform: strptr("p"), Environment: strptr("e"), Metadata: platformMetadata()}, "environment requires an account"},
		{"empty environment still requires account", contextInput{Platform: strptr("p"), Environment: strptr(""), Metadata: platformMetadata()}, "environment requires an account"},
		{"service without environment", contextInput{Platform: strptr("p"), Account: strptr("a"), Service: strptr("s"), Metadata: withAccount("ecosystem", "production")}, "service requires an environment"},
		{"empty account under an environment", contextInput{Platform: strptr("p"), Account: strptr(""), Environment: strptr("e"), Metadata: platformMetadata()}, `environment "e" requires an account`},
		{"missing ecosystem", contextInput{Platform: strptr("p"), Account: strptr("a"), Metadata: platformMetadata()}, `"ecosystem" is required`},
		{"null ecosystem", fromInit(contextInit{Platform: strptr("p"), Account: strptr("a"), Metadata: withAccount("ecosystem", nullv)}), `"ecosystem" must not be null`},
		{"invalid ecosystem", contextInput{Platform: strptr("p"), Account: strptr("a"), Metadata: withAccount("ecosystem", "staging")}, `"ecosystem" must be`},
		{"invalid cloud", contextInput{Platform: strptr("p"), Account: strptr("a"), Metadata: withAccount("ecosystem", "production", "cloud", "azure")}, `"cloud" must be`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := computeContext(tt.in)
			if err == nil {
				t.Fatalf("expected error containing %q, got result %#v", tt.errPart, got)
			}
			if !strings.Contains(err.Error(), tt.errPart) {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), tt.errPart)
			}
			if !reflect.DeepEqual(got, contextResult{}) {
				t.Fatalf("error result must be the zero value, got %#v", got)
			}
		})
	}
}
