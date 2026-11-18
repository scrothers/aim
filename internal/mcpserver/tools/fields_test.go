package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/scrothers/aim/internal/mcpserver/limits"
)

// TestSchemaFieldDescriptionsFitBudget checks every property description in
// the input and output schemas of the tools as clients receive them: each
// fits limits.MaxFieldDescriptionBytes, and every input property has one.
func TestSchemaFieldDescriptionsFitBudget(t *testing.T) {
	t.Parallel()
	cs := newEnv(t).Client(t, "a")
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) == 0 {
		t.Fatal("no tools listed")
	}
	for _, tool := range res.Tools {
		for kind, schema := range map[string]any{"input": tool.InputSchema, "output": tool.OutputSchema} {
			b, err := json.Marshal(schema)
			if err != nil {
				t.Fatal(err)
			}
			var root map[string]any
			if err := json.Unmarshal(b, &root); err != nil {
				t.Fatalf("%s %s schema: %v", tool.Name, kind, err)
			}
			walkSchema(root, tool.Name+" "+kind, func(path string, s map[string]any) {
				if d, ok := s["description"].(string); ok && len(d) > limits.MaxFieldDescriptionBytes {
					t.Errorf("%s: description is %d bytes; keep it at most %d", path, len(d), limits.MaxFieldDescriptionBytes)
				}
			}, func(path string, prop map[string]any) {
				if kind != "input" {
					return
				}
				if d, _ := prop["description"].(string); d == "" {
					t.Errorf("%s: input property has no description", path)
				}
			})
		}
	}
}

// walkSchema calls onSchema for s and every subschema below it (properties,
// items, $defs, anyOf and any other keyword), and onProperty for each named
// property. path names the location for error messages.
func walkSchema(s map[string]any, path string, onSchema, onProperty func(path string, s map[string]any)) {
	onSchema(path, s)
	for key, v := range s {
		switch key {
		case "properties", "$defs", "definitions", "patternProperties":
			named, _ := v.(map[string]any)
			for name, sub := range named {
				m, ok := sub.(map[string]any)
				if !ok {
					continue
				}
				p := path + "." + name
				if key == "properties" {
					onProperty(p, m)
				}
				walkSchema(m, p, onSchema, onProperty)
			}
		default:
			walkValue(v, path+"/"+key, onSchema, onProperty)
		}
	}
}

// walkValue walks the schemas inside a keyword's value: a subschema, or a
// list of them as in anyOf, allOf, oneOf and prefixItems.
func walkValue(v any, path string, onSchema, onProperty func(path string, s map[string]any)) {
	switch v := v.(type) {
	case map[string]any:
		walkSchema(v, path, onSchema, onProperty)
	case []any:
		for _, e := range v {
			walkValue(e, path, onSchema, onProperty)
		}
	}
}
