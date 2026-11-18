package mcpserver

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"text/template"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mdview"
)

// workflowDir holds one <prompt name>.md workflow script per user-invocable
// MCP prompt. Unlike tool descriptions these load only when invoked, so they
// are not subject to the 2 KB client cap.
const workflowDir = "prompts/workflows"

// standupLimit caps how much mail a standup digest embeds.
const standupLimit = mail.MaxListLimit

// workflowArg describes one prompt argument from a workflow's front matter.
type workflowArg struct {
	name, description string
	required          bool
}

// workflow is a parsed prompts/workflows/<name>.md file.
type workflow struct {
	name, title, description string
	args                     []workflowArg
	body                     *template.Template
}

// parseWorkflow parses a workflow file: a front matter block between two
// "---" lines followed by a text/template body. The front matter is a tiny
// YAML subset:
//
//	title: Text
//	description: Text
//	arguments:
//	  - name: arg
//	    required: true
//	    description: Text
func parseWorkflow(name, src string) (workflow, error) {
	w := workflow{name: name}
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return w, fmt.Errorf("mcpserver: workflow %s: missing front matter", name)
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return w, fmt.Errorf("mcpserver: workflow %s: unterminated front matter", name)
	}
	if err := w.parseFrontMatter(lines[1:end]); err != nil {
		return w, err
	}
	if w.title == "" || w.description == "" {
		return w, fmt.Errorf("mcpserver: workflow %s: title and description are required", name)
	}
	body := strings.TrimSpace(strings.Join(lines[end+1:], "\n"))
	t, err := template.New(name).Option("missingkey=zero").Parse(body)
	if err != nil {
		return w, fmt.Errorf("mcpserver: workflow %s: %w", name, err)
	}
	w.body = t
	return w, nil
}

func (w *workflow) parseFrontMatter(lines []string) error {
	inArgs := false
	for _, raw := range lines {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		bad := func(why string) error {
			return fmt.Errorf("mcpserver: workflow %s: front matter line %q: %s", w.name, raw, why)
		}
		if !strings.HasPrefix(raw, " ") {
			inArgs = false
			k, v, ok := strings.Cut(raw, ":")
			if !ok {
				return bad("want key: value")
			}
			v = strings.TrimSpace(v)
			switch k {
			case "title":
				w.title = v
			case "description":
				w.description = v
			case "arguments":
				inArgs = true
			default:
				return bad("unknown key")
			}
			continue
		}
		if !inArgs {
			return bad("indented line outside arguments")
		}
		item := strings.TrimSpace(raw)
		if rest, isNew := strings.CutPrefix(item, "- "); isNew {
			w.args = append(w.args, workflowArg{})
			item = rest
		}
		if len(w.args) == 0 {
			return bad("argument attribute before '- name:'")
		}
		k, v, ok := strings.Cut(item, ":")
		if !ok {
			return bad("want key: value")
		}
		v = strings.TrimSpace(v)
		a := &w.args[len(w.args)-1]
		switch k {
		case "name":
			a.name = v
		case "description":
			a.description = v
		case "required":
			switch v {
			case "true":
				a.required = true
			case "false":
				a.required = false
			default:
				return bad("required must be true or false")
			}
		default:
			return bad("unknown argument key")
		}
	}
	for _, a := range w.args {
		if a.name == "" {
			return fmt.Errorf("mcpserver: workflow %s: argument without a name", w.name)
		}
	}
	return nil
}

// workflows parses every embedded workflow. The files are compiled in and
// covered by tests, so a malformed one is a programming error.
func workflows() []workflow {
	entries, err := fs.ReadDir(promptFS, workflowDir)
	if err != nil {
		panic(fmt.Sprintf("mcpserver: read %s: %v", workflowDir, err))
	}
	var out []workflow
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".md")
		if e.IsDir() || !ok {
			continue
		}
		w, err := parseWorkflow(name, prompt(path.Join(workflowDir, e.Name())))
		if err != nil {
			panic(err.Error())
		}
		out = append(out, w)
	}
	return out
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

// registerPrompts adds one MCP prompt per embedded workflow.
func (h *handlers) registerPrompts(srv *mcp.Server) {
	for _, w := range workflows() {
		p := &mcp.Prompt{Name: w.name, Title: w.title, Description: w.description}
		for _, a := range w.args {
			p.Arguments = append(p.Arguments, &mcp.PromptArgument{Name: a.name, Description: a.description, Required: a.required})
		}
		srv.AddPrompt(p, h.workflowHandler(w))
	}
}

func (h *handlers) workflowHandler(w workflow) mcp.PromptHandler {
	return func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		args := make(map[string]string, len(w.args))
		for _, a := range w.args {
			v := strings.TrimSpace(req.Params.Arguments[a.name])
			if v == "" && a.required {
				return nil, fmt.Errorf("mcpserver: prompt %s: %w: argument %q is required", w.name, mail.ErrInvalidInput, a.name)
			}
			args[a.name] = v
		}
		now := time.Now().UTC()
		if w.name == "standup" && args["address"] == "" {
			args["address"] = h.conn.Preamble().Namespace
			if args["address"] == "" {
				return nil, fmt.Errorf("mcpserver: prompt standup: %w: no address given and this session has no git namespace", mail.ErrInvalidInput)
			}
		}
		data := promptData{Args: args, Namespace: h.conn.Preamble().Namespace, Today: now.Format(time.DateOnly)}
		if id, ok := h.conn.Identity(); ok {
			data.SignedOn, data.Identity = true, id.String()
		}
		var script bytes.Buffer
		if err := w.body.Execute(&script, data); err != nil {
			return nil, fmt.Errorf("mcpserver: prompt %s: %w", w.name, err)
		}
		extra, err := h.attachments(ctx, w.name, args, now)
		if err != nil {
			return nil, toolErr(err)
		}
		first := &mcp.PromptMessage{Role: "user", Content: &mcp.TextContent{Text: script.String()}}
		return &mcp.GetPromptResult{Description: w.description, Messages: append([]*mcp.PromptMessage{first}, extra...)}, nil
	}
}

// attachments returns the data a workflow is about, as extra user messages.
// None of them marks mail read.
func (h *handlers) attachments(ctx context.Context, name string, args map[string]string, now time.Time) ([]*mcp.PromptMessage, error) {
	switch name {
	case "check-inbox":
		text, err := h.inboxMarkdown(ctx)
		if err != nil {
			return nil, err
		}
		return []*mcp.PromptMessage{embedded(uriInbox, text)}, nil
	case "reply":
		m, err := h.svc.Read(ctx, h.caller(), mail.ReadInput{MessageID: args["message_id"], Peek: true})
		if err != nil {
			return nil, err
		}
		return h.threadAttachment(ctx, m.ThreadID)
	case "escalate":
		if args["thread_id"] == "" {
			return nil, nil
		}
		return h.threadAttachment(ctx, args["thread_id"])
	case "standup":
		since := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		res, err := h.svc.List(ctx, mail.ListInput{Address: args["address"], State: mail.StateInbox, Since: &since, Limit: standupLimit})
		if err != nil {
			return nil, err
		}
		text := mdview.Digest("Mail since "+since.Format(time.DateOnly)+" under "+args["address"], res.Items, res.NextCursor != "")
		return []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: text}}}, nil
	}
	return nil, nil
}

func (h *handlers) threadAttachment(ctx context.Context, threadID string) ([]*mcp.PromptMessage, error) {
	text, err := h.threadMarkdown(ctx, threadID)
	if err != nil {
		return nil, err
	}
	return []*mcp.PromptMessage{embedded(prefixThread+encodeSegment(threadID), text)}, nil
}

func embedded(uri, text string) *mcp.PromptMessage {
	return &mcp.PromptMessage{Role: "user", Content: &mcp.EmbeddedResource{
		Resource: &mcp.ResourceContents{URI: uri, MIMEType: markdownMIME, Text: text},
	}}
}
