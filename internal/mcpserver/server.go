package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
	"github.com/scrothers/aim/internal/presence"
)

// New returns an MCP server for one connection. Handlers act on behalf of
// conn, whose preamble and sign-on state identify the caller.
func New(svc *mail.Service, conn *presence.Entry, version string, opts ...Option) *mcp.Server {
	h := &handlers{svc: svc, conn: conn, c: &session.Conn{Svc: svc, Entry: conn}, progressInterval: defaultProgressInterval}
	h.subs.debounce = defaultSubscribeDebounce
	for _, o := range opts {
		o(h)
	}
	srv := mcp.NewServer(
		&mcp.Implementation{Name: "aim", Version: version},
		&mcp.ServerOptions{
			Instructions:       instructions(),
			SubscribeHandler:   h.subscribe,
			UnsubscribeHandler: h.unsubscribe,
			CompletionHandler:  h.complete,
		},
	)
	h.srv = srv
	h.registerResources(srv)
	h.registerPrompts(srv)
	srv.AddReceivingMiddleware(h.captureClient)
	// Descriptions live in prompts/tools/<name>.md (see prompts.go); schemas
	// are made portable across clients (see schema.go).
	addTool(srv, "whoami", h.whoami)
	addTool(srv, "register", h.register)
	addTool(srv, "roster", h.roster)
	addTool(srv, "sign_on", h.signOn)
	addTool(srv, "sign_off", h.signOff)
	addTool(srv, "send", h.send)
	addTool(srv, "list", h.list)
	addTool(srv, "read", h.read)
	addTool(srv, "thread", h.thread)
	addTool(srv, "mark", h.mark)
	addTool(srv, "search", h.search)
	addTool(srv, "wait", h.wait)
	return srv
}

// defaultProgressInterval is how often wait reports progress while blocked.
const defaultProgressInterval = 15 * time.Second

// Option configures a per-connection server.
type Option func(*handlers)

// WithProgressInterval sets how often wait reports progress while blocked.
// Non-positive values keep the default.
func WithProgressInterval(d time.Duration) Option {
	return func(h *handlers) {
		if d > 0 {
			h.progressInterval = d
		}
	}
}

// SubagentResolver returns the subagent that issued the tool call with the
// given runtime tool-use id, as recorded by a pre-tool hook.
type SubagentResolver func(toolUseID string) (string, bool)

// WithSubagentResolver fills in the `subagent` argument of send, read and
// mark calls that omit it, when the resolver knows which subagent made the
// call. Claude Code sends the tool-use id as _meta["claudecode/toolUseId"].
func WithSubagentResolver(r SubagentResolver) Option {
	return func(h *handlers) { h.resolveSubagent = r }
}

// toolUseMetaKey is the _meta key carrying Claude Code's tool-use id.
const toolUseMetaKey = "claudecode/toolUseId"

type handlers struct {
	svc              *mail.Service
	conn             *presence.Entry
	c                *session.Conn
	srv              *mcp.Server
	progressInterval time.Duration
	clientSeen       atomic.Bool
	subs             subscriptions
	resolveSubagent  SubagentResolver
}

// captureClient records the MCP clientInfo on the presence entry the first
// time a tool is called on the connection, and attributes subagent calls.
func (h *handlers) captureClient(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if method == "tools/call" {
			h.conn.Touch(time.Now())
			h.attributeSubagent(req)
		}
		if method == "tools/call" && !h.clientSeen.Load() {
			if ss, ok := req.GetSession().(*mcp.ServerSession); ok {
				if ip := ss.InitializeParams(); ip != nil && ip.ClientInfo != nil {
					h.conn.SetClient(ip.ClientInfo.Name, ip.ClientInfo.Version)
					h.clientSeen.Store(true)
				}
			}
		}
		return next(ctx, method, req)
	}
}

// attributeSubagent sets the subagent argument of a send, read or mark call
// that left it empty, when the resolver knows the call's subagent.
func (h *handlers) attributeSubagent(req mcp.Request) {
	ctr, ok := req.(*mcp.CallToolRequest)
	if !ok || h.resolveSubagent == nil || ctr.Params == nil {
		return
	}
	switch ctr.Params.Name {
	case "send", "read", "mark":
	default:
		return
	}
	id, _ := ctr.Params.GetMeta()[toolUseMetaKey].(string)
	sub, ok := h.resolveSubagent(id)
	if !ok {
		return
	}
	args := map[string]any{}
	if len(ctr.Params.Arguments) > 0 {
		if err := json.Unmarshal(ctr.Params.Arguments, &args); err != nil || args == nil {
			return // leave malformed arguments for the handler to reject
		}
	}
	if s, _ := args["subagent"].(string); s != "" {
		return
	}
	args["subagent"] = sub
	if b, err := json.Marshal(args); err == nil {
		ctr.Params.Arguments = b
	}
}

func (h *handlers) caller() mail.Caller { return h.c.Caller() }

// WhoamiOutput is the result of whoami.
type WhoamiOutput struct {
	SessionID  string `json:"session_id" jsonschema:"Claude session id of this connection"`
	Machine    string `json:"machine" jsonschema:"hostname of the machine the shim runs on"`
	ProjectDir string `json:"project_dir" jsonschema:"project directory of this session"`
	GitRemote  string `json:"git_remote,omitempty" jsonschema:"origin remote URL of the project"`
	Namespace  string `json:"namespace,omitempty" jsonschema:"your git namespace host/org/repo, usable as an address"`
	SignedOnAs string `json:"signed_on_as,omitempty" jsonschema:"identity this connection is signed on as"`
	Client     string `json:"client,omitempty" jsonschema:"MCP client name, for example claude-code"`
	ClientVer  string `json:"client_version,omitempty" jsonschema:"MCP client version"`
}

func (h *handlers) whoami(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, WhoamiOutput, error) {
	p := h.conn.Preamble()
	out := WhoamiOutput{
		SessionID: p.SessionID, Machine: p.Machine, ProjectDir: p.ProjectDir,
		GitRemote: p.GitRemote, Namespace: p.Namespace,
	}
	if id, ok := h.conn.Identity(); ok {
		out.SignedOnAs = id.String()
	}
	st := h.conn.Status()
	out.Client, out.ClientVer = st.Client, st.ClientVersion
	return nil, out, nil
}

// RegisterInput is the input of register.
type RegisterInput struct {
	Address     string `json:"address" jsonschema:"identity address name@host/org[/repo]"`
	Role        string `json:"role" jsonschema:"short role title, for example reviewer"`
	Description string `json:"description" jsonschema:"what this identity does"`
	ReportsTo   string `json:"reports_to,omitempty" jsonschema:"address of the identity this one reports to"`
}

func (h *handlers) register(ctx context.Context, _ *mcp.CallToolRequest, in RegisterInput) (*mcp.CallToolResult, mail.Identity, error) {
	id, err := h.svc.Register(ctx, h.caller(), mail.RegisterInput{
		Address: in.Address, Role: in.Role, Description: in.Description, ReportsTo: in.ReportsTo,
	})
	return nil, id, session.ToolErr(err)
}

// AddressInput is the input of tools that take only an address.
type AddressInput struct {
	Address string `json:"address" jsonschema:"mailbox address; covers everything below it"`
}

// RosterOutput is the result of roster.
type RosterOutput struct {
	Identities []mail.RosterEntry `json:"identities"`
}

func (h *handlers) roster(ctx context.Context, _ *mcp.CallToolRequest, in AddressInput) (*mcp.CallToolResult, RosterOutput, error) {
	if err := session.RequireAddress("roster", in.Address); err != nil {
		return nil, RosterOutput{Identities: []mail.RosterEntry{}}, err
	}
	es, err := h.svc.Roster(ctx, in.Address)
	if es == nil {
		es = []mail.RosterEntry{}
	}
	return nil, RosterOutput{Identities: es}, session.ToolErr(err)
}

// SignOnInput is the input of sign_on.
type SignOnInput struct {
	Address string `json:"address" jsonschema:"registered identity address to sign on as"`
	Away    string `json:"away,omitempty" jsonschema:"optional away message shown in the roster"`
}

// SignOnOutput is the result of sign_on.
type SignOnOutput struct {
	SignedOnAs string `json:"signed_on_as"`
}

func (h *handlers) signOn(ctx context.Context, _ *mcp.CallToolRequest, in SignOnInput) (*mcp.CallToolResult, SignOnOutput, error) {
	a, err := h.svc.ResolveIdentity(ctx, in.Address)
	if err != nil {
		return nil, SignOnOutput{}, session.ToolErr(err)
	}
	if err := h.conn.SignOn(a, in.Away, time.Now()); err != nil {
		return nil, SignOnOutput{}, err
	}
	st := h.conn.Status()
	if err := h.svc.SaveSignon(ctx, mail.Signon{
		SessionID: st.SessionID, Identity: a, Away: in.Away, Client: st.Client, Machine: st.Machine,
	}); err != nil {
		return nil, SignOnOutput{}, err
	}
	return nil, SignOnOutput{SignedOnAs: a.String()}, nil
}

// SignOffOutput is the result of sign_off.
type SignOffOutput struct {
	WasSignedOn bool `json:"was_signed_on"`
}

func (h *handlers) signOff(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, SignOffOutput, error) {
	_, was := h.conn.Identity()
	h.conn.SignOff()
	if err := h.svc.ClearSignon(ctx, h.conn.Status().SessionID); err != nil {
		return nil, SignOffOutput{}, err
	}
	return nil, SignOffOutput{WasSignedOn: was}, nil
}

// SendInput is the input of send.
type SendInput struct {
	To        []string `json:"to" jsonschema:"recipient addresses"`
	Cc        []string `json:"cc,omitempty" jsonschema:"carbon-copy addresses"`
	Subject   string   `json:"subject" jsonschema:"subject, at most 200 characters"`
	Body      string   `json:"body" jsonschema:"message body, at most 64 KiB"`
	InReplyTo string   `json:"in_reply_to,omitempty" jsonschema:"id of the message being replied to"`
	ReplyAll  bool     `json:"reply_all,omitempty" jsonschema:"also address the original sender, to and cc minus yourself"`
	Subagent  string   `json:"subagent,omitempty" jsonschema:"send as this subagent of your identity"`
}

func (h *handlers) send(ctx context.Context, _ *mcp.CallToolRequest, in SendInput) (*mcp.CallToolResult, mail.Message, error) {
	m, err := h.svc.Send(ctx, h.caller(), mail.SendInput{
		To: in.To, Cc: in.Cc, Subject: in.Subject, Body: in.Body,
		InReplyTo: in.InReplyTo, Subagent: in.Subagent, ReplyAll: in.ReplyAll,
	})
	return nil, m, session.ToolErr(err)
}

// ListInput is the input of list.
type ListInput struct {
	Address string `json:"address" jsonschema:"mailbox address; covers everything below it"`
	State   string `json:"state,omitempty" jsonschema:"unread (default), read, archived, inbox (read or unread, not archived) or all; archived requires since and until"`
	From    string `json:"from,omitempty" jsonschema:"only messages from this sender address"`
	Since   string `json:"since,omitempty" jsonschema:"RFC3339 lower time bound"`
	Until   string `json:"until,omitempty" jsonschema:"RFC3339 upper time bound"`
	Limit   int    `json:"limit,omitempty" jsonschema:"maximum items, default 50, max 200"`
	Cursor  string `json:"cursor,omitempty" jsonschema:"next_cursor from a previous result"`
}

func (h *handlers) list(ctx context.Context, _ *mcp.CallToolRequest, in ListInput) (*mcp.CallToolResult, mail.ListResult, error) {
	if err := session.RequireAddress("list", in.Address); err != nil {
		return nil, mail.ListResult{Items: []mail.Header{}}, err
	}
	since, err := session.ParseTime("since", in.Since)
	if err != nil {
		return nil, mail.ListResult{}, err
	}
	until, err := session.ParseTime("until", in.Until)
	if err != nil {
		return nil, mail.ListResult{}, err
	}
	res, err := h.svc.List(ctx, mail.ListInput{
		Address: in.Address, State: in.State, From: in.From, Since: since, Until: until,
		Limit: in.Limit, Cursor: in.Cursor,
	})
	if res.Items == nil {
		res.Items = []mail.Header{}
	}
	return nil, res, session.ToolErr(err)
}

// ReadInput is the input of read.
type ReadInput struct {
	MessageID string `json:"message_id" jsonschema:"message id, msg-..."`
	Peek      bool   `json:"peek,omitempty" jsonschema:"read without marking the delivery read"`
	Subagent  string `json:"subagent,omitempty" jsonschema:"read as this subagent of your identity"`
}

// ReadOutput is the result of read: the message plus whether the read left
// the delivery marked read.
type ReadOutput struct {
	mail.Message
	Marked bool `json:"marked" jsonschema:"true when a delivery of this message is marked read for you; false for peek or when you are not signed on as an owner"`
}

func (h *handlers) read(ctx context.Context, _ *mcp.CallToolRequest, in ReadInput) (*mcp.CallToolResult, ReadOutput, error) {
	m, err := h.svc.Read(ctx, h.caller(), mail.ReadInput{MessageID: in.MessageID, Peek: in.Peek, Subagent: in.Subagent})
	return nil, ReadOutput{Message: m, Marked: m.Marked}, session.ToolErr(err)
}

// ThreadInput is the input of thread.
type ThreadInput struct {
	ThreadID string `json:"thread_id" jsonschema:"thread id of any message in the thread"`
}

// ThreadOutput is the result of thread.
type ThreadOutput struct {
	Messages []mail.Message `json:"messages"`
}

func (h *handlers) thread(ctx context.Context, _ *mcp.CallToolRequest, in ThreadInput) (*mcp.CallToolResult, ThreadOutput, error) {
	ms, err := h.svc.Thread(ctx, in.ThreadID)
	if ms == nil {
		ms = []mail.Message{}
	}
	return nil, ThreadOutput{Messages: ms}, session.ToolErr(err)
}

// MarkInput is the input of mark.
type MarkInput struct {
	MessageIDs []string `json:"message_ids" jsonschema:"ids of the messages to change"`
	State      string   `json:"state" jsonschema:"read, unread, archived or unarchived"`
	Subagent   string   `json:"subagent,omitempty" jsonschema:"act as this subagent of your identity"`
}

// MarkOutput is the result of mark.
type MarkOutput struct {
	Changed int `json:"changed" jsonschema:"number of deliveries that changed"`
}

func (h *handlers) mark(ctx context.Context, _ *mcp.CallToolRequest, in MarkInput) (*mcp.CallToolResult, MarkOutput, error) {
	n, err := h.svc.Mark(ctx, h.caller(), mail.MarkInput{MessageIDs: in.MessageIDs, State: in.State, Subagent: in.Subagent})
	return nil, MarkOutput{Changed: n}, session.ToolErr(err)
}

// SearchInput is the input of search.
type SearchInput struct {
	Address string `json:"address" jsonschema:"mailbox address; covers everything below it"`
	Query   string `json:"query" jsonschema:"FTS5 query over subject and body"`
	Limit   int    `json:"limit,omitempty" jsonschema:"maximum hits, default 20, max 100"`
	Cursor  string `json:"cursor,omitempty" jsonschema:"next_cursor from a previous result"`
}

func (h *handlers) search(ctx context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, mail.SearchResult, error) {
	if err := session.RequireAddress("search", in.Address); err != nil {
		return nil, mail.SearchResult{Hits: []mail.Hit{}}, err
	}
	res, err := h.svc.Search(ctx, mail.SearchInput{Address: in.Address, Query: in.Query, Limit: in.Limit, Cursor: in.Cursor})
	if res.Hits == nil {
		res.Hits = []mail.Hit{}
	}
	return nil, res, session.ToolErr(err)
}

// WaitInput is the input of wait.
type WaitInput struct {
	Address        string `json:"address,omitempty" jsonschema:"mailbox address to watch, covering everything below it; defaults to your signed-on identity"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"how long to block, default 60, max 300; keep below your client's tool-call timeout"`
	SinceID        string `json:"since_id,omitempty" jsonschema:"only wake for unread messages with an id newer than this"`
}

func (h *handlers) wait(ctx context.Context, req *mcp.CallToolRequest, in WaitInput) (*mcp.CallToolResult, mail.WaitResult, error) {
	if in.TimeoutSeconds < 0 {
		return nil, mail.WaitResult{Items: []mail.Header{}}, fmt.Errorf("mcpserver: wait: %w: timeout_seconds must not be negative", mail.ErrInvalidInput)
	}
	if token := req.Params.GetProgressToken(); token != nil {
		var stop context.CancelFunc
		ctx, stop = context.WithCancel(ctx)
		defer stop()
		go h.reportProgress(ctx, req.Session, token)
	}
	res, err := h.svc.Wait(ctx, h.caller(), mail.WaitInput{
		Address: in.Address, SinceID: in.SinceID, Timeout: time.Duration(in.TimeoutSeconds) * time.Second,
	})
	if res.Items == nil {
		res.Items = []mail.Header{}
	}
	return nil, res, session.ToolErr(err)
}

// reportProgress sends a progress notification every progressInterval until
// ctx ends, which keeps long calls alive on clients that reset their timeout
// on progress.
func (h *handlers) reportProgress(ctx context.Context, ss *mcp.ServerSession, token any) {
	t := time.NewTicker(h.progressInterval)
	defer t.Stop()
	start := time.Now()
	for n := 1; ; n++ {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = ss.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
				ProgressToken: token,
				Progress:      float64(n),
				Message:       fmt.Sprintf("waiting for mail (%s)", time.Since(start).Round(time.Second)),
			})
		}
	}
}
