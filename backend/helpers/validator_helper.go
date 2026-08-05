package helpers

import (
	"strings"
	"unicode"
)

// IsEmailIdentifier checks if the login identifier contains an "@" symbol.
func IsEmailIdentifier(identifier string) bool {
	return strings.Contains(identifier, "@")
}

// ValidatePasswordStrength checks that a password is at least 8 characters, has 1 uppercase, and 1 number.
func ValidatePasswordStrength(pwd string) error {
	var hasUpper, hasDigit bool
	for _, r := range pwd {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}

	var problems []string
	if len(pwd) < 8 {
		problems = append(problems, "at least 8 characters")
	}
	if !hasUpper {
		problems = append(problems, "at least 1 uppercase letter")
	}
	if !hasDigit {
		problems = append(problems, "at least 1 number")
	}
	if len(problems) > 0 {
		return ErrBadRequest("Password must contain " + strings.Join(problems, ", "))
	}
	return nil
}