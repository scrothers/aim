//go:build e2e

package hooks_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

type approvalItem struct {
	ID      string `json:"id"`
	Tool    string `json:"tool"`
	Summary string `json:"summary"`
	Client  string `json:"client"`
}

func (d *daemon) setApprovals(ctx context.Context, t *testing.T, enabled bool, timeout int) {
	t.Helper()
	body := map[string]any{"enabled": enabled, "timeout_seconds": timeout}
	if code := d.api(ctx, http.MethodPut, "/api/settings/approvals", body, nil); code != http.StatusOK {
		t.Fatalf("set approvals: status %d", code)
	}
}

func permissionPayload(cwd string) string {
	return fmt.Sprintf(`{"session_id":"sess-appr","cwd":%q,"hook_event_name":"PermissionRequest","permission_mode":"default",`+
		`"tool_name":"Bash","tool_input":{"command":"git push origin main"}}`, cwd)
}

// TestApprovalAllowFromConsole runs a Claude PermissionRequest hook with
// remote approvals on, allows it over HTTP, and checks the hook prints
// Claude's allow decision.
func TestApprovalAllowFromConsole(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	d := startDaemon(t)
	d.setApprovals(ctx, t, true, 60)

	cmd, stdout, stderr := hookCmd(ctx, d, permissionPayload(t.TempDir()))
	if err := cmd.Start(); err != nil {
		t.Fatalf("start hook: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var item approvalItem
	for deadline := time.Now().Add(10 * time.Second); ; {
		var list struct {
			Items []approvalItem `json:"items"`
		}
		if d.api(ctx, http.MethodGet, "/api/approvals", nil, &list) == http.StatusOK && len(list.Items) == 1 {
			item = list.Items[0]
			break
		}
		select {
		case err := <-done:
			t.Fatalf("hook returned before a decision: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("approval never appeared")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if item.Tool != "Bash" || item.Summary != "Bash: git push origin main" || item.Client != "claude-code" {
		t.Fatalf("approval = %+v", item)
	}
	if code := d.api(ctx, http.MethodPost, "/api/approvals/"+item.ID, map[string]any{"decision": "allow"}, nil); code != http.StatusOK {
		t.Fatalf("decide: status %d", code)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("hook: %v\nstderr: %s", err, stderr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("hook did not return after the decision")
	}
	var out struct {
		Out struct {
			Event    string `json:"hookEventName"`
			Decision struct {
				Behavior string `json:"behavior"`
			} `json:"decision"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("hook stdout is not JSON: %q: %v", stdout, err)
	}
	if out.Out.Event != "PermissionRequest" || out.Out.Decision.Behavior != "allow" {
		t.Fatalf("hook stdout = %s", stdout)
	}
}

// TestApprovalFallsThrough checks the hook prints nothing, so Claude shows
// its own dialog, when approvals are off (at once) or time out.
func TestApprovalFallsThrough(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	d := startDaemon(t)

	start := time.Now()
	if out := runHook(ctx, t, d, permissionPayload(t.TempDir())); out != "" {
		t.Fatalf("disabled hook printed %q", out)
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Fatalf("disabled hook took %s", took)
	}

	d.setApprovals(ctx, t, true, 10)
	start = time.Now()
	if out := runHook(ctx, t, d, permissionPayload(t.TempDir())); out != "" {
		t.Fatalf("timed-out hook printed %q", out)
	}
	if took := time.Since(start); took < 9*time.Second || took > 20*time.Second {
		t.Fatalf("timed-out hook took %s, want about 10s", took)
	}
	var list struct {
		Items []approvalItem `json:"items"`
	}
	if d.api(ctx, http.MethodGet, "/api/approvals", nil, &list) != http.StatusOK || len(list.Items) != 0 {
		t.Fatalf("pending after timeout = %+v", list.Items)
	}
}
