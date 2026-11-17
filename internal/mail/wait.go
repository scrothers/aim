package mail

import (
	"context"
	"strings"
	"sync"
	"time"
)

// Wait defaults and bounds.
const (
	DefaultWaitTimeout = 60 * time.Second
	MaxWaitTimeout     = 300 * time.Second
	// waitSafetyTick re-queries even without a wake-up, as a backstop for any
	// commit path that does not broadcast.
	waitSafetyTick = 5 * time.Second
)

// notifier is an in-process broadcast: waiters grab the current channel, and
// each broadcast closes it and installs a fresh one. It is internal to the
// mail service and not exposed to the UI. The zero value is ready to use.
type notifier struct {
	mu sync.Mutex
	ch chan struct{}
}

// wait returns a channel that is closed by the next broadcast.
func (n *notifier) wait() <-chan struct{} {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.ch == nil {
		n.ch = make(chan struct{})
	}
	return n.ch
}

// broadcast wakes every goroutine currently waiting.
func (n *notifier) broadcast() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.ch != nil {
		close(n.ch)
		n.ch = nil
	}
}

// WaitInput is the argument of Service.Wait.
type WaitInput struct {
	// Address scopes the wait (covering everything below it). It defaults to
	// the caller's signed-on identity; anonymous callers must set it.
	Address string
	// SinceID, if set, ignores deliveries of messages whose id is not newer.
	SinceID string
	// Timeout bounds the wait; zero means DefaultWaitTimeout and values above
	// MaxWaitTimeout are clamped.
	Timeout time.Duration
}

// WaitResult is the outcome of Service.Wait.
type WaitResult struct {
	Items    []Header `json:"items"`
	TimedOut bool     `json:"timed_out"`
}

// Wait blocks until an unread delivery newer than in.SinceID exists in scope,
// then returns its header(s) like List, or returns TimedOut when the timeout
// passes first. It returns ctx's error if ctx ends.
func (s *Service) Wait(ctx context.Context, c Caller, in WaitInput) (WaitResult, error) {
	const op = "wait"
	addr := strings.TrimSpace(in.Address)
	if addr == "" {
		if !c.SignedOn {
			return WaitResult{}, invalidf(op, "address is required unless you are signed on (then it defaults to your identity)")
		}
		addr = c.Identity.String()
	}
	if in.SinceID != "" && !strings.HasPrefix(in.SinceID, prefixMessage) {
		return WaitResult{}, invalidf(op, "since_id %q is not a message id (msg-...)", in.SinceID)
	}
	timeout := in.Timeout
	switch {
	case timeout < 0:
		return WaitResult{}, invalidf(op, "timeout must not be negative")
	case timeout == 0:
		timeout = DefaultWaitTimeout
	}
	timeout = min(timeout, MaxWaitTimeout)

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(waitSafetyTick)
	defer tick.Stop()
	for {
		// Subscribe before querying so a delivery committed in between wakes
		// the next iteration instead of being missed.
		wake := s.wake.wait()
		res, err := s.List(ctx, ListInput{Address: addr, State: StateUnread, Limit: MaxListLimit})
		if err != nil {
			return WaitResult{}, err
		}
		items := res.Items[:0:0]
		for _, h := range res.Items {
			if h.ID > in.SinceID {
				items = append(items, h)
			}
		}
		if len(items) > 0 {
			return WaitResult{Items: items}, nil
		}
		select {
		case <-wake:
		case <-tick.C:
		case <-deadline.C:
			return WaitResult{Items: []Header{}, TimedOut: true}, nil
		case <-ctx.Done():
			return WaitResult{}, ctx.Err()
		}
	}
}
