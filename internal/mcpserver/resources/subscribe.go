package resources

import (
	"context"
	"crypto/sha256"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/scrothers/aim/internal/mcpserver/session"
)

// DefaultSubscribeDebounce coalesces bursts of mail activity into one
// re-check of the subscribed resources.
const DefaultSubscribeDebounce = time.Second

// Subscriptions tracks the resources one connection subscribed to. A single
// watcher goroutine starts on the first subscribe and ends with the session.
// Create it with NewSubscriptions, pass Subscribe and Unsubscribe to the
// server options, then Bind the server.
type Subscriptions struct {
	c        *session.Conn
	srv      *mcp.Server
	debounce time.Duration
	// watcherExit, when set (tests), is closed once the watcher and its
	// session-end goroutine have both returned.
	watcherExit chan<- struct{}

	mu      sync.Mutex
	started bool
	uris    map[string][sha256.Size]byte // subscribed URI -> fingerprint of last seen content
}

// NewSubscriptions returns subscriptions for c that re-check debounce after
// each mail change (zero or less means DefaultSubscribeDebounce). A non-nil
// watcherExit is closed once the watcher has stopped.
func NewSubscriptions(c *session.Conn, debounce time.Duration, watcherExit chan<- struct{}) *Subscriptions {
	if debounce <= 0 {
		debounce = DefaultSubscribeDebounce
	}
	return &Subscriptions{c: c, debounce: debounce, watcherExit: watcherExit}
}

// Bind sets the server that sends resource-updated notifications. Call it
// before the server accepts a session.
func (s *Subscriptions) Bind(srv *mcp.Server) { s.srv = srv }

// subscribable reports whether uri names a resource whose content follows mail
// activity.
func subscribable(uri string) bool {
	return uri == URIInbox || strings.HasPrefix(uri, PrefixMailbox) || strings.HasPrefix(uri, PrefixThread)
}

// Subscribe handles resources/subscribe.
func (s *Subscriptions) Subscribe(ctx context.Context, req *mcp.SubscribeRequest) error {
	uri := req.Params.URI
	if !subscribable(uri) {
		return mcp.ResourceNotFoundError(uri)
	}
	text, err := render(ctx, s.c, uri)
	if err != nil {
		return notFound(uri, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.uris == nil {
		s.uris = map[string][sha256.Size]byte{}
	}
	s.uris[uri] = sha256.Sum256([]byte(text))
	if !s.started {
		s.started = true
		s.startWatcher(ctx, req.Session)
	}
	return nil
}

// Unsubscribe handles resources/unsubscribe.
func (s *Subscriptions) Unsubscribe(_ context.Context, req *mcp.UnsubscribeRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.uris, req.Params.URI)
	return nil
}

// startWatcher runs the watcher until the session ends. The request context is
// detached from the subscribe call but cancelled when the session closes.
func (s *Subscriptions) startWatcher(reqCtx context.Context, ss *mcp.ServerSession) {
	ctx, cancel := context.WithCancel(context.WithoutCancel(reqCtx))
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = ss.Wait()
		cancel()
	}()
	go func() {
		defer wg.Done()
		defer cancel()
		s.watch(ctx)
	}()
	if exit := s.watcherExit; exit != nil {
		go func() {
			wg.Wait()
			close(exit)
		}()
	}
}

// watch re-checks subscribed resources after each mail change.
func (s *Subscriptions) watch(ctx context.Context) {
	for {
		// Grab the channel before checking so a change during the re-check
		// triggers the next round.
		changed := s.c.Svc.Changed()
		select {
		case <-ctx.Done():
			return
		case <-changed:
		}
		t := time.NewTimer(s.debounce)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		s.notifyChanged(ctx)
	}
}

// notifyChanged sends ResourceUpdated for every subscribed resource whose
// rendered content differs from what the client last saw.
func (s *Subscriptions) notifyChanged(ctx context.Context) {
	s.mu.Lock()
	uris := make([]string, 0, len(s.uris))
	for u := range s.uris {
		uris = append(uris, u)
	}
	s.mu.Unlock()
	for _, u := range uris {
		text, err := render(ctx, s.c, u)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue // gone or unreadable now; the client learns on its next read
		}
		sum := sha256.Sum256([]byte(text))
		s.mu.Lock()
		old, still := s.uris[u]
		changed := still && old != sum
		if changed {
			s.uris[u] = sum
		}
		s.mu.Unlock()
		if changed {
			// A failed send means the session is closing; the watcher ends with it.
			_ = s.srv.ResourceUpdated(ctx, &mcp.ResourceUpdatedNotificationParams{URI: u})
		}
	}
}
