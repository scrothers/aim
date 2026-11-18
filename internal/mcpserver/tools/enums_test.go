package tools_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mcpserver/mcptest"
)

// schemaProp returns the property at path in a tool's input or output schema,
// following array items and $refs into $defs.
func schemaProp(t *testing.T, schema any, path ...string) map[string]any {
	t.Helper()
	b, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(b, &root); err != nil {
		t.Fatal(err)
	}
	defs, _ := root["$defs"].(map[string]any)
	resolve := func(s map[string]any) map[string]any {
		for {
			if ref, ok := s["$ref"].(string); ok {
				name := ref[len("#/$defs/"):]
				s, _ = defs[name].(map[string]any)
				continue
			}
			if items, ok := s["items"].(map[string]any); ok {
				s = items
				continue
			}
			return s
		}
	}
	cur := resolve(root)
	for _, p := range path {
		props, _ := cur["properties"].(map[string]any)
		next, ok := props[p].(map[string]any)
		if !ok {
			t.Fatalf("no property %v in schema", path)
		}
		cur = resolve(next)
	}
	return cur
}

func enumOf(s map[string]any) []string {
	values, _ := s["enum"].([]any)
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, v.(string))
	}
	return out
}

func TestEnumsInToolSchemas(t *testing.T) {
	t.Parallel()
	cs := mcptest.New(t).Client(t, "a")
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	tools := map[string]*mcp.Tool{}
	for _, tool := range res.Tools {
		tools[tool.Name] = tool
	}
	tests := []struct {
		name   string
		schema any
		path   []string
		want   []string
	}{
		{"list input state", tools["mail_list"].InputSchema, []string{"state"}, []string{"unread", "inbox", "read", "archived", "all"}},
		{"mark input state", tools["mail_mark"].InputSchema, []string{"state"}, []string{"read", "unread", "archived", "unarchived"}},
		{"list output kind", tools["mail_list"].OutputSchema, []string{"items", "kind"}, []string{"to", "cc"}},
		{"roster output session state", tools["agent_roster"].OutputSchema, []string{"identities", "online", "state"}, []string{"online", "idle"}},
		{"roster output attention", tools["agent_roster"].OutputSchema, []string{"identities", "online", "attention"}, []string{"blocked", "waiting", "failed", "interrupted"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := schemaProp(t, tc.schema, tc.path...)
			if p["type"] != "string" {
				t.Errorf("type = %v, want string", p["type"])
			}
			if got := enumOf(p); !slices.Equal(got, tc.want) {
				t.Errorf("enum = %v, want %v", got, tc.want)
			}
			if d, _ := p["description"].(string); d == "" {
				t.Error("enum property lost its description")
			}
		})
	}
}

func TestEnumRejectsUnknownValue(t *testing.T) {
	t.Parallel()
	cs := mcptest.New(t).Client(t, "a")
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "mail_list", Arguments: map[string]any{"address": "github.com/acme/api", "state": "done"},
	})
	if err != nil {
		t.Fatalf("protocol error %v; want a tool error the model can read and correct", err)
	}
	if !res.IsError {
		t.Fatal("list accepted state=done")
	}
	// The SDK's validator names the field and the allowed values.
	msg := mcptest.Text(res)
	for _, want := range []string{"state", "done", "unread inbox read archived all"} {
		if !strings.Contains(msg, want) {
			t.Errorf("rejection %q lacks %q", msg, want)
		}
	}
}
