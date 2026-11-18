package hooks

import (
	"context"
	"fmt"
	"time"

	"github.com/scrothers/aim/internal/mdview"
	"github.com/scrothers/aim/internal/wire"
)

// Watch blocks until unread mail newer than the agent's watermark arrives,
// then replies with a nudge and Wake set. It returns an empty reply when the
// agent can't be tracked or ctx ends, and Superseded when a newer watch or
// the session's end replaced it. Mail that was already unread when the agent
// was first seen does not wake it: it was shown at session start.
func (s *Server) Watch(ctx context.Context, p wire.Preamble, _ wire.HookRequest) (wire.HookReply, error) {
	key := agentKey(p)
	if key == "" {
		return wire.HookReply{}, nil
	}
	stop := s.startWatch(key)
	defer s.endWatch(key, stop)
	poll := time.NewTimer(s.poll)
	defer poll.Stop()
	for {
		// Grab the broadcast channel before looking, so a send between the
		// look and the wait still wakes us.
		changed := s.mail.Changed()
		reply, done, err := s.check(ctx, p, key, stop)
		if err != nil || done {
			return reply, err
		}
		poll.Reset(s.poll)
		select {
		case <-ctx.Done():
			return wire.HookReply{}, nil
		case <-stop:
			return wire.HookReply{Superseded: true}, nil
		case <-changed:
		case <-poll.C:
		}
	}
}

// check looks for fresh mail once. done reports that the watch should reply.
func (s *Server) check(ctx context.Context, p wire.Preamble, key string, stop <-chan struct{}) (wire.HookReply, bool, error) {
	id, ok := s.identity(ctx, p)
	if !ok {
		return wire.HookReply{}, false, nil
	}
	fresh, _, err := s.unread(ctx, id.String(), s.watermark(key))
	if err != nil {
		return wire.HookReply{}, true, fmt.Errorf("hooks: watch: %w", err)
	}
	if len(fresh) == 0 {
		return wire.HookReply{}, false, nil
	}
	// Let a burst land so the agent wakes once for all of it.
	t := time.NewTimer(s.debounce)
	select {
	case <-ctx.Done():
		t.Stop()
		return wire.HookReply{}, true, nil
	case <-stop:
		t.Stop()
		return wire.HookReply{Superseded: true}, true, nil
	case <-t.C:
	}
	floor := floorID(s.now())
	fresh, total, err := s.unread(ctx, id.String(), s.watermark(key))
	if err != nil {
		return wire.HookReply{}, true, fmt.Errorf("hooks: watch: %w", err)
	}
	if len(fresh) == 0 {
		// Read elsewhere during the debounce.
		return wire.HookReply{}, false, nil
	}
	s.advance(key, floor, fresh)
	return wire.HookReply{
		SignedOn: true, Identity: id.String(), Wake: true,
		Context: mdview.Nudge(id.String(), fresh[:min(len(fresh), nudgeLimit)], total),
	}, true, nil
}

// startWatch ends the agent's previous watch and registers a new one. An
// agent seen for the first time gets a watermark of now.
func (s *Server) startWatch(key string) chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := s.stateLocked(key)
	if a.watch != nil {
		close(a.watch)
	}
	if a.watermark == "" {
		a.watermark = floorID(s.now())
	}
	a.watch = make(chan struct{})
	return a.watch
}

// endWatch unregisters stop if it is still the agent's current watch.
func (s *Server) endWatch(key string, stop chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a, ok := s.agents[key]; ok && a.watch == stop {
		a.watch = nil
	}
}
