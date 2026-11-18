package daemon_test

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/wire"
)

func TestHookConnection(t *testing.T) {
	t.Parallel()
	r := start(t, testCfg(t))
	var d net.Dialer
	c, err := d.DialContext(context.Background(), "tcp", r.addrs.TCP)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	if err := wire.WritePreamble(c, wire.Preamble{Kind: wire.KindHook, Machine: "box", AgentPID: 4242, Client: "claude-code"}); err != nil {
		t.Fatal(err)
	}
	if err := wire.WriteLine(c, wire.HookRequest{Op: wire.HookOpEvent, Event: wire.HookSessionStart}); err != nil {
		t.Fatal(err)
	}
	var reply wire.HookReply
	if err := wire.ReadLine(bufio.NewReader(c), &reply); err != nil {
		t.Fatal(err)
	}
	if reply.SignedOn || !strings.Contains(reply.Context, "not signed on") {
		t.Fatalf("reply = %+v", reply)
	}
}
