// Package middleware holds Gin middleware and request-pipeline
// utilities. RateLimiter lives here per project convention, but note
// it's invoked directly by services.AuthService rather than mounted as
// router.Use(...) — the rate-limit key is identifier+IP, and identifier
// only exists after the request body is parsed, so it's a service-level
// check rather than a true pre-routing middleware.
package middleware

import (
	"sync"
	"time"
)

// attemptRecord tracks failed attempts for one key within one window.
// Guarded by its own mutex so concurrent requests for different keys
// never contend with each other — only same-key requests do.
type attemptRecord struct {
	mu          sync.Mutex
	count       int
	windowStart time.Time
}

// RateLimiter implements the documented login brute-force guard:
// 5 failed attempts per identifier+IP within a 15-minute window, no
// Redis, bounded memory via a periodic sweep of expired entries.
type RateLimiter struct {
	attempts    sync.Map // string -> *attemptRecord
	maxAttempts int
	window      time.Duration
}

func NewRateLimiter(maxAttempts int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{maxAttempts: maxAttempts, window: window}
	go rl.sweepLoop()
	return rl
}

// IsBlocked reports whether key has hit the attempt ceiling within the
// current window. Called BEFORE the DB lookup in Login, per the doc's
// "Step 1" of the 429 logic.
func (rl *RateLimiter) IsBlocked(key string) (blocked bool, retryAfter time.Duration) {
	v, ok := rl.attempts.Load(key)
	if !ok {
		return false, 0
	}
	rec := v.(*attemptRecord)
	rec.mu.Lock()
	defer rec.mu.Unlock()

	elapsed := time.Since(rec.windowStart)
	if elapsed > rl.window {
		return false, 0 // window has expired; RecordFailure will start a fresh one
	}
	if rec.count >= rl.maxAttempts {
		return true, rl.window - elapsed
	}
	return false, 0
}

// RecordFailure increments the failure count for key, starting a new
// window if the previous one has expired. Called on wrong password AND
// on "user not found" — both surface as the same generic 401, so both
// must count the same way, or an attacker could distinguish valid from
// invalid identifiers by watching which ones trigger rate limiting.
func (rl *RateLimiter) RecordFailure(key string) {
	v, _ := rl.attempts.LoadOrStore(key, &attemptRecord{windowStart: time.Now()})
	rec := v.(*attemptRecord)
	rec.mu.Lock()
	defer rec.mu.Unlock()

	if time.Since(rec.windowStart) > rl.window {
		rec.count = 0
		rec.windowStart = time.Now()
	}
	rec.count++
}

// Reset clears the failure count for key. Called as soon as the
// password check succeeds, per the doc: "If login succeeds: reset the
// counter to 0" — a correct password is a success even if the response
// then branches into first-login onboarding or MFA.
func (rl *RateLimiter) Reset(key string) {
	rl.attempts.Delete(key)
}

// sweepLoop evicts expired entries so memory stays bounded under
// sustained traffic, instead of growing forever with one entry per
// distinct identifier+IP ever seen.
func (rl *RateLimiter) sweepLoop() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()
	for range ticker.C {
		rl.attempts.Range(func(key, v any) bool {
			rec := v.(*attemptRecord)
			rec.mu.Lock()
			expired := time.Since(rec.windowStart) > rl.window
			rec.mu.Unlock()
			if expired {
				rl.attempts.Delete(key)
			}
			return true
		})
	}
}
