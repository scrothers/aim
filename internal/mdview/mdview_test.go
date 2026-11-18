package mdview_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mdview"
)

// golden compares got with testdata/<name>.golden; UPDATE_SNAPSHOTS=1 rewrites it.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if os.Getenv("UPDATE_SNAPSHOTS") != "" {
		if err := os.MkdirAll("testdata", 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v (UPDATE_SNAPSHOTS=1 creates it)", err)
	}
	if got != string(want) {
		t.Errorf("%s mismatch\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

var t0 = time.Date(2026, 1, 2, 15, 4, 0, 0, time.UTC)

func fixtureMessages() []mail.Message {
	return []mail.Message{
		{
			ID: "msg-1", ThreadID: "thr-1", From: "alice@github.com/acme/api", Subject: "deploy\nplan",
			Body: "ship the rollout\n\n", CreatedAt: t0, To: []string{"bob@github.com/acme/api"}, Cc: []string{"github.com/acme/api"},
		},
		{
			ID: "msg-2", ThreadID: "thr-1", InReplyTo: "msg-1", From: "bob@github.com/acme/api", Subject: "re: deploy plan",
			Body: "ack", CreatedAt: t0.Add(time.Minute), To: nil,
		},
	}
}

func TestRenderGolden(t *testing.T) {
	t.Parallel()
	hdrs := []mail.Header{
		{ID: "msg-2", ThreadID: "thr-1", From: "bob@github.com/acme/api", Subject: "re: deploy plan", Mailbox: "alice@github.com/acme/api", CreatedAt: t0, Snippet: "ack\n  ok", ThreadCount: 2},
		{ID: "msg-3", ThreadID: "thr-3", From: "x@h/o", Subject: "no snippet", Mailbox: "alice@github.com/acme/api", CreatedAt: t0, ThreadCount: 1},
	}
	roster := []mail.RosterEntry{
		{
			Address: "alice@github.com/acme/api", Role: "lead", Description: "runs it", ReportsTo: "operator", Subagents: []string{"helper"},
			Online: []mail.OnlineStatus{{Machine: "box", Away: "lunch", State: "idle"}, {Machine: "box2"}},
		},
		{Address: "bob@github.com/acme/api", Role: "dev", Online: []mail.OnlineStatus{}},
	}
	tests := []struct {
		name, got string
	}{
		{"transcript", mdview.Transcript(fixtureMessages())},
		{"message", mdview.Message(fixtureMessages()[0])},
		{"digest", mdview.Digest("Unread", hdrs, true)},
		{"digest_empty", mdview.Digest("Unread", nil, false)},
		{"roster", mdview.Roster("Roster", roster)},
		{"roster_empty", mdview.Roster("Roster", nil)},
		{"whoami", mdview.Whoami(mdview.WhoamiInfo{SessionID: "s", Machine: "box", Namespace: "github.com/acme/api", SignedOnAs: "alice@github.com/acme/api", Client: "claude-code", ClientVersion: "2"})},
		{"whoami_anon", mdview.Whoami(mdview.WhoamiInfo{SessionID: "s"})},
		{"transcript_empty", mdview.Transcript(nil)},
		{"nudge", mdview.Nudge("alice@github.com/acme/api", hdrs, 5)},
		{"nudge_all", mdview.Nudge("alice@github.com/acme/api", hdrs[:1], 1)},
		{"roster_brief", mdview.RosterBrief(roster)},
		{"roster_brief_empty", mdview.RosterBrief(nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			golden(t, tt.name, tt.got)
		})
	}
}
