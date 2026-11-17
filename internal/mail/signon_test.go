package mail_test

import (
	"context"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/mail"
)

func TestSignonSaveRestoreClear(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()
	e.register(t, bobWeb)
	so := mail.Signon{SessionID: "s1", Identity: address.MustParse(bobWeb), Away: "lunch", Client: "claude-code", Machine: "box"}

	if _, ok, err := e.svc.RestoreSignon(ctx, "s1"); err != nil || ok {
		t.Fatalf("restore before save = %v, %v", ok, err)
	}
	if err := e.svc.SaveSignon(ctx, so); err != nil {
		t.Fatal(err)
	}
	got, ok, err := e.svc.RestoreSignon(ctx, "s1")
	if err != nil || !ok || got != so {
		t.Fatalf("restore = %+v, %v, %v; want %+v", got, ok, err, so)
	}
	if err := e.svc.ClearSignon(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := e.svc.RestoreSignon(ctx, "s1"); ok {
		t.Fatal("restore after clear should find nothing")
	}
	// Anonymous sessions are ignored both ways.
	if err := e.svc.SaveSignon(ctx, mail.Signon{Identity: so.Identity}); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := e.svc.RestoreSignon(ctx, ""); ok {
		t.Fatal("empty session id must not restore")
	}
}

func TestSignonExpiresAndSkipsGoneIdentities(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	e := newEnv(t)
	e.register(t, bobWeb, aliceWeb)
	if err := e.svc.SaveSignon(ctx, mail.Signon{SessionID: "old", Identity: address.MustParse(bobWeb)}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SaveSignon(ctx, mail.Signon{SessionID: "ghost", Identity: address.MustParse("nobody@github.com/acme/web")}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SaveSignon(ctx, mail.Signon{SessionID: "gone", Identity: address.MustParse(aliceWeb)}); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := e.svc.RestoreSignon(ctx, "ghost"); ok {
		t.Fatal("unregistered identity must not restore")
	}
	if _, err := e.svc.Retire(ctx, aliceWeb); err != nil {
		t.Fatalf("retire: %v", err)
	}
	if _, ok, _ := e.svc.RestoreSignon(ctx, "gone"); ok {
		t.Fatal("retired identity must not restore")
	}

	// A service whose clock is past the TTL sees the row as expired, and a
	// later save prunes it.
	late := epoch.Add(mail.SignonTTL + time.Hour)
	e2 := mail.New(e.st, e.pres, mail.WithClock(func() time.Time { return late }))
	if _, ok, _ := e2.RestoreSignon(ctx, "old"); ok {
		t.Fatal("expired sign-on must not restore")
	}
}
