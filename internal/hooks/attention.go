package hooks

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mdview"
	"github.com/scrothers/aim/internal/presence"
	"github.com/scrothers/aim/internal/wire"
)

// detailLimit caps the runtime message copied into presence and alerts.
const detailLimit = 500

// setAttention records req's attention state on the agent's live connection
// (an empty state clears it) and schedules or cancels the operator alert.
// Agents without a live connection have no presence to mark.
func (s *Server) setAttention(ctx context.Context, p wire.Preamble, req wire.HookRequest, id address.Address, signedOn bool) {
	e, ok := s.reg.Agent(p.Machine, p.SessionID, p.AgentPID)
	if !ok {
		return
	}
	state := req.Attention
	since := e.SetAttention(state, truncate(req.Detail, detailLimit), s.now())
	key := agentKey(p)
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	a := s.stateLocked(key)
	if !signedOn || s.alertAfter <= 0 || (state != wire.AttentionBlocked && state != wire.AttentionFailed) {
		a.stopAlert()
		return
	}
	if a.alertState == state && a.alert != nil {
		return // already scheduled, or sent, for this spell
	}
	a.stopAlert()
	a.alertState = state
	actx := context.WithoutCancel(ctx)
	a.alert = time.AfterFunc(s.alertAfter, func() { s.sendAlert(actx, p, e, id, state, since) })
}

// sendAlert tells the operator the agent is still in state, unless it has
// moved on or disconnected since. It sends from the agent's identity so a
// reply reaches the agent.
func (s *Server) sendAlert(ctx context.Context, p wire.Preamble, e *presence.Entry, id address.Address, state string, since time.Time) {
	if cur, ok := s.reg.Agent(p.Machine, p.SessionID, p.AgentPID); !ok || cur != e {
		return
	}
	st := e.Status()
	if st.Attention != state || !st.AttentionSince.Equal(since) {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	c := mail.Caller{SessionID: st.SessionID, Identity: id, SignedOn: true, Client: st.Client, Machine: st.Machine}
	subject, body := alertText(id, st, s.now())
	if _, err := s.mail.Send(ctx, c, mail.SendInput{To: []string{address.OperatorName}, Subject: subject, Body: body}); err != nil {
		s.log.LogAttrs(ctx, slog.LevelWarn, "hooks: operator alert failed",
			slog.String("identity", id.String()), slog.String("attention", state), slog.Any("error", err))
	}
}

// alertText renders the operator alert for an agent's attention state.
func alertText(id address.Address, st presence.Status, now time.Time) (subject, body string) {
	what := "is blocked"
	lead := "is waiting on you (a permission prompt or a question)"
	if st.Attention == wire.AttentionFailed {
		what, lead = "stopped on an error", "ended its turn on an error and is not working"
	}
	subject = fmt.Sprintf("%s %s", id, what)
	var b strings.Builder
	fmt.Fprintf(&b, "`%s` %s, for %s.\n", id, lead, now.Sub(st.AttentionSince).Round(time.Second))
	if st.AttentionDetail != "" {
		fmt.Fprintf(&b, "\n> %s\n", strings.ReplaceAll(st.AttentionDetail, "\n", "\n> "))
	}
	fmt.Fprintf(&b, "\n- Machine: %s\n", st.Machine)
	if st.ProjectDir != "" {
		fmt.Fprintf(&b, "- Project: `%s`\n", st.ProjectDir)
	}
	if st.Client != "" {
		fmt.Fprintf(&b, "- Client: %s\n", st.Client)
	}
	fmt.Fprintf(&b, "- Since: %s\n", mdview.Stamp(st.AttentionSince))
	b.WriteString("\nThe agent's terminal needs you; a reply here reaches the agent only once it is working again.\n")
	return subject, b.String()
}

// truncate shortens s to at most n bytes on a rune boundary, marking the cut.
func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8Start(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }
