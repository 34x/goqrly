package main

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// RateLimiter implements a per-key rate limiter using a sliding window
type RateLimiter struct {
	mu       sync.RWMutex
	attempts map[string][]time.Time
	maxReqs  int
	window   time.Duration
	cleanup  *time.Ticker
	done     chan struct{}
}

// NewRateLimiter creates a new rate limiter
// maxReqs: maximum number of requests allowed in the window
// window: time window for rate limiting
func NewRateLimiter(maxReqs int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		attempts: make(map[string][]time.Time),
		maxReqs:  maxReqs,
		window:   window,
		cleanup:  time.NewTicker(window),
		done:     make(chan struct{}),
	}

	// Start cleanup goroutine
	go rl.cleanupLoop()

	return rl
}

// cleanupLoop periodically cleans up old entries
func (rl *RateLimiter) cleanupLoop() {
	for {
		select {
		case <-rl.cleanup.C:
			rl.cleanupOld()
		case <-rl.done:
			rl.cleanup.Stop()
			return
		}
	}
}

// cleanupOld removes expired entries
func (rl *RateLimiter) cleanupOld() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-rl.window)
	for key, times := range rl.attempts {
		// Filter out old timestamps
		valid := make([]time.Time, 0)
		for _, t := range times {
			if t.After(cutoff) {
				valid = append(valid, t)
			}
		}
		if len(valid) == 0 {
			delete(rl.attempts, key)
		} else {
			rl.attempts[key] = valid
		}
	}
}

// Allow checks if a request from the given key should be allowed
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Get existing attempts and filter out old ones
	attempts := rl.attempts[key]
	valid := make([]time.Time, 0, len(attempts))
	for _, t := range attempts {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	// Check if under limit
	if len(valid) >= rl.maxReqs {
		rl.attempts[key] = valid
		return false
	}

	// Record this attempt
	valid = append(valid, now)
	rl.attempts[key] = valid
	return true
}

// Remaining returns how many requests are remaining for the key
func (rl *RateLimiter) Remaining(key string) int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	cutoff := time.Now().Add(-rl.window)
	attempts := rl.attempts[key]
	count := 0
	for _, t := range attempts {
		if t.After(cutoff) {
			count++
		}
	}

	remaining := rl.maxReqs - count
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Reset clears all attempts for a key (e.g., after successful auth)
func (rl *RateLimiter) Reset(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, key)
}

// Stop stops the cleanup goroutine
func (rl *RateLimiter) Stop() {
	close(rl.done)
}

// HTTPLimiter is middleware that applies rate limiting to HTTP requests
type HTTPLimiter struct {
	rl  *RateLimiter
	key func(*http.Request) string // function to extract rate limit key from request
}

// NewHTTPLimiter creates an HTTP middleware rate limiter
// keyFunc extracts the key from the request (e.g., IP address)
func NewHTTPLimiter(maxReqs int, window time.Duration, keyFunc func(*http.Request) string) *HTTPLimiter {
	return &HTTPLimiter{
		rl:  NewRateLimiter(maxReqs, window),
		key: keyFunc,
	}
}

// Middleware returns an HTTP handler that rate limits requests
func (hl *HTTPLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := hl.key(r)

		if !hl.rl.Allow(key) {
			http.Error(w, "Rate limit exceeded. Please try again later.", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// LimitByIP returns a key function that uses the client's IP address
func LimitByIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr (strip port if present)
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// LimitByKey returns a key function that uses a specific request value
func LimitByKey(key string) func(*http.Request) string {
	return func(r *http.Request) string {
		return r.FormValue(key)
	}
}
