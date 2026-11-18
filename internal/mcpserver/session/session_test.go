package session_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/mail"
	"github.com/scrothers/aim/internal/mcpserver/session"
	"github.com/scrothers/aim/internal/presence"
	"github.com/scrothers/aim/internal/wire"
)

func TestCaller(t *testing.T) {
	t.Parallel()
	reg := presence.New()
	e := reg.Add("c1", wire.Preamble{SessionID: "s1", Machine: "box"}, time.Now())
	e.SetClient("claude-code", "1")
	c := &session.Conn{Entry: e}
	if got := c.Caller(); got.SignedOn || got.SessionID != "s1" || got.Machine != "box" || got.Client != "claude-code" {
		t.Errorf("not signed on caller = %+v", got)
	}
	id := address.MustParse("alice@github.com/acme/api")
	if err := e.SignOn(id, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := c.Caller(); !got.SignedOn || got.Identity != id {
		t.Errorf("signed on caller = %+v", got)
	}
}

func TestRequireAddress(t *testing.T) {
	t.Parallel()
	if err := session.RequireAddress("list", "github.com/acme"); err != nil {
		t.Errorf("valid address: %v", err)
	}
	err := session.RequireAddress("list", "  ")
	if !errors.Is(err, mail.ErrInvalidInput) || !strings.Contains(err.Error(), session.AddressHint) {
		t.Errorf("blank address error = %v", err)
	}
}

func TestToolErr(t *testing.T) {
	t.Parallel()
	if session.ToolErr(nil) != nil {
		t.Error("nil error decorated")
	}
	plain := errors.New("boom")
	if !errors.Is(session.ToolErr(plain), plain) || strings.Contains(session.ToolErr(plain).Error(), "grammar") {
		t.Errorf("plain error = %v", session.ToolErr(plain))
	}
	_, perr := address.Parse("not an address")
	got := session.ToolErr(perr)
	if !errors.Is(got, address.ErrInvalid) || !strings.HasSuffix(got.Error(), session.AddressHint) {
		t.Errorf("address error = %v", got)
	}
}

func TestParseTime(t *testing.T) {
	t.Parallel()
	if got, err := session.ParseTime("since", ""); got != nil || err != nil {
		t.Errorf("empty = %v, %v", got, err)
	}
	got, err := session.ParseTime("since", "2026-01-02T15:04:05Z")
	if err != nil || got == nil || got.Year() != 2026 {
		t.Errorf("valid = %v, %v", got, err)
	}
	if _, err := session.ParseTime("since", "yesterday"); err == nil || !strings.HasPrefix(err.Error(), "since must be") {
		t.Errorf("invalid error = %v", err)
	}
}
