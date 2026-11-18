package wire

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// Version is the preamble protocol version.
const Version = 1

// MaxPreambleBytes bounds the size of the preamble line.
const MaxPreambleBytes = 64 << 10

// ErrBadPreamble is returned when the preamble is missing, oversized,
// malformed or of an unsupported version.
var ErrBadPreamble = errors.New("bad preamble")

// Preamble describes the session on the other end of a connection.
type Preamble struct {
	V          int    `json:"v"`
	SessionID  string `json:"session_id"`
	ProjectDir string `json:"project_dir"`
	Machine    string `json:"machine"`
	GitRemote  string `json:"git_remote,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	// Resume is set by the shim when it redials after losing the daemon
	// connection and replays its MCP handshake.
	Resume bool `json:"resume,omitempty"`
	// Kind selects the protocol after the preamble: empty for MCP, KindHook
	// for one HookRequest/HookReply exchange.
	Kind string `json:"kind,omitempty"`
	// Client names the agent runtime when known before MCP initialize (hook
	// connections set it; MCP connections report clientInfo instead).
	Client string `json:"client,omitempty"`
	// AgentPID is the process id of the agent runtime that spawned the shim
	// or hook, on Machine. It joins hook connections to MCP connections of
	// the same agent when no session id is shared.
	AgentPID int `json:"agent_pid,omitempty"`
}

// KindHook marks a hook connection.
const KindHook = "hook"

// Hook operations.
const (
	// HookOpEvent handles one normalized hook event and replies at once.
	HookOpEvent = "event"
	// HookOpWatch blocks until new mail arrives for the agent, then replies.
	HookOpWatch = "watch"
	// HookOpApprove asks the operator to answer a permission prompt. It
	// blocks until the operator decides, the approval times out or the hook
	// goes away; an empty HookReply.Decision means the runtime should prompt
	// in its terminal as usual.
	HookOpApprove = "approve"
)

// Approval decisions a HookReply carries.
const (
	DecisionAllow = "allow"
	DecisionDeny  = "deny"
)

// MaxToolInputBytes bounds HookRequest.ToolInput; see TruncateToolInput.
const MaxToolInputBytes = 16 << 10

// toolInputPreview is how many bytes of an oversized tool input survive in
// its summary object.
const toolInputPreview = 4 << 10

// TruncateToolInput returns raw when it is valid JSON of at most
// MaxToolInputBytes. A larger input becomes a summary object
// {"truncated":true,"bytes":N,"preview":"..."} holding the first few KiB as
// a string; invalid JSON becomes nil.
func TruncateToolInput(raw json.RawMessage) json.RawMessage {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || !json.Valid(raw) {
		return nil
	}
	if len(raw) <= MaxToolInputBytes {
		return raw
	}
	cut := toolInputPreview
	for cut > 0 && raw[cut]&0xC0 == 0x80 {
		cut--
	}
	b, err := json.Marshal(struct {
		Truncated bool   `json:"truncated"`
		Bytes     int    `json:"bytes"`
		Preview   string `json:"preview"`
	}{true, len(raw), string(raw[:cut])})
	if err != nil {
		return nil
	}
	return b
}

// Normalized hook events, shared by every runtime adapter.
const (
	HookSessionStart  = "session-start"
	HookPrompt        = "prompt"
	HookSubagentStart = "subagent-start"
	HookPreTool       = "pre-tool"
	HookPostTool      = "post-tool"
	HookSessionEnd    = "session-end"
	// HookStatus reports that the agent needs attention (HookRequest.Attention)
	// or, with an empty Attention, that it no longer does.
	HookStatus = "status"
)

// Attention states a status event reports. Activity (a prompt, a tool call)
// clears them.
const (
	// AttentionBlocked means the agent waits on the operator: a permission
	// prompt or a question.
	AttentionBlocked = "blocked"
	// AttentionWaiting means the agent finished its turn and waits for input.
	AttentionWaiting = "waiting"
	// AttentionFailed means the turn ended on an error, such as an API failure.
	AttentionFailed = "failed"
	// AttentionInterrupted means the user interrupted the turn.
	AttentionInterrupted = "interrupted"
)

// HookRequest is the single request line a hook connection sends after its
// preamble.
type HookRequest struct {
	Op      string `json:"op"`
	Event   string `json:"event,omitempty"`
	Runtime string `json:"runtime,omitempty"`
	// Source qualifies session-start: startup, resume, clear, compact.
	Source string `json:"source,omitempty"`
	// AgentID and AgentType identify a subagent when the hook fires inside
	// one; Subagent is the mailbox name derived from AgentType.
	AgentID   string `json:"agent_id,omitempty"`
	AgentType string `json:"agent_type,omitempty"`
	Subagent  string `json:"subagent,omitempty"`
	// ToolUseID is the runtime's id for the tool call a pre-tool hook
	// precedes (Claude Code sends the same id as MCP _meta).
	ToolUseID string `json:"tool_use_id,omitempty"`
	ToolName  string `json:"tool_name,omitempty"`
	// Attention is a status event's state (an Attention* constant, or empty
	// to clear); Detail is the runtime's message, such as which permission
	// it asks for. An approve request carries a one-line summary of the
	// tool call in Detail.
	Attention string `json:"attention,omitempty"`
	Detail    string `json:"detail,omitempty"`
	// ToolInput is the tool call's arguments as JSON, for approve requests;
	// pass it through TruncateToolInput first.
	ToolInput json.RawMessage `json:"tool_input,omitempty"`
}

// HookReply is the daemon's single reply line to a HookRequest.
type HookReply struct {
	SignedOn bool   `json:"signed_on"`
	Identity string `json:"identity,omitempty"`
	// Context is Markdown for the agent's context; empty means say nothing.
	Context string `json:"context,omitempty"`
	// Wake reports that a watch ended because new mail arrived.
	Wake bool `json:"wake,omitempty"`
	// Superseded reports that a newer watch for the same agent replaced
	// this one.
	Superseded bool `json:"superseded,omitempty"`
	// Decision answers an approve request: DecisionAllow, DecisionDeny, or
	// empty for no decision (the runtime prompts in its terminal).
	Decision string `json:"decision,omitempty"`
	// Reason is the operator's optional note with a decision.
	Reason string `json:"reason,omitempty"`
}

// WriteLine writes v as one JSON line.
func WriteLine(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("wire: marshal line: %w", err)
	}
	b = append(b, '\n')
	if _, err := w.Write(b); err != nil {
		return fmt.Errorf("wire: write line: %w", err)
	}
	return nil
}

// ReadLine reads one JSON line of at most MaxPreambleBytes into v.
func ReadLine(r *bufio.Reader, v any) error {
	var line []byte
	for {
		chunk, err := r.ReadSlice('\n')
		line = append(line, chunk...)
		if len(line) > MaxPreambleBytes {
			return fmt.Errorf("wire: line exceeds %d bytes", MaxPreambleBytes)
		}
		if err == nil || (errors.Is(err, io.EOF) && len(bytes.TrimSpace(line)) > 0) {
			break
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return fmt.Errorf("wire: read line: %w", err)
	}
	if err := json.Unmarshal(bytes.TrimSpace(line), v); err != nil {
		return fmt.Errorf("wire: decode line: %w", err)
	}
	return nil
}

// WritePreamble writes p as a single JSON line. V is forced to Version.
func WritePreamble(w io.Writer, p Preamble) error {
	p.V = Version
	b, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal preamble: %w", err)
	}
	b = append(b, '\n')
	if _, err := w.Write(b); err != nil {
		return fmt.Errorf("write preamble: %w", err)
	}
	return nil
}

// ReadPreamble reads and validates one preamble line from r. Bytes after the
// line remain buffered in r.
func ReadPreamble(r *bufio.Reader) (Preamble, error) {
	var line []byte
	for {
		chunk, err := r.ReadSlice('\n')
		line = append(line, chunk...)
		if len(line) > MaxPreambleBytes {
			return Preamble{}, fmt.Errorf("%w: exceeds %d bytes", ErrBadPreamble, MaxPreambleBytes)
		}
		if err == nil {
			break
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return Preamble{}, fmt.Errorf("%w: read: %w", ErrBadPreamble, err)
	}
	var p Preamble
	if err := json.Unmarshal(bytes.TrimSpace(line), &p); err != nil {
		return Preamble{}, fmt.Errorf("%w: %w", ErrBadPreamble, err)
	}
	if p.V != Version {
		return Preamble{}, fmt.Errorf("%w: unsupported version %d", ErrBadPreamble, p.V)
	}
	return p, nil
}

// NamespaceFromRemote derives a lowercase host/org/repo namespace from a git
// remote URL. It reports false if the remote cannot be parsed.
func NamespaceFromRemote(remote string) (string, bool) {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return "", false
	}
	var host, path string
	if strings.Contains(remote, "://") {
		u, err := url.Parse(remote)
		if err != nil {
			return "", false
		}
		host, path = u.Hostname(), u.Path
	} else {
		// scp-like: [user@]host:path
		hostPart, p, ok := strings.Cut(remote, ":")
		if !ok {
			return "", false
		}
		if _, h, found := strings.Cut(hostPart, "@"); found {
			hostPart = h
		}
		host, path = hostPart, p
	}
	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	if host == "" || path == "" || !strings.Contains(path, "/") {
		return "", false
	}
	return strings.ToLower(host + "/" + path), true
}

type bufferedConn struct {
	io.Reader
	io.Closer
}

// BufferedConn returns a reader that drains r, which may already hold bytes
// read past the preamble, and whose Close closes c.
func BufferedConn(r *bufio.Reader, c io.Closer) io.ReadCloser {
	return bufferedConn{Reader: r, Closer: c}
}
