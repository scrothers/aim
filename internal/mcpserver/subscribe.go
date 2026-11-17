package mcpserver

import (
	"context"
	"crypto/sha256"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// defaultSubscribeDebounce coalesces bursts of mail activity into one
// re-check of the subscribed resources.
const defaultSubscribeDebounce = time.Second

// WithSubscribeDebounce sets how long the subscription watcher waits after a
// mail change before re-checking subscribed resources. Non-positive values keep
// the default.
func WithSubscribeDebounce(d time.Duration) Option {
	return func(h *handlers) {
		if d > 0 {
			h.subs.debounce = d
		}
	}
}

// subscriptions tracks the resources one connection subscribed to. A single
// watcher goroutine starts on the first subscribe and ends with the session.
type subscriptions struct {
	debounce time.Duration
	// watcherExit, when set (tests), is closed once the watcher and its
	// session-end goroutine have both returned.
	watcherExit chan<- struct{}

	mu      sync.Mutex
	started bool
	uris    map[string][sha256.Size]byte // subscribed URI -> fingerprint of last seen content
}

// subscribable reports whether uri names a resource whose content follows mail
// activity.
func subscribable(uri string) bool {
	return uri == uriInbox || strings.HasPrefix(uri, prefixMailbox) || strings.HasPrefix(uri, prefixThread)
}

func (h *handlers) subscribe(ctx context.Context, req *mcp.SubscribeRequest) error {
	uri := req.Params.URI
	if !subscribable(uri) {
		return mcp.ResourceNotFoundError(uri)
	}
	text, err := h.renderURI(ctx, uri)
	if err != nil {
		return h.resourceErr(uri, err)
	}
	h.subs.mu.Lock()
	defer h.subs.mu.Unlock()
	if h.subs.uris == nil {
		h.subs.uris = map[string][sha256.Size]byte{}
	}
	h.subs.uris[uri] = sha256.Sum256([]byte(text))
	if !h.subs.started {
		h.subs.started = true
		h.startWatcher(ctx, req.Session)
	}
	return nil
}

func (h *handlers) unsubscribe(_ context.Context, req *mcp.UnsubscribeRequest) error {
	h.subs.mu.Lock()
	defer h.subs.mu.Unlock()
	delete(h.subs.uris, req.Params.URI)
	return nil
}

// startWatcher runs the watcher until the session ends. The request context is
// detached from the subscribe call but cancelled when the session closes.
func (h *handlers) startWatcher(reqCtx context.Context, ss *mcp.ServerSession) {
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
		h.watch(ctx)
	}()
	if exit := h.subs.watcherExit; exit != nil {
		go func() {
			wg.Wait()
			close(exit)
		}()
	}
}

// watch re-checks subscribed resources after each mail change.
func (h *handlers) watch(ctx context.Context) {
	for {
		// Grab the channel before checking so a change during the re-check
		// triggers the next round.
		changed := h.svc.Changed()
		select {
		case <-ctx.Done():
			return
		case <-changed:
		}
		t := time.NewTimer(h.subs.debounce)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		h.notifyChanged(ctx)
	}
}

// notifyChanged sends ResourceUpdated for every subscribed resource whose
// rendered content differs from what the client last saw.
func (h *handlers) notifyChanged(ctx context.Context) {
	h.subs.mu.Lock()
	uris := make([]string, 0, len(h.subs.uris))
	for u := range h.subs.uris {
		uris = append(uris, u)
	}
	h.subs.mu.Unlock()
	for _, u := range uris {
		text, err := h.renderURI(ctx, u)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue // gone or unreadable now; the client learns on its next read
		}
		sum := sha256.Sum256([]byte(text))
		h.subs.mu.Lock()
		old, still := h.subs.uris[u]
		changed := still && old != sum
		if changed {
			h.subs.uris[u] = sum
		}
		h.subs.mu.Unlock()
		if changed {
			// A failed send means the session is closing; the watcher ends with it.
			_ = h.srv.ResourceUpdated(ctx, &mcp.ResourceUpdatedNotificationParams{URI: u})
		}
	}
}
