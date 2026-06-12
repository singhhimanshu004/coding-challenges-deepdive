package main

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"time"
)

// keyFunc derives the rate-limit key from a request. The default keys on the
// client IP, so each caller gets its own bucket.
type keyFunc func(*http.Request) string

// clientIPKey extracts the client IP from RemoteAddr (which is "host:port").
// Behind a real proxy you would instead trust a vetted X-Forwarded-For header;
// we keep it simple and honest here.
func clientIPKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Middleware wraps an http.Handler with per-client rate limiting using the given
// Limiter. This is the idiomatic Go middleware shape: a function that takes a
// handler and returns a new handler that does some work and then (maybe) calls
// the original.
//
// 🐍 Python analogy: this is exactly a decorator — `def middleware(handler):
// def wrapped(req): ...; return wrapped`. Go uses the http.Handler interface
// instead of a callable, and http.HandlerFunc adapts a plain function into one.
//
// On every request it sets the standard advisory headers:
//
//	X-RateLimit-Limit     — the ceiling for this key
//	X-RateLimit-Remaining — requests left right now
//	X-RateLimit-Reset     — seconds until the limiter replenishes
//
// When the limit is exceeded it responds 429 Too Many Requests and adds a
// Retry-After header (seconds) so well-behaved clients know when to come back.
func Middleware(l Limiter, key keyFunc) func(http.Handler) http.Handler {
	if key == nil {
		key = clientIPKey
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			d := l.Allow(key(r))

			h := w.Header()
			h.Set("X-RateLimit-Limit", strconv.Itoa(d.Limit))
			h.Set("X-RateLimit-Remaining", strconv.Itoa(d.Remaining))
			h.Set("X-RateLimit-Reset", strconv.Itoa(secondsCeil(d.ResetAfter)))

			if !d.Allowed {
				// Retry-After is defined in whole seconds; round up so we never
				// invite the client back too early.
				h.Set("Retry-After", strconv.Itoa(secondsCeil(d.RetryAfter)))
				http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// secondsCeil rounds a duration up to whole seconds (minimum 1 when positive),
// which is what HTTP's integer-seconds headers expect.
func secondsCeil(d time.Duration) int {
	if d <= 0 {
		return 0
	}
	return int(math.Ceil(d.Seconds()))
}
