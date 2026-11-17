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
		})
	}
	return out
}
