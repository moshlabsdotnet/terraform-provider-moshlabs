package provider

import (
	"encoding/json"
	"math/big"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// ---------------------------------------------------------------------
// stateValueAndType
// ---------------------------------------------------------------------

func TestStateValueAndType_String(t *testing.T) {
	v, typ, err := stateValueAndType(tftypes.NewValue(tftypes.String, "moshlabs-dev"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != "moshlabs-dev" || typ != "string" {
		t.Fatalf("got (%#v, %#v), want (\"moshlabs-dev\", \"string\")", v, typ)
	}
}

func TestStateValueAndType_Bool(t *testing.T) {
	v, typ, err := stateValueAndType(tftypes.NewValue(tftypes.Bool, true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != true || typ != "bool" {
		t.Fatalf("got (%#v, %#v), want (true, \"bool\")", v, typ)
	}
}

func TestStateValueAndType_Number(t *testing.T) {
	v, typ, err := stateValueAndType(tftypes.NewValue(tftypes.Number, big.NewFloat(84)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != json.Number("84") || typ != "number" {
		t.Fatalf("got (%#v, %#v), want (json.Number(\"84\"), \"number\")", v, typ)
	}
}

func TestStateValueAndType_Null(t *testing.T) {
	// A null is always recorded as type "string", regardless of the
	// declared/eventual type of the field — see the doc comment on
	// stateValueAndType for why (matches go-cty's own ImpliedType default,
	// and sidesteps needing to trust how far Terraform core happened to
	// propagate a concrete type through a `null` literal).
	v, typ, err := stateValueAndType(tftypes.NewValue(tftypes.String, nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != nil || typ != "string" {
		t.Fatalf("got (%#v, %#v), want (nil, \"string\")", v, typ)
	}
}

func TestStateValueAndType_Unknown(t *testing.T) {
	_, _, err := stateValueAndType(tftypes.NewValue(tftypes.String, tftypes.UnknownValue))
	if err == nil {
		t.Fatal("expected an error for an unknown value, got nil")
	}
}

func TestStateValueAndType_Object(t *testing.T) {
	obj := tftypes.NewValue(
		tftypes.Object{AttributeTypes: map[string]tftypes.Type{
			"account":     tftypes.String,
			"environment": tftypes.String,
		}},
		map[string]tftypes.Value{
			"account":     tftypes.NewValue(tftypes.String, "moshlabs-dev"),
			"environment": tftypes.NewValue(tftypes.String, nil),
		},
	)

	v, typ, err := stateValueAndType(obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantValue := map[string]interface{}{"account": "moshlabs-dev", "environment": nil}
	if !reflect.DeepEqual(v, wantValue) {
		t.Fatalf("value: got %#v, want %#v", v, wantValue)
	}

	wantType := []interface{}{"object", map[string]interface{}{"account": "string", "environment": "string"}}
	if !reflect.DeepEqual(typ, wantType) {
		t.Fatalf("type: got %#v, want %#v", typ, wantType)
	}
}

func TestStateValueAndType_MapEncodesAsObjectType(t *testing.T) {
	// Without a declared variable type, a map(string) and an object(...) are
	// indistinguishable from the value alone — both come out as "object".
	m := tftypes.NewValue(
		tftypes.Map{ElementType: tftypes.String},
		map[string]tftypes.Value{
			"ecosystem": tftypes.NewValue(tftypes.String, "development"),
		},
	)

	_, typ, err := stateValueAndType(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantType := []interface{}{"object", map[string]interface{}{"ecosystem": "string"}}
	if !reflect.DeepEqual(typ, wantType) {
		t.Fatalf("type: got %#v, want %#v", typ, wantType)
	}
}

func TestStateValueAndType_TupleAndList(t *testing.T) {
	tuple := tftypes.NewValue(
		tftypes.Tuple{ElementTypes: []tftypes.Type{tftypes.String, tftypes.Number}},
		[]tftypes.Value{
			tftypes.NewValue(tftypes.String, "a"),
			tftypes.NewValue(tftypes.Number, big.NewFloat(1)),
		},
	)

	v, typ, err := stateValueAndType(tuple)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantValue := []interface{}{"a", json.Number("1")}
	if !reflect.DeepEqual(v, wantValue) {
		t.Fatalf("value: got %#v, want %#v", v, wantValue)
	}
	wantType := []interface{}{"tuple", []interface{}{"string", "number"}}
	if !reflect.DeepEqual(typ, wantType) {
		t.Fatalf("type: got %#v, want %#v", typ, wantType)
	}
}

func TestStateValueAndType_NestedObject(t *testing.T) {
	inner := tftypes.NewValue(
		tftypes.Object{AttributeTypes: map[string]tftypes.Type{"region": tftypes.String}},
		map[string]tftypes.Value{"region": tftypes.NewValue(tftypes.String, "us-east-1")},
	)
	outer := tftypes.NewValue(
		tftypes.Object{AttributeTypes: map[string]tftypes.Type{"metadata": inner.Type()}},
		map[string]tftypes.Value{"metadata": inner},
	)

	_, typ, err := stateValueAndType(outer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantType := []interface{}{"object", map[string]interface{}{
		"metadata": []interface{}{"object", map[string]interface{}{"region": "string"}},
	}}
	if !reflect.DeepEqual(typ, wantType) {
		t.Fatalf("type: got %#v, want %#v", typ, wantType)
	}
}

// ---------------------------------------------------------------------
// renderStateJSON
// ---------------------------------------------------------------------

func TestRenderStateJSON_Shape(t *testing.T) {
	value := map[string]interface{}{"account": "moshlabs-dev"}
	typ := []interface{}{"object", map[string]interface{}{"account": "string"}}

	out, err := renderStateJSON("platform", value, typ, true, "1.5.0", 1, "aaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}

	if doc["version"] != float64(4) {
		t.Fatalf("version: got %#v, want 4", doc["version"])
	}
	if doc["terraform_version"] != "1.5.0" {
		t.Fatalf("terraform_version: got %#v, want \"1.5.0\"", doc["terraform_version"])
	}
	if doc["serial"] != float64(1) {
		t.Fatalf("serial: got %#v, want 1", doc["serial"])
	}
	if doc["lineage"] != "aaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
		t.Fatalf("lineage: got %#v", doc["lineage"])
	}

	resources, ok := doc["resources"].([]interface{})
	if !ok || len(resources) != 0 {
		t.Fatalf("resources: got %#v, want empty array", doc["resources"])
	}
	checkResults, ok := doc["check_results"].([]interface{})
	if !ok || len(checkResults) != 0 {
		t.Fatalf("check_results: got %#v, want empty array", doc["check_results"])
	}

	outputs, ok := doc["outputs"].(map[string]interface{})
	if !ok {
		t.Fatalf("outputs: got %#v, want an object", doc["outputs"])
	}
	platform, ok := outputs["platform"].(map[string]interface{})
	if !ok {
		t.Fatalf("outputs.platform: got %#v, want an object", outputs["platform"])
	}
	if platform["sensitive"] != true {
		t.Fatalf("outputs.platform.sensitive: got %#v, want true", platform["sensitive"])
	}
	wantValue := map[string]interface{}{"account": "moshlabs-dev"}
	if !reflect.DeepEqual(platform["value"], wantValue) {
		t.Fatalf("outputs.platform.value: got %#v, want %#v", platform["value"], wantValue)
	}
}
