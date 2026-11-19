package service

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
)

// ErrUnsupported means no supported service manager was found on the host.
var ErrUnsupported = errors.New("service: no supported service manager (systemd --user or launchd); run `aim daemon` under your init system")

// Spec describes the service to render.
type Spec struct {
	// Bin is the absolute path of the aim binary.
	Bin string
	// Env holds extra environment variables for the daemon.
	Env map[string]string
	// Home is the user's home directory.
	Home string
}

// Cmd is one command to run through the host's service manager.
type Cmd struct {
	Argv []string
	// IgnoreErr marks a command whose failure is expected, such as booting
	// out a service that is not loaded.
	IgnoreErr bool
}

// InstallOpts says what changed on disk so a Manager can choose commands.
type InstallOpts struct {
	// NoStart enables or registers the service without starting it.
	NoStart bool
	// Changed is true when the unit file was created or rewritten.
	Changed bool
	// Existed is true when a unit file was already present.
	Existed bool
}

// Manager manages the AIM service on one platform.
type Manager interface {
	// Name is "systemd" or "launchd".
	Name() string
	// Path is the unit or plist file.
	Path() string
	// Dirs lists directories, besides the parent of Path, to create on install.
	Dirs() []string
	// Render returns the unit or plist contents.
	Render(Spec) []byte
	// Install returns the commands to run after the file is written.
	Install(InstallOpts) []Cmd
	// Uninstall returns the commands to run before and after the file is removed.
	Uninstall() (before, after []Cmd)
	Start() []Cmd
	Stop() []Cmd
	Restart() []Cmd
	Status() []Cmd
	// Logs follows the log when follow is set and shows the last n lines.
	Logs(follow bool, n int) []Cmd
	// LingerNote is a hint printed after install, or empty.
	LingerNote() string
}

// Host carries the facts Detect and the constructors need, so tests can fake
// the machine.
type Host struct {
	GOOS          string
	Home          string
	XDGConfigHome string
	UID           int
	LookPath      func(string) (string, error)
	// Exists reports whether a path exists.
	Exists func(string) bool
}

// Detect picks the Manager for the host.
func Detect(h Host) (Manager, error) {
	switch h.GOOS {
	case "darwin":
		return NewLaunchd(h.Home, h.UID), nil
	case "linux":
		if _, err := h.LookPath("systemctl"); err == nil && h.Exists("/run/systemd/system") {
			cfg := h.XDGConfigHome
			if cfg == "" {
				cfg = filepath.Join(h.Home, ".config")
			}
			return NewSystemd(cfg), nil
		}
	}
	return nil, ErrUnsupported
}

func uidString(uid int) string { return strconv.Itoa(uid) }

func lines(n int) string { return fmt.Sprintf("%d", n) }
