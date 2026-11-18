package mcpserver

import (
	"strings"
	"testing"
)

func TestParseWorkflow(t *testing.T) {
	t.Parallel()
	good := "---\ntitle: T\ndescription: D\narguments:\n  - name: a\n    required: true\n    description: first\n  - name: b\n    required: false\n---\nhello {{.Args.a}}\n"
	w, err := parseWorkflow("x", good)
	if err != nil {
		t.Fatal(err)
	}
	if w.title != "T" || w.description != "D" || len(w.args) != 2 {
		t.Fatalf("workflow = %+v", w)
	}
	if a := w.args[0]; a.name != "a" || !a.required || a.description != "first" {
		t.Errorf("arg 0 = %+v", a)
	}
	if a := w.args[1]; a.name != "b" || a.required {
		t.Errorf("arg 1 = %+v", a)
	}
	var sb strings.Builder
	if err := w.body.Execute(&sb, promptData{Args: map[string]string{"a": "world"}}); err != nil || sb.String() != "hello world" {
		t.Errorf("body = %q, %v", sb.String(), err)
	}

	bad := []struct{ name, src, want string }{
		{"no front matter", "title: T\n", "missing front matter"},
		{"unterminated", "---\ntitle: T\n", "unterminated"},
		{"missing title", "---\ndescription: D\n---\nbody", "title and description"},
		{"unknown key", "---\ntitle: T\ndescription: D\nbogus: 1\n---\n", "unknown key"},
		{"no colon", "---\ntitle: T\ndescription: D\nnonsense\n---\n", "key: value"},
		{"indent outside args", "---\ntitle: T\ndescription: D\n  - name: a\n---\n", "outside arguments"},
		{"attr before item", "---\ntitle: T\ndescription: D\narguments:\n    required: true\n---\n", "before '- name:'"},
		{"arg no colon", "---\ntitle: T\ndescription: D\narguments:\n  - name\n---\n", "key: value"},
		{"bad required", "---\ntitle: T\ndescription: D\narguments:\n  - name: a\n    required: maybe\n---\n", "true or false"},
		{"unknown arg key", "---\ntitle: T\ndescription: D\narguments:\n  - name: a\n    color: red\n---\n", "unknown argument key"},
		{"nameless arg", "---\ntitle: T\ndescription: D\narguments:\n  - description: x\n---\n", "without a name"},
		{"bad template", "---\ntitle: T\ndescription: D\n---\n{{.Args.a", "unclosed"},
	}
	for _, tt := range bad {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseWorkflow("x", tt.src)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestEmbeddedWorkflowsParse(t *testing.T) {
	t.Parallel()
	ws := workflows()
	if len(ws) != 5 {
		t.Fatalf("workflows = %d, want 5", len(ws))
	}
	// Claude Code splits arguments on whitespace: required ones must come first.
	for _, w := range ws {
		seenOptional := false
		for _, a := range w.args {
			if a.required && seenOptional {
				t.Errorf("%s: required argument %q follows an optional one", w.name, a.name)
			}
			seenOptional = seenOptional || !a.required
		}
	}
}
