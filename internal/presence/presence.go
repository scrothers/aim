package presence

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/wire"
)

// ErrNotIdentity is returned by sign-on when the address is not an identity
// address.
var ErrNotIdentity = errors.New("presence: sign-on requires an identity address")

// Status is a point-in-time snapshot of one connection.
type Status struct {
	ConnID      string
	SessionID   string
	Machine     string
	ProjectDir  string
	Away        string
	ConnectedAt time.Time
	// SignedOnAt is the zero time when the connection is not signed on.
	SignedOnAt time.Time
	// Client and ClientVersion are the MCP clientInfo name and version, empty
	// until the client has been recorded with Entry.SetClient.
	Client        string
	ClientVersion string
	// Identity is the signed-on address, or "" when not signed on.
	Identity string
	// State is StateOnline or StateIdle. A connection is idle when it has made
	// no tool call for the registry's idle threshold.
	State string
	// LastActive is the time of the connection's most recent tool call, or its
	// connection time when it has made none.
	LastActive time.Time
}

// Connection states reported in Status.State.
const (
	StateOnline = "online"
	StateIdle   = "idle"
)

// DefaultIdleAfter is the idle threshold used when none is configured.
const DefaultIdleAfter = 10 * time.Minute

// Event kinds recorded in the event log.
const (
	EventSignedOn     = "signed_on"
	EventSignedOff    = "signed_off"
	EventConnected    = "connected"
	EventDisconnected = "disconnected"
	EventWentIdle     = "went_idle"
	EventActive       = "active"
)

// maxEvents is the capacity of the event ring buffer.
const maxEvents = 500

// Event is one entry of the presence log.
type Event struct {
	Kind     string
	Identity string
	Machine  string
	Client   string
	At       time.Time
}

// Option configures a Registry.
type Option func(*Registry)

// WithClock sets the time source used for events that carry no explicit time
// (sign-off and disconnect). The default is time.Now.
func WithClock(now func() time.Time) Option {
	return func(r *Registry) { r.now = now }
}

// WithIdleAfter sets how long a connection may go without a tool call before
// it is reported idle. Zero or less selects DefaultIdleAfter.
func WithIdleAfter(d time.Duration) Option {
	return func(r *Registry) { r.idleAfter = d }
}

// Registry holds the live connections. Use New to create one.
type Registry struct {
	mu        sync.RWMutex
	entries   map[string]*Entry
	now       func() time.Time
	idleAfter time.Duration
	events    []Event // ring buffer, oldest first once full
	next      int     // index of the slot to overwrite when full
}

// threshold returns the idle threshold, tolerating a zero value.
func (r *Registry) threshold() time.Duration {
	if r.idleAfter <= 0 {
		return DefaultIdleAfter
	}
	return r.idleAfter
}

// sweepLocked records went_idle for signed-on entries that have crossed the
// idle threshold since the last look. Idle transitions are derived lazily, on
// read, rather than by a timer. The caller must hold r.mu for writing.
func (r *Registry) sweepLocked() {
	now := r.clock()
	for _, e := range r.entries {
		e.noteIdleLocked(now)
	}
}

// New returns an empty Registry.
func New(opts ...Option) *Registry {
	r := &Registry{entries: make(map[string]*Entry), now: time.Now}
	for _, o := range opts {
		o(r)
	}
	return r
}

// record appends an event. The caller must hold r.mu for writing.
func (r *Registry) record(ev Event) {
	if len(r.events) < maxEvents {
		r.events = append(r.events, ev)
		return
	}
	r.events[r.next] = ev
	r.next = (r.next + 1) % maxEvents
}

// Events returns up to limit recorded events, newest first. A limit of zero
// or less returns every retained event.
func (r *Registry) Events(limit int) []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sweepLocked()
	n := len(r.events)
	if limit <= 0 || limit > n {
		limit = n
	}
	out := make([]Event, 0, limit)
	// Oldest event is at r.next when the buffer is full, else at 0.
	for i := range limit {
		idx := (r.next + n - 1 - i) % n
		out = append(out, r.events[idx])
	}
	return out
}

// All returns a snapshot of every registered connection, signed on or not,
// ordered by connection time then connection ID.
func (r *Registry) All() []Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sweepLocked()
	all := make([]*Entry, 0, len(r.entries))
	for _, e := range r.entries {
		all = append(all, e)
	}
	slices.SortFunc(all, func(a, b *Entry) int {
		if c := a.connectedAt.Compare(b.connectedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.connID, b.connID)
	})
	out := make([]Status, 0, len(all))
	for _, e := range all {
		out = append(out, e.statusLocked())
	}
	return out
}

// Add registers a connection under connID, replacing any existing connection
// with the same ID. The returned Entry starts signed off.
func (r *Registry) Add(connID string, p wire.Preamble, now time.Time) *Entry {
	e := &Entry{
		reg:         r,
		connID:      connID,
		preamble:    p,
		connectedAt: now,
		lastActive:  now,
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.entries == nil {
		r.entries = make(map[string]*Entry)
	}
	if old, ok := r.entries[connID]; ok {
		r.recordDisconnect(old)
	}
	r.entries[connID] = e
	r.record(Event{Kind: EventConnected, Machine: p.Machine, At: now})
	return e
}

// Remove unregisters the connection connID. It is a no-op for unknown IDs.
func (r *Registry) Remove(connID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[connID]
	if !ok {
		return
	}
	delete(r.entries, connID)
	r.recordDisconnect(e)
}

// recordDisconnect logs the disconnect of e. The caller must hold r.mu for
// writing.
func (r *Registry) recordDisconnect(e *Entry) {
	ev := Event{Kind: EventDisconnected, Machine: e.preamble.Machine, Client: e.client, At: r.clock()}
	if e.signedOn {
		ev.Identity = e.identity.String()
	}
	r.record(ev)
}

// Online returns the connections currently signed on as exactly identity,
// ordered by the time they signed on, oldest first.
func (r *Registry) Online(identity address.Address) []Status {
	want := identity.String()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sweepLocked()
	var matches []*Entry
	for _, e := range r.entries {
		if e.signedOn && e.identity.String() == want {
			matches = append(matches, e)
		}
	}
	slices.SortFunc(matches, func(a, b *Entry) int {
		if c := a.signedOnAt.Compare(b.signedOnAt); c != 0 {
			return c
		}
		return cmp.Compare(a.connID, b.connID)
	})
	out := make([]Status, 0, len(matches))
	for _, e := range matches {
		out = append(out, e.statusLocked())
	}
	return out
}

// Agent returns the connection of the agent identified by a session id, or
// failing that by its process id on machine: the MCP connection a hook
// process of the same agent belongs to. Signed-on connections win over
// signed-off ones, then the most recent connection.
func (r *Registry) Agent(machine, sessionID string, agentPID int) (*Entry, bool) {
	if sessionID == "" && agentPID <= 0 {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var best *Entry
	for _, e := range r.entries {
		p := e.preamble
		match := (sessionID != "" && p.SessionID == sessionID) ||
			(agentPID > 0 && p.AgentPID == agentPID && p.Machine == machine)
		if !match {
			continue
		}
		if best == nil || better(e, best) {
			best = e
		}
	}
	return best, best != nil
}

// better reports whether a is a better agent match than b. The caller must
// hold r.mu.
func better(a, b *Entry) bool {
	if a.signedOn != b.signedOn {
		return a.signedOn
	}
	return a.connectedAt.After(b.connectedAt)
}

// Len returns the number of registered connections, signed on or not.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries)
}

// Entry is one registered connection. Its methods are safe for concurrent use
// and synchronize on the owning Registry's lock.
type Entry struct {
	reg         *Registry
	connID      string
	preamble    wire.Preamble
	connectedAt time.Time

	// Guarded by reg.mu.
	signedOn   bool
	identity   address.Address
	signedOnAt time.Time
	away       string
	client     string
	clientVer  string
	lastActive time.Time
	// idleNoted is true once a went_idle event has been recorded for the
	// current idle spell.
	idleNoted bool
}

// idleLocked reports whether e has been inactive for the idle threshold at
// now. The caller must hold reg.mu.
func (e *Entry) idleLocked(now time.Time) bool {
	return now.Sub(e.lastActive) >= e.reg.threshold()
}

// noteIdleLocked records went_idle once for a signed-on entry that has crossed
// the threshold, timestamped at the moment it crossed. The caller must hold
// reg.mu for writing.
func (e *Entry) noteIdleLocked(now time.Time) {
	if !e.signedOn || e.idleNoted || !e.idleLocked(now) {
		return
	}
	e.idleNoted = true
	e.reg.record(Event{Kind: EventWentIdle, Identity: e.identity.String(), Machine: e.preamble.Machine, Client: e.client, At: e.lastActive.Add(e.reg.threshold())})
}

// Touch records tool-call activity at now. A connection that had gone idle
// becomes active again, which is logged as an active event.
func (e *Entry) Touch(now time.Time) {
	e.reg.mu.Lock()
	defer e.reg.mu.Unlock()
	e.noteIdleLocked(now)
	if e.idleNoted {
		e.idleNoted = false
		e.reg.record(Event{Kind: EventActive, Identity: e.identity.String(), Machine: e.preamble.Machine, Client: e.client, At: now})
	}
	e.lastActive = now
}

// ConnID returns the connection ID this entry was registered under.
func (e *Entry) ConnID() string {
	return e.connID
}

// Preamble returns the preamble the connection presented when it connected.
func (e *Entry) Preamble() wire.Preamble {
	return e.preamble
}

// SignOn signs the connection on as identity, replacing any previous sign-on,
// and sets the away message to away. It returns an error wrapping
// ErrNotIdentity if identity is not an identity address.
func (e *Entry) SignOn(identity address.Address, away string, now time.Time) error {
	if identity.Kind() != address.KindIdentity {
		return fmt.Errorf("%w: got %s address %q", ErrNotIdentity, identity.Kind(), identity)
	}
	e.reg.mu.Lock()
	defer e.reg.mu.Unlock()
	if e.signedOn {
		e.reg.record(Event{Kind: EventSignedOff, Identity: e.identity.String(), Machine: e.preamble.Machine, Client: e.client, At: now})
	}
	e.signedOn = true
	e.idleNoted = false
	e.identity = identity
	e.signedOnAt = now
	e.away = away
	e.reg.record(Event{Kind: EventSignedOn, Identity: identity.String(), Machine: e.preamble.Machine, Client: e.client, At: now})
	return nil
}

// SetClient records the MCP clientInfo name and version of the connection.
func (e *Entry) SetClient(name, version string) {
	e.reg.mu.Lock()
	defer e.reg.mu.Unlock()
	e.client = name
	e.clientVer = version
}

// SetAway sets the away message of the connection.
func (e *Entry) SetAway(away string) {
	e.reg.mu.Lock()
	defer e.reg.mu.Unlock()
	e.away = away
}

// SignOff clears the sign-on and the away message. The connection stays
// registered.
func (e *Entry) SignOff() {
	e.reg.mu.Lock()
	defer e.reg.mu.Unlock()
	if e.signedOn {
		e.reg.record(Event{Kind: EventSignedOff, Identity: e.identity.String(), Machine: e.preamble.Machine, Client: e.client, At: e.reg.clock()})
	}
	e.signedOn = false
	e.idleNoted = false
	e.identity = address.Address{}
	e.signedOnAt = time.Time{}
	e.away = ""
}

// Identity returns the identity the connection is signed on as. The boolean
// is false when the connection is not signed on.
func (e *Entry) Identity() (address.Address, bool) {
	e.reg.mu.RLock()
	defer e.reg.mu.RUnlock()
	return e.identity, e.signedOn
}

// Status returns a snapshot of the connection.
func (e *Entry) Status() Status {
	e.reg.mu.Lock()
	defer e.reg.mu.Unlock()
	e.noteIdleLocked(e.reg.clock())
	return e.statusLocked()
}

// statusLocked builds a Status. The caller must hold reg.mu.
func (e *Entry) statusLocked() Status {
	s := Status{
		ConnID:      e.connID,
		SessionID:   e.preamble.SessionID,
		Machine:     e.preamble.Machine,
		ProjectDir:  e.preamble.ProjectDir,
		Away:        e.away,
		ConnectedAt: e.connectedAt,

		Client:        e.client,
		ClientVersion: e.clientVer,
		State:         StateOnline,
		LastActive:    e.lastActive,
	}
	if e.idleLocked(e.reg.clock()) {
		s.State = StateIdle
	}
	if e.signedOn {
		s.SignedOnAt = e.signedOnAt
		s.Identity = e.identity.String()
	}
	return s
}

// clock returns the registry time, tolerating a zero-value Registry.
func (r *Registry) clock() time.Time {
	if r.now == nil {
		return time.Now()
	}
	return r.now()
}
