// Copyright 2026 Mosh Labs
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// stateDocument mirrors a real Terraform state v4 file, but only ever
// carries a single output — this provider has no other resources to track,
// so "resources" and "check_results" are always empty.
type stateDocument struct {
	Version          int                         `json:"version"`
	TerraformVersion string                      `json:"terraform_version"`
	Serial           int64                       `json:"serial"`
	Lineage          string                      `json:"lineage"`
	Outputs          map[string]stateOutputEntry `json:"outputs"`
	Resources        []interface{}               `json:"resources"`
	CheckResults     []interface{}               `json:"check_results"`
}

type stateOutputEntry struct {
	Value     interface{} `json:"value"`
	Type      interface{} `json:"type"`
	Sensitive bool        `json:"sensitive"`
}

// renderStateJSON assembles the full document and marshals it exactly like
// `terraform show -json`/a real .tfstate file: 2-space indented, single
// output keyed by name.
func renderStateJSON(name string, value, typ interface{}, sensitive bool, terraformVersion string, serial int64, lineage string) (string, error) {
	doc := stateDocument{
		Version:          4,
		TerraformVersion: terraformVersion,
		Serial:           serial,
		Lineage:          lineage,
		Outputs: map[string]stateOutputEntry{
			name: {Value: value, Type: typ, Sensitive: sensitive},
		},
		Resources:    []interface{}{},
		CheckResults: []interface{}{},
	}

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encoding state document: %w", err)
	}
	return string(out), nil
}

// stateValueAndType walks an arbitrary tftypes.Value and produces both its
// plain-JSON representation and a matching ctyjson-style type descriptor —
// the same paired ("value", "type") encoding Terraform itself uses for state
// file outputs.
//
// Every keyed collection (an HCL object literal or a map alike) is encoded
// as an "object" type, and every ordered collection (list, set, or tuple
// alike) as a "tuple" type. Without the caller's original variable/output
// type declaration, the narrower distinction (`map(string)` vs `object(...)`,
// `list(string)` vs `tuple(...)`) can't be recovered from a value alone —
// "object" and "tuple" are the more permissive of each pair, so anything a
// narrower type would allow a consumer to do, these still allow.
//
// A null is always recorded as type "string", regardless of what the field
// actually holds when non-null — this matches the default go-cty's own
// ImpliedType() picks when a concrete type can't be inferred from a bare
// JSON null, and keeps null handling independent of exactly how far
// Terraform core happened to propagate a concrete type for that value.
func stateValueAndType(v tftypes.Value) (interface{}, interface{}, error) {
	if !v.IsKnown() {
		return nil, nil, fmt.Errorf("value contains an unknown element — system-state requires a fully known value (e.g. an already-applied output, not an in-flight resource attribute)")
	}
	if v.IsNull() {
		return nil, "string", nil
	}

	t := v.Type()
	switch {
	case t.Is(tftypes.String):
		var s string
		if err := v.As(&s); err != nil {
			return nil, nil, err
		}
		return s, "string", nil

	case t.Is(tftypes.Bool):
		var b bool
		if err := v.As(&b); err != nil {
			return nil, nil, err
		}
		return b, "bool", nil

	case t.Is(tftypes.Number):
		var f big.Float
		if err := v.As(&f); err != nil {
			return nil, nil, err
		}
		return json.Number(f.Text('f', -1)), "number", nil

	case t.Is(tftypes.List{}), t.Is(tftypes.Set{}), t.Is(tftypes.Tuple{}):
		var elems []tftypes.Value
		if err := v.As(&elems); err != nil {
			return nil, nil, err
		}
		values := make([]interface{}, len(elems))
		types := make([]interface{}, len(elems))
		for i, e := range elems {
			ev, et, err := stateValueAndType(e)
			if err != nil {
				return nil, nil, err
			}
			values[i] = ev
			types[i] = et
		}
		return values, []interface{}{"tuple", types}, nil

	case t.Is(tftypes.Map{}), t.Is(tftypes.Object{}):
		var elems map[string]tftypes.Value
		if err := v.As(&elems); err != nil {
			return nil, nil, err
		}
		values := make(map[string]interface{}, len(elems))
		types := make(map[string]interface{}, len(elems))
		for k, e := range elems {
			ev, et, err := stateValueAndType(e)
			if err != nil {
				return nil, nil, err
			}
			values[k] = ev
			types[k] = et
		}
		return values, []interface{}{"object", types}, nil

	default:
		return nil, nil, fmt.Errorf("unsupported type %s", t)
	}
}

// dynamicValueAndType validates a config/plan Dynamic value (non-null,
// fully known) and hands it off to stateValueAndType. Shared by Create and
// Update — both need the identical value/type derivation before deciding
// what to do with serial/lineage.
func dynamicValueAndType(ctx context.Context, v types.Dynamic) (interface{}, interface{}, error) {
	if v.IsNull() || v.IsUnderlyingValueNull() {
		return nil, nil, fmt.Errorf("system-state requires a non-null value to emit as an output")
	}
	if v.IsUnknown() || v.IsUnderlyingValueUnknown() {
		return nil, nil, fmt.Errorf("value contains unknown elements — system-state requires a fully known value (e.g. an already-applied output, not an in-flight resource attribute)")
	}

	tfVal, err := v.UnderlyingValue().ToTerraformValue(ctx)
	if err != nil {
		return nil, nil, err
	}
	return stateValueAndType(tfVal)
}
