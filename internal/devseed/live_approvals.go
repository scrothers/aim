package devseed

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/scrothers/aim/internal/harness"
	"github.com/scrothers/aim/internal/wire"
)

const (
	// maxPendingApprovals caps the permission prompts waiting at once.
	maxPendingApprovals = 3
	defaultApprovalWait = 5 * time.Minute
	httpTimeout         = 10 * time.Second
)

// approvalSettings is the body of /api/settings/approvals.
type approvalSettings struct {
	Enabled        bool `json:"enabled"`
	TimeoutSeconds int  `json:"timeout_seconds"`
}

// apiCall sends one request to the daemon's HTTP API with the header its
// guard requires, decoding a JSON reply into out when given.
func apiCall(ctx context.Context, method, url string, body, out any) error {
	var rd *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("devseed: %w", err)
		}
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	ctx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return fmt.Errorf("devseed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-AIM-Request", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("devseed: %s %s: %w", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("devseed: %s %s: %s", method, url, resp.Status)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("devseed: %s %s: decode: %w", method, url, err)
	}
	return nil
}

// setupApprovals turns remote approvals on when asked, or says once that
// approval requests will fall through while they are off.
func (l *liveLoop) setupApprovals(ctx context.Context, cfg LiveConfig) {
	url := "http://" + cfg.HTTP + "/api/settings/approvals"
	if cfg.EnableApprovals {
		wait := cfg.ApprovalTimeout
		if wait <= 0 {
			wait = defaultApprovalWait
		}
		var got approvalSettings
		err := apiCall(ctx, http.MethodPut, url, approvalSettings{Enabled: true, TimeoutSeconds: int(wait.Seconds())}, &got)
		if err != nil {
			say(l.out, "approvals: could not enable: %v\n", err)
			return
		}
		say(l.out, "approvals: enabled, timeout %ds\n", got.TimeoutSeconds)
		return
	}
	if cfg.ApprovalEvery <= 0 {
		return
	}
	var got approvalSettings
	if err := apiCall(ctx, http.MethodGet, url, nil, &got); err != nil {
		say(l.out, "approvals: could not read the setting: %v\n", err)
		return
	}
	if !got.Enabled {
		say(l.out, "approvals: remote approvals are off, so approval requests fall through; pass --enable-approvals\n")
	}
}

// approve starts one simulated permission prompt from a signed-on Claude Code
// agent, unless too many are pending. The prompt waits in its own goroutine,
// as a real hook process does.
func (l *liveLoop) approve(ctx context.Context) {
	var cands []*agent
	l.mu.Lock()
	full := l.pending >= maxPendingApprovals
	for _, a := range l.online() {
		if a.p.client == "claude-code" && !l.asking[a] {
			cands = append(cands, a)
		}
	}
	l.mu.Unlock()
	if full || len(cands) == 0 {
		return
	}
	a := pick(l.rng, cands)
	tool, input := l.approvalCase(a.p)
	raw, err := json.Marshal(input)
	if err != nil {
		say(l.out, "approval: %v\n", err)
		return
	}
	l.mu.Lock()
	if l.asking == nil {
		l.asking = map[*agent]bool{}
	}
	l.asking[a] = true
	l.pending++
	l.mu.Unlock()
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		decision, reason, err := hookApprove(ctx, l.addr, a, tool, raw)
		l.mu.Lock()
		delete(l.asking, a)
		l.pending--
		l.mu.Unlock()
		summary := harness.ToolSummary(tool, raw)
		switch {
		case err != nil && ctx.Err() != nil:
		case err != nil:
			say(l.out, "approval: %s %s: %v\n", a.p, summary, err)
		case decision == "":
			say(l.out, "approval: %s %s -> no decision (timed out or approvals off)\n", a.p, summary)
		case reason != "":
			say(l.out, "approval: %s %s -> %s (%s)\n", a.p, summary, decision, reason)
		default:
			say(l.out, "approval: %s %s -> %s\n", a.p, summary, decision)
		}
	}()
	say(l.out, "approval: %s asks to run %s\n", a.p, harness.ToolSummary(tool, raw))
}

// wait blocks until every approval goroutine has returned.
func (l *liveLoop) wait() { l.wg.Wait() }

// hookApprove opens a hook connection the way `aim hook claude` does for a
// PermissionRequest and waits for the daemon's reply. Closing ctx closes the
// connection, which the daemon treats as the hook going away.
func hookApprove(ctx context.Context, addr string, a *agent, tool string, input json.RawMessage) (decision, reason string, err error) {
	nw, target := network(addr)
	var d net.Dialer
	conn, err := d.DialContext(ctx, nw, target)
	if err != nil {
		return "", "", fmt.Errorf("devseed: dial hook %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	pre := a.preamble()
	pre.Kind, pre.Client = wire.KindHook, "claude-code"
	if err := wire.WritePreamble(conn, pre); err != nil {
		return "", "", fmt.Errorf("devseed: %w", err)
	}
	req := harness.ApprovalRequest(tool, wire.TruncateToolInput(input))
	req.Runtime = "claude"
	if err := wire.WriteLine(conn, req); err != nil {
		return "", "", fmt.Errorf("devseed: %w", err)
	}
	var reply wire.HookReply
	if err := wire.ReadLine(bufio.NewReader(conn), &reply); err != nil {
		return "", "", fmt.Errorf("devseed: hook reply: %w", err)
	}
	return reply.Decision, reply.Reason, nil
}

var langCommands = map[string][]string{
	"go":  {"go test -race ./...", "go build ./cmd/...", "golangci-lint run ./..."},
	"ts":  {"pnpm test --run", "pnpm install", "pnpm exec tsc --noEmit"},
	"tf":  {"terraform plan -out=tfplan", "terraform apply tfplan", "tflint --recursive"},
	"py":  {"pytest -x -q", "pip install -e .", "ruff check ."},
	"kt":  {"./gradlew testDebugUnitTest", "./gradlew assembleRelease"},
	"any": {"git push origin HEAD", "git rebase origin/main", "rm -rf build dist", "docker compose up -d --build"},
}

// approvalCase builds a realistic tool call for p's repo: a shell command, an
// edit, a file write or a web fetch.
func (l *liveLoop) approvalCase(p *persona) (tool string, input map[string]any) {
	scope := p.scope()
	fl, ok := flavors()[scope]
	if !ok {
		scope = acme + "/api"
		fl = flavors()[scope]
	}
	file := "/home/dev/src/" + strings.TrimPrefix(scope, p.addr.Host+"/") + "/" + pick(l.rng, fl.files)
	switch r := l.rng.IntN(20); {
	case r < 9:
		cmds := append(append([]string{}, langCommands[fl.lang]...), langCommands["any"]...)
		cmd := pick(l.rng, cmds)
		return "Bash", map[string]any{"command": cmd, "description": "Run " + cmd}
	case r < 14:
		return "Edit", map[string]any{"file_path": file, "old_string": "TODO(" + p.role + ")", "new_string": "// handled in #" + fmt.Sprint(700+l.rng.IntN(300))}
	case r < 16:
		return "Write", map[string]any{"file_path": file + ".bak", "content": "// scratch copy before the refactor\n"}
	default:
		urls := []string{
			"https://pkg.go.dev/net/http#Server", "https://developer.mozilla.org/en-US/docs/Web/API/Intersection_Observer_API",
			"https://docs.aws.amazon.com/eks/latest/userguide/update-managed-node-group.html", "https://" + scope + "/pulls",
		}
		return "WebFetch", map[string]any{"url": pick(l.rng, urls), "prompt": "Summarize the part relevant to " + pick(l.rng, fl.titles)}
	}
}
