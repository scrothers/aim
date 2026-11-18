package devseed

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"slices"
	"sync"
	"time"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/store"
)

// ErrHistoryExists is returned when the database already holds messages and
// Force is not set.
var ErrHistoryExists = errors.New("database already has messages; use --force to add more history")

// HistoryConfig configures SeedHistory.
type HistoryConfig struct {
	DB       string
	Seed     uint64
	Days     int
	Messages int
	Force    bool
	// Approvals turns on remote approvals in the database settings, so a
	// later live run produces visible approvals. Off leaves settings alone.
	Approvals bool
	// Now is the end of the generated window; zero means time.Now.
	Now func() time.Time
	Out io.Writer
}

// Summary describes what SeedHistory did.
type Summary struct {
	Identities     int
	Existing       int
	Retired        int
	Subagents      int
	Messages       int
	Threads        int
	OperatorUnread int
	Oldest, Newest time.Time
	Subjects       []string
	Board          BoardSummary
	// ApprovalsEnabled reports that SeedHistory turned remote approvals on.
	ApprovalsEnabled bool
}

// Print writes the summary as a few lines.
func (s Summary) Print(w io.Writer) {
	say(w, "identities: %d registered, %d already present (%d retired, %d subagent mailboxes)\n", s.Identities, s.Existing, s.Retired, s.Subagents)
	say(w, "messages:   %d in %d threads, %s to %s\n", s.Messages, s.Threads, s.Oldest.Format("Mon Jan 2 15:04"), s.Newest.Format("Mon Jan 2 15:04"))
	s.Board.Print(w)
	say(w, "operator:   %d unread\n", s.OperatorUnread)
	if s.ApprovalsEnabled {
		say(w, "approvals:  remote approvals enabled; run the live seeder to produce some\n")
	}
}

// clock is the controllable time source behind mail.WithClock.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) set(t time.Time) {
	c.mu.Lock()
	c.t = t
	c.mu.Unlock()
}

type delivered struct {
	at  time.Time
	msg mail.Message
}

// SeedHistory opens the database directly and writes backdated identities and
// mail. It is safe next to a running daemon (SQLite WAL).
func SeedHistory(ctx context.Context, cfg HistoryConfig) (Summary, error) {
	if cfg.Days < 1 || cfg.Messages < 1 {
		return Summary{}, errors.New("devseed: days and messages must be positive")
	}
	nowFn := cfg.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn().Truncate(time.Second)
	st, err := store.Open(ctx, cfg.DB)
	if err != nil {
		return Summary{}, fmt.Errorf("devseed: open %s: %w", cfg.DB, err)
	}
	defer func() { _ = st.Close() }()

	clk := &clock{t: now}
	svc := mail.New(st, nil, mail.WithClock(clk.now))
	if cfg.Out == nil {
		cfg.Out = io.Discard
	}

	stats, err := svc.Stats(ctx)
	if err != nil {
		return Summary{}, fmt.Errorf("devseed: %w", err)
	}
	if stats.Messages > 0 && !cfg.Force {
		return Summary{}, fmt.Errorf("devseed: %w (found %d)", ErrHistoryExists, stats.Messages)
	}

	world, err := buildWorld()
	if err != nil {
		return Summary{}, err
	}
	rng := rand.New(rand.NewPCG(cfg.Seed, 0x9e3779b97f4a7c15))
	g := newGen(rng, world, now, cfg.Days)

	var sum Summary
	if err := svc.EnsureOperator(ctx); err != nil {
		return Summary{}, fmt.Errorf("devseed: %w", err)
	}
	if err := register(ctx, svc, clk, world, now, cfg.Days, &sum); err != nil {
		return Summary{}, err
	}

	threads := g.plan(cfg.Messages)
	sum.Threads = len(threads)
	sent, err := send(ctx, svc, clk, threads)
	if err != nil {
		return Summary{}, err
	}
	sum.Messages = len(sent)
	for _, d := range sent {
		sum.Subjects = append(sum.Subjects, d.msg.Subject)
	}
	if len(sent) > 0 {
		sum.Oldest, sum.Newest = sent[0].at, sent[len(sent)-1].at
	}
	if err := markAll(ctx, svc, clk, g, sent); err != nil {
		return Summary{}, err
	}

	clk.set(now.Add(-time.Duration(cfg.Days) * 12 * time.Hour))
	for _, p := range world {
		if !p.retired {
			continue
		}
		if _, err := svc.Retire(ctx, p.String()); err != nil {
			return Summary{}, fmt.Errorf("devseed: retire %s: %w", p, err)
		}
		sum.Retired++
	}
	clk.set(now)

	if sum.Board, err = seedBoard(ctx, st, svc, clk, g, sent, cfg.Seed, now); err != nil {
		return Summary{}, err
	}
	if cfg.Approvals {
		if _, err := svc.SetApprovalSettings(ctx, mail.ApprovalSettings{Enabled: true}); err != nil {
			return Summary{}, fmt.Errorf("devseed: enable approvals: %w", err)
		}
		sum.ApprovalsEnabled = true
	}

	v, err := svc.Views(ctx, time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()))
	if err != nil {
		return Summary{}, fmt.Errorf("devseed: %w", err)
	}
	sum.OperatorUnread = v.OperatorUnread
	return sum, nil
}

// register creates every persona, oldest first, skipping existing ones.
func register(ctx context.Context, svc *mail.Service, clk *clock, world []*persona, now time.Time, days int, sum *Summary) error {
	clk.set(now.Add(-time.Duration(days)*24*time.Hour - time.Hour))
	defer clk.set(now)
	for _, p := range world {
		_, err := svc.Register(ctx, mail.Caller{}, mail.RegisterInput{
			Address: p.String(), Role: p.title, Description: p.desc, ReportsTo: p.reports,
		})
		switch {
		case err == nil:
			sum.Identities++
			sum.Subagents += len(p.subagents)
		case errors.Is(err, mail.ErrExists):
			sum.Existing++
		default:
			return fmt.Errorf("devseed: register %s: %w", p, err)
		}
		if p.retired {
			// A rerun may find it retired; it must send before retiring again.
			if _, err := svc.Unretire(ctx, p.String()); err != nil {
				return fmt.Errorf("devseed: unretire %s: %w", p, err)
			}
		}
		clk.set(clk.now().Add(time.Minute))
	}
	return nil
}

type event struct {
	th   *thread
	idx  int
	seq  int
	step *step
}

func callerOf(p *persona) mail.Caller {
	if p == nil {
		return mail.OperatorCaller("seed-operator", "aim-web", "devbox")
	}
	return mail.Caller{SessionID: p.sessionID(), Identity: p.addr, SignedOn: true, Client: p.client, Machine: p.machine}
}

// send executes every step in time order and returns the delivered messages.
func send(ctx context.Context, svc *mail.Service, clk *clock, threads []*thread) ([]delivered, error) {
	var evs []event
	for _, th := range threads {
		for i := range th.steps {
			evs = append(evs, event{th: th, idx: i, seq: len(evs), step: &th.steps[i]})
		}
	}
	slices.SortStableFunc(evs, func(a, b event) int {
		return cmp.Or(a.step.at.Compare(b.step.at), cmp.Compare(a.seq, b.seq))
	})
	ids := map[*thread][]string{}
	out := make([]delivered, 0, len(evs))
	for _, ev := range evs {
		s := ev.step
		clk.set(s.at)
		in := mail.SendInput{To: s.to, Cc: s.cc, Subject: s.subject, Body: s.body, Subagent: s.sub, ReplyAll: s.replyAll}
		if s.parent >= 0 {
			in.InReplyTo = ids[ev.th][s.parent]
		}
		msg, err := svc.Send(ctx, callerOf(s.from), in)
		if err != nil {
			return nil, fmt.Errorf("devseed: send %q from %s: %w", s.subject, s.sender(), err)
		}
		if ids[ev.th] == nil {
			ids[ev.th] = make([]string, len(ev.th.steps))
		}
		ids[ev.th][ev.idx] = msg.ID
		out = append(out, delivered{at: s.at, msg: msg})
	}
	return out, nil
}

// markAll has the recipients read and archive a realistic share of their mail.
func markAll(ctx context.Context, svc *mail.Service, clk *clock, g *gen, sent []delivered) error {
	for _, d := range sent {
		for _, to := range append(slices.Clone(d.msg.To), d.msg.Cc...) {
			a, err := address.Parse(to)
			if err != nil {
				return fmt.Errorf("devseed: parse %q: %w", to, err)
			}
			var owner *persona
			var sub string
			switch a.Kind() {
			case address.KindOperator:
			case address.KindIdentity:
				owner = g.byAddr[a.String()]
			case address.KindSubagent:
				owner, sub = g.byAddr[a.IdentityAddress().String()], a.Subagent
			default:
				continue
			}
			if a.Kind() != address.KindOperator && owner == nil {
				continue
			}
			read, archive := g.fate(d.at, a.Kind() == address.KindOperator)
			if !read {
				continue
			}
			readAt := d.at.Add(g.mins(1, 180))
			if readAt.After(g.now) {
				readAt = g.now
			}
			c := callerOf(owner)
			states := []struct {
				state string
				at    time.Time
			}{{mail.MarkRead, readAt}}
			if archive {
				states = append(states, struct {
					state string
					at    time.Time
				}{mail.MarkArchived, minTime(readAt.Add(g.mins(5, 24*60)), g.now)})
			}
			for _, st := range states {
				clk.set(st.at)
				if _, err := svc.Mark(ctx, c, mail.MarkInput{MessageIDs: []string{d.msg.ID}, State: st.state, Subagent: sub}); err != nil {
					return fmt.Errorf("devseed: mark %s for %s: %w", d.msg.ID, to, err)
				}
			}
		}
	}
	return nil
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// fate decides whether a delivery was read and whether it was then archived.
func (g *gen) fate(at time.Time, operator bool) (read, archive bool) {
	age := g.now.Sub(at)
	u := g.rng.Float64()
	if operator {
		switch {
		case age < 36*time.Hour:
			return u >= 0.70, u >= 0.95
		default:
			return u >= 0.12, u >= 0.72
		}
	}
	if age < 4*time.Hour {
		return u < 0.5, false
	}
	return u < 0.85, u < 0.12
}

// say writes formatted output, ignoring write errors (progress output only).
func say(w io.Writer, format string, args ...any) { _, _ = fmt.Fprintf(w, format, args...) }
