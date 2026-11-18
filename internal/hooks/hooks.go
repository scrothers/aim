package hooks

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mdview"
	"github.com/scrothers/aim/internal/presence"
	"github.com/scrothers/aim/internal/tasks"
	"github.com/scrothers/aim/internal/wire"
)

// Limits and timings.
const (
	nudgeLimit      = 10
	rosterLimit     = 20
	unreadScan      = 50
	toolUseTTL      = time.Minute
	agentTTL        = 24 * time.Hour
	requestTimeout  = 5 * time.Second
	defaultDebounce = 2 * time.Second
	defaultPoll     = 5 * time.Second
	messagePrefix   = "msg-"
)

// Mail is the slice of the mail service the hook server uses.
type Mail interface {
	List(ctx context.Context, in mail.ListInput) (mail.ListResult, error)
	Roster(ctx context.Context, addr string) ([]mail.RosterEntry, error)
	RestoreSignon(ctx context.Context, sessionID string) (mail.Signon, bool, error)
	Changed() <-chan struct{}
	Send(ctx context.Context, c mail.Caller, in mail.SendInput) (mail.Message, error)
}

// Tasks is the slice of the task board the hook server uses: every hook
// event is lease activity for the session, and nudges mention open work.
type Tasks interface {
	Touch(ctx context.Context, sessionID string) error
	Counts(ctx context.Context, identity string) (tasks.Counts, error)
}

// Server answers hook requests. Use New to create one; it is safe for
// concurrent use.
type Server struct {
	mail       Mail
	tasks      Tasks // nil disables task activity and nudges
	reg        *presence.Registry
	log        *slog.Logger
	now        func() time.Time
	debounce   time.Duration
	poll       time.Duration
	alertAfter time.Duration
	// approvalMode reports the remote approval setting; nil disables it.
	approvalMode ApprovalModeFunc
	// done is closed by Close to end pending approvals.
	done chan struct{}

	mu        sync.Mutex
	agents    map[string]*agentState
	tools     map[string]toolUse
	approvals map[string]*pendingApproval
	closed    bool
}

// agentState is what the server remembers about one agent between hooks.
type agentState struct {
	// watermark is the newest message id the agent has been told about, or
	// a time-floor id; empty means never nudged.
	watermark string
	seen      time.Time
	// watch is closed to end the agent's current watch, if any.
	watch chan struct{}
	// alert fires the operator alert for alertState, if one is pending.
	alert      *time.Timer
	alertState string
}

type toolUse struct {
	subagent string
	expires  time.Time
}

// Option configures a Server.
type Option func(*Server)

// WithClock sets the time source. The default is time.Now.
func WithClock(now func() time.Time) Option { return func(s *Server) { s.now = now } }

// WithDebounce sets how long a watch waits after the first new message so
// a burst wakes the agent once. The default is 2s.
func WithDebounce(d time.Duration) Option { return func(s *Server) { s.debounce = d } }

// WithPoll sets how often a watch re-checks without a mail broadcast, which
// catches sign-ons. The default is 5s.
func WithPoll(d time.Duration) Option { return func(s *Server) { s.poll = d } }

// WithAlertAfter sets how long an agent may stay blocked or failed before
// the operator gets a message about it. Zero, the default, sends none.
func WithAlertAfter(d time.Duration) Option { return func(s *Server) { s.alertAfter = d } }

// WithLogger sets the logger for failures no hook waits on, such as operator
// alerts. The default is slog.Default().
func WithLogger(l *slog.Logger) Option { return func(s *Server) { s.log = l } }

// WithTasks makes every hook event count as task lease activity for its
// session and adds open-task counts to session-start and prompt nudges.
func WithTasks(t Tasks) Option { return func(s *Server) { s.tasks = t } }

// New returns a Server over the mail service and the presence registry.
func New(m Mail, reg *presence.Registry, opts ...Option) *Server {
	s := &Server{
		mail: m, reg: reg, log: slog.Default(), now: time.Now, debounce: defaultDebounce, poll: defaultPoll,
		agents: make(map[string]*agentState), tools: make(map[string]toolUse),
		approvals: make(map[string]*pendingApproval), done: make(chan struct{}),
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Serve handles one hook connection whose preamble br has already consumed.
func (s *Server) Serve(ctx context.Context, c net.Conn, br *bufio.Reader, p wire.Preamble) error {
	_ = c.SetReadDeadline(time.Now().Add(requestTimeout))
	var req wire.HookRequest
	if err := wire.ReadLine(br, &req); err != nil {
		return fmt.Errorf("hooks: read request: %w", err)
	}
	_ = c.SetReadDeadline(time.Time{})
	var reply wire.HookReply
	var err error
	switch req.Op {
	case wire.HookOpWatch, wire.HookOpApprove:
		wctx, cancel := context.WithCancel(ctx)
		defer cancel()
		// The hook sends nothing more; a read returning means it went away.
		go func() { _, _ = br.ReadByte(); cancel() }()
		if req.Op == wire.HookOpWatch {
			reply, err = s.Watch(wctx, p, req)
		} else {
			reply, err = s.Approve(wctx, p, req)
		}
	default:
		reply, err = s.Handle(ctx, p, req)
	}
	if err != nil {
		return err
	}
	if err := wire.WriteLine(c, reply); err != nil {
		return fmt.Errorf("hooks: write reply: %w", err)
	}
	return nil
}

// Handle answers one event request.
func (s *Server) Handle(ctx context.Context, p wire.Preamble, req wire.HookRequest) (wire.HookReply, error) {
	s.touchTasks(ctx, p)
	id, ok := s.identity(ctx, p)
	reply := wire.HookReply{SignedOn: ok}
	if ok {
		reply.Identity = id.String()
	}
	var err error
	switch req.Event {
	case wire.HookSessionStart:
		s.setAttention(ctx, p, wire.HookRequest{}, id, ok)
		reply.Context, err = s.sessionStart(ctx, p, id, ok)
		if err == nil && ok {
			reply.Context = joinContext(reply.Context, s.taskNudge(ctx, id))
		}
	case wire.HookPrompt, wire.HookPostTool:
		s.setAttention(ctx, p, wire.HookRequest{}, id, ok)
		if ok {
			reply.Context, err = s.nudgeAgent(ctx, p, id, req)
		}
		if err == nil && ok && req.Event == wire.HookPrompt {
			reply.Context = joinContext(reply.Context, s.taskNudge(ctx, id))
		}
	case wire.HookSubagentStart:
		s.setAttention(ctx, p, wire.HookRequest{}, id, ok)
		if ok {
			reply.Context, err = s.subagentStart(ctx, id, req)
		}
	case wire.HookPreTool:
		s.setAttention(ctx, p, wire.HookRequest{}, id, ok)
		s.recordTool(req)
	case wire.HookStatus:
		s.setAttention(ctx, p, req, id, ok)
	case wire.HookSessionEnd:
		s.end(agentKey(p))
	}
	if err != nil {
		return wire.HookReply{}, fmt.Errorf("hooks: %s: %w", req.Event, err)
	}
	return reply, nil
}

// identity resolves the agent's signed-on identity: its live MCP connection
// first, then the sign-on persisted for its session.
func (s *Server) identity(ctx context.Context, p wire.Preamble) (address.Address, bool) {
	if e, ok := s.reg.Agent(p.Machine, p.SessionID, p.AgentPID); ok {
		if id, ok := e.Identity(); ok {
			return id, true
		}
	}
	if so, ok, err := s.mail.RestoreSignon(ctx, p.SessionID); err == nil && ok {
		return so.Identity, true
	}
	return address.Address{}, false
}

// namespace returns the preamble's namespace, else the joined connection's.
func (s *Server) namespace(p wire.Preamble) string {
	if p.Namespace != "" {
		return p.Namespace
	}
	if e, ok := s.reg.Agent(p.Machine, p.SessionID, p.AgentPID); ok {
		return e.Preamble().Namespace
	}
	return ""
}

func (s *Server) sessionStart(ctx context.Context, p wire.Preamble, id address.Address, signedOn bool) (string, error) {
	if !signedOn {
		return s.notSignedOn(ctx, p)
	}
	key := agentKey(p)
	floor := floorID(s.now())
	items, total, err := s.unread(ctx, id.String(), "")
	if err != nil {
		return "", err
	}
	s.advance(key, floor, items)
	var b strings.Builder
	fmt.Fprintf(&b, "# AIM\n\nThis session is signed on to AIM as `%s`.", id)
	if total == 0 {
		b.WriteString(" No unread mail.\n")
		return b.String(), nil
	}
	b.WriteString("\n\n")
	b.WriteString(mdview.Nudge(id.String(), items[:min(len(items), nudgeLimit)], total))
	return b.String(), nil
}

func (s *Server) notSignedOn(ctx context.Context, p wire.Preamble) (string, error) {
	var b strings.Builder
	b.WriteString("# AIM\n\nThis session is not signed on to AIM (agent instant messaging). " +
		"To send or receive mail, call `sign_on` with your identity address " +
		"(`register` it first if it is new; `whoami` shows this session's namespace).\n")
	ns := s.namespace(p)
	if ns == "" {
		b.WriteString("\nUse the `roster` tool to find identities.\n")
		return b.String(), nil
	}
	es, err := s.mail.Roster(ctx, ns)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(&b, "\nIdentities in `%s`:\n\n", ns)
	b.WriteString(mdview.RosterBrief(es[:min(len(es), rosterLimit)]))
	return b.String(), nil
}

// nudge reports unread mail newer than the agent's watermark, then advances
// it. It is silent when there is none or the agent can't be tracked.
func (s *Server) nudge(ctx context.Context, key string, id address.Address) (string, error) {
	if key == "" {
		return "", nil
	}
	floor := floorID(s.now())
	fresh, total, err := s.unread(ctx, id.String(), s.watermark(key))
	if err != nil {
		return "", err
	}
	s.advance(key, floor, fresh)
	if len(fresh) == 0 {
		return "", nil
	}
	return mdview.Nudge(id.String(), fresh[:min(len(fresh), nudgeLimit)], total), nil
}

// nudgeAgent nudges the agent, or the subagent the hook fired inside: a
// subagent sees only its own mailbox, under its own watermark, so it never
// consumes its parent's nudges.
func (s *Server) nudgeAgent(ctx context.Context, p wire.Preamble, id address.Address, req wire.HookRequest) (string, error) {
	key := agentKey(p)
	sub := req.Subagent
	if sub == "" {
		sub = SubagentName(req.AgentType)
	}
	if sub == "" || key == "" {
		return s.nudge(ctx, key, id)
	}
	addr, err := id.WithSubagent(sub)
	if err != nil {
		return "", nil //nolint:nilerr // an unusable name just means no subagent mailbox
	}
	return s.nudge(ctx, key+"/"+sub, addr)
}

func (s *Server) subagentStart(ctx context.Context, id address.Address, req wire.HookRequest) (string, error) {
	sub := req.Subagent
	if sub == "" {
		sub = SubagentName(req.AgentType)
	}
	if sub == "" {
		return "", nil
	}
	addr, err := id.WithSubagent(sub)
	if err != nil {
		return "", nil //nolint:nilerr // an unusable name just means no mailbox hint
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# AIM\n\nYou are subagent `%s` of `%s`. Your AIM mailbox is `%s`: "+
		"pass `subagent: \"%s\"` to `send`, `read` and `mark`.\n", sub, id, addr, sub)
	items, total, err := s.unread(ctx, addr.String(), "")
	if err != nil {
		return "", err
	}
	if total > 0 {
		b.WriteString("\n")
		b.WriteString(mdview.Nudge(addr.String(), items[:min(len(items), nudgeLimit)], total))
	}
	return b.String(), nil
}

// unread lists unread mail under addr, newest first, keeping only messages
// newer than since; total counts all unread scanned.
func (s *Server) unread(ctx context.Context, addr, since string) ([]mail.Header, int, error) {
	res, err := s.mail.List(ctx, mail.ListInput{Address: addr, State: mail.StateUnread, Limit: unreadScan})
	if err != nil {
		return nil, 0, err
	}
	fresh := make([]mail.Header, 0, len(res.Items))
	for _, h := range res.Items {
		if h.ID > since {
			fresh = append(fresh, h)
		}
	}
	return fresh, len(res.Items), nil
}

// SubagentFor returns and forgets the subagent a pre-tool hook recorded for
// toolUseID.
func (s *Server) SubagentFor(toolUseID string) (string, bool) {
	if toolUseID == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tools[toolUseID]
	delete(s.tools, toolUseID)
	if !ok || s.now().After(t.expires) {
		return "", false
	}
	return t.subagent, true
}

func (s *Server) recordTool(req wire.HookRequest) {
	sub := req.Subagent
	if sub == "" {
		sub = SubagentName(req.AgentType)
	}
	if sub == "" || req.ToolUseID == "" {
		return
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, t := range s.tools {
		if now.After(t.expires) {
			delete(s.tools, k)
		}
	}
	s.tools[req.ToolUseID] = toolUse{subagent: sub, expires: now.Add(toolUseTTL)}
}

// SubagentName derives a mailbox name from a runtime's agent type, such as
// "general-purpose" or "Code Reviewer", or returns "" if none fits.
func SubagentName(agentType string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(agentType)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	name := strings.TrimLeft(b.String(), "._-")
	if len(name) > 64 {
		name = name[:64]
	}
	if address.ValidName(name) != nil {
		return ""
	}
	return name
}

// agentKey identifies an agent across hook calls, or "" when it can't be.
func agentKey(p wire.Preamble) string {
	switch {
	case p.SessionID != "":
		return "s:" + p.SessionID
	case p.AgentPID > 0:
		return "p:" + p.Machine + ":" + strconv.Itoa(p.AgentPID)
	}
	return ""
}

// floorID returns an id that sorts before every message created at or after
// t: the message prefix and a ULID with t's timestamp and zero entropy.
func floorID(t time.Time) string {
	var u ulid.ULID
	_ = u.SetTime(ulid.Timestamp(t))
	return messagePrefix + strings.ToLower(u.String())
}

// state returns the agent's state, creating it, and prunes stale agents. The
// caller must hold s.mu.
func (s *Server) stateLocked(key string) *agentState {
	now := s.now()
	for k, a := range s.agents {
		if a.watch == nil && now.Sub(a.seen) > agentTTL {
			delete(s.agents, k)
		}
	}
	a, ok := s.agents[key]
	if !ok {
		a = &agentState{}
		s.agents[key] = a
	}
	a.seen = now
	return a
}

func (s *Server) watermark(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stateLocked(key).watermark
}

// advance moves the agent's watermark past floor and every item shown.
func (s *Server) advance(key, floor string, shown []mail.Header) {
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.stateLocked(key)
	a.watermark = max(a.watermark, floor)
	for _, h := range shown {
		a.watermark = max(a.watermark, h.ID)
	}
}

func (s *Server) end(key string) {
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if a, ok := s.agents[key]; ok {
		if a.watch != nil {
			close(a.watch)
		}
		a.stopAlert()
		delete(s.agents, key)
	}
}

// Close stops pending operator alerts and releases pending approvals with
// no decision. Hooks served afterward schedule no alerts and wait for no
// approval.
func (s *Server) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		close(s.done)
	}
	s.closed = true
	for _, a := range s.agents {
		a.stopAlert()
	}
}

func (a *agentState) stopAlert() {
	if a.alert != nil {
		a.alert.Stop()
	}
	a.alert, a.alertState = nil, ""
}
