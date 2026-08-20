package helpers

import (
	"regexp"
	"strings"
)

var (
	// Regex to match JWT tokens starting with eyJ...
	jwtRegex = regexp.MustCompile(`\beyJ[A-Za-z0-9-_=]+\.[A-Za-z0-9-_=]+\.?[A-Za-z0-9-_=]*\b`)

	// Regex to match JSON fields for sensitive data
	jsonFieldRegex = regexp.MustCompile(`(?i)"(client_secret|code|access_token|refresh_token|id_token|cookie|jwt)"\s*:\s*"[^"]*"`)

	// Regex to match URL-encoded or query parameter sensitive values
	queryParamRegex = regexp.MustCompile(`(?i)(client_secret|code|access_token|refresh_token|id_token|cookie|jwt)=[^&\s]*`)

	// Regex to match Cookie and Set-Cookie headers/strings
	cookieHeaderRegex = regexp.MustCompile(`(?i)(cookie|set-cookie):\s*[^\r\n]*`)
)

// SanitizeString redacts client secret, authorization code, JWTs, cookies, and tokens from the input.
func SanitizeString(input string, clientSecret string, authCode string) string {
	if input == "" {
		return input
	}

	// 1. Redact exact client secret if provided
	if clientSecret != "" {
		input = strings.ReplaceAll(input, clientSecret, "[REDACTED]")
	}

	// 2. Redact exact auth code if provided
	if authCode != "" {
		input = strings.ReplaceAll(input, authCode, "[REDACTED]")
	}

	// 3. Redact JWTs
	input = jwtRegex.ReplaceAllString(input, "[REDACTED_JWT]")

	// 4. Redact JSON field values
	input = jsonFieldRegex.ReplaceAllStringFunc(input, func(match string) string {
		parts := strings.SplitN(match, ":", 2)
		if len(parts) == 2 {
			return parts[0] + `: "[REDACTED]"`
		}
		return match
	})

	// 5. Redact URL query parameters
	input = queryParamRegex.ReplaceAllStringFunc(input, func(match string) string {
		parts := strings.SplitN(match, "=", 2)
		if len(parts) == 2 {
			return parts[0] + "=[REDACTED]"
		}
		return match
	})

	// 6. Redact Cookie / Set-Cookie headers
	input = cookieHeaderRegex.ReplaceAllStringFunc(input, func(match string) string {
		parts := strings.SplitN(match, ":", 2)
		if len(parts) == 2 {
			return parts[0] + ": [REDACTED]"
		}
		return match
	})

	return input
}
