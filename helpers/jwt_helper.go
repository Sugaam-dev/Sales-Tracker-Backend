package helpers

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"crm-auth-service/conf"
)

// ---------------------------------------------------------------------
// Claims
// ---------------------------------------------------------------------

type TokenType string

const (
	TokenTypeAccess     TokenType = "access"
	TokenTypeFirstLogin TokenType = "first_login"
	TokenTypeMFAPending TokenType = "mfa_pending"
)

// Claims covers every JWT this service issues. Only Type is required on
// every token; Role and Email are populated for access tokens only —
// temp_token and mfa_pending_token carry nothing but user_id and type,
// by design, so they can never be used as session credentials.
type Claims struct {
	UserID uuid.UUID `json:"user_id"`
	Role   string    `json:"role,omitempty"`
	Email  string    `json:"email,omitempty"`
	Type   TokenType `json:"type"`
	jwt.RegisteredClaims
}

// ---------------------------------------------------------------------
// JWT Manager
// ---------------------------------------------------------------------

var (
	ErrInvalidToken = errors.New("jwt: invalid token")
	ErrWrongType    = errors.New("jwt: token type mismatch")
)

// JWTManager holds the signing secret and TTLs so no other package
// needs direct access to conf.JWTConfig or the raw secret.
type JWTManager struct {
	secret        []byte
	accessTTL     time.Duration
	tempTTL       time.Duration
	mfaPendingTTL time.Duration
}

func NewJWTManager(cfg conf.JWTConfig) *JWTManager {
	return &JWTManager{
		secret:        []byte(cfg.Secret),
		accessTTL:     cfg.AccessTokenTTL,
		tempTTL:       cfg.TempTokenTTL,
		mfaPendingTTL: cfg.MFAPendingTokenTTL,
	}
}

// GenerateAccessToken signs the full session JWT — payload
// { user_id, role, email }, 15 min expiry, per the documented contract.
func (m *JWTManager) GenerateAccessToken(userID uuid.UUID, role, email string) (string, error) {
	return m.sign(Claims{UserID: userID, Role: role, Email: email, Type: TokenTypeAccess}, m.accessTTL)
}

// ValidateAccessToken parses and verifies an access token, rejecting
// anything not specifically typed "access" — a stolen temp_token or
// mfa_pending_token can never authenticate a real request.
func (m *JWTManager) ValidateAccessToken(tokenString string) (*Claims, error) {
	return m.parse(tokenString, TokenTypeAccess)
}

// GenerateTempToken signs the first-login onboarding token — payload
// { user_id, type: "first_login" }, 15 min expiry.
func (m *JWTManager) GenerateTempToken(userID uuid.UUID) (string, error) {
	return m.sign(Claims{UserID: userID, Type: TokenTypeFirstLogin}, m.tempTTL)
}

// GenerateMFAPendingToken signs the OTP-step token — payload
// { user_id, type: "mfa_pending" }, 5 min expiry.
func (m *JWTManager) GenerateMFAPendingToken(userID uuid.UUID) (string, error) {
	return m.sign(Claims{UserID: userID, Type: TokenTypeMFAPending}, m.mfaPendingTTL)
}

func (m *JWTManager) sign(claims Claims, ttl time.Duration) (string, error) {
	now := time.Now()
	claims.RegisteredClaims = jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

func (m *JWTManager) parse(tokenString string, want TokenType) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return m.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	if claims.Type != want {
		return nil, ErrWrongType
	}
	return claims, nil
}

// ---------------------------------------------------------------------
// Refresh token (opaque, NOT a JWT — per the doc's crypto.randomBytes(64))
// ---------------------------------------------------------------------

// GenerateRefreshToken produces a 64-byte cryptographically random
// token, hex-encoded. Unlike access/temp tokens, this is opaque: it
// carries no claims, so a leaked token reveals nothing about the user.
func GenerateRefreshToken() (raw string, err error) {
	buf := make([]byte, 64)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("helpers: generate refresh token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// HashRefreshToken returns the SHA-256 hex digest stored in
// refresh_tokens. The raw token is only ever returned to the client
// once, at issue time.
func HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// ValidateRefreshToken re-hashes the raw token and compares it against
// the stored hash in constant time, guarding against timing attacks.
func ValidateRefreshToken(raw, storedHash string) bool {
	computed := HashRefreshToken(raw)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(storedHash)) == 1
}