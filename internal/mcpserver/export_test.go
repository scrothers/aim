package mcpserver

import "time"

// SetProgressInterval shortens the wait progress interval for a test and
// returns a function that restores it. Tests that use it must not run in
// parallel.
func SetProgressInterval(d time.Duration) (restore func()) {
	old := progressInterval
	progressInterval = d
	return func() { progressInterval = old }
}
