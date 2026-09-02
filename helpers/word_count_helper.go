package helpers

import "strings"

// CountWords counts the number of words in a string using whitespace boundaries.
// Repeated spaces, tabs, and newlines do not inflate the count.
func CountWords(s string) int {
	return len(strings.Fields(strings.TrimSpace(s)))
}
