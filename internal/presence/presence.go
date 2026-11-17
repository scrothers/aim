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
}

// Registry holds the live connections. Use New to create one.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]*Entry
}

// New returns an empty Registry.
func New() *Registry {
	return &Registry{entries: make(map[string]*Entry)}
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
	return e
}

// Remove unregisters the connection connID. It is a no-op for unknown IDs.
func (r *Registry) Remove(connID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, connID)
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
	return nil
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
	}
	if e.signedOn {
		s.SignedOnAt = e.signedOnAt
	}
	return s
}
