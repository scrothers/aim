package devseed

import (
	"cmp"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/store"
	tsk "github.com/scrothers/aim/internal/tasks"
)

const (
	minute = time.Minute
	hour   = time.Hour
	day    = 24 * time.Hour
)

// Seeded identities that appear in the task board.
const (
	apiBuilder    = "builder@" + acme + "/api"
	apiReviewer   = "reviewer@" + acme + "/api"
	apiTriage     = "triage@" + acme + "/api"
	webBuilder    = "builder@" + acme + "/web"
	webReviewer   = "reviewer@" + acme + "/web"
	webQA         = "qa@" + acme + "/web"
	infraDeployer = "deployer@" + acme + "/infra"
	infraSecurity = "security@" + acme + "/infra"
	mobBuilder    = "builder@" + acme + "/mobile"
	mobQA         = "qa@" + acme + "/mobile"
	acmeCoord     = "coordinator@" + acme
	acmeRelease   = "release-manager@" + acme
	labsCoord     = "coordinator@" + labs
	radarResearch = "researcher@" + labs + "/radar"
	radarBuilder  = "builder@" + labs + "/radar"
	nbDocs        = "docs@" + labs + "/notebook"
	nbReviewer    = "reviewer@" + labs + "/notebook"
	fooCoord      = "coordinator@" + foo
	fooBuilder    = "builder@" + foo + "/bar"
	fooReviewer   = "reviewer@" + foo + "/bar"
)

// tstep is one action on a task. ago is how long before now it happens; the
// steps of a task are listed oldest first.
type tstep struct {
	ago   time.Duration
	who   string // identity address or opAddr
	act   tsk.Action
	note  string
	to    string // assignee, for assign
	sweep bool   // run the lease sweeper instead of an action
}

func tact(ago time.Duration, who string, act tsk.Action, note string) tstep {
	return tstep{ago: ago, who: who, act: act, note: note}
}

func comment(ago time.Duration, who, note string) tstep {
	return tact(ago, who, tsk.ActionComment, note)
}

func sweepAt(ago time.Duration) tstep { return tstep{ago: ago, sweep: true} }

// taskSpec describes one seeded task and its history.
type taskSpec struct {
	creator  string
	scope    string
	title    string
	body     string
	prio     tsk.Priority
	assignee string
	created  time.Duration // ago
	due      time.Duration // from now (negative: overdue); zero means none
	// link asks for the task to be linked to a message the creator sent or
	// received before it; hint prefers a message with this subject prefix.
	link  bool
	hint  string
	steps []tstep
}

// acceptance renders a task body: the goal, then a checklist.
func acceptance(goal string, criteria ...string) string {
	var b strings.Builder
	b.WriteString(md(goal))
	b.WriteString("\n\n**Acceptance criteria**\n\n")
	for _, c := range criteria {
		b.WriteString("- [ ] " + md(c) + "\n")
	}
	return strings.TrimSpace(b.String())
}

// taskSpecs is the seeded board: 10 not started (4 assigned, 2 overdue), 9
// held at every lease stage, 5 finished and 2 abandoned after decay.
func taskSpecs() []taskSpec {
	return []taskSpec{
		// Not started.
		{
			creator: opAddr, scope: acme + "/infra", title: "Decide on the staging maintenance window", prio: tsk.PriorityHigh,
			assignee: infraDeployer, created: 5 * hour, due: 20 * hour,
			body: acceptance("Pick a low-traffic window for the staging network move and tell the org.",
				"Window agreed with the web and mobile QA owners", "Announcement posted to `github.com/acme`", "Rollback plan linked from the task"),
			steps: []tstep{comment(3*hour, infraDeployer, "Proposing Thursday 02:00 UTC; QA suites are idle then.")},
		},
		{
			creator: acmeCoord, scope: acme + "/infra", title: "Add alarms for queue depth", prio: tsk.PriorityNormal,
			created: 2 * day, due: -day,
			body: acceptance("The export queue backed up for 40 minutes last week without anyone noticing.",
				"Alarm at depth > 500 for 5 minutes", "Page goes to the infra rotation", "Runbook entry added"),
			steps: []tstep{comment(30*hour, infraDeployer, "Terraform module is in `terraform/modules/queue/alarms.tf`; needs thresholds from the coordinator.")},
		},
		{
			creator: apiReviewer, scope: acme + "/api", title: "Add idempotency keys to POST /payments", prio: tsk.PriorityHigh,
			assignee: apiBuilder, created: 26 * hour, due: -6 * hour, link: true,
			body: acceptance("Retried payment requests must not double charge.",
				"`Idempotency-Key` header accepted and stored for 24h", "Replay returns the original response", "`TestPaymentIdempotency` covers replay and conflict"),
		},
		{
			creator: webReviewer, scope: acme + "/web", title: "Fix focus trap in the checkout dialog", prio: tsk.PriorityUrgent,
			assignee: webBuilder, created: 3 * hour, link: true,
			body: acceptance("Tab escapes the dialog to the page behind it, which breaks keyboard checkout.",
				"Focus stays inside the dialog until it closes", "Focus returns to the trigger on close", "Regression test added"),
		},
		{
			creator: nbReviewer, scope: labs + "/notebook", title: "Document the kernel protocol", prio: tsk.PriorityNormal,
			assignee: nbDocs, created: day, due: 3 * day, link: true, hint: "Docs drift",
			body: acceptance("The reference lags the implementation after the streaming change.",
				"Every message type documented with an example", "Versioning rules explained", "Examples run in CI"),
		},
		{
			creator: apiTriage, scope: acme + "/api", title: "Reproduce intermittent 500 on /v2/orders", prio: tsk.PriorityHigh,
			created: 2 * day, link: true,
			body: acceptance("Three customers report a 500 about once an hour on the list endpoint.",
				"Failing request captured with trace id", "Root cause identified or ruled out", "Fix or follow-up task filed"),
			steps: []tstep{
				tact(36*hour, apiBuilder, tsk.ActionClaim, ""),
				comment(35*hour, apiBuilder, "Cannot reproduce on staging after 2k requests; looks tied to the replica lag alert."),
				tact(34*hour, apiBuilder, tsk.ActionRelease, "Needs production traces; handing back to triage."),
			},
		},
		{
			creator: acmeRelease, scope: acme, title: "Draft the changelog for the next release", prio: tsk.PriorityNormal,
			assignee: acmeRelease, created: 6 * hour, due: 2 * day,
			body: acceptance("Collect merged PRs since the last tag and group them by repo.",
				"Breaking changes called out first", "Upgrade notes for api and web", "Reviewed by the coordinator"),
		},
		{
			creator: infraSecurity, scope: acme + "/infra", title: "Tighten the S3 bucket policy for exports", prio: tsk.PriorityHigh,
			assignee: infraDeployer, created: 40 * hour, due: 4 * day, link: true,
			body: acceptance("The exports bucket allows `s3:*` from the whole account.",
				"Policy limited to the exporter role", "`checkov CKV_AWS_18` passes", "Change applied to staging before production"),
			steps: []tstep{
				comment(30*hour, infraSecurity, "The advisory affects every bucket with a wildcard principal; exports is the worst one."),
				comment(28*hour, infraDeployer, "Will pick this up right after the EKS node group upgrade."),
			},
		},
		{
			creator: mobQA, scope: acme + "/mobile", title: "Support Android 15 predictive back", prio: tsk.PriorityNormal,
			created: 3 * day, due: 5 * day,
			body: acceptance("The back gesture preview is missing on Android 15 devices in the farm.",
				"Back preview animates on supported devices", "`BackHandlerTest.predictiveBack` passes", "Behavior unchanged on Android 13"),
		},
		{
			creator: opAddr, title: "Review the weekly cloud cost report", prio: tsk.PriorityLow, created: 4 * day,
			body: acceptance("A quick look for surprises before the Monday planning.", "Anything over budget flagged", "Owners pinged for unexplained spend"),
		},

		// Held, from healthy to decayed.
		{
			creator: acmeCoord, scope: acme + "/api", title: "Paginate /v2/orders with cursor tokens", prio: tsk.PriorityHigh,
			assignee: apiBuilder, created: 2 * day, link: true,
			body: acceptance("Offset pagination times out on large accounts.",
				"Opaque cursor tokens in `next_cursor`", "Stable ordering under concurrent inserts", "`TestListOrdersCursor` covers first, middle and last page"),
			steps: []tstep{
				tact(6*hour, apiBuilder, tsk.ActionClaim, ""),
				tact(345*minute, apiBuilder, tsk.ActionStart, ""),
				comment(2*hour, apiBuilder, "Cursor encoding is done; adding the Link header next."),
				comment(4*minute, apiBuilder, "Tests for the empty and last page pass locally."),
			},
		},
		{
			creator: webReviewer, scope: acme + "/web", title: "Virtualize the orders table", prio: tsk.PriorityNormal, created: day,
			body: acceptance("10k rows freeze the tab for two seconds.", "Only visible rows rendered", "Keyboard navigation unchanged", "Scroll stays at 60fps in the profiler"),
			steps: []tstep{
				tact(3*hour, webBuilder, tsk.ActionStart, ""),
				comment(hour, webBuilder, "Prototype with fixed row height works; measuring variable heights."),
				comment(11*minute, webBuilder, "Sticky header needs a wrapper; pushing a draft PR."),
			},
		},
		{
			creator: radarResearch, scope: labs + "/radar", title: "Batch the feature store reads", prio: tsk.PriorityNormal,
			assignee: radarBuilder, created: 8 * hour,
			body: acceptance("Scoring makes one store call per entity.", "Single batched read per scoring run", "p95 scoring latency under 200ms"),
			steps: []tstep{
				tact(50*minute, radarBuilder, tsk.ActionClaim, ""),
				comment(2*minute, radarBuilder, "Read the store schema; starting on the batch API."),
			},
		},
		{
			creator: acmeCoord, scope: acme + "/infra", title: "Upgrade EKS node group to 1.31", prio: tsk.PriorityHigh, created: day, due: 2 * day,
			body: acceptance("1.29 leaves extended support next month.", "Canary node group on 1.31 healthy for 24h", "All workloads drained and rescheduled", "Old group removed"),
			steps: []tstep{
				tact(3*hour, infraDeployer, tsk.ActionClaim, ""),
				tact(170*minute, infraDeployer, tsk.ActionStart, ""),
				comment(hour, infraDeployer, "Canary node group is healthy."),
				comment(22*minute, infraDeployer, "Draining the first batch of nodes."),
			},
		},
		{
			creator: nbDocs, scope: labs + "/notebook", title: "Add keyboard shortcuts cheat sheet", prio: tsk.PriorityLow, created: 2 * day,
			body: acceptance("Users keep asking which keys run a cell.", "All bindings listed by platform", "Shown from the help menu"),
			steps: []tstep{
				tact(hour, nbDocs, tsk.ActionClaim, ""),
				comment(18*minute, nbDocs, "Collected the bindings from `packages/ui/src/shortcuts.tsx`."),
			},
		},
		{
			creator: fooReviewer, scope: foo + "/bar", title: "Handle empty CSV uploads", prio: tsk.PriorityNormal, created: day,
			body: acceptance("A 0-byte upload returns a stack trace.", "400 with a clear message", "`tests/test_csv.py::test_empty_file` passes"),
			steps: []tstep{
				tact(2*hour, fooBuilder, tsk.ActionClaim, ""),
				tact(110*minute, fooBuilder, tsk.ActionStart, ""),
				comment(41*minute, fooBuilder, "Reproduced with a 0-byte file; fixing the parser."),
			},
		},
		{
			creator: mobQA, scope: acme + "/mobile", title: "Fix crash when resuming a backgrounded checkout", prio: tsk.PriorityUrgent,
			created: 20 * hour, due: -3 * hour, link: true,
			body: acceptance("Process death during checkout crashes on resume.", "State restored from the saved handle", "`CheckoutViewModelTest.resumesAfterProcessDeath` passes"),
			steps: []tstep{
				tact(70*minute, mobBuilder, tsk.ActionClaim, ""),
				comment(52*minute, mobBuilder, "Crash is a null saved-state handle after process death."),
			},
		},
		{
			creator: webReviewer, scope: acme + "/web", title: "Add Playwright coverage for the checkout focus trap", prio: tsk.PriorityNormal,
			assignee: webQA, created: day,
			body: acceptance("The focus trap fix needs an end-to-end test.", "Tab and Shift+Tab stay inside the dialog", "Runs in the release-candidate suite"),
			steps: []tstep{
				tact(150*minute, webQA, tsk.ActionClaim, ""),
				tact(140*minute, webQA, tsk.ActionStart, ""),
				comment(95*minute, webQA, "Page object written; waiting on the dialog fix to land."),
			},
		},
		{
			creator: apiTriage, scope: acme + "/api", title: "Rate limit the public search endpoint", prio: tsk.PriorityNormal, created: 2 * day,
			body: acceptance("One client sent 4k searches a minute.", "Token bucket per API key", "429 with `Retry-After`", "`TestRateLimitBurst` passes"),
			steps: []tstep{
				tact(210*minute, apiReviewer, tsk.ActionClaim, ""),
				comment(190*minute, apiReviewer, "Checking how the existing limiter keys its buckets."),
			},
		},

		// Finished.
		{
			creator: apiReviewer, scope: acme + "/api", title: "Fix N+1 query in invoice export", prio: tsk.PriorityNormal,
			assignee: apiBuilder, created: 3 * day, link: true,
			body: acceptance("Invoice export issues one query per line item.", "Query count independent of invoice size", "`TestInvoiceExportQueryCount` added"),
			steps: []tstep{
				tact(54*hour, apiBuilder, tsk.ActionClaim, ""),
				tact(53*hour, apiBuilder, tsk.ActionStart, ""),
				comment(50*hour, apiBuilder, "Query count 214 to 3 with a batched load."),
				tact(49*hour, apiBuilder, tsk.ActionLink, ""),
				tact(47*hour+30*minute, apiBuilder, tsk.ActionFinish, "Merged with the query count test."),
			},
		},
		{
			creator: acmeCoord, scope: acme + "/web", title: "Add dark mode tokens to the design system", prio: tsk.PriorityNormal, created: 4 * day,
			body: acceptance("Components hard-code light colors.", "Semantic tokens for surface, text and border", "Storybook shows both themes"),
			steps: []tstep{
				tact(92*hour, webBuilder, tsk.ActionStart, ""),
				comment(72*hour, webBuilder, "Token names agreed with the reviewer."),
				comment(60*hour, webBuilder, "Charts still use raw hex values; converting them too."),
				tact(59*hour, webBuilder, tsk.ActionFinish, "Shipped in the 4.2 component library."),
			},
		},
		{
			creator: infraSecurity, scope: acme + "/infra", title: "Rotate the RDS master credentials", prio: tsk.PriorityUrgent,
			assignee: infraDeployer, created: 108 * hour, due: -2 * day,
			body: acceptance("The credential is past its 90 day rotation window.", "Rotated in staging then production", "Applications reconnect without errors", "Old secret revoked"),
			steps: []tstep{
				tact(96*hour, infraDeployer, tsk.ActionClaim, ""),
				tact(95*hour, infraDeployer, tsk.ActionStart, ""),
				comment(84*hour, infraDeployer, "Rotated in staging; verifying the app reconnects."),
				tact(82*hour, infraDeployer, tsk.ActionFinish, "Production rotated and the old secret revoked."),
			},
		},
		{
			creator: nbDocs, scope: labs + "/notebook", title: "Render Mermaid diagrams in cells", prio: tsk.PriorityNormal, created: 2 * day,
			body: acceptance("Docs authors paste diagram images by hand.", "Flowchart and sequence diagrams render", "Dark theme supported"),
			steps: []tstep{
				tact(36*hour, nbReviewer, tsk.ActionClaim, ""),
				tact(35*hour, nbReviewer, tsk.ActionStart, ""),
				comment(34*hour, nbReviewer, "Renderer works; lazy-loading the mermaid bundle."),
				tact(31*hour, nbReviewer, tsk.ActionFinish, "Merged behind the `mermaid` cell option."),
			},
		},
		{
			creator: acmeCoord, scope: acme, title: "Publish release notes for the latest tag", prio: tsk.PriorityNormal,
			created: 12 * hour, link: true, hint: "Release notes",
			body: acceptance("Announce what shipped to every acme repo.", "Notes posted to the org mailbox", "Operator in cc"),
			steps: []tstep{
				tact(9*hour, acmeRelease, tsk.ActionClaim, ""),
				tact(530*minute, acmeRelease, tsk.ActionStart, ""),
				tact(5*hour, acmeRelease, tsk.ActionFinish, "Notes posted to the org mailbox."),
			},
		},

		// Abandoned after their claimants went quiet.
		{
			creator: labsCoord, scope: labs + "/radar", title: "Backfill 90 days of embeddings", prio: tsk.PriorityNormal,
			assignee: radarResearch, created: 3 * day,
			body: acceptance("The new model needs embeddings for historical windows.", "90 days backfilled in batches of 10k", "Progress logged per batch"),
			steps: []tstep{
				tact(2*day, radarResearch, tsk.ActionClaim, ""),
				tact(2*day-5*minute, radarResearch, tsk.ActionStart, ""),
				comment(2*day-30*minute, radarResearch, "First batch finished; 3 of 9 remaining windows queued."),
				sweepAt(2*day - 70*minute),
				sweepAt(2*day - 105*minute),
			},
		},
		{
			creator: fooCoord, scope: foo + "/bar", title: "Cache the lookup table", prio: tsk.PriorityLow, created: 2 * day,
			body: acceptance("Every request reloads the lookup table from disk.", "TTL cache with a 60s expiry", "`tests/test_lookup.py::test_cache_ttl` passes"),
			steps: []tstep{
				tact(30*hour, fooBuilder, tsk.ActionClaim, ""),
				comment(30*hour-10*minute, fooBuilder, "Looking at `bar/lookup.py`."),
				sweepAt(30*hour - 45*minute),
				sweepAt(30*hour - 110*minute),
			},
		},
	}
}

// BoardSummary describes the seeded or existing task board.
type BoardSummary struct {
	Tasks    int
	Existing int // tasks already on the board; nothing is seeded then
	Overdue  int
	Events   int
	Links    int
	Status   map[tsk.Status]int
	Health   map[tsk.Health]int
}

// entropy returns a deterministic ULID entropy source for the seed.
func entropy(seed uint64) *rand.ChaCha8 {
	var key [32]byte
	binary.LittleEndian.PutUint64(key[:], seed)
	return rand.NewChaCha8(key)
}

// findMessage returns the newest delivered message at or before t that
// involves person and is not yet used; a message whose subject starts with
// hint (ignoring "Re: ") wins when there is one.
func findMessage(sent []delivered, person, hint string, t time.Time, used map[string]bool) (delivered, bool) {
	involves := func(m mail.Message) bool {
		if m.From == person || strings.HasSuffix(m.From, "@"+person) {
			return true
		}
		return slices.Contains(m.To, person) || slices.Contains(m.Cc, person)
	}
	var fallback delivered
	var haveFallback bool
	for i := len(sent) - 1; i >= 0; i-- {
		d := sent[i]
		if d.at.After(t) || used[d.msg.ID] {
			continue
		}
		if hint != "" && strings.HasPrefix(strings.TrimPrefix(d.msg.Subject, "Re: "), hint) {
			return d, true
		}
		if !haveFallback && involves(d.msg) {
			fallback, haveFallback = d, true
			if hint == "" {
				break
			}
		}
	}
	return fallback, haveFallback
}

// seedBoard writes the task board through the task service, so every event,
// notice and lease timestamp is what the real service would have produced.
// It skips seeding when the board already has tasks.
func seedBoard(ctx context.Context, st *store.Store, svc *mail.Service, clk *clock, g *gen, sent []delivered, seed uint64, now time.Time) (BoardSummary, error) {
	board := tsk.New(st, svc, tsk.WithClock(clk.now), tsk.WithEntropy(entropy(seed)))
	clk.set(now)
	existing, err := board.List(ctx, tsk.ListInput{Limit: 1})
	if err != nil {
		return BoardSummary{}, fmt.Errorf("devseed: %w", err)
	}
	if len(existing.Items) > 0 {
		sum, err := summarizeBoard(ctx, board, now)
		sum.Existing, sum.Tasks = sum.Tasks, 0
		return sum, err
	}
	caller := func(who string) (mail.Caller, error) {
		if who == opAddr {
			return callerOf(nil), nil
		}
		p := g.byAddr[who]
		if p == nil {
			return mail.Caller{}, fmt.Errorf("devseed: task board: unknown identity %q", who)
		}
		return callerOf(p), nil
	}
	run := &boardRun{
		board: board, clk: clk, caller: caller, sent: sent, now: now,
		specs: taskSpecs(), used: map[string]bool{},
	}
	if err := run.execute(ctx); err != nil {
		return BoardSummary{}, err
	}
	clk.set(now)
	return summarizeBoard(ctx, board, now)
}

// boardRun replays every task's creation and steps in time order, as they
// would have happened. Order matters: an agent's update renews every lease it
// holds, and a sweep decays every lease that is stale at that moment.
type boardRun struct {
	board  *tsk.Service
	clk    *clock
	caller func(string) (mail.Caller, error)
	sent   []delivered
	now    time.Time
	specs  []taskSpec
	used   map[string]bool // messages already linked
	ids    []string        // task id per spec, once created
}

// taskEvent is one creation (idx -1) or step of one spec.
type taskEvent struct {
	at        time.Time
	spec, idx int
}

func (r *boardRun) execute(ctx context.Context) error {
	var evs []taskEvent
	for i, sp := range r.specs {
		last := r.now.Add(-sp.created)
		evs = append(evs, taskEvent{at: last, spec: i, idx: -1})
		for j, s := range sp.steps {
			at := r.now.Add(-s.ago)
			if at.Before(last) {
				return fmt.Errorf("devseed: task %q: step %d at %s precedes the previous one", sp.title, j, at.Format(time.RFC3339))
			}
			last = at
			evs = append(evs, taskEvent{at: at, spec: i, idx: j})
		}
	}
	slices.SortStableFunc(evs, func(a, b taskEvent) int {
		return cmp.Or(a.at.Compare(b.at), cmp.Compare(a.spec, b.spec), cmp.Compare(a.idx, b.idx))
	})
	r.ids = make([]string, len(r.specs))
	for _, ev := range evs {
		r.clk.set(ev.at)
		var err error
		if ev.idx < 0 {
			err = r.create(ctx, ev)
		} else {
			err = r.step(ctx, ev)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *boardRun) create(ctx context.Context, ev taskEvent) error {
	sp := r.specs[ev.spec]
	c, err := r.caller(sp.creator)
	if err != nil {
		return err
	}
	in := tsk.CreateInput{Title: sp.title, Body: sp.body, Priority: sp.prio, Scope: sp.scope, Assignee: sp.assignee}
	if sp.due != 0 {
		due := r.now.Add(sp.due)
		in.DueAt = &due
	}
	if sp.link || sp.hint != "" {
		if d, ok := findMessage(r.sent, sp.creator, sp.hint, ev.at, r.used); ok {
			in.LinkMessageID = d.msg.ID
			r.used[d.msg.ID] = true
		}
	}
	t, err := r.board.Create(ctx, c, in)
	if err != nil {
		return fmt.Errorf("devseed: create task %q: %w", sp.title, err)
	}
	r.ids[ev.spec] = t.ID
	return nil
}

func (r *boardRun) step(ctx context.Context, ev taskEvent) error {
	sp := r.specs[ev.spec]
	s := sp.steps[ev.idx]
	if s.sweep {
		if _, err := r.board.Sweep(ctx); err != nil {
			return fmt.Errorf("devseed: sweep for %q: %w", sp.title, err)
		}
		return nil
	}
	c, err := r.caller(s.who)
	if err != nil {
		return err
	}
	up := tsk.UpdateInput{TaskID: r.ids[ev.spec], Action: s.act, Note: s.note, Assignee: s.to}
	if s.act == tsk.ActionLink {
		d, ok := findMessage(r.sent, s.who, "", ev.at, r.used)
		if !ok {
			return nil
		}
		up.MessageID = d.msg.ID
		r.used[d.msg.ID] = true
	}
	if _, err := r.board.Update(ctx, c, up); err != nil {
		return fmt.Errorf("devseed: task %q %s by %s: %w", sp.title, s.act, s.who, err)
	}
	return nil
}

// summarizeBoard counts the tasks by status and health, with their events and
// links.
func summarizeBoard(ctx context.Context, board *tsk.Service, now time.Time) (BoardSummary, error) {
	sum := BoardSummary{Status: map[tsk.Status]int{}, Health: map[tsk.Health]int{}}
	cursor := ""
	for {
		res, err := board.List(ctx, tsk.ListInput{Limit: tsk.MaxListLimit, Cursor: cursor})
		if err != nil {
			return sum, fmt.Errorf("devseed: %w", err)
		}
		for _, t := range res.Items {
			sum.Tasks++
			sum.Status[t.Status]++
			if t.Health != "" {
				sum.Health[t.Health]++
			}
			if t.Status != tsk.StatusFinished && t.DueAt != nil && t.DueAt.Before(now) {
				sum.Overdue++
			}
			d, err := board.Get(ctx, t.ID)
			if err != nil {
				return sum, fmt.Errorf("devseed: %w", err)
			}
			sum.Events += len(d.Events)
			sum.Links += len(d.Links)
		}
		if res.NextCursor == "" {
			return sum, nil
		}
		cursor = res.NextCursor
	}
}

// Print writes the board summary as a few lines.
func (b BoardSummary) Print(w io.Writer) {
	if b.Existing > 0 {
		say(w, "tasks:      %d already on the board, none added\n", b.Existing)
		return
	}
	say(w, "tasks:      %d (%d not started, %d claimed, %d started, %d finished, %d abandoned), %d overdue\n", b.Tasks,
		b.Status[tsk.StatusNotStarted], b.Status[tsk.StatusClaimed], b.Status[tsk.StatusStarted],
		b.Status[tsk.StatusFinished], b.Status[tsk.StatusAbandoned], b.Overdue)
	say(w, "task lease: %d active, %d idle, %d stale, %d decayed; %d events, %d message links\n",
		b.Health[tsk.HealthActive], b.Health[tsk.HealthIdle], b.Health[tsk.HealthStale], b.Health[tsk.HealthAbandoned], b.Events, b.Links)
}
