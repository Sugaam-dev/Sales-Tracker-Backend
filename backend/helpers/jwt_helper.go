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

// TokenType defines the purpose or category of the generated JSON Web Token.
type TokenType string

const (
	// TokenTypeAccess represents a standard session authorization token.
	TokenTypeAccess TokenType = "access"
	// TokenTypeFirstLogin represents a temporary token indicating onboarding is required.
	TokenTypeFirstLogin TokenType = "first_login"
	// TokenTypeMFAPending represents a temporary token indicating MFA verification is required.
	TokenTypeMFAPending TokenType = "mfa_pending"
)

// Claims represents the JWT payload metadata.
type Claims struct {
	UserID uuid.UUID `json:"user_id"`
	Role   string    `json:"role,omitempty"`
	Email  string    `json:"email,omitempty"`
	Type   TokenType `json:"type"`
	jwt.RegisteredClaims
}

var (
	// ErrInvalidToken is returned when the token is signature-invalid or expired.
	ErrInvalidToken = errors.New("jwt: invalid token")
	// ErrWrongType is returned when the token category mismatch occurs.
	ErrWrongType = errors.New("jwt: token type mismatch")
)

// JWTManager handles signing and validating JSON Web Tokens.
type JWTManager struct {
	secret        []byte
	accessTTL     time.Duration
	tempTTL       time.Duration
	mfaPendingTTL time.Duration
}

// NewJWTManager constructs a new instance of JWTManager.
func NewJWTManager(cfg conf.JWTConfig) *JWTManager {
	return &JWTManager{
		secret:        []byte(cfg.Secret),
		accessTTL:     cfg.AccessTokenTTL,
		tempTTL:       cfg.TempTokenTTL,
		mfaPendingTTL: cfg.MFAPendingTokenTTL,
	}
}

// GenerateAccessToken signs a session authorization token.
func (m *JWTManager) GenerateAccessToken(userID uuid.UUID, role, email string) (string, error) {
	return m.sign(Claims{UserID: userID, Role: role, Email: email, Type: TokenTypeAccess}, m.accessTTL)
}

// ValidateAccessToken parses and verifies an access token.
func (m *JWTManager) ValidateAccessToken(tokenString string) (*Claims, error) {
	return m.parse(tokenString, TokenTypeAccess)
}

// GenerateTempToken signs an onboarding registration token.
func (m *JWTManager) GenerateTempToken(userID uuid.UUID) (string, error) {
	return m.sign(Claims{UserID: userID, Type: TokenTypeFirstLogin}, m.tempTTL)
}

// ValidateTempToken parses and verifies a temp token.
func (m *JWTManager) ValidateTempToken(tokenString string) (*Claims, error) {
	return m.parse(tokenString, TokenTypeFirstLogin)
}

// GenerateMFAPendingToken signs a multi-factor authentication validation token.
func (m *JWTManager) GenerateMFAPendingToken(userID uuid.UUID) (string, error) {
	return m.sign(Claims{UserID: userID, Type: TokenTypeMFAPending}, m.mfaPendingTTL)
}

// ValidateMFAPendingToken parses and verifies an mfa pending token.
func (m *JWTManager) ValidateMFAPendingToken(tokenString string) (*Claims, error) {
	return m.parse(tokenString, TokenTypeMFAPending)
}

// sign generates and signs a new JWT with specified claims and duration.
func (m *JWTManager) sign(claims Claims, ttl time.Duration) (string, error) {
	now := time.Now()
	claims.RegisteredClaims = jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// parse decodes the JWT and validates the signature and expected token type.
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

// GenerateRefreshToken produces a cryptographically secure random token.
func GenerateRefreshToken() (string, error) {
	buf := make([]byte, 64)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("helpers: generate refresh token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// HashRefreshToken generates a SHA-256 hex signature from the raw token.
func HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// ValidateRefreshToken validates the raw token against the stored hex digest in constant time.
func ValidateRefreshToken(raw, storedHash string) bool {
	computed := HashRefreshToken(raw)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(storedHash)) == 1
}