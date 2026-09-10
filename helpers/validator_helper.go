package helpers

import (
	"strings"
	"unicode"
)

// IsEmailIdentifier checks if the login identifier contains an "@" symbol.
func IsEmailIdentifier(identifier string) bool {
	return strings.Contains(identifier, "@")
}

// ValidatePasswordStrength checks that a password is at least 8 characters,
// and contains at least 1 uppercase letter, 1 lowercase letter, 1 number, and 1 special character.
func ValidatePasswordStrength(pwd string) error {
	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, r := range pwd {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSpecial = true
		}
	}

	var problems []string
	if len(pwd) < 8 {
		problems = append(problems, "at least 8 characters")
	}
	if !hasUpper {
		problems = append(problems, "at least 1 uppercase letter")
	}
	if !hasLower {
		problems = append(problems, "at least 1 lowercase letter")
	}
	if !hasDigit {
		problems = append(problems, "at least 1 number")
	}
	if !hasSpecial {
		problems = append(problems, "at least 1 special character")
	}
	if len(problems) > 0 {
		return ErrBadRequest("Password must contain " + strings.Join(problems, ", "))
	}
	return nil
}