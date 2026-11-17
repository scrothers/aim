package render

import (
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strings"

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

var (
	blockEnd = regexp.MustCompile(`(?i)</(p|h[1-6]|li|tr|td|th|pre|blockquote)>|<br\s*/?>|<hr\s*/?>`)
	anyTag   = regexp.MustCompile(`<[^>]*>`)
	spaces   = regexp.MustCompile(`\s+`)
)

// Snippet returns a plain-text preview of the Markdown src: syntax is
// rendered away, whitespace is collapsed and the result is cut to at most
// maxRunes runes, ending in an ellipsis when truncated.
func (r *Renderer) Snippet(src string, maxRunes int) string {
	var buf bytes.Buffer
	if err := r.md.Convert([]byte(src), &buf); err != nil {
		return ""
	}
	s := blockEnd.ReplaceAllString(buf.String(), " ")
	s = html.UnescapeString(anyTag.ReplaceAllString(s, ""))
	s = strings.TrimSpace(spaces.ReplaceAllString(s, " "))
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "…"
}
