package middleware

import (
	"sync"
	"time"
)

type loginAttempt struct {
	attempts  int
	blockedAt time.Time
}

// RateLimiter tracks failed login attempts per key in memory
type RateLimiter struct {
	mu          sync.Mutex
	attempts    map[string]*loginAttempt
	maxAttempts int
	window      time.Duration
}

// NewRateLimiter creates a new RateLimiter instance
func NewRateLimiter(maxAttempts int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		attempts:    make(map[string]*loginAttempt),
		maxAttempts: maxAttempts,
		window:      window,
	}
}

// IsBlocked checks if the key is currently blocked and returns retryAfter duration
func (l *RateLimiter) IsBlocked(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	attempt, exists := l.attempts[key]
	if !exists {
		return false, 0
	}

	if attempt.attempts >= l.maxAttempts {
		elapsed := time.Since(attempt.blockedAt)
		if elapsed < l.window {
			return true, l.window - elapsed
		}
		// Reset block if window has passed
		attempt.attempts = 0
	}

	return false, 0
}

// RecordFailure increments failed attempt count and sets block time if limit exceeded
func (l *RateLimiter) RecordFailure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	attempt, exists := l.attempts[key]
	if !exists {
		attempt = &loginAttempt{}
		l.attempts[key] = attempt
	}

	attempt.attempts++
	if attempt.attempts >= l.maxAttempts {
		attempt.blockedAt = time.Now()
	}
}

// Reset clears any record of attempts for the key
func (l *RateLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.attempts, key)
}
