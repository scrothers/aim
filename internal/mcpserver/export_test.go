package mcpserver

// WithWatcherExit makes the subscription watcher close ch once its goroutines
// have returned, so tests can prove it ends with the session.
func WithWatcherExit(ch chan<- struct{}) Option {
	return func(h *handlers) { h.subs.watcherExit = ch }
}

// EncodeSegment exposes encodeSegment to external tests.
func EncodeSegment(s string) string { return encodeSegment(s) }
