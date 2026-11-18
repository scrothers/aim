package httpapi

import (
	"cmp"
	"slices"
	"strings"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/presence"
	"github.com/scrothers/aim/internal/wire"
)

const (
	offlineGroup   = "Offline"
	presenceEvents = 50
)

// buildPresence groups the active roster into buddy groups by namespace.
// Identities with signed-on sessions appear once per session in their
// namespace's group; identities with none are listed in the offline group.
func buildPresence(roster []mail.RosterEntry, reg *presence.Registry) PresenceBody {
	versions := map[string]string{}
	events := []PresenceEvent{}
	if reg != nil {
		for _, s := range reg.All() {
			versions[s.SessionID] = s.ClientVersion
		}
		for _, e := range reg.Events(presenceEvents) {
			events = append(events, PresenceEvent{Kind: e.Kind, Identity: e.Identity, Machine: e.Machine, Client: e.Client, At: e.At, Attention: e.Attention})
		}
	}

	groups := map[string]*BuddyGroup{}
	group := func(name string) *BuddyGroup {
		g, ok := groups[name]
		if !ok {
			g = &BuddyGroup{Name: name, Buddies: []Buddy{}}
			groups[name] = g
		}
		return g
	}
	offline := &BuddyGroup{Name: offlineGroup, Offline: true, Buddies: []Buddy{}}

	for _, r := range roster {
		ns := namespaceOf(r.Address)
		g := group(ns)
		g.Total++
		name, _, _ := strings.Cut(r.Address, "@")
		subs := r.Subagents
		if subs == nil {
			subs = []string{}
		}
		if len(r.Online) == 0 {
			offline.Total++
			offline.Buddies = append(offline.Buddies, Buddy{Address: r.Address, Name: name, Role: r.Role, Subagents: subs})
			continue
		}
		g.Online++
		allIdle, needsAttention := true, false
		for _, o := range r.Online {
			allIdle = allIdle && o.State == presence.StateIdle
			needsAttention = needsAttention || o.Attention == wire.AttentionBlocked || o.Attention == wire.AttentionFailed
			g.Buddies = append(g.Buddies, Buddy{
				Address: r.Address, Name: name, Role: r.Role, Online: true, Away: o.Away,
				Client: o.Client, ClientVersion: versions[o.SessionID], Machine: o.Machine,
				Subagents: subs, SignedOnAt: o.SignedOnAt, State: string(o.State), LastActive: o.LastActive,
				Attention: string(o.Attention), AttentionDetail: o.AttentionDetail, AttentionSince: o.AttentionSince,
			})
		}
		if needsAttention {
			g.Attention++
		}
		if allIdle {
			g.Idle++
		}
	}

	out := make([]BuddyGroup, 0, len(groups)+1)
	for _, g := range groups {
		if g.Online == 0 {
			continue // nothing online in this namespace; its identities are in the offline group
		}
		slices.SortFunc(g.Buddies, func(a, b Buddy) int {
			return cmp.Or(cmp.Compare(a.Address, b.Address), a.SignedOnAt.Compare(b.SignedOnAt))
		})
		out = append(out, *g)
	}
	slices.SortFunc(out, func(a, b BuddyGroup) int { return cmp.Compare(a.Name, b.Name) })
	slices.SortFunc(offline.Buddies, func(a, b Buddy) int { return cmp.Compare(a.Address, b.Address) })
	out = append(out, *offline)
	return PresenceBody{Groups: out, Events: events}
}

func namespaceOf(addr string) string {
	a, err := address.Parse(addr)
	if err != nil {
		return addr
	}
	return a.Namespace().String()
}
