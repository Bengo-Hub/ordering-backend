package ordering

import (
	"context"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// ConfigKeyGuestPlaceholderNames is the service-config key holding the names a guest may not
// give at checkout ("customer", "test" and the like), comma separated. A tenant value replaces
// the platform default; with neither set, DefaultGuestPlaceholderNames applies.
const ConfigKeyGuestPlaceholderNames = "orders.guest_placeholder_names"

// DefaultGuestPlaceholderNames is the fallback list. The storefront keeps the same list bundled
// for when /config cannot be read, so keep the two in step.
var DefaultGuestPlaceholderNames = []string{
	"a customer", "customer", "guest", "test", "testing", "name", "my name", "client",
	"mteja", "me", "n/a", "na", "none", "user", "buyer", "anonymous", "unknown",
	"xxx", "abc", "asdf", "qwerty",
}

// Contact name limits, shared with the storefront's guest form.
const (
	ContactNameMinLength  = 2
	ContactNameMaxLength  = 60
	ContactNameMinLetters = 2
)

// Messages the guest form shows next to the name field.
const (
	ContactNameMsgRequired    = "Please enter your name."
	ContactNameMsgPlaceholder = "Please enter your real first name, e.g. Wanjiku"
	ContactNameMsgTooLong     = "Please keep your name under 60 characters."
	ContactNameMsgCharacters  = "Names can only use letters, spaces, apostrophes, hyphens and dots."
	ContactNameMsgTooShort    = "Please enter at least 2 letters."
)

// contactNamePattern allows letters of any script (with their combining marks), spaces,
// apostrophes, hyphens and dots.
var contactNamePattern = regexp.MustCompile(`^[\p{L}\p{M} '’.\-]+$`)

// ContactNameError is a guest contact name the checkout refuses, with the message to show.
type ContactNameError struct{ Message string }

func (e *ContactNameError) Error() string { return e.Message }

// NormalizeContactName trims the name and folds runs of spaces into one.
func NormalizeContactName(name string) string {
	return strings.Join(strings.Fields(name), " ")
}

// ValidateContactName checks a guest's name: 2 to 60 characters, at least 2 letters, only
// letters, spaces, apostrophes, hyphens and dots, and not a placeholder such as "customer" or
// one letter repeated. A nil placeholders list uses DefaultGuestPlaceholderNames.
func ValidateContactName(name string, placeholders []string) error {
	n := NormalizeContactName(name)
	if n == "" {
		return &ContactNameError{Message: ContactNameMsgRequired}
	}
	// Placeholders first, so "n/a" gets the placeholder message rather than the characters one.
	if placeholders == nil {
		placeholders = DefaultGuestPlaceholderNames
	}
	lower := strings.ToLower(n)
	for _, p := range placeholders {
		if lower == strings.ToLower(NormalizeContactName(p)) {
			return &ContactNameError{Message: ContactNameMsgPlaceholder}
		}
	}
	if utf8.RuneCountInString(n) > ContactNameMaxLength {
		return &ContactNameError{Message: ContactNameMsgTooLong}
	}
	if !contactNamePattern.MatchString(n) {
		return &ContactNameError{Message: ContactNameMsgCharacters}
	}
	letters := make([]rune, 0, len(n))
	for _, r := range n {
		if unicode.IsLetter(r) {
			letters = append(letters, unicode.ToLower(r))
		}
	}
	if utf8.RuneCountInString(n) < ContactNameMinLength || len(letters) < ContactNameMinLetters {
		return &ContactNameError{Message: ContactNameMsgTooShort}
	}
	if oneRepeatedLetter(letters) {
		return &ContactNameError{Message: ContactNameMsgPlaceholder}
	}
	return nil
}

func oneRepeatedLetter(letters []rune) bool {
	if len(letters) == 0 {
		return false
	}
	for _, r := range letters[1:] {
		if r != letters[0] {
			return false
		}
	}
	return true
}

// ParsePlaceholderNames reads the comma separated config value. An empty value gives nil so the
// default list applies.
func ParsePlaceholderNames(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if p := NormalizeContactName(part); p != "" {
			out = append(out, strings.ToLower(p))
		}
	}
	return out
}

// GuestPlaceholderNames is the tenant's placeholder list: its own value, else the platform
// default, else DefaultGuestPlaceholderNames.
func (s *OrderService) GuestPlaceholderNames(ctx context.Context, tenantID uuid.UUID) []string {
	if v, ok := s.repo.GetConfigValue(ctx, tenantID, ConfigKeyGuestPlaceholderNames); ok {
		if names := ParsePlaceholderNames(v); len(names) > 0 {
			return names
		}
	}
	return DefaultGuestPlaceholderNames
}
