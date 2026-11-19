package service

import (
	"bytes"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

const unitName = "aim"

// unitTemplate must stay in step with deploy/aim.service; a golden test
// compares the two.
const unitTemplate = `# Managed by ` + "`aim service install`" + `; re-run it to change this file.
[Unit]
Description=AIM agent inbox daemon
Documentation=https://github.com/scrothers/aim

[Service]
Type=simple
ExecStart={{.ExecStart}}
Restart=on-failure
RestartSec=2

# %t/aim and ~/.local/state/aim are created by systemd.
RuntimeDirectory=aim
StateDirectory=aim
Environment=AIM_SOCKET=%t/aim/aim.sock
Environment=AIM_DB=%S/aim/aim.db
{{- range .Env}}
Environment={{.}}
{{- end}}

NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=%S/aim %t/aim
ProtectKernelTunables=yes
ProtectControlGroups=yes
RestrictSUIDSGID=yes
LockPersonality=yes
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6

[Install]
WantedBy=default.target
`

var unitTmpl = template.Must(template.New("unit").Parse(unitTemplate))

// Systemd manages a systemd --user unit.
type Systemd struct {
	path string
}

var _ Manager = (*Systemd)(nil)

// NewSystemd returns a Manager whose unit lives under configHome/systemd/user.
func NewSystemd(configHome string) *Systemd {
	return &Systemd{path: filepath.Join(configHome, "systemd", "user", unitName+".service")}
}

// Name implements Manager.
func (*Systemd) Name() string { return "systemd" }

// Path implements Manager.
func (s *Systemd) Path() string { return s.path }

// Dirs implements Manager.
func (*Systemd) Dirs() []string { return nil }

// Render implements Manager. Extra variables follow the stock ones, so they
// win under systemd's last-assignment rule. Values may use systemd
// specifiers such as %h and %S.
func (*Systemd) Render(spec Spec) []byte {
	keys := make([]string, 0, len(spec.Env))
	for k := range spec.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, k := range keys {
		env = append(env, systemdQuote(k+"="+spec.Env[k]))
	}
	var buf bytes.Buffer
	// The template is static and the data plain strings; Execute cannot fail.
	_ = unitTmpl.Execute(&buf, struct {
		ExecStart string
		Env       []string
	}{systemdQuote(spec.Bin) + " daemon", env})
	return buf.Bytes()
}

// systemdQuote wraps s in double quotes when it holds characters that systemd
// would split or interpret.
func systemdQuote(s string) string {
	if !strings.ContainsAny(s, " \t\"\\'") {
		return s
	}
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func ctl(args ...string) Cmd {
	return Cmd{Argv: append([]string{"systemctl", "--user"}, args...)}
}

// Install implements Manager.
func (*Systemd) Install(o InstallOpts) []Cmd {
	var cmds []Cmd
	if o.Changed {
		cmds = append(cmds, ctl("daemon-reload"))
	}
	if o.NoStart {
		return append(cmds, ctl("enable", unitName))
	}
	cmds = append(cmds, ctl("enable", "--now", unitName))
	if o.Changed && o.Existed {
		cmds = append(cmds, ctl("restart", unitName))
	}
	return cmds
}

// Uninstall implements Manager.
func (*Systemd) Uninstall() (before, after []Cmd) {
	return []Cmd{ctl("disable", "--now", unitName)}, []Cmd{ctl("daemon-reload")}
}

// Start implements Manager.
func (*Systemd) Start() []Cmd { return []Cmd{ctl("start", unitName)} }

// Stop implements Manager.
func (*Systemd) Stop() []Cmd { return []Cmd{ctl("stop", unitName)} }

// Restart implements Manager.
func (*Systemd) Restart() []Cmd { return []Cmd{ctl("restart", unitName)} }

// Status implements Manager.
func (*Systemd) Status() []Cmd { return []Cmd{ctl("status", unitName, "--no-pager")} }

// Logs implements Manager.
func (*Systemd) Logs(follow bool, n int) []Cmd {
	argv := []string{"journalctl", "--user", "-u", unitName, "-n", lines(n)}
	if follow {
		argv = append(argv, "-f")
	}
	return []Cmd{{Argv: argv}}
}

// LingerNote implements Manager.
func (*Systemd) LingerNote() string {
	return "the daemon stops when you log out; run `loginctl enable-linger $USER` to keep it running"
}
