package helpers

import (
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// IsEmailIdentifier checks if the given identifier contains "@"
func IsEmailIdentifier(identifier string) bool {
	return strings.Contains(identifier, "@")
}

// ComparePassword compares a hashed bcrypt password with its plain text version
func ComparePassword(plain, hashed string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain))
	return err == nil
}
