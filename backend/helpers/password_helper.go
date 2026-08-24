package helpers

import "golang.org/x/crypto/bcrypt"

const bcryptCost = 12

// HashPassword bcrypt-hashes a plaintext password.
func HashPassword(plain string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// ComparePassword reports whether plain matches the stored bcrypt hash.
func ComparePassword(plain, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
