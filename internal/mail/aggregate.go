package mail

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/store/db"
)

const (
	// ActivityDays is the number of UTC days covered by the Activity series.
	ActivityDays = 7
	msPerDay     = int64(24 * time.Hour / time.Millisecond)
)

// SessionCounter is optionally implemented by a Presence to report how many
// sessions are connected and on how many distinct machines.
type SessionCounter interface {
	Sessions() (sessions, machines int)
}

// Activity describes mail traffic for a node of the Tree.
//
// Unread counts unread, unarchived deliveries. LastActivity is the newest
// time any mailbox in the node received or sent a message (nil if never).
// Activity holds ActivityDays counters, oldest first; the last entry is the
// current UTC day. Each counts deliveries (messages received) to mailboxes
// in the node on that UTC day, so a message addressed to two mailboxes in the
// same node counts twice.
type Activity struct {
	Unread       int        `json:"unread"`
	LastActivity *time.Time `json:"last_activity,omitempty"`
	Activity     []int      `json:"activity"`
}

// MailboxNode is one identity in the Tree. Its counters include the
// identity's subagent mailboxes.
type MailboxNode struct {
	Activity
	Address   string   `json:"address"`
	Role      string   `json:"role"`
	Online    int      `json:"online"`
	Retired   bool     `json:"retired"`
	Subagents []string `json:"subagents"`
}

// RepoNode is one repo inside an org of the Tree. Its counters cover the
// repo namespace mailbox and every identity beneath it.
type RepoNode struct {
	Activity
	Address    string        `json:"address"`
	Name       string        `json:"name"`
	Identities []MailboxNode `json:"identities"`
}

// OrgNode is one org in the Tree. Its counters cover everything in the org.
// Identities lists org-level identities (those registered without a repo).
type OrgNode struct {
	Activity
	Address    string        `json:"address"`
	Host       string        `json:"host"`
	Org        string        `json:"org"`
	Identities []MailboxNode `json:"identities"`
	Repos      []RepoNode    `json:"repos"`
}

// Tree is the mailbox hierarchy shown in the navigation: orgs, their
// org-level identities and repos, and repo identities. Retired identities are
// included and flagged.
type Tree struct {
	Orgs []OrgNode `json:"orgs"`
}

// ViewCounts are the badge counts of the fixed navigation views.
type ViewCounts struct {
	// OperatorUnread is the unread count of the operator mailbox.
	OperatorUnread int `json:"operator_unread"`
	// UnreadEverywhere counts unread, unarchived deliveries in all mailboxes,
	// the operator's included.
	UnreadEverywhere int `json:"unread_everywhere"`
	// Today counts messages created at or after the todaySince argument.
	Today int `json:"today"`
}

// Stats are daemon-wide totals.
type Stats struct {
	Messages    int64 `json:"messages"`
	DBSizeBytes int64 `json:"db_size_bytes"`
	// Sessions and Machines come from presence and are zero when the Presence
	// does not implement SessionCounter.
	Sessions int `json:"sessions"`
	Machines int `json:"machines"`
}

// OrgChartNode is one identity (or the operator) in the org chart.
type OrgChartNode struct {
	Address     string         `json:"address"`
	Role        string         `json:"role"`
	Description string         `json:"description"`
	ReportsTo   string         `json:"reports_to,omitempty"`
	Online      []OnlineStatus `json:"online"`
	Operator    bool           `json:"operator,omitempty"`
}

// OrgChartEdge points from a reporter to the identity (or operator) it
// reports to.
type OrgChartEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// OrgChart is the reporting graph of active identities plus a synthetic
// operator node. Edges whose target is not a listed node are omitted; the
// raw value stays on the node's ReportsTo.
type OrgChart struct {
	Nodes []OrgChartNode `json:"nodes"`
	Edges []OrgChartEdge `json:"edges"`
}

// stat accumulates the counters of one mailbox or a roll-up of several.
type stat struct {
	unread int
	last   int64
	days   [ActivityDays]int
}

func (a *stat) add(b stat) {
	a.unread += b.unread
	a.last = max(a.last, b.last)
	for i := range a.days {
		a.days[i] += b.days[i]
	}
}

func (a stat) activity() Activity {
	out := Activity{Unread: a.unread, Activity: slices.Clone(a.days[:])}
	if a.last > 0 {
		t := fromMS(a.last)
		out.LastActivity = &t
	}
	return out
}

// mailboxStats loads per-mailbox counters keyed by mailbox id.
func (s *Service) mailboxStats(ctx context.Context, q *db.Queries, now time.Time) (map[string]stat, error) {
	today := now.UnixMilli() / msPerDay
	first := today - (ActivityDays - 1)
	act, err := q.MailboxActivity(ctx)
	if err != nil {
		return nil, fmt.Errorf("mail: stats: activity: %w", err)
	}
	sent, err := q.MailboxLastSent(ctx)
	if err != nil {
		return nil, fmt.Errorf("mail: stats: last sent: %w", err)
	}
	days, err := q.MailboxDailyDeliveries(ctx, first*msPerDay)
	if err != nil {
		return nil, fmt.Errorf("mail: stats: daily: %w", err)
	}
	out := map[string]stat{}
	for _, a := range act {
		st := out[a.MailboxID]
		st.unread = int(a.Unread)
		st.last = a.LastReceived
		out[a.MailboxID] = st
	}
	for _, a := range sent {
		st := out[a.MailboxID]
		st.last = max(st.last, a.LastSent)
		out[a.MailboxID] = st
	}
	for _, d := range days {
		idx := d.Day - first
		if idx < 0 || idx >= ActivityDays {
			continue // a message timestamped in the future
		}
		st := out[d.MailboxID]
		st.days[idx] += int(d.N)
		out[d.MailboxID] = st
	}
	return out, nil
}

type orgKey struct{ host, org string }

// Tree builds the mailbox hierarchy with unread counts, last activity and
// seven-day activity at every level.
func (s *Service) Tree(ctx context.Context) (Tree, error) {
	q := s.store.Read()
	now := s.now()
	stats, err := s.mailboxStats(ctx, q, now)
	if err != nil {
		return Tree{}, err
	}
	boxes, err := q.ListAllMailboxes(ctx)
	if err != nil {
		return Tree{}, fmt.Errorf("mail: tree: %w", err)
	}

	type repoAcc struct {
		node RepoNode
		st   stat
		ids  map[string]int // identity name -> index into node.Identities
		idst []stat
	}
	type orgAcc struct {
		node  OrgNode
		st    stat
		ids   map[string]int
		idst  []stat
		repos map[string]*repoAcc
	}
	orgs := map[orgKey]*orgAcc{}
	getOrg := func(m db.Mailbox) *orgAcc {
		k := orgKey{m.Host, m.Org}
		o := orgs[k]
		if o == nil {
			o = &orgAcc{ids: map[string]int{}, repos: map[string]*repoAcc{}}
			o.node = OrgNode{
				Address: m.Host + "/" + m.Org, Host: m.Host, Org: m.Org,
				Identities: []MailboxNode{}, Repos: []RepoNode{},
			}
			orgs[k] = o
		}
		return o
	}
	getRepo := func(o *orgAcc, m db.Mailbox) *repoAcc {
		r := o.repos[m.Repo]
		if r == nil {
			r = &repoAcc{ids: map[string]int{}}
			r.node = RepoNode{Address: o.node.Address + "/" + m.Repo, Name: m.Repo, Identities: []MailboxNode{}}
			o.repos[m.Repo] = r
		}
		return r
	}

	// Identities first, so subagent and namespace rows can attach to them.
	for _, m := range boxes {
		if m.Kind != address.KindIdentity.String() {
			continue
		}
		o := getOrg(m)
		node := MailboxNode{
			Address: addrOf(m).String(), Role: m.Role, Retired: m.RetiredAt.Valid, Subagents: []string{},
		}
		if s.presence != nil {
			node.Online = len(s.presence.Online(addrOf(m)))
		}
		st := stats[m.ID]
		if m.Repo == "" {
			o.ids[m.Identity] = len(o.node.Identities)
			o.node.Identities = append(o.node.Identities, node)
			o.idst = append(o.idst, st)
			continue
		}
		r := getRepo(o, m)
		r.ids[m.Identity] = len(r.node.Identities)
		r.node.Identities = append(r.node.Identities, node)
		r.idst = append(r.idst, st)
	}
	for _, m := range boxes {
		switch m.Kind {
		case address.KindSubagent.String():
			o := getOrg(m)
			if m.Repo == "" {
				if i, ok := o.ids[m.Identity]; ok {
					o.node.Identities[i].Subagents = append(o.node.Identities[i].Subagents, m.Subagent)
					o.idst[i].add(stats[m.ID])
				}
				continue
			}
			r := getRepo(o, m)
			if i, ok := r.ids[m.Identity]; ok {
				r.node.Identities[i].Subagents = append(r.node.Identities[i].Subagents, m.Subagent)
				r.idst[i].add(stats[m.ID])
			}
		case address.KindNamespace.String():
			o := getOrg(m)
			if m.Repo == "" {
				o.st.add(stats[m.ID])
				continue
			}
			getRepo(o, m).st.add(stats[m.ID])
		}
	}

	keys := make([]orgKey, 0, len(orgs))
	for k := range orgs {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b orgKey) int {
		return strings.Compare(a.host+"/"+a.org, b.host+"/"+b.org)
	})
	tree := Tree{Orgs: make([]OrgNode, 0, len(keys))}
	for _, k := range keys {
		o := orgs[k]
		for i := range o.node.Identities {
			o.node.Identities[i].Activity = o.idst[i].activity()
			o.st.add(o.idst[i])
		}
		repoNames := make([]string, 0, len(o.repos))
		for n := range o.repos {
			repoNames = append(repoNames, n)
		}
		slices.Sort(repoNames)
		for _, n := range repoNames {
			r := o.repos[n]
			for i := range r.node.Identities {
				r.node.Identities[i].Activity = r.idst[i].activity()
				r.st.add(r.idst[i])
			}
			r.node.Activity = r.st.activity()
			o.st.add(r.st)
			o.node.Repos = append(o.node.Repos, r.node)
		}
		o.node.Activity = o.st.activity()
		tree.Orgs = append(tree.Orgs, o.node)
	}
	return tree, nil
}

// Views returns the navigation badge counts. todaySince is the start of
// "today" in the viewer's time zone.
func (s *Service) Views(ctx context.Context, todaySince time.Time) (ViewCounts, error) {
	q := s.store.Read()
	var v ViewCounts
	unread, err := q.CountUnreadDeliveries(ctx)
	if err != nil {
		return ViewCounts{}, fmt.Errorf("mail: views: unread: %w", err)
	}
	v.UnreadEverywhere = int(unread)
	today, err := q.CountDeliveriesSince(ctx, todaySince.UnixMilli())
	if err != nil {
		return ViewCounts{}, fmt.Errorf("mail: views: today: %w", err)
	}
	v.Today = int(today)
	op, err := q.GetMailboxByKey(ctx, keyOf(address.Operator()))
	if errors.Is(err, sql.ErrNoRows) {
		return v, nil
	}
	if err != nil {
		return ViewCounts{}, fmt.Errorf("mail: views: operator mailbox: %w", err)
	}
	act, err := q.MailboxActivity(ctx)
	if err != nil {
		return ViewCounts{}, fmt.Errorf("mail: views: activity: %w", err)
	}
	for _, a := range act {
		if a.MailboxID == op.ID {
			v.OperatorUnread = int(a.Unread)
		}
	}
	return v, nil
}

// Stats returns daemon-wide totals.
func (s *Service) Stats(ctx context.Context) (Stats, error) {
	var out Stats
	n, err := s.store.Read().CountMessages(ctx)
	if err != nil {
		return Stats{}, fmt.Errorf("mail: stats: messages: %w", err)
	}
	out.Messages = n
	if out.DBSizeBytes, err = s.store.Size(ctx); err != nil {
		return Stats{}, fmt.Errorf("mail: stats: %w", err)
	}
	if sc, ok := s.presence.(SessionCounter); ok {
		out.Sessions, out.Machines = sc.Sessions()
	}
	return out, nil
}

// OrgChart returns the reporting graph of active identities and the operator.
func (s *Service) OrgChart(ctx context.Context) (OrgChart, error) {
	rows, err := s.store.Read().ListAllIdentities(ctx)
	if err != nil {
		return OrgChart{}, fmt.Errorf("mail: org chart: %w", err)
	}
	rows = activeOnly(rows)
	chart := OrgChart{
		Nodes: make([]OrgChartNode, 0, len(rows)+1),
		Edges: []OrgChartEdge{},
	}
	chart.Nodes = append(chart.Nodes, OrgChartNode{
		Address: address.OperatorName, Role: "Operator", Online: []OnlineStatus{}, Operator: true,
	})
	known := map[string]bool{address.OperatorName: true}
	for _, r := range rows {
		a := addrOf(r)
		known[a.String()] = true
		n := OrgChartNode{
			Address: a.String(), Role: r.Role, Description: r.Description, ReportsTo: r.ReportsTo,
			Online: []OnlineStatus{},
		}
		if s.presence != nil {
			if on := s.presence.Online(a); on != nil {
				n.Online = on
			}
		}
		chart.Nodes = append(chart.Nodes, n)
	}
	for _, n := range chart.Nodes {
		if n.ReportsTo != "" && known[n.ReportsTo] {
			chart.Edges = append(chart.Edges, OrgChartEdge{From: n.Address, To: n.ReportsTo})
		}
	}
	return chart, nil
}
