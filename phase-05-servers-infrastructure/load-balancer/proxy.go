package main

import (
	"log"
	"net/http"
)

// LoadBalancer is the http.Handler clients actually talk to. For each inbound
// request it: (1) asks the pool for the currently healthy backends, (2) asks the
// scheduler to pick one, (3) accounts the in-flight request, and (4) hands the
// request to that backend's reverse proxy, which streams the response back.
//
// 🐍 Implementing ServeHTTP(w, r) makes this type an http.Handler — the same
// "duck-typed interface" idea as the Scheduler. Go's net/http calls ServeHTTP
// for every request, each on its own goroutine, so this method MUST be safe for
// concurrent use. It is: the pool uses a lock, and the per-backend counters are
// atomic.
type LoadBalancer struct {
	pool      *Pool
	scheduler Scheduler
	logger    *log.Logger
}

// NewLoadBalancer wires a pool and a scheduling strategy together.
func NewLoadBalancer(pool *Pool, scheduler Scheduler, logger *log.Logger) *LoadBalancer {
	return &LoadBalancer{pool: pool, scheduler: scheduler, logger: logger}
}

func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	healthy := lb.pool.HealthyBackends()
	backend := lb.scheduler.Next(healthy)
	if backend == nil {
		// Every backend is down (or we have none). Fail loudly with 503 rather
		// than hanging — the client should retry or see the outage.
		http.Error(w, "503 Service Unavailable: no healthy backend", http.StatusServiceUnavailable)
		if lb.logger != nil {
			lb.logger.Printf("%s %s -> no healthy backend (503)", r.Method, r.URL.Path)
		}
		return
	}

	// Account the in-flight request for the WHOLE duration it is being served.
	// This is what makes least-connections work: the counter is high exactly
	// while the backend is busy, and defer guarantees we decrement even if the
	// proxy panics or the client disconnects mid-stream.
	backend.acquire()
	defer backend.release()

	if lb.logger != nil {
		lb.logger.Printf("%s %s -> %s (active=%d)", r.Method, r.URL.Path, backend.URL, backend.ActiveConnections())
	}

	// Hand off to the standard-library reverse proxy. It copies the request to
	// the backend, then streams the backend's status, headers, and body back to
	// the client untouched. On a transport failure its ErrorHandler (set in
	// newBackend) marks this backend down and returns 502.
	backend.Proxy.ServeHTTP(w, r)
}
