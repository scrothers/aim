package devwatch

import (
	"os"
	"os/signal"
	"syscall"
)

func ignoreTerm() { signal.Ignore(syscall.SIGTERM) }

// blockForever parks the fake daemon until it is killed. A bare `select {}`
// trips the runtime's deadlock detector when no other goroutine exists (the
// race detector's helper goroutine masked this under -race); a registered
// signal channel keeps the runtime's signal goroutine alive instead.
func blockForever() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGUSR1)
	for {
		<-c // SIGUSR1 is never sent; only SIGKILL ends this process.
	}
}
