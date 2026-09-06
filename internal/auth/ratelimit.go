package auth

import (
	"sync"
	"time"
)

// RateLimiter is a fixed-window in-memory limiter keyed by string (e.g. IP).
type RateLimiter struct {
	mu      sync.Mutex
	entries map[string][]time.Time
	max     int
	window  time.Duration
}

// NewRateLimiter allows at most max events per window per key.
func NewRateLimiter(max int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		entries: make(map[string][]time.Time),
		max:     max,
		window:  window,
	}
}

// Allow records an event and reports whether it is within the limit.
func (r *RateLimiter) Allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-r.window)
	kept := r.entries[key][:0]
	for _, t := range r.entries[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	r.entries[key] = kept
	if len(kept) >= r.max {
		return false
	}
	r.entries[key] = append(kept, now)
	return true
}

// Reset clears the recorded attempts for key (e.g. after a successful
// login, so a correct password never counts against the quota).
func (r *RateLimiter) Reset(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.entries, key)
}
