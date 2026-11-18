package hooks

import (
	"context"
	"log/slog"
	"strings"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/mdview"
	"github.com/scrothers/aim/internal/wire"
)

// touchTasks records the hook as lease activity for the agent's session.
// It is best-effort: a failure is logged at debug and never fails the hook.
func (s *Server) touchTasks(ctx context.Context, p wire.Preamble) {
	if s.tasks == nil || p.SessionID == "" {
		return
	}
	if err := s.tasks.Touch(ctx, p.SessionID); err != nil {
		s.log.LogAttrs(ctx, slog.LevelDebug, "task touch failed", slog.String("session", p.SessionID), slog.String("error", err.Error()))
	}
}

// taskNudge is the one-line open-work reminder for id, or empty when it
// holds and is assigned nothing.
func (s *Server) taskNudge(ctx context.Context, id address.Address) string {
	if s.tasks == nil {
		return ""
	}
	c, err := s.tasks.Counts(ctx, id.IdentityAddress().String())
	if err != nil {
		s.log.LogAttrs(ctx, slog.LevelDebug, "task counts failed", slog.String("identity", id.String()), slog.String("error", err.Error()))
		return ""
	}
	return mdview.TaskNudge(c)
}

// joinContext appends extra to a hook context, separated by a blank line.
func joinContext(ctx, extra string) string {
	switch {
	case extra == "":
		return ctx
	case ctx == "":
		return extra
	default:
		return strings.TrimRight(ctx, "\n") + "\n\n" + extra
	}
}
