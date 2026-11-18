package harness

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/scrothers/aim/internal/wire"
)

const (
	dialTimeout  = time.Second
	eventTimeout = 5 * time.Second
)

// Deps are the host services RunHook uses, injected for tests.
type Deps struct {
	Getenv func(string) string
	// Dial connects to the daemon.
	Dial func(ctx context.Context) (net.Conn, error)
	// Hostname returns the machine name.
	Hostname func() (string, error)
	// Getwd returns the working directory.
	Getwd func() (string, error)
	// AgentPID returns the agent runtime's pid, or 0.
	AgentPID func() int
	// Git returns the origin remote URL of the repository containing dir.
	Git func(ctx context.Context, dir string) (string, error)
}

// RunHook runs one hook invocation: parse the payload, exchange one request
// with the daemon, render the reply. It fails open: any error yields an empty
// exit-0 Output, with a single stderr line when AIM_HOOK_DEBUG=1.
func RunHook(ctx context.Context, h Hook, event string, stdin []byte, d Deps) Output {
	out, err := runHook(ctx, h, event, stdin, d)
	if err != nil {
		if d.Getenv != nil && d.Getenv("AIM_HOOK_DEBUG") == "1" {
			return Output{Stderr: []byte(fmt.Sprintf("aim hook %s: %v\n", h.Name(), err))}
		}
		return Output{}
	}
	return out
}

func runHook(ctx context.Context, h Hook, event string, stdin []byte, d Deps) (Output, error) {
	p, ok, err := h.Parse(event, stdin, d.Getenv)
	if err != nil {
		return Output{}, fmt.Errorf("parse: %w", err)
	}
	if !ok {
		return Output{}, nil
	}
	pre := d.preamble(ctx, h, p)
	req := p.Request
	req.Runtime = h.Name()
	req.ToolInput = wire.TruncateToolInput(req.ToolInput)

	dctx, cancel := context.WithTimeout(ctx, dialTimeout)
	conn, err := d.Dial(dctx)
	cancel()
	if err != nil {
		return Output{}, fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = conn.Close() }()
	switch req.Op {
	case wire.HookOpWatch, wire.HookOpApprove:
		// Both block on the daemon for as long as it takes (the daemon bounds
		// an approval by its timeout; the runtime bounds the hook process).
		// Closing the connection is how a cancelled wait stops blocking.
		stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
		defer stop()
	default:
		if err := conn.SetDeadline(time.Now().Add(eventTimeout)); err != nil {
			return Output{}, fmt.Errorf("deadline: %w", err)
		}
	}
	if err := wire.WritePreamble(conn, pre); err != nil {
		return Output{}, fmt.Errorf("preamble: %w", err)
	}
	if err := wire.WriteLine(conn, req); err != nil {
		return Output{}, fmt.Errorf("request: %w", err)
	}
	var reply wire.HookReply
	if err := wire.ReadLine(bufio.NewReader(conn), &reply); err != nil {
		return Output{}, fmt.Errorf("reply: %w", err)
	}
	return h.Render(req, reply), nil
}

func (d Deps) preamble(ctx context.Context, h Hook, p Parsed) wire.Preamble {
	pre := wire.Preamble{
		V:          wire.Version,
		SessionID:  p.SessionID,
		ProjectDir: p.Cwd,
		Kind:       wire.KindHook,
		Client:     h.Client(),
	}
	if pre.ProjectDir == "" && d.Getwd != nil {
		if wd, err := d.Getwd(); err == nil {
			pre.ProjectDir = wd
		}
	}
	if d.Hostname != nil {
		if host, err := d.Hostname(); err == nil {
			pre.Machine = host
		}
	}
	if d.AgentPID != nil {
		pre.AgentPID = d.AgentPID()
	}
	if d.Git != nil && pre.ProjectDir != "" {
		gctx, cancel := context.WithTimeout(ctx, dialTimeout)
		defer cancel()
		if remote, err := d.Git(gctx, pre.ProjectDir); err == nil && remote != "" {
			pre.GitRemote = remote
			if ns, ok := wire.NamespaceFromRemote(remote); ok {
				pre.Namespace = ns
			}
		}
	}
	return pre
}

// EnvDeps returns Deps backed by the real process environment; dial connects
// to the daemon address.
func EnvDeps(dial func(ctx context.Context) (net.Conn, error), agentPID func() int, git func(ctx context.Context, dir string) (string, error)) Deps {
	return Deps{Getenv: os.Getenv, Dial: dial, Hostname: os.Hostname, Getwd: os.Getwd, AgentPID: agentPID, Git: git}
}

// AdditionalContext renders the hookSpecificOutput JSON object shared by
// Claude Code, Codex, Gemini CLI and Grok Build; empty context renders
// nothing.
func AdditionalContext(event, text string) []byte {
	if text == "" {
		return nil
	}
	b, err := json.Marshal(map[string]any{"hookSpecificOutput": map[string]string{
		"hookEventName":     event,
		"additionalContext": text,
	}})
	if err != nil {
		return nil
	}
	return append(b, '\n')
}
