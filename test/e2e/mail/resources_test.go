//go:build e2e

package mail_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestResourcesAndPrompts reads an aim:// resource and fetches a workflow
// prompt through real shim and daemon processes.
func TestResourcesAndPrompts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()

	d := startDaemon(t)
	a := startShim(ctx, t, "sess-ra", d.socket)
	b := startShim(ctx, t, "sess-rb", d.tcp)

	const (
		alice = "alice@github.com/acme/tool"
		bob   = "bob@github.com/acme/tool"
	)
	a.call(ctx, "agent_register", map[string]any{"address": alice, "role": "lead", "description": "d"}, nil)
	a.call(ctx, "agent_register", map[string]any{"address": bob, "role": "dev", "description": "d"}, nil)
	a.call(ctx, "agent_sign_on", map[string]any{"address": alice}, nil)
	b.call(ctx, "agent_sign_on", map[string]any{"address": bob}, nil)
	var sent message
	a.call(ctx, "mail_send", map[string]any{"to": []string{bob}, "subject": "resource check", "body": "the quokka report"}, &sent)

	res, err := b.cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "aim://inbox"})
	if err != nil {
		t.Fatalf("read aim://inbox: %v", err)
	}
	if len(res.Contents) != 1 || res.Contents[0].MIMEType != "text/markdown" ||
		!strings.Contains(res.Contents[0].Text, "resource check") || !strings.Contains(res.Contents[0].Text, sent.ID) {
		t.Fatalf("aim://inbox = %+v", res.Contents)
	}

	p, err := b.cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "check-inbox"})
	if err != nil {
		t.Fatalf("get check-inbox: %v", err)
	}
	if len(p.Messages) != 2 {
		t.Fatalf("check-inbox messages = %d, want script plus inbox", len(p.Messages))
	}
	script, ok := p.Messages[0].Content.(*mcp.TextContent)
	if !ok || !strings.Contains(script.Text, "signed on as `"+bob+"`") {
		t.Fatalf("check-inbox script = %+v", p.Messages[0].Content)
	}

	// Neither read marked the message read.
	var unread listOut
	b.call(ctx, "mail_list", map[string]any{"address": bob}, &unread)
	if len(unread.Items) != 1 {
		t.Fatalf("resource or prompt marked mail read: %+v", unread.Items)
	}
}
