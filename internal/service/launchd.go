package service

import (
	"bytes"
	"encoding/xml"
	"path/filepath"
	"sort"
)

// Label is the launchd job label.
const Label = "io.github.scrothers.aim"

// Launchd manages a per-user LaunchAgent.
type Launchd struct {
	home string
	uid  int
}

var _ Manager = (*Launchd)(nil)

// NewLaunchd returns a Manager for the user's LaunchAgent under home.
func NewLaunchd(home string, uid int) *Launchd { return &Launchd{home: home, uid: uid} }

// Name implements Manager.
func (*Launchd) Name() string { return "launchd" }

// Path implements Manager.
func (l *Launchd) Path() string {
	return filepath.Join(l.home, "Library", "LaunchAgents", Label+".plist")
}

// LogPath is the file the daemon's stdout and stderr go to.
func (l *Launchd) LogPath() string { return filepath.Join(l.home, "Library", "Logs", "aim", "aim.log") }

// Dirs implements Manager.
func (l *Launchd) Dirs() []string { return []string{filepath.Dir(l.LogPath())} }

func (l *Launchd) domain() string { return "gui/" + uidString(l.uid) }

func (l *Launchd) target() string { return l.domain() + "/" + Label }

// Render implements Manager.
func (l *Launchd) Render(spec Spec) []byte {
	env := map[string]string{"HOME": spec.Home}
	for k, v := range spec.Env {
		env[k] = v
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b bytes.Buffer
	raw := func(s string) { b.WriteString(s) }
	text := func(s string) { _ = xml.EscapeText(&b, []byte(s)) }
	key := func(k string) { raw("\t<key>"); text(k); raw("</key>\n") }
	str := func(k, v string) { key(k); raw("\t<string>"); text(v); raw("</string>\n") }

	raw(xml.Header)
	raw("<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n")
	raw("<plist version=\"1.0\">\n<dict>\n")
	str("Label", Label)
	key("ProgramArguments")
	raw("\t<array>\n")
	for _, a := range []string{spec.Bin, "daemon"} {
		raw("\t\t<string>")
		text(a)
		raw("</string>\n")
	}
	raw("\t</array>\n")
	key("RunAtLoad")
	raw("\t<true/>\n")
	key("KeepAlive")
	raw("\t<dict>\n\t\t<key>SuccessfulExit</key>\n\t\t<false/>\n\t</dict>\n")
	str("ProcessType", "Background")
	key("EnvironmentVariables")
	raw("\t<dict>\n")
	for _, k := range keys {
		raw("\t\t<key>")
		text(k)
		raw("</key>\n\t\t<string>")
		text(env[k])
		raw("</string>\n")
	}
	raw("\t</dict>\n")
	str("StandardOutPath", l.LogPath())
	str("StandardErrorPath", l.LogPath())
	raw("</dict>\n</plist>\n")
	return b.Bytes()
}

func launchctl(ignore bool, args ...string) Cmd {
	return Cmd{Argv: append([]string{"launchctl"}, args...), IgnoreErr: ignore}
}

// Install implements Manager. A changed plist is booted out and bootstrapped
// again; an unchanged one is only (re)loaded and started if idle.
func (l *Launchd) Install(o InstallOpts) []Cmd {
	if o.NoStart {
		return nil
	}
	if o.Changed {
		return []Cmd{
			launchctl(true, "bootout", l.target()),
			launchctl(false, "bootstrap", l.domain(), l.Path()),
		}
	}
	return []Cmd{
		launchctl(true, "bootstrap", l.domain(), l.Path()),
		launchctl(false, "kickstart", l.target()),
	}
}

// Uninstall implements Manager.
func (l *Launchd) Uninstall() (before, after []Cmd) {
	return []Cmd{launchctl(true, "bootout", l.target())}, nil
}

// Start implements Manager.
func (l *Launchd) Start() []Cmd { return []Cmd{launchctl(false, "kickstart", l.target())} }

// Stop implements Manager. A clean exit does not trigger KeepAlive.
func (l *Launchd) Stop() []Cmd {
	return []Cmd{launchctl(false, "kill", "SIGTERM", l.target())}
}

// Restart implements Manager.
func (l *Launchd) Restart() []Cmd {
	return []Cmd{launchctl(false, "kickstart", "-k", l.target())}
}

// Status implements Manager.
func (l *Launchd) Status() []Cmd { return []Cmd{launchctl(false, "print", l.target())} }

// Logs implements Manager.
func (l *Launchd) Logs(follow bool, n int) []Cmd {
	argv := []string{"tail", "-n", lines(n)}
	if follow {
		argv = append(argv, "-f")
	}
	return []Cmd{{Argv: append(argv, l.LogPath())}}
}

// LingerNote implements Manager; LaunchAgents need no linger setting.
func (*Launchd) LingerNote() string { return "" }
