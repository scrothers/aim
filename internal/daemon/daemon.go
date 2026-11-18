package daemon

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/oklog/ulid/v2"

	"github.com/scrothers/aim/internal/hooks"
	"github.com/scrothers/aim/internal/httpapi"
	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/maintenance"
	"github.com/scrothers/aim/internal/mcpserver"
	"github.com/scrothers/aim/internal/notify"
	"github.com/scrothers/aim/internal/presence"
	"github.com/scrothers/aim/internal/store"
	"github.com/scrothers/aim/internal/webui"
	"github.com/scrothers/aim/internal/wire"
)

// Defaults and bounds.
const (
	DefaultPreambleTimeout = 5 * time.Second
	shutdownTimeout        = 10 * time.Second
	dialProbeTimeout       = time.Second
)

// ErrAlreadyRunning is returned when another daemon answers on the socket.
var ErrAlreadyRunning = errors.New("daemon already running")

// Addrs holds the bound listener addresses; unused listeners are empty.
type Addrs struct {
	Socket string
	TCP    string
	HTTP   string
}

// Config configures Run. Empty Socket, Listen or HTTP disable that listener.
type Config struct {
	DBPath string
	Socket string
	Listen string
	HTTP   string
	// HTTPAllowedHosts extends the default Host header allowlist of the HTTP
	// listener (localhost, loopback addresses and this machine's hostname).
	HTTPAllowedHosts []string
	Version          string
	PreambleTimeout  time.Duration
	// ShutdownTimeout bounds the wait for connections to drain on shutdown;
	// zero means 10 seconds.
	ShutdownTimeout time.Duration
	// BackupDir is the default scheduled-backup directory, used when the
	// backup settings name none. Empty (with no settings dir) turns scheduled
	// backups off.
	BackupDir string
	// MaintenanceInterval is how often retention and backups are checked;
	// zero means hourly.
	MaintenanceInterval time.Duration
	// NotifyWebhooks pre-seeds the notification webhooks on the first start
	// only (AIM_NOTIFY_WEBHOOKS).
	NotifyWebhooks []string
	// IdleAfter is how long a session may go without a tool call before it is
	// reported idle; zero means presence.DefaultIdleAfter.
	IdleAfter time.Duration
	// AttentionAlert is how long an agent may stay blocked or failed, as its
	// hooks report, before the operator gets a message; zero sends none.
	AttentionAlert time.Duration
	// Ready, if set, is called once all listeners are bound.
	Ready func(Addrs)
}

type daemon struct {
	cfg Config
	// leaked is set when connections outlived the shutdown timeout, so
	// handlers may still be using the store.
	leaked bool
	logger *slog.Logger
	svc    *mail.Service
	reg    *presence.Registry
	maint  *maintenance.Runner
	notify *notify.Manager
	hooks  *hooks.Server

	mu    sync.Mutex
	conns map[net.Conn]struct{}
	wg    sync.WaitGroup
}

// Run serves until ctx is canceled or a listener fails, then shuts down in
// reverse order: listeners, connections, HTTP, store.
func Run(ctx context.Context, cfg Config, logger *slog.Logger) (err error) {
	if cfg.PreambleTimeout <= 0 {
		cfg.PreambleTimeout = DefaultPreambleTimeout
	}
	if cfg.Version == "" {
		cfg.Version = "dev"
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = shutdownTimeout
	}
	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return fmt.Errorf("daemon: open store: %w", err)
	}

	reg := presence.New(presence.WithIdleAfter(cfg.IdleAfter))
	d := &daemon{
		cfg: cfg, logger: logger, reg: reg, conns: make(map[net.Conn]struct{}),
		svc: mail.New(st, mcpserver.NewPresence(reg)),
	}
	d.hooks = hooks.New(d.svc, reg, hooks.WithAlertAfter(cfg.AttentionAlert), hooks.WithLogger(logger),
		hooks.WithApprovalMode(approvalMode(d.svc)))
	defer d.hooks.Close() // runs before the store closes (registered first, runs last otherwise)

	if err := d.svc.EnsureOperator(ctx); err != nil {
		_ = st.Close()
		return fmt.Errorf("daemon: ensure operator mailbox: %w", err)
	}

	defer func() {
		// Closing the store under a live handler would fail its in-flight
		// queries; if conns did not drain, leave it for process exit.
		if d.leakedConns() {
			logger.LogAttrs(ctx, slog.LevelWarn, "leaving store open: connection handlers still running")
			return
		}
		if cerr := st.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("daemon: close store: %w", cerr))
		}
	}()

	mctx, stopMaint := context.WithCancel(ctx)
	maintDone := d.startMaintenance(mctx, st)
	notifyDone := d.startNotify(mctx)
	defer func() { // registered after the store close, so it runs before it
		stopMaint()
		<-maintDone
		<-notifyDone
	}()

	var addrs Addrs
	var lns []net.Listener
	closeLns := func() {
		for _, l := range lns {
			_ = l.Close()
		}
	}
	var lc net.ListenConfig
	if cfg.Socket != "" {
		l, lerr := listenUnix(ctx, &lc, cfg.Socket)
		if lerr != nil {
			return lerr
		}
		defer func() { _ = os.Remove(cfg.Socket) }()
		lns = append(lns, l)
		addrs.Socket = cfg.Socket
		go d.accept(ctx, l)
	}
	if cfg.Listen != "" {
		l, lerr := lc.Listen(ctx, "tcp", cfg.Listen)
		if lerr != nil {
			closeLns()
			d.shutdownConns(ctx)
			return fmt.Errorf("daemon: listen tcp %s: %w", cfg.Listen, lerr)
		}
		lns = append(lns, l)
		addrs.TCP = l.Addr().String()
		go d.accept(ctx, l)
	}

	var hs *http.Server
	httpErr := make(chan error, 1)
	if cfg.HTTP != "" {
		l, lerr := lc.Listen(ctx, "tcp", cfg.HTTP)
		if lerr != nil {
			closeLns()
			d.shutdownConns(ctx)
			return fmt.Errorf("daemon: listen http %s: %w", cfg.HTTP, lerr)
		}
		addrs.HTTP = l.Addr().String()
		d.setPublicURL(addrs.HTTP)
		hs = &http.Server{
			Handler: httpapi.New(httpapi.Deps{
				Pinger: st, Mail: d.svc, Presence: d.reg, Maint: d.maint, Notify: d.notify, Approvals: d.hooks, Static: webui.FS(),
				Version: cfg.Version, Logger: logger, AllowedHosts: cfg.HTTPAllowedHosts,
			}),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       60 * time.Second,
		}
		go func() {
			if serr := hs.Serve(l); serr != nil && !errors.Is(serr, http.ErrServerClosed) {
				httpErr <- serr
			}
		}()
	}

	logger.LogAttrs(ctx, slog.LevelInfo, "daemon started",
		slog.String("socket", addrs.Socket), slog.String("tcp", addrs.TCP), slog.String("http", addrs.HTTP))
	if cfg.Ready != nil {
		cfg.Ready(addrs)
	}

	select {
	case <-ctx.Done():
	case herr := <-httpErr:
		err = fmt.Errorf("daemon: http server: %w", herr)
	}

	logger.LogAttrs(ctx, slog.LevelInfo, "daemon shutting down")
	closeLns()
	d.shutdownConns(ctx)
	if hs != nil {
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
		defer cancel()
		if serr := hs.Shutdown(sctx); serr != nil {
			err = errors.Join(err, fmt.Errorf("daemon: http shutdown: %w", serr))
		}
	}
	return err
}

// approvalMode reads the remote approval setting from the settings store on
// each approve request, so a console toggle applies to the next prompt.
func approvalMode(svc *mail.Service) hooks.ApprovalModeFunc {
	return func(ctx context.Context) (hooks.ApprovalMode, error) {
		a, err := svc.ApprovalSettings(ctx)
		if err != nil {
			return hooks.ApprovalMode{}, fmt.Errorf("daemon: approval settings: %w", err)
		}
		return hooks.ApprovalMode{Enabled: a.Enabled, Timeout: time.Duration(a.TimeoutSeconds) * time.Second}, nil
	}
}

func listenUnix(ctx context.Context, lc *net.ListenConfig, path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("daemon: create socket dir: %w", err)
	}
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("daemon: %s exists and is not a socket", path)
		}
		dctx, cancel := context.WithTimeout(ctx, dialProbeTimeout)
		defer cancel()
		var dialer net.Dialer
		if c, derr := dialer.DialContext(dctx, "unix", path); derr == nil {
			_ = c.Close()
			return nil, fmt.Errorf("daemon: %w at %s", ErrAlreadyRunning, path)
		}
		if rerr := os.Remove(path); rerr != nil {
			return nil, fmt.Errorf("daemon: remove stale socket: %w", rerr)
		}
	}
	l, err := lc.Listen(ctx, "unix", path)
	if err != nil {
		return nil, fmt.Errorf("daemon: listen unix %s: %w", path, err)
	}
	return l, nil
}

func (d *daemon) accept(ctx context.Context, l net.Listener) {
	for {
		c, err := l.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				d.logger.LogAttrs(ctx, slog.LevelError, "accept failed", slog.String("error", err.Error()))
			}
			return
		}
		if !d.track(c) {
			_ = c.Close()
			return
		}
		go d.serve(ctx, c)
	}
}

// track registers a live conn; it reports false once shutdown has begun.
func (d *daemon) track(c net.Conn) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conns == nil {
		return false
	}
	d.conns[c] = struct{}{}
	d.wg.Add(1)
	return true
}

func (d *daemon) untrack(c net.Conn) {
	d.mu.Lock()
	delete(d.conns, c)
	d.mu.Unlock()
	d.wg.Done()
}

func (d *daemon) leakedConns() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.leaked
}

// shutdownConns refuses new conns, closes live ones and waits for handlers;
// when they do not finish in time it records the leak.
func (d *daemon) shutdownConns(ctx context.Context) {
	d.mu.Lock()
	live := d.conns
	d.conns = nil
	d.mu.Unlock()
	for c := range live {
		_ = c.Close()
	}
	done := make(chan struct{})
	go func() { d.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(d.cfg.ShutdownTimeout):
		d.mu.Lock()
		d.leaked = true
		d.mu.Unlock()
		d.logger.LogAttrs(ctx, slog.LevelWarn, "timed out waiting for connections")
	}
}

// restoreSignon signs entry back on when its session signed on within the last
// day, so a daemon restart or a shim reconnect keeps the identity. Failures
// are logged and leave the connection signed off.
func (d *daemon) restoreSignon(ctx context.Context, log *slog.Logger, entry *presence.Entry, p wire.Preamble) {
	so, ok, err := d.svc.RestoreSignon(ctx, p.SessionID)
	if err != nil {
		log.LogAttrs(ctx, slog.LevelWarn, "restore sign-on failed", slog.String("error", err.Error()))
		return
	}
	if !ok {
		return
	}
	if err := entry.SignOn(so.Identity, so.Away, time.Now()); err != nil {
		log.LogAttrs(ctx, slog.LevelWarn, "restore sign-on failed", slog.String("error", err.Error()))
		return
	}
	if so.Client != "" {
		entry.SetClient(so.Client, "")
	}
	log.LogAttrs(ctx, slog.LevelInfo, "sign-on restored", slog.String("identity", so.Identity.String()), slog.Bool("resume", p.Resume))
}

func (d *daemon) serve(ctx context.Context, c net.Conn) {
	defer d.untrack(c)
	defer func() { _ = c.Close() }()

	br := bufio.NewReader(c)
	_ = c.SetReadDeadline(time.Now().Add(d.cfg.PreambleTimeout))
	p, err := wire.ReadPreamble(br)
	if err != nil {
		d.logger.LogAttrs(ctx, slog.LevelWarn, "rejected connection",
			slog.String("remote", c.RemoteAddr().String()), slog.String("error", err.Error()))
		return
	}
	_ = c.SetReadDeadline(time.Time{})

	if p.Kind == wire.KindHook {
		if err := d.hooks.Serve(ctx, c, br, p); err != nil {
			d.logger.LogAttrs(ctx, slog.LevelWarn, "hook failed",
				slog.String("session", p.SessionID), slog.String("client", p.Client), slog.String("error", err.Error()))
		}
		return
	}

	connID := "conn-" + ulid.Make().String()
	entry := d.reg.Add(connID, p, time.Now())
	defer d.reg.Remove(connID)
	log := d.logger.With(slog.String("conn", connID), slog.String("session", p.SessionID), slog.String("machine", p.Machine))
	log.LogAttrs(ctx, slog.LevelInfo, "client connected")
	defer log.LogAttrs(ctx, slog.LevelInfo, "client disconnected")

	d.restoreSignon(ctx, log, entry, p)

	srv := mcpserver.New(d.svc, entry, d.cfg.Version, mcpserver.WithSubagentResolver(d.hooks.SubagentFor))
	f := newInflight(halfCloseGrace)
	ss, err := srv.Connect(ctx, &mcp.IOTransport{Reader: f.reader(br, c), Writer: f.writer(c)}, nil)
	if err != nil {
		log.LogAttrs(ctx, slog.LevelError, "mcp connect failed", slog.String("error", err.Error()))
		return
	}
	_ = ss.Wait()
}
