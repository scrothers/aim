package opencode_test

import (
	"testing"

	"github.com/scrothers/aim/internal/harness"
	"github.com/scrothers/aim/internal/harness/opencode"
	"github.com/scrothers/aim/internal/wire"
)

func TestParse(t *testing.T) {
	t.Parallel()
	const in = `{"sessionID":"o1","directory":"/p","agent":"build"}`
	tests := []struct {
		name    string
		event   string
		stdin   string
		wantOK  bool
		wantErr bool
		want    harness.Parsed
	}{
		{
			name: "session start", event: "session-start", stdin: in, wantOK: true,
			want: harness.Parsed{SessionID: "o1", Cwd: "/p", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookSessionStart}},
		},
		{
			name: "prompt", event: "prompt", stdin: in, wantOK: true,
			want: harness.Parsed{SessionID: "o1", Cwd: "/p", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookPrompt}},
		},
		{
			name: "session end", event: "session-end", stdin: in, wantOK: true,
			want: harness.Parsed{SessionID: "o1", Cwd: "/p", Request: wire.HookRequest{
				Op: wire.HookOpEvent, Event: wire.HookSessionEnd}},
		},
		{
			name: "watch", event: "watch", stdin: `{"sessionID":"o1","directory":"/p"}`, wantOK: true,
			want: harness.Parsed{SessionID: "o1", Cwd: "/p", Request: wire.HookRequest{Op: wire.HookOpWatch}},
		},
		{name: "missing event", stdin: in},
		{name: "unknown event", event: "idle", stdin: in},
		{name: "bad json", event: "prompt", stdin: `{`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok, err := opencode.New().Parse(tt.event, []byte(tt.stdin), func(string) string { return "" })
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("parsed = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRender(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		req   wire.HookRequest
		reply wire.HookReply
		want  string
	}{
		{"event context", wire.HookRequest{Op: wire.HookOpEvent, Event: wire.HookPrompt}, wire.HookReply{Context: "## Mail"}, "## Mail\n"},
		{"event empty", wire.HookRequest{Op: wire.HookOpEvent, Event: wire.HookPrompt}, wire.HookReply{}, ""},
		{"watch wake", wire.HookRequest{Op: wire.HookOpWatch}, wire.HookReply{Wake: true, Context: "new"}, "new\n"},
		{"watch no wake", wire.HookRequest{Op: wire.HookOpWatch}, wire.HookReply{Context: "stale"}, ""},
		{"watch superseded", wire.HookRequest{Op: wire.HookOpWatch}, wire.HookReply{Superseded: true}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out := opencode.New().Render(tt.req, tt.reply)
			if string(out.Stdout) != tt.want || out.Exit != 0 || len(out.Stderr) != 0 {
				t.Errorf("out = %+v, want stdout %q", out, tt.want)
			}
		})
	}
}

func TestIdentity(t *testing.T) {
	t.Parallel()
	if h := opencode.New(); h.Name() != "opencode" || h.Client() != "opencode" {
		t.Errorf("name/client = %s/%s", h.Name(), h.Client())
	}
}
