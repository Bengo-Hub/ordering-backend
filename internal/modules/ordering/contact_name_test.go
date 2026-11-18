package ordering

import (
	"errors"
	"testing"
)

func TestValidateContactName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // "" means accepted
	}{
		{"first name", "Wanjiku", ""},
		{"full name", "  Wanjiku   Kamau ", ""},
		{"apostrophe and hyphen", "O'Neil-Smith", ""},
		{"initial with dot", "J. Otieno", ""},
		{"other script", "Ζωή", ""},
		{"two letters", "Ng", ""},
		{"empty", "   ", ContactNameMsgRequired},
		{"placeholder", "Customer", ContactNameMsgPlaceholder},
		{"placeholder two words", "A  customer", ContactNameMsgPlaceholder},
		{"swahili placeholder", "MTEJA", ContactNameMsgPlaceholder},
		{"n/a", "N/A", ContactNameMsgPlaceholder},
		{"repeated letter", "aaaa", ContactNameMsgPlaceholder},
		{"repeated letter with spaces", "X x X", ContactNameMsgPlaceholder},
		{"digits", "John2", ContactNameMsgCharacters},
		{"emoji", "Ann 🙂", ContactNameMsgCharacters},
		{"one letter", "J", ContactNameMsgTooShort},
		{"one letter and dot", "J.", ContactNameMsgTooShort},
		{"too long", "Abcdefghij Abcdefghij Abcdefghij Abcdefghij Abcdefghij Abcdefghij", ContactNameMsgTooLong},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateContactName(tc.in, nil)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("ValidateContactName(%q) = %v, want accepted", tc.in, err)
				}
				return
			}
			var cnErr *ContactNameError
			if !errors.As(err, &cnErr) || cnErr.Message != tc.want {
				t.Fatalf("ValidateContactName(%q) = %v, want %q", tc.in, err, tc.want)
			}
		})
	}
}

func TestValidateContactNameTenantList(t *testing.T) {
	list := ParsePlaceholderNames(" Walk In , ,Counter")
	if err := ValidateContactName("walk in", list); err == nil {
		t.Fatal("tenant placeholder accepted")
	}
	// A tenant list replaces the default one.
	if err := ValidateContactName("Customer", list); err != nil {
		t.Fatalf("default placeholder refused under a tenant list: %v", err)
	}
	if got := ParsePlaceholderNames("  "); got != nil {
		t.Fatalf("empty value parsed to %v, want nil", got)
	}
}
