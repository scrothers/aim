package wire

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
)

// Version is the preamble protocol version.
const Version = 1

// MaxPreambleBytes bounds the size of the preamble line.
const MaxPreambleBytes = 64 << 10

// ErrBadPreamble is returned when the preamble is missing, oversized,
// malformed or of an unsupported version.
var ErrBadPreamble = errors.New("bad preamble")

// Preamble describes the session on the other end of a connection.
type Preamble struct {
	V          int    `json:"v"`
	SessionID  string `json:"session_id"`
	ProjectDir string `json:"project_dir"`
	Machine    string `json:"machine"`
	GitRemote  string `json:"git_remote,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	// Resume is set by the shim when it redials after losing the daemon
	// connection and replays its MCP handshake.
	Resume bool `json:"resume,omitempty"`
}

// WritePreamble writes p as a single JSON line. V is forced to Version.
func WritePreamble(w io.Writer, p Preamble) error {
	p.V = Version
	b, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshal preamble: %w", err)
	}
	b = append(b, '\n')
	if _, err := w.Write(b); err != nil {
		return fmt.Errorf("write preamble: %w", err)
	}
	return nil
}

// ReadPreamble reads and validates one preamble line from r. Bytes after the
// line remain buffered in r.
func ReadPreamble(r *bufio.Reader) (Preamble, error) {
	var line []byte
	for {
		chunk, err := r.ReadSlice('\n')
		line = append(line, chunk...)
		if len(line) > MaxPreambleBytes {
			return Preamble{}, fmt.Errorf("%w: exceeds %d bytes", ErrBadPreamble, MaxPreambleBytes)
		}
		if err == nil {
			break
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return Preamble{}, fmt.Errorf("%w: read: %w", ErrBadPreamble, err)
	}
	var p Preamble
	if err := json.Unmarshal(bytes.TrimSpace(line), &p); err != nil {
		return Preamble{}, fmt.Errorf("%w: %w", ErrBadPreamble, err)
	}
	if p.V != Version {
		return Preamble{}, fmt.Errorf("%w: unsupported version %d", ErrBadPreamble, p.V)
	}
	return p, nil
}

// NamespaceFromRemote derives a lowercase host/org/repo namespace from a git
// remote URL. It reports false if the remote cannot be parsed.
func NamespaceFromRemote(remote string) (string, bool) {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return "", false
	}
	var host, path string
	if strings.Contains(remote, "://") {
		u, err := url.Parse(remote)
		if err != nil {
			return "", false
		}
		host, path = u.Hostname(), u.Path
	} else {
		// scp-like: [user@]host:path
		hostPart, p, ok := strings.Cut(remote, ":")
		if !ok {
			return "", false
		}
		if _, h, found := strings.Cut(hostPart, "@"); found {
			hostPart = h
		}
		host, path = hostPart, p
	}
	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	if host == "" || path == "" || !strings.Contains(path, "/") {
		return "", false
	}
	return strings.ToLower(host + "/" + path), true
}

type bufferedConn struct {
	io.Reader
	io.Closer
}

// BufferedConn returns a reader that drains r, which may already hold bytes
// read past the preamble, and whose Close closes c.
func BufferedConn(r *bufio.Reader, c io.Closer) io.ReadCloser {
	return bufferedConn{Reader: r, Closer: c}
}
