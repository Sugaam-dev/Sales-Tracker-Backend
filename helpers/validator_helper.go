package helpers

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/go-playground/validator/v10"
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

// FormatValidationError formats validation errors into a clean, human-readable string without exposing struct tags.
func FormatValidationError(err error) string {
	if err == nil {
		return ""
	}

	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		var msgs []string
		for _, fe := range ve {
			field := fe.Field()
			switch fe.Tag() {
			case "required":
				msgs = append(msgs, fmt.Sprintf("%s is required", field))
			case "email":
				msgs = append(msgs, fmt.Sprintf("%s must be a valid email", field))
			case "min":
				msgs = append(msgs, fmt.Sprintf("%s must be at least %s characters", field, fe.Param()))
			case "max":
				msgs = append(msgs, fmt.Sprintf("%s must not exceed %s characters", field, fe.Param()))
			default:
				msgs = append(msgs, fmt.Sprintf("%s is invalid", field))
			}
		}
		return strings.Join(msgs, ", ")
	}

	return "Invalid input data provided."
}
