package helpers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
)

// GenerateOTP produces a 6-digit numeric OTP as a string (e.g. "042917",
// leading zeros preserved). The doc's reference implementation uses
// Math.random(), but crypto/rand is used here instead — a login-flow
// OTP is a security control, and a non-cryptographic RNG is guessable
// in principle. Same output shape, stronger source of randomness.
func GenerateOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", fmt.Errorf("helpers: generate otp: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// HashOTP returns the SHA-256 hex digest stored in mfa_otps. The raw
// OTP is sent to the user (email/SMS) and never persisted or logged.
func HashOTP(otp string) string {
	sum := sha256.Sum256([]byte(otp))
	return hex.EncodeToString(sum[:])
}