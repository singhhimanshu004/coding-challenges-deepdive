package main

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
)

// Backend is one upstream origin server the load balancer can forward to.
//
// 🐍 For a Python/Java dev: think of this as a small class that bundles the
// target URL, a ready-to-use reverse proxy, and two pieces of *live* state that
// many goroutines touch at once — whether the backend is currently "up", and how
// many requests are in flight to it right now. Because those two fields are read
// and written concurrently, we do NOT use a plain bool/int; we use the atomic
// types from sync/atomic, which give us race-free updates without a mutex.
type Backend struct {
	// URL is the origin's base address, e.g. http://127.0.0.1:9001
	URL *url.URL

	// Proxy is the standard-library reverse proxy that actually copies the
	// request to URL and streams the response back. We build the *mechanics*
	// on httputil.ReverseProxy on purpose — the lesson of this challenge is the
	// scheduling and health-checking logic around it, not re-implementing HTTP
	// proxying byte-by-byte (we already did that in Challenge 29).
	Proxy *httputil.ReverseProxy

	// Weight is used by weighted scheduling strategies (higher = more traffic).
	// A weight of 0 or less is treated as 1.
	Weight int

	// alive is the health flag. atomic.Bool means "a bool that is safe to read
	// and write from multiple goroutines" — the health-check goroutine writes
	// it while request goroutines read it.
	alive atomic.Bool

	// active is the number of in-flight requests currently being served by this
	// backend. The least-connections scheduler reads this to find the least
	// busy backend; every request increments it on entry and decrements on exit.
	active atomic.Int64
}

// newBackend builds a Backend for target and wires up its reverse proxy.
// markDown is an optional callback (used for logging) invoked when the proxy's
// ErrorHandler fires because a forwarded request failed at the transport level
// (connection refused, reset, timeout). The backend marks ITSELF down in that
// case — that is *passive* health checking: we learn a backend is sick by
// trying to use it.
func newBackend(target *url.URL, weight int, markDown func(*Backend)) *Backend {
	b := &Backend{URL: target, Weight: weight}

	// NewSingleHostReverseProxy rewrites the inbound request's scheme+host to
	// point at target and copies it upstream, then streams the upstream
	// response back to the client. It handles hop-by-hop headers, the
	// X-Forwarded-For chain, and response streaming for us.
	proxy := httputil.NewSingleHostReverseProxy(target)

	// ErrorHandler runs when the upstream cannot be reached or errors mid-copy.
	// The default behaviour logs and returns 502; we additionally mark the
	// backend down so the scheduler stops sending it traffic immediately,
	// without waiting for the next active health probe.
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		b.SetAlive(false) // passive health check: a failed forward fails the backend out
		if markDown != nil {
			markDown(b)
		}
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("502 Bad Gateway: backend unreachable\n"))
	}

	b.Proxy = proxy
	b.alive.Store(true) // optimistic: assume healthy until a probe says otherwise
	return b
}

// SetAlive records whether the backend is currently healthy. Called by the
// health checker and, in tests, directly to drive deterministic scenarios.
func (b *Backend) SetAlive(alive bool) { b.alive.Store(alive) }

// IsAlive reports whether the backend is currently healthy.
func (b *Backend) IsAlive() bool { return b.alive.Load() }

// ActiveConnections returns the number of in-flight requests to this backend.
func (b *Backend) ActiveConnections() int64 { return b.active.Load() }

// acquire marks the start of a request to this backend (active++).
func (b *Backend) acquire() { b.active.Add(1) }

// release marks the end of a request to this backend (active--).
func (b *Backend) release() { b.active.Add(-1) }

// effectiveWeight clamps non-positive weights to 1 so every backend gets at
// least some share of traffic under weighted scheduling.
func (b *Backend) effectiveWeight() int {
	if b.Weight < 1 {
		return 1
	}
	return b.Weight
}

// Pool is the concurrency-safe set of backends behind the load balancer.
//
// 🐍 The sync.RWMutex is Go's reader/writer lock: many readers (request
// goroutines asking "who's healthy?") can hold it at once, but a writer (adding
// or removing a backend) gets exclusive access. Our backend slice is fixed at
// startup here, but the lock keeps the design correct if you later add dynamic
// add/remove of backends.
type Pool struct {
	mu       sync.RWMutex
	backends []*Backend
}

// NewPool creates a pool from a list of backends.
func NewPool(backends []*Backend) *Pool {
	return &Pool{backends: backends}
}

// Backends returns a snapshot copy of every backend (healthy or not). The
// health checker iterates this to probe each one.
func (p *Pool) Backends() []*Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]*Backend, len(p.backends))
	copy(out, p.backends)
	return out
}

// HealthyBackends returns only the backends currently marked alive, preserving
// pool order (important for deterministic round-robin). The scheduler is handed
// this filtered slice so it never has to think about health itself.
func (p *Pool) HealthyBackends() []*Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]*Backend, 0, len(p.backends))
	for _, b := range p.backends {
		if b.IsAlive() {
			out = append(out, b)
		}
	}
	return out
}
