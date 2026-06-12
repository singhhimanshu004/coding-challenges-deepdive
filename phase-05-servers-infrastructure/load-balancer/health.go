package main

import (
	"context"
	"log"
	"net/http"
	"time"
)

// HealthChecker is the ACTIVE health-check loop. On a fixed interval it probes
// every backend in the pool and updates each backend's alive flag.
//
// Active vs passive health checking (the core concept):
//   - ACTIVE  — the LB *initiates* probes on a schedule (this type). It detects
//     a sick backend even when no client traffic is flowing, and it can detect
//     RECOVERY (bring a backend back) without risking real requests.
//   - PASSIVE — the LB infers health from *real* traffic outcomes (see the
//     ReverseProxy ErrorHandler in backend.go: a failed forward marks the
//     backend down instantly). Passive reacts faster but can't tell when a dead
//     backend has come back, because it has stopped sending it traffic.
//
// Real systems run BOTH: passive to fail out instantly, active to fail back in.
type HealthChecker struct {
	pool     *Pool
	interval time.Duration
	path     string        // probe path, e.g. "/health"
	timeout  time.Duration // per-probe timeout
	client   *http.Client
	logger   *log.Logger
}

// NewHealthChecker builds a checker. A short per-probe timeout matters: a slow
// backend must not stall the whole sweep.
func NewHealthChecker(pool *Pool, interval, timeout time.Duration, path string, logger *log.Logger) *HealthChecker {
	if path == "" {
		path = "/health"
	}
	return &HealthChecker{
		pool:     pool,
		interval: interval,
		path:     path,
		timeout:  timeout,
		client:   &http.Client{Timeout: timeout},
		logger:   logger,
	}
}

// Start launches the background probe loop in its own goroutine and returns
// immediately. The loop runs until ctx is cancelled.
//
// 🐍 This is the idiomatic Go background-task shape: a goroutine driven by a
// time.Ticker, selecting between "tick" and "context cancelled". The Ticker is
// like Python's threading.Timer fired repeatedly, but Go's select makes "do
// work OR stop cleanly" a single readable construct. Always Stop() the ticker
// and return on ctx.Done() so the goroutine doesn't leak.
func (h *HealthChecker) Start(ctx context.Context) {
	go func() {
		// Probe once immediately so we don't serve a full interval blind to a
		// backend that is already down at startup.
		h.CheckAll(ctx)

		ticker := time.NewTicker(h.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.CheckAll(ctx)
			}
		}
	}()
}

// CheckAll probes every backend once and updates its alive flag. It is exported
// (well, package-visible and called directly by tests) precisely so health
// transitions can be driven DETERMINISTICALLY in tests without sleeping for a
// ticker to fire.
func (h *HealthChecker) CheckAll(ctx context.Context) {
	for _, b := range h.pool.Backends() {
		was := b.IsAlive()
		now := h.probe(ctx, b)
		b.SetAlive(now)
		if was != now && h.logger != nil {
			state := "DOWN"
			if now {
				state = "UP"
			}
			h.logger.Printf("backend %s is now %s", b.URL, state)
		}
	}
}

// probe performs a single health check against one backend: GET <url><path> and
// treat any 2xx as healthy. Any error (connection refused, timeout, 5xx) means
// unhealthy.
func (h *HealthChecker) probe(ctx context.Context, b *Backend) bool {
	target := b.URL.String() + h.path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return false
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}
