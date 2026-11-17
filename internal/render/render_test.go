package render_test

import (
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/render"
)

func TestSanitizesXSS(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"script":     "<script>alert(1)</script>hi",
		"onerror":    `<img src=x onerror="alert(1)">`,
		"jslink":     "[x](javascript:alert(1))",
		"rawhtml":    `<div onclick="x()">hello</div>`,
		"dataurl":    "[x](data:text/html;base64,PHNjcmlwdD4=)",
		"iframe":     `<iframe src="http://evil"></iframe>`,
		"mdimgjs":    "![x](javascript:alert(1))",
		"stylealert": `<p style="background:url(javascript:alert(1))">x</p>`,
	}
	bad := []string{"<script", "onerror", "javascript:", "onclick", "data:", "<iframe", "style="}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			out, err := render.Markdown(src)
			if err != nil {
				t.Fatal(err)
			}
			for _, b := range bad {
				if strings.Contains(strings.ToLower(out), b) {
					t.Errorf("output contains %q: %s", b, out)
				}
			}
		})
	}
}

func TestKeepsFormatting(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, src string
		want      []string
	}{
		{"table", "|a|b|\n|-|-|\n|1|2|\n", []string{"<table>", "<th>a</th>", "<td>2</td>"}},
		{"code", "```go\nfmt.Println()\n```\n", []string{`<code class="language-go">`}},
		{"badclass", "```a.b\nx\n```\n", []string{"<code>x"}},
		{"tasks", "- [x] done\n- [ ] todo\n", []string{`type="checkbox"`, "checked", "disabled"}},
		{"autolink", "see https://example.com/x now", []string{`href="https://example.com/x"`, `rel="nofollow noopener"`, `target="_blank"`}},
		{"relative", "[a](/docs)", []string{`href="/docs"`}},
		{"emphasis", "**bold**", []string{"<strong>bold</strong>"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out, err := render.Markdown(tt.src)
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("missing %q in %s", w, out)
				}
			}
		})
	}
}
