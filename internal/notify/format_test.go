package notify

import (
	"net/url"
	"strings"
	"testing"
)

func TestDefaultKind(t *testing.T) {
	t.Parallel()
	tests := []struct {
		raw  string
		want Kind
	}{
		{"https://discord.com/api/webhooks/1/x", KindDiscord},
		{"https://discordapp.com/api/webhooks/1/x", KindDiscord},
		{"https://canary.discord.com/api/webhooks/1/x", KindDiscord},
		{"https://DISCORD.com/api/webhooks/1/x", KindDiscord},
		{"https://hooks.slack.com/services/T/B/x", KindSlack},
		{"http://discord.com/api/webhooks/1/x", ""},
		{"https://discord.com.evil.test/api/webhooks/1/x", ""},
		{"https://evildiscord.com/api/webhooks/1/x", ""},
		{"https://hooks.slack.com.evil.test/services/T/B/x", ""},
		{"https://slack.com/services/T/B/x", ""},
		{"https://example.com/hook", ""},
		{"https://user@discord.com@evil.test/x", ""},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			t.Parallel()
			u, err := url.Parse(tt.raw)
			if err != nil {
				t.Fatal(err)
			}
			got, err := defaultKind(u)
			if got != tt.want || (tt.want == "") != (err != nil) {
				t.Errorf("kind = %q, err = %v, want %q", got, err, tt.want)
			}
		})
	}
}

func TestMaskURL(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"https://discord.com/api/webhooks/123/abcDEF":           "https://discord.com/api/webhooks/…/****",
		"https://discord.com/api/webhooks/123/abcDEF?wait=true": "https://discord.com/api/webhooks/…/****",
		"https://hooks.slack.com/services/T0/B0/xyz":            "https://hooks.slack.com/services/…/****",
		"https://example.com/a/b/c":                             "https://example.com/…/****",
		"https://example.com":                                   "https://example.com/…/****",
		"not a url":                                             "****",
		"":                                                      "****",
		"https://u:pw@hooks.slack.com/services/T0/B0/xyz?token=1": "https://hooks.slack.com/services/…/****",
	}
	for in, want := range tests {
		got := MaskURL(in)
		if got != want {
			t.Errorf("MaskURL(%q) = %q, want %q", in, got, want)
		}
		if strings.Contains(got, "xyz") || strings.Contains(got, "abcDEF") || strings.Contains(got, "pw") {
			t.Errorf("MaskURL(%q) leaks: %q", in, got)
		}
	}
}

func TestDefaultPublicURLFor(t *testing.T) {
	t.Parallel()
	tests := []struct{ listen, want string }{
		{":7172", "http://devbox:7172"},
		{"0.0.0.0:17172", "http://devbox:17172"},
		{"[::]:8080", "http://devbox:8080"},
		{"", ""},
		{"nonsense", ""},
		{"host:", ""},
	}
	for _, tt := range tests {
		if got := DefaultPublicURLFor("devbox", tt.listen); got != tt.want {
			t.Errorf("DefaultPublicURLFor(%q) = %q, want %q", tt.listen, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()
	if got := truncate("héllo wörld", 5); got != "héll…" {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate = %q", got)
	}
	if got := clip(strings.Repeat("x", 400)); len([]rune(got)) != maxErrorRunes+1 {
		t.Errorf("clip length = %d", len([]rune(got)))
	}
	if _, err := (card{}).payload("teams"); err == nil {
		t.Error("unknown kind should fail")
	}
}
