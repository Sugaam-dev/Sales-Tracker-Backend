package helpers

import (
	"testing"
)

func TestSanitizeString(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		clientSecret string
		authCode     string
		expected     string
	}{
		{
			name:         "redact client secret",
			input:        "client_secret=my-secret-123",
			clientSecret: "my-secret-123",
			expected:     "client_secret=[REDACTED]",
		},
		{
			name:     "redact auth code",
			input:    "code=auth-code-xyz",
			authCode: "auth-code-xyz",
			expected: "code=[REDACTED]",
		},
		{
			name:     "redact JWT",
			input:    "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
			expected: "Bearer [REDACTED_JWT]",
		},
		{
			name:     "redact JSON fields",
			input:    `{"access_token": "secret_access", "refresh_token": "secret_refresh", "id_token": "secret_id", "client_secret": "sec", "code": "cod", "cookie": "cook"}`,
			expected: `{"access_token": "[REDACTED]", "refresh_token": "[REDACTED]", "id_token": "[REDACTED]", "client_secret": "[REDACTED]", "code": "[REDACTED]", "cookie": "[REDACTED]"}`,
		},
		{
			name:     "redact Query parameters",
			input:    "https://example.com/callback?code=123&client_secret=456&access_token=789",
			expected: "https://example.com/callback?code=[REDACTED]&client_secret=[REDACTED]&access_token=[REDACTED]",
		},
		{
			name:     "redact Cookie header",
			input:    "Cookie: session=12345; user=abc\r\nSet-Cookie: session=456; Path=/",
			expected: "Cookie: [REDACTED]\r\nSet-Cookie: [REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := SanitizeString(tt.input, tt.clientSecret, tt.authCode)
			if actual != tt.expected {
				t.Errorf("SanitizeString() = %q, want %q", actual, tt.expected)
			}
		})
	}
}
