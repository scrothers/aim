package devseed_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/daemon"
	"github.com/scrothers/aim/internal/devseed"
)

var fixedNow = time.Date(2026, 11, 17, 15, 0, 0, 0, time.UTC)

func histCfg(t *testing.T, seed uint64) devseed.HistoryConfig {
	t.Helper()
	return devseed.HistoryConfig{
		DB: filepath.Join(t.TempDir(), "aim.db"), Seed: seed, Days: 3, Messages: 50,
		Now: func() time.Time { return fixedNow },
	}
}

func TestHistoryCounts(t *testing.T) {
	t.Parallel()
	sum, err := devseed.SeedHistory(context.Background(), histCfg(t, 1))
	if err != nil {
		t.Fatal(err)
	}
	if sum.Identities != 22 || sum.Retired != 2 {
		t.Errorf("identities = %d retired = %d, want 22 and 2", sum.Identities, sum.Retired)
	}
	if sum.Messages != 50 || len(sum.Subjects) != 50 {
		t.Errorf("messages = %d subjects = %d, want 50", sum.Messages, len(sum.Subjects))
	}
	if sum.Threads < 10 || sum.Threads > sum.Messages {
		t.Errorf("threads = %d", sum.Threads)
	}
	if sum.OperatorUnread == 0 {
		t.Error("operator has no unread mail")
	}
}

func TestHistoryDeterministic(t *testing.T) {
	t.Parallel()
	a, err := devseed.SeedHistory(context.Background(), histCfg(t, 7))
	if err != nil {
		t.Fatal(err)
	}
	b, err := devseed.SeedHistory(context.Background(), histCfg(t, 7))
	if err != nil {
		t.Fatal(err)
	}
	c, err := devseed.SeedHistory(context.Background(), histCfg(t, 8))
	if err != nil {
		t.Fatal(err)
	}
	if a.Identities != b.Identities || a.Messages != b.Messages || a.Threads != b.Threads || !slices.Equal(a.Subjects, b.Subjects) {
		t.Error("same seed produced different data")
	}
	if slices.Equal(a.Subjects, c.Subjects) {
		t.Error("different seeds produced identical subjects")
	}
}

func TestHistoryBackdated(t *testing.T) {
	t.Parallel()
	sum, err := devseed.SeedHistory(context.Background(), histCfg(t, 3))
	if err != nil {
		t.Fatal(err)
	}
	if !sum.Oldest.Before(fixedNow.Add(-2 * 24 * time.Hour)) {
		t.Errorf("oldest %v is not spread over the window", sum.Oldest)
	}
	if sum.Oldest.Before(fixedNow.Add(-4 * 24 * time.Hour)) {
		t.Errorf("oldest %v is outside the window", sum.Oldest)
	}
	if sum.Newest.After(fixedNow) {
		t.Errorf("newest %v is in the future", sum.Newest)
	}
}

func TestHistoryRefusesTwiceUnlessForced(t *testing.T) {
	t.Parallel()
	cfg := histCfg(t, 1)
	if _, err := devseed.SeedHistory(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := devseed.SeedHistory(context.Background(), cfg); !errors.Is(err, devseed.ErrHistoryExists) {
		t.Fatalf("second run error = %v, want ErrHistoryExists", err)
	}
	cfg.Force = true
	sum, err := devseed.SeedHistory(context.Background(), cfg)
	if err != nil {
		t.Fatalf("forced run: %v", err)
	}
	if sum.Identities != 0 || sum.Existing != 22 || sum.Messages != 50 {
		t.Errorf("forced run = %+v", sum)
	}
}

func getJSON(t *testing.T, url string, v any) {
	t.Helper()
	resp, err := http.Get(url) //nolint:noctx // test against a local listener
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}

func TestLive(t *testing.T) {
	dir := t.TempDir()
	cfg := daemon.Config{DBPath: filepath.Join(dir, "aim.db"), Socket: filepath.Join(dir, "s"), HTTP: "127.0.0.1:0"}
	ready := make(chan daemon.Addrs, 1)
	cfg.Ready = func(a daemon.Addrs) { ready <- a }
	dctx, dcancel := context.WithCancel(context.Background())
	ddone := make(chan error, 1)
	go func() { ddone <- daemon.Run(dctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	t.Cleanup(func() { dcancel(); <-ddone })
	var addrs daemon.Addrs
	select {
	case addrs = <-ready:
	case <-time.After(10 * time.Second):
		t.Fatal("daemon not ready")
	}
	base := "http://" + addrs.HTTP

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	online := make(chan []devseed.Online, 1)
	done := make(chan error, 1)
	go func() {
		done <- devseed.RunLive(ctx, devseed.LiveConfig{
			Addr: addrs.Socket, Seed: 1, Chatter: 20 * time.Millisecond,
			OnReady: func(o []devseed.Online) { online <- o },
		})
	}()
	select {
	case o := <-online:
		if len(o) != 12 {
			t.Fatalf("online = %d, want 12", len(o))
		}
	case err := <-done:
		t.Fatalf("RunLive exited early: %v", err)
	case <-time.After(15 * time.Second):
		t.Fatal("agents not online")
	}

	var pres struct {
		Groups []struct {
			Offline bool `json:"offline"`
			Buddies []struct {
				Client  string `json:"client"`
				Machine string `json:"machine"`
				Online  bool   `json:"online"`
			} `json:"buddies"`
		} `json:"groups"`
	}
	getJSON(t, base+"/api/presence", &pres)
	n := 0
	for _, g := range pres.Groups {
		for _, b := range g.Buddies {
			if b.Online {
				n++
				if b.Client == "" || b.Machine == "" {
					t.Errorf("online buddy lacks client or machine: %+v", b)
				}
			}
		}
	}
	if n < 8 { // chatter may sign a few off already
		t.Errorf("online buddies = %d, want about 12", n)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		var stats struct {
			Messages int `json:"messages"`
		}
		getJSON(t, base+"/api/stats", &stats)
		if stats.Messages > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no chatter message was sent")
		}
		time.Sleep(50 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("RunLive: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("RunLive did not stop")
	}
}
