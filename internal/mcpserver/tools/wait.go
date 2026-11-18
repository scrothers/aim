package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
)

// WaitInput is the input of wait.
type WaitInput struct {
	Address        string `json:"address,omitempty" jsonschema:"Mailbox to watch, covering everything below it, for example github.com/acme/api. Defaults to your signed-on identity and its subagents; required when not signed on. A misspelled address just times out."`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"How long to block, in seconds; default 60, maximum 300 (larger values are clamped). Keep it below your client's tool-call timeout."`
	SinceID        string `json:"since_id,omitempty" jsonschema:"Only wake for unread mail with an id newer than this msg-... id, such as the id your last mail_send returned. Without it, any unread mail in scope returns at once."`
}

func (t *toolset) wait(ctx context.Context, req *mcp.CallToolRequest, in WaitInput) (*mcp.CallToolResult, mail.WaitResult, error) {
	if in.TimeoutSeconds < 0 {
		return nil, mail.WaitResult{Items: []mail.Header{}}, fmt.Errorf("mcpserver: mail_wait: %w: timeout_seconds must not be negative", mail.ErrInvalidInput)
	}
	if token := req.Params.GetProgressToken(); token != nil {
		var stop context.CancelFunc
		ctx, stop = context.WithCancel(ctx)
		defer stop()
		go t.reportProgress(ctx, req.Session, token)
	}
	res, err := t.c.Svc.Wait(ctx, t.c.Caller(), mail.WaitInput{
		Address: in.Address, SinceID: in.SinceID, Timeout: time.Duration(in.TimeoutSeconds) * time.Second,
	})
	if res.Items == nil {
		res.Items = []mail.Header{}
	}
	return nil, res, session.ToolErr(err)
}

// reportProgress sends a progress notification every progressInterval until
// ctx ends, which keeps long calls alive on clients that reset their timeout
// on progress.
func (t *toolset) reportProgress(ctx context.Context, ss *mcp.ServerSession, token any) {
	tick := time.NewTicker(t.progressInterval)
	defer tick.Stop()
	start := time.Now()
	for n := 1; ; n++ {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			_ = ss.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
				ProgressToken: token,
				Progress:      float64(n),
				Message:       fmt.Sprintf("waiting for mail (%s)", time.Since(start).Round(time.Second)),
			})
		}
	}
}
