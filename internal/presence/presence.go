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
}

// Event kinds recorded in the event log.
const (
	EventSignedOn     = "signed_on"
	EventSignedOff    = "signed_off"
	EventConnected    = "connected"
	EventDisconnected = "disconnected"
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

// Registry holds the live connections. Use New to create one.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]*Entry
	now     func() time.Time
	events  []Event // ring buffer, oldest first once full
	next    int     // index of the slot to overwrite when full
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
	r.mu.RLock()
	defer r.mu.RUnlock()
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
	r.mu.RLock()
	defer r.mu.RUnlock()
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
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.entries == nil {
		r.entries = make(map[string]*Entry)
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
	r.mu.RLock()
	defer r.mu.RUnlock()
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
	e.signedOn = true
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
	e.reg.mu.RLock()
	defer e.reg.mu.RUnlock()
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
