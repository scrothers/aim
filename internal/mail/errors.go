package mail

import (
	"errors"
	"fmt"
	"strings"

	"github.com/scrothers/aim/internal/store"
)

// Sentinel errors returned (wrapped) by Service methods.
var (
	// ErrInvalidInput marks a malformed or out-of-range argument.
	ErrInvalidInput = errors.New("invalid input")
	// ErrMailboxNotFound marks a recipient or owner mailbox that does not exist.
	ErrMailboxNotFound = errors.New("mailbox not found")
	// ErrMessageNotFound marks an unknown message or thread id.
	ErrMessageNotFound = errors.New("message not found")
	// ErrNotSignedOn marks a mutation attempted without a signed-on identity.
	ErrNotSignedOn = errors.New("not signed on")
	// ErrExists marks an attempt to register an identity that already exists.
	ErrExists = errors.New("already exists")
	// ErrRetired marks an operation on a retired identity.
	ErrRetired = errors.New("identity retired")
	// ErrReserved marks an attempt to use the reserved operator address as an
	// agent identity.
	ErrReserved = errors.New("address reserved")
	// ErrCycle marks a reports_to chain that loops back on itself or is too
	// deep to follow.
	ErrCycle = errors.New("reports_to cycle")
	// ErrBadQuery marks a search query that is empty or not valid FTS5 syntax.
	ErrBadQuery = fmt.Errorf("invalid search query: %w", store.ErrBadQuery)
)

// MailboxNotFoundError is the concrete error behind ErrMailboxNotFound. It
// carries the missing address and close matches so callers can present them.
type MailboxNotFoundError struct {
	Address     string
	Suggestions []string
	Hint        string
}

// Error implements error.
func (e *MailboxNotFoundError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "mailbox %s does not exist", e.Address)
	if len(e.Suggestions) > 0 {
		fmt.Fprintf(&b, "; did you mean %s?", strings.Join(e.Suggestions, ", "))
	}
	if e.Hint != "" {
		b.WriteString(". ")
		b.WriteString(e.Hint)
	}
	return b.String()
}

// Is makes errors.Is(err, ErrMailboxNotFound) true.
func (e *MailboxNotFoundError) Is(target error) bool { return target == ErrMailboxNotFound }

func invalidf(op, format string, args ...any) error {
	return fmt.Errorf("mail: %s: %w: %s", op, ErrInvalidInput, fmt.Sprintf(format, args...))
}

// invalidAddr wraps an address parse error so it matches both ErrInvalidInput
// and address.ErrInvalid.
func invalidAddr(op string, err error) error {
	return fmt.Errorf("mail: %s: %w: %w", op, ErrInvalidInput, err)
}

func notSignedOn(op string) error {
	return fmt.Errorf("mail: %s: %w: call sign_on with a registered identity first", op, ErrNotSignedOn)
}

// levenshtein returns the edit distance between a and b.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
