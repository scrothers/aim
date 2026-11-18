package devseed

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/wire"
)

const (
	liveAgents     = 12
	callTimeout    = 10 * time.Second
	shutdownBudget = 5 * time.Second
)

// Online is one simulated agent that signed on.
type Online struct {
	Address string
	Client  string
	Machine string
	Away    string
}

// LiveConfig configures RunLive.
type LiveConfig struct {
	// Addr is the daemon socket path (unix:/path, or any path) or host:port.
	Addr string
	Seed uint64
	// Chatter is the interval between simulated actions; zero disables them.
	Chatter time.Duration
	Out     io.Writer
	// HTTP is the daemon's HTTP API address (host:port); it is used to read
	// and, with EnableApprovals, turn on remote approvals. Empty skips both.
	HTTP string
	// EnableApprovals turns remote approvals on, with ApprovalTimeout.
	EnableApprovals bool
	// ApprovalTimeout is the approval timeout to set; zero means 5 minutes.
	ApprovalTimeout time.Duration
	// ApprovalEvery is the interval between simulated permission prompts;
	// zero disables them.
	ApprovalEvery time.Duration
	// QuietAfter is how long one agent keeps the task it claimed at startup
	// before it disconnects for good, leaving its lease to decay at the
	// daemon's task decay thresholds. Zero disconnects it at once.
	QuietAfter time.Duration
	// OnReady, if set, is called once every agent has signed on.
	OnReady func([]Online)
}

var clientVersions = map[string]string{
	"claude-code": "2.1.0", "codex": "0.46.0", "gemini-cli": "0.8.1", "opencode": "0.15.3", "grok-cli": "0.3.2",
	"cline": "3.2.0", "kilo-code": "4.1.0", "deepseek-cli": "0.2.4", "mcp-inspector": "0.16.0",
}

var awayMessages = []string{
	"reviewing the open PRs", "heads down on a migration", "waiting on CI", "running the test suite",
	"writing the changelog", "triaging new issues", "back in 15 minutes", "pairing on a flaky test",
}

type agent struct {
	p    *persona
	away string
	conn net.Conn
	cs   *mcp.ClientSession
	// signedOn is true while the agent is connected and signed on.
	signedOn bool
	// pid is a stable fake agent process id, shared by MCP and hook
	// connections so the daemon can join them.
	pid int
	// quiet marks the agent that claimed a task and stops acting.
	quiet bool
}

func network(addr string) (string, string) {
	if rest, ok := strings.CutPrefix(addr, "unix:"); ok {
		return "unix", rest
	}
	if strings.Contains(addr, "/") || !strings.Contains(addr, ":") {
		return "unix", addr
	}
	return "tcp", addr
}

func (a *agent) preamble() wire.Preamble {
	p := a.p
	repo := p.addr.Repo
	if repo == "" {
		repo = "org"
	}
	return wire.Preamble{
		SessionID: p.sessionID(), Machine: p.machine, ProjectDir: "/home/dev/src/" + p.addr.Org + "/" + repo,
		GitRemote: "https://" + p.scope() + ".git", Namespace: p.scope(), AgentPID: a.pid,
	}
}

// connect dials the daemon, sends the preamble and runs the MCP handshake
// with this agent's client name.
func (a *agent) connect(ctx context.Context, addr string) error {
	nw, target := network(addr)
	var d net.Dialer
	conn, err := d.DialContext(ctx, nw, target)
	if err != nil {
		return fmt.Errorf("devseed: dial %s: %w", addr, err)
	}
	if err := wire.WritePreamble(conn, a.preamble()); err != nil {
		_ = conn.Close()
		return fmt.Errorf("devseed: %w", err)
	}
	impl := &mcp.Implementation{Name: a.p.client, Version: clientVersions[a.p.client]}
	cs, err := mcp.NewClient(impl, nil).Connect(ctx, &mcp.IOTransport{Reader: conn, Writer: conn}, nil)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("devseed: mcp connect %s: %w", a.p, err)
	}
	a.conn, a.cs = conn, cs
	return nil
}

func (a *agent) call(ctx context.Context, tool string, args any) error {
	_, err := a.callRes(ctx, tool, args)
	return err
}

func (a *agent) callRes(ctx context.Context, tool string, args any) (*mcp.CallToolResult, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	res, err := a.cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		return nil, fmt.Errorf("devseed: %s %s: %w", a.p, tool, err)
	}
	if res.IsError {
		return nil, fmt.Errorf("devseed: %s %s: tool error: %s", a.p, tool, textOf(res))
	}
	return res, nil
}

func textOf(res *mcp.CallToolResult) string {
	var parts []string
	for _, c := range res.Content {
		if t, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, t.Text)
		}
	}
	return strings.Join(parts, " ")
}

func (a *agent) signOn(ctx context.Context) error {
	if err := a.call(ctx, "agent_sign_on", map[string]any{"address": a.p.String(), "away": a.away}); err != nil {
		return err
	}
	a.signedOn = true
	return nil
}

func (a *agent) signOff(ctx context.Context) error {
	a.signedOn = false
	return a.call(ctx, "agent_sign_off", map[string]any{})
}

func (a *agent) disconnect() {
	a.signedOn = false
	if a.cs != nil {
		_ = a.cs.Close()
		a.cs = nil
	}
	if a.conn != nil {
		_ = a.conn.Close()
		a.conn = nil
	}
}

func (a *agent) send(ctx context.Context, to []string, subject, body string) error {
	return a.call(ctx, "mail_send", map[string]any{"to": to, "subject": subject, "body": body})
}

// chooseAgents picks the simulated agents: every org coordinator, then a
// seeded sample of the rest.
func chooseAgents(rng *rand.Rand, world []*persona, n int) []*persona {
	var coords, rest []*persona
	for _, p := range world {
		switch {
		case p.retired:
		case p.role == roleCoordinator:
			coords = append(coords, p)
		default:
			rest = append(rest, p)
		}
	}
	rng.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
	out := append(coords, rest...)
	return out[:min(n, len(out))]
}

// RunLive connects simulated agents to a running daemon and keeps them
// online until ctx ends, then disconnects them.
func RunLive(ctx context.Context, cfg LiveConfig) error {
	if cfg.Out == nil {
		cfg.Out = io.Discard
	}
	// Approval prompts log from their own goroutines.
	cfg.Out = &lockedWriter{w: cfg.Out}
	world, err := buildWorld()
	if err != nil {
		return err
	}
	rng := rand.New(rand.NewPCG(cfg.Seed, 0xc2b2ae3d27d4eb4f))
	var agents []*agent
	for i, p := range chooseAgents(rng, world, liveAgents) {
		agents = append(agents, &agent{p: p, away: pick(rng, awayMessages), pid: 41000 + i})
	}
	defer func() {
		// Sign off and disconnect even when ctx is canceled.
		bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownBudget)
		defer cancel()
		for _, a := range agents {
			if a.cs != nil && a.signedOn {
				_ = a.signOff(bg)
			}
			a.disconnect()
		}
	}()

	for _, a := range agents {
		if err := a.connect(ctx, cfg.Addr); err != nil {
			return err
		}
	}
	if err := registerWorld(ctx, agents[0], world); err != nil {
		return err
	}
	for _, a := range agents {
		if err := a.signOn(ctx); err != nil {
			return err
		}
	}
	online := make([]Online, 0, len(agents))
	for _, a := range agents {
		online = append(online, Online{Address: a.p.String(), Client: a.p.client, Machine: a.p.machine, Away: a.away})
	}
	say(cfg.Out, "online: %d agents\n", len(online))
	for _, o := range online {
		say(cfg.Out, "  %-34s %-14s %-8s %s\n", o.Address, o.Client, o.Machine, o.Away)
	}
	if cfg.OnReady != nil {
		cfg.OnReady(online)
	}

	l := &liveLoop{addr: cfg.Addr, rng: rng, world: world, agents: agents, out: cfg.Out}
	defer l.wait()
	if cfg.HTTP != "" {
		l.setupApprovals(ctx, cfg)
	}
	var quietC <-chan time.Time
	if cfg.Chatter > 0 || cfg.ApprovalEvery > 0 {
		if q := l.startQuiet(ctx); q != nil {
			timer := time.NewTimer(cfg.QuietAfter)
			defer timer.Stop()
			quietC = timer.C
		}
	}
	tickC, stopTick := every(cfg.Chatter)
	defer stopTick()
	approveC, stopApprove := every(cfg.ApprovalEvery)
	defer stopApprove()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-quietC:
			quietC = nil
			l.goQuiet()
		case <-tickC:
			if err := l.tick(ctx); err != nil && ctx.Err() == nil {
				say(cfg.Out, "chatter: %v\n", err)
			}
		case <-approveC:
			l.approve(ctx)
		}
	}
}

// every returns a ticker channel for d, nil when d is not positive, and its
// stop function.
func every(d time.Duration) (<-chan time.Time, func()) {
	if d <= 0 {
		return nil, func() {}
	}
	t := time.NewTicker(d)
	return t.C, t.Stop
}

// lockedWriter serializes writes from concurrent goroutines.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// registerWorld registers every active persona through one connection;
// identities that already exist are fine.
func registerWorld(ctx context.Context, via *agent, world []*persona) error {
	for _, p := range world {
		if p.retired {
			continue
		}
		err := via.call(ctx, "agent_register", map[string]any{
			"address": p.String(), "role": p.title, "description": p.desc, "reports_to": p.reports,
		})
		if err != nil && !strings.Contains(err.Error(), "already registered") {
			return err
		}
	}
	return nil
}

type liveLoop struct {
	addr   string
	rng    *rand.Rand
	world  []*persona
	agents []*agent
	out    io.Writer
	n      int

	// board is the tasks the simulated agents created and still track.
	board []*liveTask
	// quiet is the agent that claimed a task and went silent.
	quiet     *agent
	quietTask string
	// Pending approval prompts, each waiting in its own goroutine.
	mu      sync.Mutex
	pending int
	asking  map[*agent]bool
	wg      sync.WaitGroup
}

func (l *liveLoop) online() []*agent {
	var out []*agent
	for _, a := range l.agents {
		if a.signedOn && !a.quiet {
			out = append(out, a)
		}
	}
	return out
}

func (l *liveLoop) tick(ctx context.Context) error {
	l.n++
	switch r := l.rng.Float64(); {
	case r < 0.15:
		return l.toggle(ctx)
	case r < 0.22:
		return l.escalate(ctx)
	case r < 0.55:
		return l.taskTick(ctx)
	default:
		return l.chat(ctx)
	}
}

// toggle brings an absent agent back, or signs one off (or drops its
// connection) so the sign-on/off feed has traffic.
func (l *liveLoop) toggle(ctx context.Context) error {
	var away []*agent
	for _, a := range l.agents {
		if !a.signedOn && !a.quiet {
			away = append(away, a)
		}
	}
	if len(away) > 0 && (len(away) > 2 || l.rng.Float64() < 0.6) {
		a := pick(l.rng, away)
		if a.cs == nil {
			if err := a.connect(ctx, l.addr); err != nil {
				return err
			}
		}
		say(l.out, "%s signs on\n", a.p)
		return a.signOn(ctx)
	}
	on := l.online()
	if len(on) <= 3 {
		return nil
	}
	a := pick(l.rng, on)
	if l.rng.Float64() < 0.25 {
		say(l.out, "%s disconnects\n", a.p)
		a.disconnect()
		return nil
	}
	say(l.out, "%s signs off\n", a.p)
	return a.signOff(ctx)
}

func (l *liveLoop) chat(ctx context.Context) error {
	on := l.online()
	if len(on) == 0 {
		return nil
	}
	a := pick(l.rng, on)
	var peers []*persona
	for _, p := range l.world {
		if !p.retired && p != a.p && p.org() == a.p.org() {
			peers = append(peers, p)
		}
	}
	if len(peers) == 0 {
		return nil
	}
	to := pick(l.rng, peers)
	if boss := l.byAddr(a.p.reports); boss != nil && l.rng.Float64() < 0.4 {
		to = boss
	}
	subject, body := l.note(a.p)
	say(l.out, "%s -> %s: %s\n", a.p, to, subject)
	return a.send(ctx, []string{to.String()}, subject, body)
}

func (l *liveLoop) escalate(ctx context.Context) error {
	on := l.online()
	if len(on) == 0 {
		return nil
	}
	a := pick(l.rng, on)
	tp := pick(l.rng, topics())
	body := fmt.Sprintf("%s\n\n- %s\n\nMy recommendation is the first option.", tp.context, strings.Join(tp.options, "\n- "))
	say(l.out, "%s -> operator: %s\n", a.p, tp.subject)
	return a.send(ctx, []string{opAddr}, tp.subject, body)
}

func (l *liveLoop) byAddr(addr string) *persona {
	i := slices.IndexFunc(l.world, func(p *persona) bool { return p.String() == addr })
	if i < 0 || l.world[i].retired {
		return nil
	}
	return l.world[i]
}

// note builds a short, repo-flavored message.
func (l *liveLoop) note(p *persona) (subject, body string) {
	fl, ok := flavors()[p.scope()]
	if !ok {
		fl = flavors()[acme+"/api"]
	}
	pr := 700 + l.rng.IntN(300) + l.n
	test := pick(l.rng, fl.tests)
	file := pick(l.rng, fl.files)
	switch l.rng.IntN(4) {
	case 0:
		return fmt.Sprintf("Quick question on #%d", pr), md(fmt.Sprintf("Is the change to ¦%s¦ intentional? The snapshot differs on my branch.", file))
	case 1:
		return fmt.Sprintf("FYI: %s is flaky again", test), md(fmt.Sprintf("Failed 2 of the last 6 runs. Quarantining it in #%d until someone can dig in.", pr))
	case 2:
		return "Status: " + pick(l.rng, fl.titles), md(fmt.Sprintf("Finished the main work in ¦%s¦. Opening #%d shortly.\n\n- [x] implementation\n- [ ] docs", file, pr))
	default:
		return fmt.Sprintf("Heads up: touching %s", file), md(fmt.Sprintf("I am about to refactor ¦%s¦; shout if you have open changes there.", file))
	}
}
