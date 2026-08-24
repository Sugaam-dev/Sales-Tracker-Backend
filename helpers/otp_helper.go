package helpers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
)

// GenerateOTP produces a 6-digit numeric OTP as a string with leading zeros preserved.
func GenerateOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", fmt.Errorf("helpers: generate otp: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// HashOTP returns the SHA-256 hex digest of the raw OTP string.
func HashOTP(otp string) string {
	sum := sha256.Sum256([]byte(otp))
	return hex.EncodeToString(sum[:])
}