package mcpserver_test

import (
	"testing"
	"time"

	"github.com/scrothers/aim/internal/address"
	"github.com/scrothers/aim/internal/mcpserver"
	"github.com/scrothers/aim/internal/presence"
	"github.com/scrothers/aim/internal/wire"
)

func TestPresenceAdapterAttention(t *testing.T) {
	t.Parallel()
	reg := presence.New()
	now := time.Now()
	id := address.MustParse("alice@github.com/acme/web")
	e := reg.Add("c1", wire.Preamble{SessionID: "s1", Machine: "box"}, now)
	if err := e.SignOn(id, "", now); err != nil {
		t.Fatal(err)
	}
	e.SetAttention(wire.AttentionBlocked, "needs permission", now)

	got := mcpserver.NewPresence(reg).Online(id)
	if len(got) != 1 {
		t.Fatalf("online = %d, want 1", len(got))
	}
	if got[0].Attention != wire.AttentionBlocked || got[0].AttentionDetail != "needs permission" || got[0].AttentionSince.IsZero() {
		t.Errorf("status = %+v", got[0])
	}
}
