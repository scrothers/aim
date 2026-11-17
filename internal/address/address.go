package address

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalid is wrapped by every error returned for a malformed address.
var ErrInvalid = errors.New("address: invalid address")

// Kind is the level of an address.
type Kind int

// Address levels.
const (
	KindNamespace Kind = iota + 1
	KindIdentity
	KindSubagent
	// KindOperator is the single reserved address of the human operator.
	KindOperator
)

// OperatorName is the literal text of the operator address.
const OperatorName = "operator"

// String returns the lower-case name of the kind.
func (k Kind) String() string {
	switch k {
	case KindNamespace:
		return "namespace"
	case KindIdentity:
		return "identity"
	case KindSubagent:
		return "subagent"
	case KindOperator:
		return "operator"
	default:
		return fmt.Sprintf("kind(%d)", int(k))
	}
}

// Address is a parsed AIM address. Repo is empty for org-level addresses;
// Subagent is empty unless Identity is set.
type Address struct {
	Host, Org, Repo, Identity, Subagent string
}

// Scope describes a subtree of the address space for query matching. Host
// and Org are always matched; the other fields are matched only when the
// corresponding Match flag is set.
type Scope struct {
	Host, Org, Repo, Identity, Subagent     string
	MatchRepo, MatchIdentity, MatchSubagent bool
}

const (
	maxPathSegment = 100
	maxName        = 64
	maxLabel       = 63
	maxHost        = 253
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Parse parses and normalizes s into an Address.
func Parse(s string) (Address, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == OperatorName {
		return Operator(), nil
	}
	if s == "" {
		return Address{}, invalid("address is empty; expected [subagent@][identity@]host/org[/repo]")
	}
	parts := strings.Split(s, "@")
	if len(parts) > 3 {
		return Address{}, invalid("%q has %d '@' separators; at most 2 are allowed (subagent@identity@host/org)", s, len(parts)-1)
	}
	path := parts[len(parts)-1]
	var a Address
	switch len(parts) {
	case 3:
		a.Subagent, a.Identity = parts[0], parts[1]
		if err := checkName("subagent", a.Subagent); err != nil {
			return Address{}, err
		}
		if err := checkName("identity", a.Identity); err != nil {
			return Address{}, err
		}
	case 2:
		a.Identity = parts[0]
		if err := checkName("identity", a.Identity); err != nil {
			return Address{}, err
		}
	}

	segs := strings.Split(path, "/")
	switch {
	case len(segs) == 1:
		return Address{}, invalid("%q has no org; expected host/org[/repo] (host-only mailboxes do not exist)", path)
	case len(segs) > 3:
		return Address{}, invalid("path %q is too deep; expected host/org or host/org/repo", path)
	}
	a.Host, a.Org = segs[0], segs[1]
	if len(segs) == 3 {
		a.Repo = segs[2]
		if err := checkPathName("repo", a.Repo); err != nil {
			return Address{}, err
		}
	}
	if err := checkHost(a.Host); err != nil {
		return Address{}, err
	}
	if err := checkPathName("org", a.Org); err != nil {
		return Address{}, err
	}
	return a, nil
}

// Operator returns the reserved operator address. It has no host or org: the
// operator is a single global mailbox.
func Operator() Address { return Address{Identity: OperatorName} }

// MustParse is like Parse but panics on error. Intended for tests.
func MustParse(s string) Address {
	a, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return a
}

// ValidName reports whether name is a valid identity or subagent name:
// 1-64 characters of [a-z0-9._-], starting with a letter or digit.
func ValidName(name string) error {
	return checkName("name", name)
}

func checkName(what, n string) error {
	if n == "" {
		return invalid("%s is empty; give a name before '@'", what)
	}
	if len(n) > maxName {
		return invalid("%s %q is %d characters; maximum is %d", what, n, len(n), maxName)
	}
	for i := range len(n) {
		c := n[i]
		if isAlnum(c) {
			continue
		}
		if i > 0 && (c == '.' || c == '_' || c == '-') {
			continue
		}
		if i == 0 {
			return invalid("%s %q must start with a lower-case letter or digit", what, n)
		}
		return invalid("%s %q contains %q; allowed characters are a-z 0-9 . _ -", what, n, string(c))
	}
	return nil
}

func checkPathName(what, n string) error {
	if n == "" {
		return invalid("%s is empty; remove the extra or trailing '/'", what)
	}
	if len(n) > maxPathSegment {
		return invalid("%s %q is %d characters; maximum is %d", what, n, len(n), maxPathSegment)
	}
	if n[0] == '.' || n[0] == '-' {
		return invalid("%s %q must not start with %q", what, n, string(n[0]))
	}
	for i := range len(n) {
		c := n[i]
		if !isAlnum(c) && c != '.' && c != '_' && c != '-' {
			return invalid("%s %q contains %q; allowed characters are a-z 0-9 . _ -", what, n, string(c))
		}
	}
	return nil
}

func checkHost(h string) error {
	if h == "" {
		return invalid("host is empty; expected host/org, e.g. github.com/acme")
	}
	if h == "localhost" {
		return nil
	}
	if len(h) > maxHost {
		return invalid("host %q is longer than %d characters", h, maxHost)
	}
	if !strings.Contains(h, ".") {
		return invalid("host %q must be a dotted hostname such as github.com, or localhost", h)
	}
	for l := range strings.SplitSeq(h, ".") {
		if l == "" {
			return invalid("host %q has an empty label; check for leading, trailing or doubled dots", h)
		}
		if len(l) > maxLabel {
			return invalid("host %q has a label longer than %d characters", h, maxLabel)
		}
		if l[0] == '-' || l[len(l)-1] == '-' {
			return invalid("host %q has label %q starting or ending with '-'", h, l)
		}
		for i := range len(l) {
			if c := l[i]; !isAlnum(c) && c != '-' {
				return invalid("host %q contains %q; allowed characters are a-z 0-9 - and '.'", h, string(c))
			}
		}
	}
	return nil
}

func isAlnum(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
}

// String returns the canonical form of a.
func (a Address) String() string {
	if a.IsOperator() {
		return OperatorName
	}
	var b strings.Builder
	if a.Subagent != "" {
		b.WriteString(a.Subagent)
		b.WriteByte('@')
	}
	if a.Identity != "" {
		b.WriteString(a.Identity)
		b.WriteByte('@')
	}
	b.WriteString(a.Host)
	b.WriteByte('/')
	b.WriteString(a.Org)
	if a.Repo != "" {
		b.WriteByte('/')
		b.WriteString(a.Repo)
	}
	return b.String()
}

// Kind returns the level of a.
func (a Address) Kind() Kind {
	switch {
	case a.IsOperator():
		return KindOperator
	case a.Subagent != "":
		return KindSubagent
	case a.Identity != "":
		return KindIdentity
	default:
		return KindNamespace
	}
}

// IsOperator reports whether a is the operator address.
func (a Address) IsOperator() bool {
	return a == Address{Identity: OperatorName}
}

// RepoScoped reports whether a lives under a repo rather than directly
// under an org.
func (a Address) RepoScoped() bool { return a.Repo != "" }

// Namespace returns a with identity and subagent stripped.
func (a Address) Namespace() Address {
	a.Identity, a.Subagent = "", ""
	return a
}

// IdentityAddress returns a with the subagent stripped.
func (a Address) IdentityAddress() Address {
	a.Subagent = ""
	return a
}

// WithSubagent returns the subagent address under a, which must be an
// identity address.
func (a Address) WithSubagent(name string) (Address, error) {
	if a.Kind() != KindIdentity {
		return Address{}, invalid("cannot add subagent to %s address %q; it needs an identity address", a.Kind(), a)
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if err := checkName("subagent", name); err != nil {
		return Address{}, err
	}
	a.Subagent = name
	return a, nil
}

// Contains reports whether b lies within the subtree read by a.
func (a Address) Contains(b Address) bool {
	if a.IsOperator() || b.IsOperator() {
		return a == b
	}
	if a.Host != b.Host || a.Org != b.Org {
		return false
	}
	switch a.Kind() {
	case KindNamespace:
		return a.Repo == "" || a.Repo == b.Repo
	case KindIdentity:
		return a.Repo == b.Repo && a.Identity == b.Identity
	default:
		return a.Repo == b.Repo && a.Identity == b.Identity && a.Subagent == b.Subagent
	}
}

// Scope returns the query scope equivalent to Contains for a.
func (a Address) Scope() Scope {
	if a.IsOperator() {
		return Scope{Identity: OperatorName, MatchRepo: true, MatchIdentity: true}
	}
	k := a.Kind()
	return Scope{
		Host:          a.Host,
		Org:           a.Org,
		Repo:          a.Repo,
		Identity:      a.Identity,
		Subagent:      a.Subagent,
		MatchRepo:     k != KindNamespace || a.Repo != "",
		MatchIdentity: k != KindNamespace,
		MatchSubagent: k == KindSubagent,
	}
}
