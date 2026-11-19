package service_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/scrothers/aim/internal/service"
)

func argvs(cmds []service.Cmd) [][]string {
	out := make([][]string, 0, len(cmds))
	for _, c := range cmds {
		out = append(out, c.Argv)
	}
	return out
}

// body drops the leading comment block, which differs by design.
func body(t *testing.T, b []byte) string {
	t.Helper()
	s := string(b)
	i := strings.Index(s, "[Unit]")
	if i < 0 {
		t.Fatalf("no [Unit] section in %q", s)
	}
	return s[i:]
}

func TestSystemdGolden(t *testing.T) {
	t.Parallel()
	want, err := os.ReadFile(filepath.Join("..", "..", "deploy", "aim.service"))
	if err != nil {
		t.Fatal(err)
	}
	got := service.NewSystemd("/cfg").Render(service.Spec{Bin: "%h/go/bin/aim"})
	if body(t, got) != body(t, want) {
		t.Errorf("rendered unit drifted from deploy/aim.service:\n%s", got)
	}
}

func TestSystemdRenderEnvAndQuoting(t *testing.T) {
	t.Parallel()
	got := string(service.NewSystemd("/cfg").Render(service.Spec{
		Bin: "/opt/my aim/aim",
		Env: map[string]string{"AIM_HTTP": ":9", "AIM_BACKUP_DIR": "/a b/c"},
	}))
	for _, want := range []string{
		`ExecStart="/opt/my aim/aim" daemon`,
		"Environment=AIM_HTTP=:9\n",
		`Environment="AIM_BACKUP_DIR=/a b/c"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Index(got, "AIM_BACKUP_DIR") > strings.Index(got, "AIM_HTTP") {
		t.Errorf("env not sorted:\n%s", got)
	}
	if strings.Index(got, "Environment=AIM_DB") > strings.Index(got, "AIM_BACKUP_DIR") {
		t.Errorf("user env must follow stock env:\n%s", got)
	}
}

func TestSystemdPathAndCommands(t *testing.T) {
	t.Parallel()
	s := service.NewSystemd("/cfg")
	if s.Path() != "/cfg/systemd/user/aim.service" || s.Name() != "systemd" || s.Dirs() != nil {
		t.Errorf("path/name/dirs: %q %q %v", s.Path(), s.Name(), s.Dirs())
	}
	tests := []struct {
		name string
		got  []service.Cmd
		want [][]string
	}{
		{"fresh", s.Install(service.InstallOpts{Changed: true}), [][]string{
			{"systemctl", "--user", "daemon-reload"}, {"systemctl", "--user", "enable", "--now", "aim"}}},
		{"changed", s.Install(service.InstallOpts{Changed: true, Existed: true}), [][]string{
			{"systemctl", "--user", "daemon-reload"}, {"systemctl", "--user", "enable", "--now", "aim"},
			{"systemctl", "--user", "restart", "aim"}}},
		{"unchanged", s.Install(service.InstallOpts{Existed: true}), [][]string{
			{"systemctl", "--user", "enable", "--now", "aim"}}},
		{"no-start", s.Install(service.InstallOpts{Changed: true, NoStart: true}), [][]string{
			{"systemctl", "--user", "daemon-reload"}, {"systemctl", "--user", "enable", "aim"}}},
		{"start", s.Start(), [][]string{{"systemctl", "--user", "start", "aim"}}},
		{"stop", s.Stop(), [][]string{{"systemctl", "--user", "stop", "aim"}}},
		{"restart", s.Restart(), [][]string{{"systemctl", "--user", "restart", "aim"}}},
		{"status", s.Status(), [][]string{{"systemctl", "--user", "status", "aim", "--no-pager"}}},
		{"logs", s.Logs(false, 20), [][]string{{"journalctl", "--user", "-u", "aim", "-n", "20"}}},
		{"logs follow", s.Logs(true, 5), [][]string{{"journalctl", "--user", "-u", "aim", "-n", "5", "-f"}}},
	}
	for _, tc := range tests {
		if got := argvs(tc.got); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
	before, after := s.Uninstall()
	if !reflect.DeepEqual(argvs(before), [][]string{{"systemctl", "--user", "disable", "--now", "aim"}}) ||
		!reflect.DeepEqual(argvs(after), [][]string{{"systemctl", "--user", "daemon-reload"}}) {
		t.Errorf("uninstall: %v %v", before, after)
	}
	if !strings.Contains(s.LingerNote(), "enable-linger") {
		t.Errorf("linger note: %q", s.LingerNote())
	}
}

func TestLaunchdRender(t *testing.T) {
	t.Parallel()
	l := service.NewLaunchd("/Users/me", 501)
	got := string(l.Render(service.Spec{
		Bin:  "/opt/a&b/aim",
		Home: "/Users/me",
		Env:  map[string]string{"AIM_HTTP": "<:7172>", "AIM_DB": "/Users/me/x.db"},
	}))
	for _, want := range []string{
		"<string>io.github.scrothers.aim</string>",
		"<string>/opt/a&amp;b/aim</string>\n\t\t<string>daemon</string>",
		"<key>SuccessfulExit</key>\n\t\t<false/>",
		"<string>Background</string>",
		"<key>HOME</key>\n\t\t<string>/Users/me</string>",
		"<string>&lt;:7172&gt;</string>",
		"<key>StandardOutPath</key>\n\t<string>/Users/me/Library/Logs/aim/aim.log</string>",
		"<key>StandardErrorPath</key>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Index(got, "AIM_DB") > strings.Index(got, "AIM_HTTP") {
		t.Errorf("env not sorted:\n%s", got)
	}
	if !strings.HasPrefix(got, "<?xml") || !strings.HasSuffix(got, "</plist>\n") {
		t.Errorf("bad envelope:\n%s", got)
	}
}

func TestLaunchdPathAndCommands(t *testing.T) {
	t.Parallel()
	l := service.NewLaunchd("/Users/me", 501)
	const plist = "/Users/me/Library/LaunchAgents/io.github.scrothers.aim.plist"
	const target = "gui/501/io.github.scrothers.aim"
	if l.Path() != plist || l.Name() != "launchd" || l.LingerNote() != "" {
		t.Errorf("path/name: %q %q", l.Path(), l.Name())
	}
	if !reflect.DeepEqual(l.Dirs(), []string{"/Users/me/Library/Logs/aim"}) {
		t.Errorf("dirs: %v", l.Dirs())
	}
	tests := []struct {
		name string
		got  []service.Cmd
		want [][]string
	}{
		{"changed", l.Install(service.InstallOpts{Changed: true}), [][]string{
			{"launchctl", "bootout", target}, {"launchctl", "bootstrap", "gui/501", plist}}},
		{"unchanged", l.Install(service.InstallOpts{Existed: true}), [][]string{
			{"launchctl", "bootstrap", "gui/501", plist}, {"launchctl", "kickstart", target}}},
		{"no-start", l.Install(service.InstallOpts{Changed: true, NoStart: true}), [][]string{}},
		{"start", l.Start(), [][]string{{"launchctl", "kickstart", target}}},
		{"stop", l.Stop(), [][]string{{"launchctl", "kill", "SIGTERM", target}}},
		{"restart", l.Restart(), [][]string{{"launchctl", "kickstart", "-k", target}}},
		{"status", l.Status(), [][]string{{"launchctl", "print", target}}},
		{"logs", l.Logs(false, 100), [][]string{{"tail", "-n", "100", "/Users/me/Library/Logs/aim/aim.log"}}},
		{"logs follow", l.Logs(true, 3), [][]string{{"tail", "-n", "3", "-f", "/Users/me/Library/Logs/aim/aim.log"}}},
	}
	for _, tc := range tests {
		if got := argvs(tc.got); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
	before, after := l.Uninstall()
	if !reflect.DeepEqual(argvs(before), [][]string{{"launchctl", "bootout", target}}) || len(after) != 0 || !before[0].IgnoreErr {
		t.Errorf("uninstall: %v %v", before, after)
	}
	if !l.Install(service.InstallOpts{Changed: true})[0].IgnoreErr {
		t.Error("bootout before bootstrap must tolerate failure")
	}
}

func TestDetect(t *testing.T) {
	t.Parallel()
	have := func(string) (string, error) { return "/bin/systemctl", nil }
	lack := func(string) (string, error) { return "", errors.New("not found") }
	yes := func(string) bool { return true }
	no := func(string) bool { return false }
	tests := []struct {
		name string
		host service.Host
		want string
		path string
		err  bool
	}{
		{"darwin", service.Host{GOOS: "darwin", Home: "/h", LookPath: lack, Exists: no}, "launchd", "", false},
		{"linux", service.Host{GOOS: "linux", Home: "/h", LookPath: have, Exists: yes}, "systemd", "/h/.config/systemd/user/aim.service", false},
		{"linux xdg", service.Host{GOOS: "linux", Home: "/h", XDGConfigHome: "/x", LookPath: have, Exists: yes}, "systemd", "/x/systemd/user/aim.service", false},
		{"linux no systemctl", service.Host{GOOS: "linux", LookPath: lack, Exists: yes}, "", "", true},
		{"linux not booted by systemd", service.Host{GOOS: "linux", LookPath: have, Exists: no}, "", "", true},
		{"windows", service.Host{GOOS: "windows", LookPath: have, Exists: yes}, "", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, err := service.Detect(tc.host)
			if tc.err {
				if !errors.Is(err, service.ErrUnsupported) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil || m.Name() != tc.want {
				t.Fatalf("got %v, %v", m, err)
			}
			if tc.path != "" && m.Path() != tc.path {
				t.Errorf("path = %q, want %q", m.Path(), tc.path)
			}
		})
	}
}
