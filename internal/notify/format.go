package notify

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

// Kind is the webhook flavor, chosen by URL host.
type Kind string

// Webhook kinds.
const (
	KindDiscord Kind = "discord"
	KindSlack   Kind = "slack"
)

const (
	discordBlurple = 0x5865F2
	maxListed      = 5
)

// card is a rendered notification, independent of the target service.
type card struct {
	Title string
	Body  string
	Link  string
	// Meta is the sender line ("alice@github.com/acme/web via claude-code on devbox").
	Meta string
	At   time.Time
}

// payload renders c for the webhook flavor.
func (c card) payload(k Kind) ([]byte, error) {
	var v any
	switch k {
	case KindDiscord:
		v = c.discord()
	case KindSlack:
		v = c.slack()
	default:
		return nil, fmt.Errorf("notify: unknown kind %q", k)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("notify: encode: %w", err)
	}
	return b, nil
}

func (c card) discord() map[string]any {
	e := map[string]any{
		"title":       truncate(c.Title, 256),
		"description": truncate(c.Body, 2000),
		"url":         c.Link,
		"color":       discordBlurple,
		"footer":      map[string]string{"text": truncate(c.Meta, 2000)},
		"timestamp":   c.At.UTC().Format(time.RFC3339),
	}
	return map[string]any{
		"username":         "AIM",
		"allowed_mentions": map[string]any{"parse": []string{}}, // never ping anyone from message text
		"embeds":           []any{e},
	}
}

func (c card) slack() map[string]any {
	text := fmt.Sprintf("*<%s|%s>*", c.Link, slackEscape(truncate(c.Title, 150)))
	if c.Body != "" {
		text += "\n" + slackEscape(truncate(c.Body, 2800))
	}
	return map[string]any{
		"text": truncate(c.Title, 150),
		"blocks": []any{
			map[string]any{"type": "section", "text": map[string]string{"type": "mrkdwn", "text": text}},
			map[string]any{"type": "context", "elements": []any{
				map[string]string{"type": "mrkdwn", "text": slackEscape(truncate(c.Meta, 500))},
			}},
		},
	}
}

func slackEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

// MaskURL hides the secret part of a webhook URL: it keeps the scheme, host
// and the fixed path prefix and replaces the rest, e.g.
// https://discord.com/api/webhooks/…/****. Query strings and credentials are
// dropped. An unparseable URL is fully masked.
func MaskURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "****"
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	keep := 0
	switch k, _ := defaultKind(u); k {
	case KindDiscord:
		keep = 2 // api/webhooks
	case KindSlack:
		keep = 1 // services
	}
	keep = min(keep, max(len(segs)-1, 0))
	base := u.Scheme + "://" + u.Host
	for _, s := range segs[:keep] {
		base += "/" + s
	}
	return base + "/…/****"
}

// HostCheck classifies a webhook URL, rejecting hosts that are not
// allowed.
type HostCheck func(u *url.URL) (Kind, error)

// defaultKind accepts https URLs on Discord and Slack webhook hosts.
func defaultKind(u *url.URL) (Kind, error) {
	if u.Scheme != "https" {
		return "", fmt.Errorf("webhook url must use https")
	}
	h := strings.ToLower(u.Hostname())
	for _, d := range []string{"discord.com", "discordapp.com"} {
		if h == d || strings.HasSuffix(h, "."+d) {
			return KindDiscord, nil
		}
	}
	if h == "hooks.slack.com" {
		return KindSlack, nil
	}
	return "", fmt.Errorf("webhook host %q is not supported (use discord.com, discordapp.com or hooks.slack.com)", h)
}
