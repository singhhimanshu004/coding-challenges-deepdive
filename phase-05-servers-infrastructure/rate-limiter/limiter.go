package main

import "time"

// Decision is what every Limiter returns for a single request. It carries enough
// information for the HTTP middleware to populate the standard rate-limit
// response headers (X-RateLimit-* and Retry-After) without the middleware
// needing to know *which* algorithm produced it.
type Decision struct {
	Allowed bool // may this request proceed?

	Limit     int // the ceiling we advertise to clients (X-RateLimit-Limit)
	Remaining int // requests still available right now (X-RateLimit-Remaining)

	// RetryAfter is how long the client should wait before its next request has
	// a chance of succeeding. Only meaningful when Allowed == false.
	RetryAfter time.Duration

	// ResetAfter is how long until the limiter is fully replenished for this key
	// (used for the X-RateLimit-Reset header). Best-effort, per algorithm.
	ResetAfter time.Duration
}

// Limiter is the single abstraction the whole program is built around. Each
// algorithm (token bucket, sliding window, ...) is just a different
// implementation of this one method. The middleware, the CLI, and the tests all
// speak to this interface and never care which concrete algorithm is behind it.
//
// 🐍 Python analogy: think of an abstract base class with one abstract method
// `allow(self, key) -> Decision`. The difference is that in Go a type satisfies
// the interface automatically just by having an `Allow(string) Decision`
// method — there is no explicit "implements Limiter" declaration.
type Limiter interface {
	// Allow records one request for the given key (e.g. a client IP) and reports
	// whether it is permitted under the limit. It must be safe for concurrent use.
	Allow(key string) Decision
}
