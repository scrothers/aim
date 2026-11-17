package mcpserver

import (
	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/presence"
)

type presenceAdapter struct{ r *presence.Registry }

// NewPresence adapts a presence registry to the mail.Presence interface.
func NewPresence(r *presence.Registry) mail.Presence { return presenceAdapter{r: r} }

// Online implements mail.Presence.
func (p presenceAdapter) Online(identity address.Address) []mail.OnlineStatus {
	sts := p.r.Online(identity)
	out := make([]mail.OnlineStatus, 0, len(sts))
	for _, s := range sts {
		out = append(out, mail.OnlineStatus{
			SessionID:  s.SessionID,
			Machine:    s.Machine,
			Away:       s.Away,
			SignedOnAt: s.SignedOnAt,
			Client:     s.Client,
			State:      s.State,
			LastActive: s.LastActive,
		})
	}
	return out
}

// Sessions implements mail.SessionCounter: connected sessions and the number
// of distinct machines they run on.
func (p presenceAdapter) Sessions() (sessions, machines int) {
	all := p.r.All()
	seen := make(map[string]struct{}, len(all))
	for _, s := range all {
		seen[s.Machine] = struct{}{}
	}
	return len(all), len(seen)
}

var _ mail.SessionCounter = presenceAdapter{}
