package address

import (
	"errors"
	"strings"
	"testing"
)

func TestParseValid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want Address
		kind Kind
		str  string
	}{
		{"github.com/acme", Address{Host: "github.com", Org: "acme"}, KindNamespace, "github.com/acme"},
		{"  GitHub.com/ACME/Repo.Go  ", Address{Host: "github.com", Org: "acme", Repo: "repo.go"}, KindNamespace, "github.com/acme/repo.go"},
		{"localhost/org", Address{Host: "localhost", Org: "org"}, KindNamespace, "localhost/org"},
		{"bob@github.com/acme", Address{Host: "github.com", Org: "acme", Identity: "bob"}, KindIdentity, "bob@github.com/acme"},
		{"bob@gitlab.example.org/a_b/c-d", Address{Host: "gitlab.example.org", Org: "a_b", Repo: "c-d", Identity: "bob"}, KindIdentity, "bob@gitlab.example.org/a_b/c-d"},
		{"sub@bob@github.com/acme/r", Address{Host: "github.com", Org: "acme", Repo: "r", Identity: "bob", Subagent: "sub"}, KindSubagent, "sub@bob@github.com/acme/r"},
		{"a1.b-c_d@x.io/o", Address{Host: "x.io", Org: "o", Identity: "a1.b-c_d"}, KindIdentity, "a1.b-c_d@x.io/o"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got, err := Parse(tc.in)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("Parse(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
			if got.Kind() != tc.kind {
				t.Errorf("Kind = %v, want %v", got.Kind(), tc.kind)
			}
			if got.String() != tc.str {
				t.Errorf("String = %q, want %q", got.String(), tc.str)
			}
			again, err := Parse(got.String())
			if err != nil || again != got {
				t.Errorf("round trip = %+v, %v; want %+v", again, err, got)
			}
		})
	}
}

func TestParseInvalid(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("a", 101)
	tests := []struct {
		name, in, wantMsg string
	}{
		{"empty", "", "empty"},
		{"spaces", "   ", "empty"},
		{"host only", "github.com", "no org"},
		{"host only slash", "github.com/", "org is empty"},
		{"trailing slash", "github.com/acme/", "repo is empty"},
		{"double slash", "github.com//acme", "org is empty"},
		{"too deep", "github.com/a/b/c", "too deep"},
		{"no dot host", "github/acme", "dotted hostname"},
		{"empty host", "/acme", "host is empty"},
		{"empty label", "a..com/acme", "empty label"},
		{"trailing dot host", "github.com./acme", "empty label"},
		{"hyphen label", "-a.com/acme", "starting or ending"},
		{"bad host char", "gi_thub.com/acme", "contains"},
		{"long label", strings.Repeat("a", 64) + ".com/acme", "label longer"},
		{"long host", strings.Repeat("a.", 130) + "com/acme", "longer than"},
		{"org dot start", "github.com/.acme", "must not start"},
		{"org dash start", "github.com/-acme", "must not start"},
		{"org bad char", "github.com/ac me", "contains"},
		{"org too long", "github.com/" + long, "maximum"},
		{"repo dot start", "github.com/a/.git", "must not start"},
		{"repo bad char", "github.com/a/r!", "contains"},
		{"three at", "a@b@c@github.com/x", "at most 2"},
		{"empty identity", "@github.com/x", "identity is empty"},
		{"empty subagent", "@bob@github.com/x", "subagent is empty"},
		{"identity bad start", "-bob@github.com/x", "must start"},
		{"identity bad char", "bo!b@github.com/x", "contains"},
		{"identity too long", strings.Repeat("a", 65) + "@github.com/x", "maximum"},
		{"subagent bad", "s!@bob@github.com/x", "contains"},
		{"at in path", "bob@github.com/x@y", "contains"},
		{"slash in identity", "bo/b@github.com/x", "contains"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse(tc.in)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Parse(%q) err = %v, want ErrInvalid", tc.in, err)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("Parse(%q) err = %q, want containing %q", tc.in, err, tc.wantMsg)
			}
		})
	}
}

func TestValidName(t *testing.T) {
	t.Parallel()
	for _, n := range []string{"a", "bob", "a.b_c-d", strings.Repeat("a", 64)} {
		if err := ValidName(n); err != nil {
			t.Errorf("ValidName(%q) = %v", n, err)
		}
	}
	for _, n := range []string{"", "_a", ".a", "A", "a b", strings.Repeat("a", 65)} {
		if err := ValidName(n); !errors.Is(err, ErrInvalid) {
			t.Errorf("ValidName(%q) = %v, want ErrInvalid", n, err)
		}
	}
}

func TestKindString(t *testing.T) {
	t.Parallel()
	want := map[Kind]string{KindNamespace: "namespace", KindIdentity: "identity", KindSubagent: "subagent", Kind(0): "kind(0)"}
	for k, s := range want {
		if k.String() != s {
			t.Errorf("Kind(%d).String() = %q, want %q", int(k), k.String(), s)
		}
	}
}

func TestMustParse(t *testing.T) {
	t.Parallel()
	if got := MustParse("x.io/o").String(); got != "x.io/o" {
		t.Errorf("got %q", got)
	}
	defer func() {
		if recover() == nil {
			t.Error("MustParse did not panic")
		}
	}()
	MustParse("bad")
}

func TestDerivations(t *testing.T) {
	t.Parallel()
	sub := MustParse("s@bob@x.io/o/r")
	if got := sub.Namespace().String(); got != "x.io/o/r" {
		t.Errorf("Namespace = %q", got)
	}
	if got := sub.IdentityAddress().String(); got != "bob@x.io/o/r" {
		t.Errorf("IdentityAddress = %q", got)
	}
	if !sub.RepoScoped() || MustParse("x.io/o").RepoScoped() {
		t.Error("RepoScoped wrong")
	}
	id := MustParse("bob@x.io/o")
	got, err := id.WithSubagent(" Helper ")
	if err != nil || got.String() != "helper@bob@x.io/o" {
		t.Errorf("WithSubagent = %v, %v", got, err)
	}
	if _, err := id.WithSubagent("bad!"); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad name err = %v", err)
	}
	for _, a := range []Address{MustParse("x.io/o"), sub} {
		if _, err := a.WithSubagent("h"); !errors.Is(err, ErrInvalid) {
			t.Errorf("WithSubagent on %s err = %v", a.Kind(), err)
		}
	}
}

func TestContains(t *testing.T) {
	t.Parallel()
	addrs := []string{
		"h.io/o",
		"h.io/o/r",
		"h.io/o/r2",
		"h.io/p",
		"i.io/o",
		"id@h.io/o",
		"id@h.io/o/r",
		"id2@h.io/o",
		"s@id@h.io/o",
		"s@id@h.io/o/r",
		"s2@id@h.io/o",
	}
	// contains[a] lists the addresses that a must contain (including itself).
	contains := map[string][]string{
		"h.io/o":        {"h.io/o", "h.io/o/r", "h.io/o/r2", "id@h.io/o", "id@h.io/o/r", "id2@h.io/o", "s@id@h.io/o", "s@id@h.io/o/r", "s2@id@h.io/o"},
		"h.io/o/r":      {"h.io/o/r", "id@h.io/o/r", "s@id@h.io/o/r"},
		"h.io/o/r2":     {"h.io/o/r2"},
		"h.io/p":        {"h.io/p"},
		"i.io/o":        {"i.io/o"},
		"id@h.io/o":     {"id@h.io/o", "s@id@h.io/o", "s2@id@h.io/o"},
		"id@h.io/o/r":   {"id@h.io/o/r", "s@id@h.io/o/r"},
		"id2@h.io/o":    {"id2@h.io/o"},
		"s@id@h.io/o":   {"s@id@h.io/o"},
		"s@id@h.io/o/r": {"s@id@h.io/o/r"},
		"s2@id@h.io/o":  {"s2@id@h.io/o"},
	}
	for _, as := range addrs {
		want := map[string]bool{}
		for _, w := range contains[as] {
			want[w] = true
		}
		a := MustParse(as)
		for _, bs := range addrs {
			if got := a.Contains(MustParse(bs)); got != want[bs] {
				t.Errorf("%s.Contains(%s) = %v, want %v", as, bs, got, want[bs])
			}
		}
	}
}

func TestScope(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want Scope
	}{
		{"h.io/o", Scope{Host: "h.io", Org: "o"}},
		{"h.io/o/r", Scope{Host: "h.io", Org: "o", Repo: "r", MatchRepo: true}},
		{"id@h.io/o", Scope{Host: "h.io", Org: "o", Identity: "id", MatchRepo: true, MatchIdentity: true}},
		{"id@h.io/o/r", Scope{Host: "h.io", Org: "o", Repo: "r", Identity: "id", MatchRepo: true, MatchIdentity: true}},
		{"s@id@h.io/o/r", Scope{Host: "h.io", Org: "o", Repo: "r", Identity: "id", Subagent: "s", MatchRepo: true, MatchIdentity: true, MatchSubagent: true}},
	}
	for _, tc := range tests {
		if got := MustParse(tc.in).Scope(); got != tc.want {
			t.Errorf("Scope(%s) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func FuzzParse(f *testing.F) {
	for _, s := range []string{"github.com/acme", "a@b@c.io/o/r", "", "@", "x.io//", " X.IO/O ", "localhost/o", "a@@b.io/o"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		a, err := Parse(s)
		if err != nil {
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("error %v does not wrap ErrInvalid", err)
			}
			return
		}
		canon := a.String()
		b, err := Parse(canon)
		if err != nil {
			t.Fatalf("reparse of %q failed: %v", canon, err)
		}
		if b != a || b.String() != canon {
			t.Fatalf("unstable: %+v -> %q -> %+v", a, canon, b)
		}
		if !a.Contains(a) {
			t.Fatalf("%q does not contain itself", canon)
		}
	})
}
