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

func TestSnippetIsPlainText(t *testing.T) {
	t.Parallel()
	r := render.New()
	cases := []struct {
		name, src string
		max       int
		want      string
	}{
		{"heading and table", "## Plan\n\n| a | b |\n|---|---|\n| 1 | 2 |\n", 100, "Plan a b 1 2"},
		{"emphasis and link", "**bold** and _it_ [site](http://x.test)", 100, "bold and it site"},
		{"code fence", "```go\nfmt.Println(1)\n```\ndone", 100, "fmt.Println(1) done"},
		{"entities", "a &amp; b <script>x</script>", 100, "a & b x"},
		{"truncated", "abcdefghij", 4, "abcd…"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := r.Snippet(tt.src, tt.max); got != tt.want {
				t.Errorf("Snippet(%q) = %q, want %q", tt.src, got, tt.want)
			}
		})
	}
}
