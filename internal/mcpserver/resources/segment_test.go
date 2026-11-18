package resources

import (
	"strings"
	"testing"
)

func TestEncodeDecodeSegmentRoundTrip(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"github.com/acme/api", "bob@github.com/acme/api", "sub@bob@h/o/r", "msg-01abc", "a b+c%d", "ünï", "~._-"} {
		enc := EncodeSegment(s)
		if strings.ContainsAny(enc, "/@ +") {
			t.Errorf("EncodeSegment(%q) = %q keeps reserved characters", s, enc)
		}
		if got, ok := decodeSegment(enc); !ok || got != s {
			t.Errorf("decodeSegment(%q) = %q, %v; want %q", enc, got, ok, s)
		}
	}
	for _, s := range []string{"", "a/b", "%zz", "%"} {
		if _, ok := decodeSegment(s); ok {
			t.Errorf("decodeSegment(%q) accepted", s)
		}
	}
}

func TestDescriptionsPresent(t *testing.T) {
	t.Parallel()
	for _, r := range all {
		if got := description(r.name); got == "" || got[len(got)-1] == '\n' {
			t.Errorf("description(%s) = %q, want trimmed non-empty text", r.name, got)
		}
	}
	defer func() {
		if recover() == nil {
			t.Fatal("description of a missing resource did not panic")
		}
	}()
	_ = description("no-such-resource")
}
