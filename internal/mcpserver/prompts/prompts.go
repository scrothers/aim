package prompts

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/resources"
	"github.com/scrothers/aim/internal/mcpserver/session"
)

// behavior is the Go side of a workflow prompt, declared in <name>.go beside
// its markdown/<name>.md script. Both hooks are optional.
type behavior struct {
	// defaults fills in arguments the caller left empty, or rejects the call.
	defaults func(c *session.Conn, args map[string]string) error
	// attach returns the data the workflow is about, as extra user messages.
	// None of them may mark mail read.
	attach func(ctx context.Context, c *session.Conn, args map[string]string, now time.Time) ([]*mcp.PromptMessage, error)
}

// behaviors maps each workflow name to its behavior. Every markdown/<name>.md
// must have an entry, and every entry a file; tests enforce both.
var behaviors = map[string]behavior{
	"check-inbox": checkInbox,
	"escalate":    escalate,
	"handoff":     handoff,
	"reply":       reply,
	"standup":     standup,
}

// promptData is what workflow templates are rendered with.
type promptData struct {
	// Args holds the prompt arguments; missing ones are the empty string.
	Args      map[string]string
	SignedOn  bool
	Identity  string
	Namespace string
	// Today is the current UTC date, YYYY-MM-DD.
	Today string
}

// Register adds one MCP prompt per embedded workflow to srv, acting for c.
func Register(srv *mcp.Server, c *session.Conn) {
	for _, w := range workflows() {
		p := &mcp.Prompt{Name: w.name, Title: w.title, Description: w.description}
		for _, a := range w.args {
			p.Arguments = append(p.Arguments, &mcp.PromptArgument{Name: a.name, Description: a.description, Required: a.required})
		}
		srv.AddPrompt(p, handler(c, w, behaviors[w.name]))
	}
}

func handler(c *session.Conn, w workflow, b behavior) mcp.PromptHandler {
	return func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		args := make(map[string]string, len(w.args))
		for _, a := range w.args {
			v := strings.TrimSpace(req.Params.Arguments[a.name])
			if v == "" && a.required {
				return nil, fmt.Errorf("mcpserver: prompt %s: %w: argument %q is required", w.name, mail.ErrInvalidInput, a.name)
			}
			args[a.name] = v
		}
		if b.defaults != nil {
			if err := b.defaults(c, args); err != nil {
				return nil, err
			}
		}
		now := time.Now().UTC()
		data := promptData{Args: args, Namespace: c.Entry.Preamble().Namespace, Today: now.Format(time.DateOnly)}
		if id, ok := c.Entry.Identity(); ok {
			data.SignedOn, data.Identity = true, id.String()
		}
		var script bytes.Buffer
		if err := w.body.Execute(&script, data); err != nil {
			return nil, fmt.Errorf("mcpserver: prompt %s: %w", w.name, err)
		}
		var extra []*mcp.PromptMessage
		if b.attach != nil {
			var err error
			if extra, err = b.attach(ctx, c, args, now); err != nil {
				return nil, session.ToolErr(err)
			}
		}
		first := &mcp.PromptMessage{Role: "user", Content: &mcp.TextContent{Text: script.String()}}
		return &mcp.GetPromptResult{Description: w.description, Messages: append([]*mcp.PromptMessage{first}, extra...)}, nil
	}
}

// threadAttachment embeds a thread's transcript as its resource.
func threadAttachment(ctx context.Context, c *session.Conn, threadID string) ([]*mcp.PromptMessage, error) {
	text, err := resources.Thread(ctx, c, threadID)
	if err != nil {
		return nil, err
	}
	return []*mcp.PromptMessage{embedded(resources.ThreadURI(threadID), text)}, nil
}

func embedded(uri, text string) *mcp.PromptMessage {
	return &mcp.PromptMessage{Role: "user", Content: &mcp.EmbeddedResource{
		Resource: &mcp.ResourceContents{URI: uri, MIMEType: resources.MIMEMarkdown, Text: text},
	}}
}
