package render

import (
	"bytes"
	"fmt"
	"regexp"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// Renderer converts Markdown to sanitized HTML. It is safe for concurrent use.
type Renderer struct {
	md     goldmark.Markdown
	policy *bluemonday.Policy
}

// New returns a Renderer using GFM and a UGC sanitization policy.
func New() *Renderer {
	p := bluemonday.UGCPolicy()
	p.AllowAttrs("class").Matching(regexp.MustCompile(`^language-[\w-]+$`)).OnElements("code")
	p.AllowAttrs("type").Matching(regexp.MustCompile(`^checkbox$`)).OnElements("input")
	p.AllowAttrs("checked", "disabled").OnElements("input")
	p.RequireNoFollowOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	return &Renderer{
		md:     goldmark.New(goldmark.WithExtensions(extension.GFM)),
		policy: p,
	}
}

// HTML renders src as sanitized HTML.
func (r *Renderer) HTML(src string) (string, error) {
	var buf bytes.Buffer
	if err := r.md.Convert([]byte(src), &buf); err != nil {
		return "", fmt.Errorf("render: markdown: %w", err)
	}
	return r.policy.Sanitize(buf.String()), nil
}

// Markdown renders src as sanitized HTML using a default Renderer.
func Markdown(src string) (string, error) {
	return New().HTML(src)
}
