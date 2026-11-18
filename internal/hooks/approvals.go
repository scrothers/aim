package hooks

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/scrothers/aim/internal/wire"
)

const (
	approvalPrefix = "appr-"
	// reasonLimit caps the operator's note relayed to the agent.
	reasonLimit = 500
)

// ErrApprovalNotFound is returned by Decide for an approval that is not
// pending: unknown, already decided, timed out or abandoned by its hook.
var ErrApprovalNotFound = errors.New("approval not found")

// ErrInvalidDecision is returned by Decide for a decision other than
// wire.DecisionAllow or wire.DecisionDeny.
var ErrInvalidDecision = errors.New("invalid decision")

// ApprovalMode is the operator's remote approval setting.
type ApprovalMode struct {
	// Enabled makes approve requests wait for the operator.
	Enabled bool
	// Timeout is how long a request waits before it falls back to the
	// agent's terminal prompt.
	Timeout time.Duration
}

// ApprovalModeFunc reports the current approval mode. It is called once per
// approve request, so a changed setting applies to the next prompt.
type ApprovalModeFunc func(ctx context.Context) (ApprovalMode, error)

// WithApprovalMode sets where the server reads the approval mode. Without
// it, remote approvals are disabled.
func WithApprovalMode(f ApprovalModeFunc) Option { return func(s *Server) { s.approvalMode = f } }

// Approval is a permission prompt waiting for the operator.
type Approval struct {
	ID string `json:"id" doc:"Approval id (appr-<ulid>)."`
	// Identity is the agent's signed-on address, or empty when the session
	// is not signed on.
	Identity   string          `json:"identity,omitempty" doc:"Agent identity, when signed on."`
	SessionID  string          `json:"session_id,omitempty" doc:"Runtime session id."`
	Machine    string          `json:"machine" doc:"Machine the agent runs on."`
	Client     string          `json:"client,omitempty" doc:"Agent runtime, such as claude-code."`
	ProjectDir string          `json:"project_dir,omitempty" doc:"Agent working directory."`
	Tool       string          `json:"tool" doc:"Tool the agent asks to run."`
	Summary    string          `json:"summary,omitempty" doc:"One-line summary of the tool call."`
	Input      json.RawMessage `json:"input,omitempty" doc:"Tool arguments as JSON; inputs over 16 KiB become {truncated, bytes, preview}."`
	CreatedAt  time.Time       `json:"created_at" doc:"When the prompt arrived."`
	ExpiresAt  time.Time       `json:"expires_at" doc:"When it falls back to the agent's terminal."`
}

type decision struct {
	verdict, reason string
}

type pendingApproval struct {
	Approval
	// decided receives the operator's decision; it has room for one so
	// Decide never blocks.
	decided chan decision
}

// Approve answers an approve request. Disabled, or on any error reading the
// mode, it replies at once with no decision. Enabled, it registers the
// prompt for the operator, marks the agent blocked, and waits for a
// decision, the timeout, ctx (the hook disconnecting) or Close; only a
// decision carries one.
func (s *Server) Approve(ctx context.Context, p wire.Preamble, req wire.HookRequest) (wire.HookReply, error) {
	mode := ApprovalMode{}
	if s.approvalMode != nil {
		m, err := s.approvalMode(ctx)
		if err == nil {
			mode = m
		}
	}
	id, ok := s.identity(ctx, p)
	reply := wire.HookReply{SignedOn: ok}
	if ok {
		reply.Identity = id.String()
	}
	if !mode.Enabled || mode.Timeout <= 0 {
		return reply, nil
	}

	summary := truncate(req.Detail, detailLimit)
	if summary == "" {
		summary = req.ToolName
	}
	now := s.now()
	pa := &pendingApproval{
		Approval: Approval{
			ID:         approvalPrefix + strings.ToLower(ulid.Make().String()),
			Identity:   reply.Identity,
			SessionID:  p.SessionID,
			Machine:    p.Machine,
			Client:     p.Client,
			ProjectDir: p.ProjectDir,
			Tool:       req.ToolName,
			Summary:    summary,
			Input:      wire.TruncateToolInput(req.ToolInput),
			CreatedAt:  now,
			ExpiresAt:  now.Add(mode.Timeout),
		},
		decided: make(chan decision, 1),
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return reply, nil
	}
	s.approvals[pa.ID] = pa
	s.mu.Unlock()
	defer s.dropApproval(pa.ID)

	detail := "Approval pending: " + summary
	s.setAttention(ctx, p, wire.HookRequest{Attention: wire.AttentionBlocked, Detail: detail}, id, ok)

	timer := time.NewTimer(mode.Timeout)
	defer timer.Stop()
	select {
	case d := <-pa.decided:
		reply.Decision, reply.Reason = d.verdict, d.reason
		// The operator answered; the agent moves on without its terminal.
		s.setAttention(ctx, p, wire.HookRequest{}, id, ok)
	case <-timer.C:
		// The terminal prompt takes over and the agent stays blocked on it.
	case <-ctx.Done():
	case <-s.done:
	}
	return reply, nil
}

func (s *Server) dropApproval(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.approvals, id)
}

// Approvals returns the pending approvals, oldest first.
func (s *Server) Approvals() []Approval {
	s.mu.Lock()
	out := make([]Approval, 0, len(s.approvals))
	for _, pa := range s.approvals {
		out = append(out, pa.Approval)
	}
	s.mu.Unlock()
	slices.SortFunc(out, func(a, b Approval) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), strings.Compare(a.ID, b.ID))
	})
	return out
}

// Decide answers the pending approval id with verdict (wire.DecisionAllow or
// wire.DecisionDeny) and an optional reason for the agent. The approval
// stops being pending at once, so a second decision is ErrApprovalNotFound.
func (s *Server) Decide(id, verdict, reason string) error {
	if verdict != wire.DecisionAllow && verdict != wire.DecisionDeny {
		return fmt.Errorf("hooks: decide %s: %w: %q", id, ErrInvalidDecision, verdict)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pa, ok := s.approvals[id]
	if !ok {
		return fmt.Errorf("hooks: decide %s: %w", id, ErrApprovalNotFound)
	}
	delete(s.approvals, id)
	pa.decided <- decision{verdict: verdict, reason: truncate(reason, reasonLimit)}
	return nil
}
