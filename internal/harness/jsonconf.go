package harness

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// JSONObject is a JSON object that remembers key order, so config files can
// be edited without reshuffling the user's keys. Values are *JSONObject,
// []any, string, json.Number, bool, nil, or anything json.Marshal accepts.
type JSONObject struct {
	keys []string
	vals map[string]any
}

// NewJSONObject returns an empty object.
func NewJSONObject() *JSONObject {
	return &JSONObject{vals: map[string]any{}}
}

// Obj builds an object from alternating string keys and values.
func Obj(kv ...any) *JSONObject {
	o := NewJSONObject()
	for i := 0; i+1 < len(kv); i += 2 {
		o.Set(kv[i].(string), kv[i+1])
	}
	return o
}

// Len returns the number of keys.
func (o *JSONObject) Len() int { return len(o.keys) }

// Keys returns the keys in order.
func (o *JSONObject) Keys() []string { return append([]string(nil), o.keys...) }

// Get returns the value for key.
func (o *JSONObject) Get(key string) (any, bool) {
	v, ok := o.vals[key]
	return v, ok
}

// Set stores v under key, keeping the position of an existing key and
// appending a new one.
func (o *JSONObject) Set(key string, v any) {
	if _, ok := o.vals[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = v
}

// Delete removes key.
func (o *JSONObject) Delete(key string) {
	if _, ok := o.vals[key]; !ok {
		return
	}
	delete(o.vals, key)
	for i, k := range o.keys {
		if k == key {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

// Child returns the object stored under key. When the key is absent and
// create is true it is added. ok is false when the key is missing (and not
// created) or holds a non-object.
func (o *JSONObject) Child(key string, create bool) (child *JSONObject, ok bool) {
	v, present := o.vals[key]
	if !present {
		if !create {
			return nil, false
		}
		c := NewJSONObject()
		o.Set(key, c)
		return c, true
	}
	c, isObj := v.(*JSONObject)
	return c, isObj
}

// ParseJSONObject decodes data as one JSON object. Empty or whitespace-only
// data yields an empty object. Anything else that is not plain JSON, such as
// JSONC with comments, is an error.
func ParseJSONObject(data []byte) (*JSONObject, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return NewJSONObject(), nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := parseJSONValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("unexpected data after the top-level value")
	}
	o, ok := v.(*JSONObject)
	if !ok {
		return nil, errors.New("top-level value is not an object")
	}
	return o, nil
}

func parseJSONValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	d, isDelim := tok.(json.Delim)
	if !isDelim {
		return tok, nil
	}
	switch d {
	case '{':
		o := NewJSONObject()
		for dec.More() {
			kt, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key, _ := kt.(string)
			v, err := parseJSONValue(dec)
			if err != nil {
				return nil, err
			}
			o.Set(key, v)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return o, nil
	case '[':
		arr := []any{}
		for dec.More() {
			v, err := parseJSONValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return arr, nil
	}
	return nil, fmt.Errorf("unexpected delimiter %q", d)
}

// Bytes encodes the object with two-space indentation and a trailing
// newline.
func (o *JSONObject) Bytes() []byte {
	var b bytes.Buffer
	writeJSON(&b, o, 0)
	b.WriteByte('\n')
	return b.Bytes()
}

func writeJSON(b *bytes.Buffer, v any, depth int) {
	indent := func(n int) { b.WriteString(strings.Repeat("  ", n)) }
	switch t := v.(type) {
	case *JSONObject:
		if t.Len() == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteString("{\n")
		for i, k := range t.keys {
			indent(depth + 1)
			writeJSON(b, k, 0)
			b.WriteString(": ")
			writeJSON(b, t.vals[k], depth+1)
			if i < len(t.keys)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		indent(depth)
		b.WriteByte('}')
	case []any:
		if len(t) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteString("[\n")
		for i, e := range t {
			indent(depth + 1)
			writeJSON(b, e, depth+1)
			if i < len(t)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		indent(depth)
		b.WriteByte(']')
	case json.Number:
		b.WriteString(t.String())
	default:
		var sb bytes.Buffer
		enc := json.NewEncoder(&sb)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(t); err != nil {
			b.WriteString("null")
			return
		}
		b.Write(bytes.TrimRight(sb.Bytes(), "\n"))
	}
}

// HookEntry is one hook handler AIM installs into a Claude-shaped hooks
// object.
type HookEntry struct {
	Event string
	// Matcher is the matcher group's matcher; empty omits the key.
	Matcher string
	// Handler is the handler object, normally {"type":"command",...}.
	Handler *JSONObject
}

// isAimHandler reports whether a handler object runs `<aim> hook <runtime>`.
func isAimHandler(h any, runtime string) bool {
	o, ok := h.(*JSONObject)
	if !ok {
		return false
	}
	cmd, _ := o.vals["command"].(string)
	return strings.Contains(cmd, " hook "+runtime) && strings.Contains(cmd, "aim")
}

// MergeHooks edits a Claude-shaped settings document. It removes every AIM
// handler for runtime, drops matcher groups and events left empty, then (unless
// uninstall) appends entries. Unrelated keys and handlers keep their order.
// Installing twice yields identical bytes. When data is not plain JSON the
// error carries the snippet for the operator to add by hand and the file is
// never rewritten.
func MergeHooks(path string, data []byte, runtime string, entries []HookEntry, uninstall bool) ([]byte, error) {
	root, err := ParseJSONObject(data)
	if err != nil {
		return nil, ConfigParseError(path, err, hookSnippet(entries, uninstall))
	}
	hooks, ok := root.Child("hooks", !uninstall)
	if !ok && root.vals["hooks"] != nil {
		return nil, fmt.Errorf("%s: \"hooks\" is not an object; fix it by hand", path)
	}
	if hooks == nil && uninstall {
		return data, nil
	}
	if hooks != nil {
		if !stripAimHooks(hooks, runtime) && uninstall {
			return data, nil // nothing of ours: leave the file byte-for-byte
		}
		if !uninstall {
			for _, e := range entries {
				appendHook(hooks, e)
			}
		}
		if hooks.Len() == 0 {
			root.Delete("hooks")
		}
	}
	return root.Bytes(), nil
}

// stripAimHooks removes AIM's handlers and reports whether it found any.
func stripAimHooks(hooks *JSONObject, runtime string) (removed bool) {
	for _, ev := range hooks.Keys() {
		groups, ok := hooks.vals[ev].([]any)
		if !ok {
			continue
		}
		var keptGroups []any
		for _, g := range groups {
			grp, ok := g.(*JSONObject)
			if !ok {
				keptGroups = append(keptGroups, g)
				continue
			}
			hs, ok := grp.vals["hooks"].([]any)
			if !ok {
				keptGroups = append(keptGroups, g)
				continue
			}
			var kept []any
			for _, h := range hs {
				if !isAimHandler(h, runtime) {
					kept = append(kept, h)
				}
			}
			if len(kept) == len(hs) {
				keptGroups = append(keptGroups, g)
			} else {
				removed = true
				if len(kept) > 0 {
					grp.vals["hooks"] = kept
					keptGroups = append(keptGroups, g)
				}
			}
		}
		if len(keptGroups) == 0 {
			hooks.Delete(ev)
		} else {
			hooks.vals[ev] = keptGroups
		}
	}
	return removed
}

func appendHook(hooks *JSONObject, e HookEntry) {
	group := NewJSONObject()
	if e.Matcher != "" {
		group.Set("matcher", e.Matcher)
	}
	group.Set("hooks", []any{e.Handler})
	groups, _ := hooks.vals[e.Event].([]any)
	hooks.Set(e.Event, append(groups, group))
}

func hookSnippet(entries []HookEntry, uninstall bool) []byte {
	if uninstall {
		return nil
	}
	root := NewJSONObject()
	hooks, _ := root.Child("hooks", true)
	for _, e := range entries {
		appendHook(hooks, e)
	}
	return root.Bytes()
}

// ConfigParseError explains that path is not plain JSON and, when snippet is
// non-empty, shows what to add by hand.
func ConfigParseError(path string, cause error, snippet []byte) error {
	if len(snippet) == 0 {
		return fmt.Errorf("%s is not plain JSON (%w); AIM will not rewrite it: remove the AIM entries by hand", path, cause)
	}
	return fmt.Errorf("%s is not plain JSON (%w); AIM will not rewrite it: add this by hand:\n%s", path, cause, snippet)
}
