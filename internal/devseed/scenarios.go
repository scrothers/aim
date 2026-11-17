package devseed

import (
	"cmp"
	"fmt"
	"math/rand/v2"
	"path"
	"slices"
	"strings"
	"time"
)

// step is one message of a thread. A nil from is the operator.
type step struct {
	from     *persona
	sub      string
	to, cc   []string
	subject  string
	body     string
	parent   int
	replyAll bool
	delay    time.Duration
	at       time.Time
}

func (s *step) sender() string {
	switch {
	case s.from == nil:
		return opAddr
	case s.sub != "":
		return s.sub + "@" + s.from.String()
	default:
		return s.from.String()
	}
}

// thread is a conversation: a run of steps whose parents precede them.
type thread struct {
	kind  string
	steps []step
}

// tb builds a thread.
type tb struct{ steps []step }

func (b *tb) send(from *persona, to, cc []string, subject, body string, delay time.Duration) int {
	b.steps = append(b.steps, step{from: from, to: to, cc: cc, subject: subject, body: body, parent: -1, delay: delay})
	return len(b.steps) - 1
}

func (b *tb) reply(parent int, from *persona, sub, body string, delay time.Duration) int {
	p := b.steps[parent]
	b.steps = append(b.steps, step{
		from: from, sub: sub, to: []string{p.sender()}, subject: reSubject(p.subject), body: body, parent: parent, delay: delay,
	})
	return len(b.steps) - 1
}

// replyAll must not be used by subagents or by the sender of the parent.
func (b *tb) replyAll(parent int, from *persona, body string, delay time.Duration) int {
	p := b.steps[parent]
	b.steps = append(b.steps, step{
		from: from, subject: reSubject(p.subject), body: body, parent: parent, replyAll: true, delay: delay,
	})
	return len(b.steps) - 1
}

func reSubject(s string) string {
	if strings.HasPrefix(s, "Re: ") {
		return s
	}
	return "Re: " + s
}

// md turns the ¦ placeholder into a backtick so bodies can live in raw strings.
func md(s string) string { return strings.ReplaceAll(s, "¦", "`") }

func fence(lang, text string) string { return "```" + lang + "\n" + text + "\n```" }

// gen produces threads for the world.
type gen struct {
	rng    *rand.Rand
	world  []*persona
	byAddr map[string]*persona
	flav   map[string]flavor
	scopes []string
	now    time.Time
	days   int
	cutoff time.Time
	prs    map[string]int
}

func newGen(rng *rand.Rand, world []*persona, now time.Time, days int) *gen {
	g := &gen{
		rng: rng, world: world, byAddr: make(map[string]*persona, len(world)), flav: flavors(),
		now: now, days: days, cutoff: now.Add(-time.Duration(days) * 24 * time.Hour / 2), prs: map[string]int{},
	}
	for _, p := range world {
		g.byAddr[p.String()] = p
	}
	for k := range g.flav {
		g.scopes = append(g.scopes, k)
	}
	slices.Sort(g.scopes)
	return g
}

func pick[T any](r *rand.Rand, s []T) T { return s[r.IntN(len(s))] }

func (g *gen) chance(p float64) bool { return g.rng.Float64() < p }

func (g *gen) between(lo, hi int) int { return lo + g.rng.IntN(hi-lo+1) }

// mins returns a random duration between lo and hi minutes.
func (g *gen) mins(lo, hi int) time.Duration {
	return time.Duration(g.between(lo*60, hi*60)) * time.Second
}

func (g *gen) sample(s []string, n int) []string {
	idx := g.rng.Perm(len(s))
	out := make([]string, 0, n)
	for _, i := range idx[:min(n, len(s))] {
		out = append(out, s[i])
	}
	return out
}

func (g *gen) sha() string { return fmt.Sprintf("%07x", g.rng.Uint32()&0xfffffff) }

func (g *gen) prNum(scope string) int {
	if _, ok := g.prs[scope]; !ok {
		g.prs[scope] = g.between(100, 700)
	}
	g.prs[scope] += g.between(1, 6)
	return g.prs[scope]
}

func (g *gen) active(p *persona, t time.Time) bool { return !p.retired || t.Before(g.cutoff) }

// find returns an active persona with the role in exactly this scope
// (host/org for org level, host/org/repo for a repo), weighted by chattiness.
func (g *gen) find(t time.Time, scope string, roles ...string) *persona {
	var cands []*persona
	total := 0
	for _, p := range g.world {
		if p.scope() == scope && slices.Contains(roles, p.role) && g.active(p, t) {
			cands = append(cands, p)
			total += p.weight
		}
	}
	if len(cands) == 0 {
		return nil
	}
	n := g.rng.IntN(total)
	for _, p := range cands {
		if n -= p.weight; n < 0 {
			return p
		}
	}
	return cands[0]
}

// scopeWith picks a repo scope where ok holds.
func (g *gen) scopeWith(ok func(scope string) bool) string {
	var cands []string
	for _, s := range g.scopes {
		if ok(s) {
			cands = append(cands, s)
		}
	}
	if len(cands) == 0 {
		return ""
	}
	return pick(g.rng, cands)
}

func (g *gen) coordinatorOf(p *persona) *persona { return g.byAddr[p.reports] }

// coordinators returns the org coordinators active at t.
func (g *gen) coordinators(t time.Time) []*persona {
	var out []*persona
	for _, p := range g.world {
		if p.role == roleCoordinator && g.active(p, t) {
			out = append(out, p)
		}
	}
	return out
}

func (g *gen) reportsOf(c *persona, t time.Time) []*persona {
	var out []*persona
	for _, p := range g.world {
		if p.reports == c.String() && g.active(p, t) {
			out = append(out, p)
		}
	}
	return out
}

func strs(ps ...*persona) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		if p != nil {
			out = append(out, p.String())
		}
	}
	return out
}

func (g *gen) maybe(p float64, ps ...*persona) []string {
	if g.chance(p) {
		return strs(ps...)
	}
	return nil
}

func (g *gen) maybeOperator(p float64) []string {
	if g.chance(p) {
		return []string{opAddr}
	}
	return nil
}

func slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func prURL(scope string, n int) string {
	if strings.HasPrefix(scope, "gitlab.com/") {
		return fmt.Sprintf("https://%s/-/merge_requests/%d", scope, n)
	}
	return fmt.Sprintf("https://%s/pull/%d", scope, n)
}

func repoName(scope string) string { return path.Base(scope) }

func failure(lang, scope, test, file string, line, got, want int) string {
	switch lang {
	case "go":
		return fmt.Sprintf("--- FAIL: %s (0.42s)\n    %s:%d: got %d, want %d\nFAIL\nFAIL\t%s/%s\t1.208s", test, path.Base(file), line, got, want, scope, path.Dir(file))
	case "ts":
		return fmt.Sprintf(" FAIL  %s\nAssertionError: expected %d to equal %d\n  at %s:%d:%d", test, got, want, file, line, 17)
	case "kt":
		return fmt.Sprintf("%s FAILED\n    java.lang.AssertionError: expected:<%d> but was:<%d>\n        at %s:%d", test, want, got, file, line)
	case "tf":
		return fmt.Sprintf("Error: %s failed\n  on %s line %d:\n  expected %d, found %d", test, file, line, want, got)
	default:
		return fmt.Sprintf("FAILED %s - AssertionError: assert %d == %d\n%s:%d: AssertionError", test, got, want, file, line)
	}
}

var findings = []string{
	"Error is swallowed; wrap it and return it to the caller",
	"Context is not propagated into the downstream call",
	"Loop never terminates if the upstream stays open",
	"Name does not match the neighbouring handlers",
	"This allocates on every call; hoist it out of the hot path",
	"No test for the empty-input case",
	"Shared state is mutated without a lock",
	"Magic number; make it a named constant",
}

// prReview: builder asks reviewer, reviewer asks for changes, builder fixes,
// reviewer approves; an occasional subagent applies nits.
func (g *gen) prReview(t time.Time) *thread {
	scope := g.scopeWith(func(s string) bool {
		return g.find(t, s, roleBuilder) != nil && g.find(t, s, roleReviewer) != nil
	})
	if scope == "" {
		return nil
	}
	builder, reviewer := g.find(t, scope, roleBuilder), g.find(t, scope, roleReviewer)
	fl := g.flav[scope]
	pr, title := g.prNum(scope), pick(g.rng, fl.titles)
	files := g.sample(fl.files, g.between(2, 4))
	var fl2 strings.Builder
	for _, f := range files {
		fmt.Fprintf(&fl2, "- ¦%s¦\n", f)
	}
	var b tb
	first := b.send(builder, strs(reviewer), g.maybe(0.3, g.coordinatorOf(builder)),
		fmt.Sprintf("Review requested: #%d %s", pr, title),
		md(fmt.Sprintf("## #%d %s\n\nBranch ¦%s¦, +%d/-%d across %d files.\n\n### Changes\n\n%s\n### Checklist\n\n- [x] Tests added\n- [x] CI green on ¦%s¦\n- [ ] Docs updated\n\n%s",
			pr, title, "feat/"+slug(title), g.between(20, 400), g.between(2, 120), len(files), fl2.String(), g.sha(), prURL(scope, pr))),
		0)
	var tbl strings.Builder
	tbl.WriteString("| File | Line | Finding |\n|------|------|---------|\n")
	for _, f := range files[:min(len(files), g.between(1, 3))] {
		fmt.Fprintf(&tbl, "| ¦%s¦ | %d | %s |\n", f, g.between(10, 240), pick(g.rng, findings))
	}
	rev := b.reply(first, reviewer, "", md(fmt.Sprintf("Thanks, a few things before I can approve **#%d**:\n\n%s\nEverything else looks good.", pr, tbl.String())), g.mins(15, 150))
	last := rev
	if g.chance(0.35) && len(reviewer.subagents) > 0 {
		sub := reviewer.subagents[0]
		b.steps = append(b.steps, step{
			from: reviewer, sub: sub, to: strs(builder), cc: strs(reviewer), parent: rev,
			subject: "Re: " + b.steps[first].subject, delay: g.mins(5, 30),
			body: md(fmt.Sprintf("Applied the mechanical nits from the review of #%d in ¦%s¦:\n\n%s", pr, g.sha(),
				fence("diff", fmt.Sprintf("-\tif err != nil { return nil }\n+\tif err != nil { return fmt.Errorf(\"%s: %%w\", err) }", slug(title))))),
		})
		last = len(b.steps) - 1
	}
	if g.chance(0.85) {
		fix := b.reply(last, builder, "", md(fmt.Sprintf("Pushed ¦%s¦ addressing the feedback.\n\n- [x] wrapped the error\n- [x] added ¦%s¦\n- [ ] follow-up for the larger refactor tracked separately", g.sha(), pick(g.rng, fl.tests))), g.mins(20, 240))
		if g.chance(0.8) {
			ap := b.reply(fix, reviewer, "", "LGTM. Approved; merge when CI is green.", g.mins(10, 90))
			if g.chance(0.5) {
				b.send(builder, strs(reviewer), strs(g.coordinatorOf(builder)), fmt.Sprintf("Merged: #%d %s", pr, title),
					md(fmt.Sprintf("Merged #%d into ¦main¦ as ¦%s¦. Deploying with the next train.", pr, g.sha())), g.mins(5, 40))
				b.steps[len(b.steps)-1].parent = ap
			}
		}
	}
	return &thread{kind: "pr-review", steps: b.steps}
}

// ciFailure: qa or triage reports a failing test to the builder.
func (g *gen) ciFailure(t time.Time) *thread {
	scope := g.scopeWith(func(s string) bool {
		return g.find(t, s, roleBuilder) != nil && g.find(t, s, roleQA, roleTriage) != nil
	})
	if scope == "" {
		return nil
	}
	builder, rep := g.find(t, scope, roleBuilder), g.find(t, scope, roleQA, roleTriage)
	fl := g.flav[scope]
	test, file := pick(g.rng, fl.tests), pick(g.rng, fl.files)
	var b tb
	first := b.send(rep, strs(builder), g.maybe(0.4, g.coordinatorOf(rep)),
		fmt.Sprintf("CI failing on main: %s", test),
		md(fmt.Sprintf("%s is red on ¦main¦ since ¦%s¦ (%d of the last 5 runs).\n\n%s\n\n**Repro**\n\n%s\n\nLogs: https://ci.%s/runs/%d",
			"¦"+test+"¦", g.sha(), g.between(3, 5), fence("", failure(fl.lang, scope, test, file, g.between(20, 300), g.between(1, 9), g.between(1, 9))),
			fence("sh", "git checkout main && task test"), strings.SplitN(scope, "/", 2)[0], g.between(1000, 9999))),
		0)
	r1 := b.reply(first, builder, "", md(fmt.Sprintf("Bisected it to ¦%s¦ touching ¦%s¦. Fix coming.", g.sha(), file)), g.mins(10, 90))
	if g.chance(0.7) {
		r2 := b.replyAll(r1, rep, "Confirmed it is deterministic locally. I will hold the release candidate until it is green.", g.mins(5, 60))
		if g.chance(0.75) {
			pr := g.prNum(scope)
			b.replyAll(r2, builder, md(fmt.Sprintf("Fixed in #%d (%s). Merging once CI passes.", pr, prURL(scope, pr))), g.mins(20, 180))
		}
	}
	return &thread{kind: "ci-failure", steps: b.steps}
}

// deploy: the deployer proposes a rollout; security and the coordinator weigh in.
func (g *gen) deploy(t time.Time) *thread {
	scope := acme + "/infra"
	dep, sec := g.find(t, scope, roleDeployer), g.find(t, scope, roleSecurity)
	coord := g.find(t, acme, roleCoordinator)
	if dep == nil || coord == nil {
		return nil
	}
	svc := pick(g.rng, []string{"api", "web", "mobile-gateway"})
	ver := fmt.Sprintf("v%d.%d.%d", 1, g.between(2, 14), g.between(0, 9))
	env := pick(g.rng, []string{"staging", "staging", "production"})
	var b tb
	first := b.send(dep, strs(coord), append(strs(sec), g.maybe(0.5, g.find(t, acme, roleRelease))...),
		fmt.Sprintf("Deploy plan: %s %s to %s", svc, ver, env),
		md(fmt.Sprintf("Plan for **%s** ¦%s¦ to **%s**:\n\n- [x] Terraform plan reviewed (no destructive changes)\n- [x] Database migration is backwards compatible\n- [ ] Security sign-off\n- [ ] Approved window\n\n%s\n\nRollback: ¦helm rollback %s 1¦.",
			svc, ver, env, fence("sh", fmt.Sprintf("terraform -chdir=terraform/%s plan -out=tfplan\nhelm upgrade %s ./charts/%s --set image.tag=%s", env, svc, svc, ver)), svc)),
		0)
	last := first
	if sec != nil {
		last = b.replyAll(first, sec, "Approved with one condition: confirm the new IAM role is scoped to the exports bucket only.", g.mins(20, 180))
	}
	r := b.replyAll(last, coord, "Go. Keep the window under 30 minutes and post the health check results here.", g.mins(10, 90))
	if g.chance(0.85) {
		b.replyAll(r, dep, md(fmt.Sprintf("Deployed ¦%s¦ to %s. Health checks green, p95 latency %dms (was %dms).", ver, env, g.between(90, 180), g.between(100, 200))), g.mins(30, 240))
	}
	return &thread{kind: "deploy", steps: b.steps}
}

type topic struct {
	subject string
	context string
	options []string
}

func topics() []topic {
	return []topic{
		{"Decision needed: upgrade to PostgreSQL 17 this sprint?", "The 15.x line loses support soon and the upgrade needs a short write freeze.", []string{"Upgrade now (2 day freeze)", "Upgrade staging only and wait", "Defer to next quarter"}},
		{"Approval needed: extend the CI runner budget by $400/month", "Queue times have doubled since the monorepo split; builds wait ~11 minutes for a runner.", []string{"Approve $400/month", "Approve $200/month and cache more", "Decline"}},
		{"Blocked: need read access to the billing sandbox", "Reproducing the invoice discrepancy needs production-shaped data and I only have fixtures.", []string{"Grant read-only access", "Share an anonymized dump instead"}},
		{"Risk: vendor SDK deprecated, end of life in 60 days", "The maintainer announced EOL. Alternatives exist but need roughly two weeks of migration.", []string{"Start migrating now", "Fork and maintain it", "Accept the risk until Q1"}},
		{"Decision needed: ship the beta to design partners on Friday?", "Two known issues remain, both cosmetic. Telemetry shows the crash rate at 0.3%.", []string{"Ship Friday", "Ship Tuesday after the fixes", "Hold"}},
		{"Request: pause nightly jobs during the audit window", "The auditors want a stable snapshot between the 3rd and 5th.", []string{"Pause all nightly jobs", "Pause only data exports"}},
		{"Heads-up: the flaky e2e suite costs about 3 hours a day", "Retries mask real failures. I would like a day to quarantine and fix the worst five tests.", []string{"Approve a stabilization day", "Keep shipping, fix opportunistically"}},
		{"Approval needed: rotate the shared deploy credentials", "They are older than the 90 day policy and live in three places.", []string{"Rotate today during business hours", "Rotate this weekend"}},
	}
}

var operatorAnswers = []string{
	"Approved. Go with the first option and keep me posted.",
	"Wait for next quarter. Revisit in January.",
	"Granted. Check your inbox in a few minutes.",
	"Do the second option first, then report back with numbers.",
	"Hold off until Monday; I want to talk to the team first.",
}

// escalation: a coordinator asks the operator for a decision.
func (g *gen) escalation(t time.Time) *thread {
	cs := g.coordinators(t)
	if len(cs) == 0 {
		return nil
	}
	c := pick(g.rng, cs)
	tp := pick(g.rng, topics())
	var opts strings.Builder
	for i, o := range tp.options {
		fmt.Fprintf(&opts, "%d. %s\n", i+1, o)
	}
	var b tb
	first := b.send(c, []string{opAddr}, nil, tp.subject,
		md(fmt.Sprintf("%s\n\n**Context**\n\n%s\n\n**Options**\n\n%s\nMy recommendation is option %d. I need an answer by end of day %s.",
			"Operator,", tp.context, opts.String(), g.between(1, len(tp.options)), pick(g.rng, []string{"today", "tomorrow", "Friday"}))), 0)
	if g.chance(0.6) {
		ans := b.reply(first, nil, "", pick(g.rng, operatorAnswers), g.mins(20, 600))
		if g.chance(0.6) {
			b.reply(ans, c, "", "Understood, proceeding. I will confirm when it is done.", g.mins(5, 60))
		}
	}
	return &thread{kind: "escalation", steps: b.steps}
}

var tasks = []struct{ ask, report string }{
	{"Compare the two retry libraries and recommend one", "Both are maintained. ¦backoff/v4¦ has jitter built in; the other needs a wrapper. Recommend ¦backoff/v4¦."},
	{"Find every caller of the deprecated export endpoint", "Found 14 call sites in 6 packages; 9 are tests. List attached below."},
	{"Check browser support for the container queries we plan to use", "Supported in all evergreen browsers; Safari 15 needs a fallback. 1.8% of our traffic."},
	{"Summarize last week's incident timeline", "Detection took 11 minutes; the alert fired on the wrong threshold. Three follow-ups proposed."},
	{"Profile the nightly import and find the slowest stage", "Stage 3 (dedupe) is 71% of wall time; an index on ¦(tenant_id, hash)¦ should cut it by about 5x."},
}

// subagentTask: an identity delegates to its subagent, which reports back.
func (g *gen) subagentTask(t time.Time) *thread {
	var cands []*persona
	for _, p := range g.world {
		if len(p.subagents) > 0 && g.active(p, t) {
			cands = append(cands, p)
		}
	}
	if len(cands) == 0 {
		return nil
	}
	p := pick(g.rng, cands)
	sub := pick(g.rng, p.subagents)
	tk := pick(g.rng, tasks)
	subAddr := sub + "@" + p.String()
	var b tb
	first := b.send(p, []string{subAddr}, nil, "Task: "+tk.ask,
		md(fmt.Sprintf("Please do the following and reply with a short summary:\n\n1. %s\n2. Note anything surprising\n3. Keep it under a page\n\nContext: %s", tk.ask, p.desc)), 0)
	rep := b.reply(first, p, sub, md(fmt.Sprintf("**Result**\n\n%s\n\n| Item | Status |\n|------|--------|\n| Analysis | done |\n| Follow-up needed | %s |", tk.report, pick(g.rng, []string{"yes", "no", "maybe"}))), g.mins(3, 40))
	b.steps[rep].cc = strs(g.coordinatorOf(p))
	if g.chance(0.6) {
		b.reply(rep, p, "", "Thanks, this is what I needed. Folding it into the plan.", g.mins(5, 60))
	}
	return &thread{kind: "subagent", steps: b.steps}
}

var advisories = []struct{ id, pkg, sev string }{
	{"GHSA-4f7c-xq2p-9m8v", "golang.org/x/net", "7.5"},
	{"GHSA-9hw2-jr7c-5q3x", "jsonwebtoken", "8.1"},
	{"GHSA-2c5r-8w6m-fx4p", "pillow", "6.1"},
	{"GHSA-7vq3-m2hf-r9gj", "openssl 3.0.x", "7.4"},
}

// securityAdvisory: security flags a vulnerable dependency.
func (g *gen) securityAdvisory(t time.Time) *thread {
	sec := g.find(t, acme+"/infra", roleSecurity)
	coord := g.find(t, acme, roleCoordinator)
	builder := g.find(t, acme+"/api", roleBuilder)
	if sec == nil || coord == nil || builder == nil {
		return nil
	}
	a := pick(g.rng, advisories)
	var b tb
	first := b.send(sec, strs(coord), strs(builder, g.find(t, acme+"/api", roleReviewer)),
		fmt.Sprintf("Advisory %s in %s (CVSS %s)", a.id, a.pkg, a.sev),
		md(fmt.Sprintf("A new advisory affects us.\n\n| Package | Affected | Fixed in | Status |\n|---------|----------|----------|--------|\n| ¦%s¦ | < 2.%d | 2.%d | **open** |\n\nExploitability is limited to authenticated callers, but please patch within the week.\n\nDetails: https://github.com/advisories/%s", a.pkg, g.between(3, 8), g.between(9, 14), a.id)), 0)
	r := b.reply(first, builder, "", md(fmt.Sprintf("Bumped in #%d; running the full suite now.", g.prNum(acme+"/api"))), g.mins(30, 300))
	if g.chance(0.6) {
		b.replyAll(r, coord, "Thanks. Security, please verify the lockfile after merge and close the advisory.", g.mins(15, 120))
	}
	return &thread{kind: "security", steps: b.steps}
}

// standup: a coordinator sets priorities for the reports, who answer reply-all.
func (g *gen) standup(t time.Time) *thread {
	cs := g.coordinators(t)
	if len(cs) == 0 {
		return nil
	}
	c := pick(g.rng, cs)
	rs := g.reportsOf(c, t)
	if len(rs) < 2 {
		return nil
	}
	g.rng.Shuffle(len(rs), func(i, j int) { rs[i], rs[j] = rs[j], rs[i] })
	rs = rs[:min(len(rs), 4)]
	var b tb
	first := b.send(c, strs(rs...), g.maybeOperator(0.3), "Priorities for this week",
		md(fmt.Sprintf("Team,\n\n1. Land the in-flight PRs before cutting the next release\n2. Fix anything red on ¦main¦ first\n3. %s\n\n- [ ] Update your status in the tracker by Wednesday", pick(g.rng, []string{"Pay down the flaky test backlog", "Prepare the design partner demo", "Review the open security advisories", "Finish the migration cleanup"}))), 0)
	for i, r := range rs[:g.between(1, min(3, len(rs)))] {
		b.replyAll(first, r, fmt.Sprintf("On it. %s", pick(g.rng, []string{"Should be done by Wednesday.", "I am blocked on review, will ping.", "Already half done.", "Need a quick sync on scope."})), g.mins(10+i*20, 200+i*60))
	}
	return &thread{kind: "standup", steps: b.steps}
}

// docsDrift: the docs writer reports documentation that no longer matches code.
func (g *gen) docsDrift(t time.Time) *thread {
	scope := labs + "/notebook"
	d, rev := g.find(t, scope, roleDocs), g.find(t, scope, roleReviewer)
	if d == nil || rev == nil {
		return nil
	}
	fl := g.flav[scope]
	f := pick(g.rng, fl.files)
	var b tb
	first := b.send(d, strs(rev), g.maybe(0.3, g.coordinatorOf(d)), "Docs drift: kernel protocol reference is out of date",
		md(fmt.Sprintf("While rewriting the tutorial I found the reference no longer matches ¦%s¦:\n\n- ¦restart¦ now returns the buffered output\n- ¦interrupt¦ takes an optional ¦timeout_ms¦\n- The example in the quickstart fails with ¦E_NOKERNEL¦\n\n%s", f, fence("json", `{"method": "interrupt", "params": {"timeout_ms": 500}}`))), 0)
	b.reply(first, rev, "", md(fmt.Sprintf("Good catch. #%d updates the code comments; please regenerate the reference from them.", g.prNum(scope))), g.mins(30, 300))
	return &thread{kind: "docs", steps: b.steps}
}

// crossOrg: two coordinators coordinate across orgs, operator in cc.
func (g *gen) crossOrg(t time.Time) *thread {
	l, a := g.find(t, labs, roleCoordinator), g.find(t, acme, roleCoordinator)
	if l == nil || a == nil {
		return nil
	}
	var b tb
	first := b.send(l, strs(a), g.maybeOperator(0.5), "Can radar read the orders API schema?",
		md("We would like to score anomalies on order volume. Could acme/api expose the ¦/v2/orders/stats¦ endpoint to a read-only token for radar?\n\n- Rate: about 1 request/minute\n- Fields: ¦tenant_id¦, ¦count¦, ¦total¦\n- No PII"), 0)
	r := b.reply(first, a, "", "Fine in principle. Security needs to approve the token scope; I will ask them today.", g.mins(30, 360))
	if g.chance(0.6) {
		b.reply(r, l, "", "Thanks. We can start with staging data while that is pending.", g.mins(15, 120))
	}
	return &thread{kind: "cross-org", steps: b.steps}
}

type scenario struct {
	weight int
	fn     func(*gen, time.Time) *thread
}

func scenarioTable() []scenario {
	return []scenario{
		{6, (*gen).prReview}, {3, (*gen).ciFailure}, {2, (*gen).deploy}, {3, (*gen).escalation},
		{3, (*gen).subagentTask}, {1, (*gen).securityAdvisory}, {2, (*gen).standup}, {1, (*gen).docsDrift}, {1, (*gen).crossOrg},
	}
}

func (g *gen) randomScenario() func(*gen, time.Time) *thread {
	tbl := scenarioTable()
	total := 0
	for _, s := range tbl {
		total += s.weight
	}
	n := g.rng.IntN(total)
	for _, s := range tbl {
		if n -= s.weight; n < 0 {
			return s.fn
		}
	}
	return tbl[0].fn
}

// randomTime picks a moment in the window, weighted toward recent days and
// working hours.
func (g *gen) randomTime() time.Time {
	var sum float64
	for i := range g.days {
		sum += pow08(i)
	}
	for range 30 {
		age := 0
		r := g.rng.Float64() * sum
		for i := range g.days {
			if r -= pow08(i); r < 0 {
				age = i
				break
			}
		}
		hour := g.between(8, 18)
		if g.chance(0.1) {
			hour = g.rng.IntN(24)
		}
		y, m, d := g.now.Date()
		t := time.Date(y, m, d-age, hour, g.rng.IntN(60), g.rng.IntN(60), 0, g.now.Location())
		if t.Before(g.now.Add(-2 * time.Minute)) {
			return t
		}
	}
	return g.now.Add(-g.mins(5, 180))
}

func pow08(n int) float64 {
	v := 1.0
	for range n {
		v *= 0.8
	}
	return v
}

// fixed returns the scheduled posts: one daily summary per org and day,
// newest first, then release notes.
func (g *gen) fixed() []*thread {
	var out []*thread
	y, m, d := g.now.Date()
	for age := range g.days {
		day := time.Date(y, m, d-age, 17, 30+g.rng.IntN(25), 0, 0, g.now.Location())
		if day.After(g.now.Add(-2 * time.Minute)) {
			continue
		}
		for _, c := range g.coordinators(day) {
			out = append(out, g.dailySummary(c, day))
		}
	}
	rel := g.find(g.now.Add(-time.Hour), acme, roleRelease)
	if rel != nil {
		for i, age := range []int{1, g.days / 2, g.days - 1} {
			if i > 0 && age == 0 {
				continue
			}
			at := time.Date(y, m, d-age, 14, g.rng.IntN(50), 0, 0, g.now.Location())
			if at.Before(g.now.Add(-2*time.Minute)) && at.After(g.now.Add(-time.Duration(g.days)*24*time.Hour)) {
				out = append(out, g.releaseNotes(rel, at, i))
			}
		}
	}
	return out
}

func (g *gen) dailySummary(c *persona, at time.Time) *thread {
	var scopes []string
	for _, s := range g.scopes {
		if strings.HasPrefix(s, c.org()+"/") {
			scopes = append(scopes, s)
		}
	}
	var tbl strings.Builder
	tbl.WriteString("| Repo | PRs merged | Open PRs | CI |\n|------|-----------:|---------:|----|\n")
	for _, s := range scopes {
		fmt.Fprintf(&tbl, "| %s | %d | %d | %s |\n", repoName(s), g.between(0, 7), g.between(1, 9), pick(g.rng, []string{"green", "green", "green", "flaky", "red"}))
	}
	cc := []string(nil)
	if c.org() == acme {
		cc = []string{opAddr}
	}
	var b tb
	b.send(c, []string{c.org()}, cc, fmt.Sprintf("Daily summary %s", at.Format("Mon Jan 2")),
		md(fmt.Sprintf("## %s daily summary\n\n%s\n**Highlights**\n\n- %s\n- %s\n\n**Tomorrow**\n\n- [ ] %s\n- [ ] Review the open advisories",
			path.Base(c.org()), tbl.String(),
			pick(g.rng, []string{"Pagination work landed", "Staging is stable", "Two flaky tests quarantined", "Design partner feedback triaged"}),
			pick(g.rng, []string{"Release candidate cut", "Migration dry run passed", "Docs refresh merged", "Dependency bumps merged"}),
			pick(g.rng, []string{"Cut the release candidate", "Finish the dependency bumps", "Close out the audit items"}))), 0)
	b.steps[0].at = at
	return &thread{kind: "summary", steps: b.steps}
}

func (g *gen) releaseNotes(rel *persona, at time.Time, n int) *thread {
	ver := fmt.Sprintf("v1.%d.0", 10+n)
	var b tb
	b.send(rel, []string{acme}, []string{opAddr}, "Release notes: "+ver,
		md(fmt.Sprintf("# acme %s\n\n## Features\n\n- Cursor pagination for ¦/v2/orders¦ ([#%d](%s))\n- Virtualized orders table (#%d)\n\n## Fixes\n\n- Checkout dialog traps focus (#%d)\n- Crash when resuming a backgrounded checkout (#%d)\n\n## Upgrade notes\n\n%s\n\nFull changelog: https://github.com/acme/api/compare/v1.%d.0...%s",
			ver, g.between(500, 800), prURL(acme+"/api", 600), g.between(500, 800), g.between(500, 800), g.between(500, 800),
			fence("sh", "task migrate && task deploy -- --env staging"), 9+n, ver)), 0)
	b.steps[0].at = at
	return &thread{kind: "release", steps: b.steps}
}

// plan generates threads totalling exactly n messages.
func (g *gen) plan(n int) []*thread {
	var out []*thread
	total := 0
	add := func(th *thread) {
		th.steps = th.steps[:min(len(th.steps), n-total)]
		total += len(th.steps)
		out = append(out, th)
	}
	for _, th := range g.fixed() {
		if total >= n/4 {
			break
		}
		add(th)
	}
	for attempts := 0; total < n && attempts < 100*n; attempts++ {
		t := g.randomTime()
		if th := g.randomScenario()(g, t); th != nil {
			th.steps[0].at = t
			add(th)
		}
	}
	for _, th := range out {
		g.schedule(th)
	}
	slices.SortStableFunc(out, func(a, b *thread) int { return cmp.Compare(a.steps[0].at.UnixNano(), b.steps[0].at.UnixNano()) })
	return out
}

// schedule assigns each step its time from its parent (or the previous step),
// shifting the thread earlier if it would end in the future.
func (g *gen) schedule(th *thread) {
	limit := g.now.Add(-time.Minute)
	for i := range th.steps {
		s := &th.steps[i]
		if i == 0 {
			continue
		}
		base := th.steps[i-1].at
		if s.parent >= 0 {
			base = th.steps[s.parent].at
		}
		if s.at.IsZero() {
			s.at = base.Add(s.delay)
		}
		if s.at.Before(base) || s.at.Equal(base) {
			s.at = base.Add(time.Minute)
		}
		// Keep time monotonic along the thread so ordering matches seq.
		if prev := th.steps[i-1].at; s.at.Before(prev) {
			s.at = prev.Add(time.Minute)
		}
	}
	last := th.steps[len(th.steps)-1].at
	if last.After(limit) {
		shift := last.Sub(limit)
		for i := range th.steps {
			th.steps[i].at = th.steps[i].at.Add(-shift)
		}
	}
}
