package tools

import (
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// addTool registers a typed tool whose input and output schemas are inferred
// from In and Out, made portable (see portable), and whose description comes
// from <name>.md in this package.
func addTool[In, Out any](srv *mcp.Server, name string, h mcp.ToolHandlerFor[In, Out]) {
	in, err := jsonschema.For[In](nil)
	if err != nil {
		panic(fmt.Sprintf("tools: infer %s input schema: %v", name, err))
	}
	out, err := jsonschema.For[Out](nil)
	if err != nil {
		panic(fmt.Sprintf("tools: infer %s output schema: %v", name, err))
	}
	portable(in)
	portable(out)
	mcp.AddTool(srv, &mcp.Tool{
		Name:         name,
		Description:  description(name),
		InputSchema:  in,
		OutputSchema: out,
	}, h)
}

// portable rewrites schema inference's type lists into forms every MCP client
// accepts. Inference maps Go slices to ["null","array"] and pointers to
// ["null",T]; some clients map schemas onto a single-`type` dialect and reject
// or mangle a type list.
//
//   - ["null","array"] becomes "array": handlers never return nil slices
//     (they normalize to empty), and a null array input is not meaningful.
//   - any other ["null",T] becomes anyOf [{"type":"null"},{"type":T}], keeping
//     null legal (e.g. read_at is null until a message is read). Keywords for
//     T (items, properties, format) stay on the parent, where they apply only
//     when the value has that type.
func portable(s *jsonschema.Schema) {
	if s == nil {
		return
	}
	if t, ok := nullable(s.Types); ok {
		s.Types = nil
		if t == "array" {
			s.Type = "array"
		} else {
			s.AnyOf = append(s.AnyOf, &jsonschema.Schema{Type: "null"}, &jsonschema.Schema{Type: t})
		}
	}
	for _, c := range s.Properties {
		portable(c)
	}
	for _, c := range s.Defs {
		portable(c)
	}
	portable(s.Items)
	portable(s.AdditionalProperties)
	for _, list := range [][]*jsonschema.Schema{s.AnyOf, s.AllOf, s.OneOf, s.PrefixItems} {
		for _, c := range list {
			portable(c)
		}
	}
}

// nullable reports the non-null type of a two-element type list containing
// "null", such as ["null","string"].
func nullable(types []string) (string, bool) {
	if len(types) != 2 {
		return "", false
	}
	switch {
	case types[0] == "null":
		return types[1], true
	case types[1] == "null":
		return types[0], true
	}
	return "", false
}
