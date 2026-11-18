package shim

import (
	"context"
	"fmt"
	"net"
)

// Dial connects to the daemon at addr, a unix socket path (optionally
// prefixed "unix:") or a host:port.
func Dial(ctx context.Context, addr string) (net.Conn, error) {
	nw, a := network(addr)
	var d net.Dialer
	conn, err := d.DialContext(ctx, nw, a)
	if err != nil {
		return nil, fmt.Errorf("shim: dial %s: %w", addr, err)
	}
	return conn, nil
}
