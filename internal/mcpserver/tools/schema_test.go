package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func TestPortableRewritesTypeLists(t *testing.T) {
	t.Parallel()
	type inner struct {
		Tags []string `json:"tags"`
	}
	type sample struct {
		List   []string  `json:"list"`
		When   *string   `json:"when"`
		Nested []inner   `json:"nested"`
		Ptr    *inner    `json:"ptr"`
		Fixed  string    `json:"fixed"`
		Pairs  [][]int64 `json:"pairs"`
	}
	s, err := jsonschema.For[sample](nil)
	if err != nil {
		t.Fatal(err)
	}
	portable(s)
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if strings.Contains(got, `"type":["`) {
		t.Fatalf("schema still has a type list: %s", got)
	}
	p := s.Properties
	if p["list"].Type != "array" || p["list"].AnyOf != nil {
		t.Errorf("list = %+v, want plain array", p["list"])
	}
	if p["nested"].Items.Properties["tags"].Type != "array" {
		t.Errorf("nested tags not rewritten: %+v", p["nested"].Items.Properties["tags"])
	}
	if p["pairs"].Items.Type != "array" {
		t.Errorf("inner slice of slice not rewritten: %+v", p["pairs"].Items)
	}
	for _, name := range []string{"when", "ptr"} {
		branches := p[name].AnyOf
		if len(branches) != 2 || branches[0].Type != "null" {
			t.Errorf("%s anyOf = %+v, want [null, T]", name, branches)
		}
	}
	if p["fixed"].Type != "string" || p["fixed"].AnyOf != nil {
		t.Errorf("fixed changed: %+v", p["fixed"])
	}
}

func TestNullable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   []string
		want string
		ok   bool
	}{
		{[]string{"null", "array"}, "array", true},
		{[]string{"string", "null"}, "string", true},
		{[]string{"string", "integer"}, "", false},
		{[]string{"null"}, "", false},
		{nil, "", false},
	}
	for _, c := range cases {
		got, ok := nullable(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("nullable(%v) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
	portable(nil) // must not panic
}
