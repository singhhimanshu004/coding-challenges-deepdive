package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

// mustBackend builds a Backend pointing at rawURL with no markDown side effects,
// for use in scheduler unit tests where we don't exercise the proxy.
func mustBackend(t *testing.T, rawURL string, weight int) *Backend {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse %q: %v", rawURL, err)
	}
	return newBackend(u, weight, func(*Backend) {})
}

// labeledBackend spins up a real httptest server that echoes a label, plus the
// Backend wired to it. The toggle lets a test make the backend's /health start
// failing (return 503) on demand, which is how we drive health transitions
// deterministically without sleeping.
type labeledBackend struct {
	label   string
	server  *httptest.Server
	backend *Backend
	healthy *atomicBool
}

type atomicBool struct {
	mu sync.Mutex
	v  bool
}

func (a *atomicBool) set(v bool) { a.mu.Lock(); a.v = v; a.mu.Unlock() }
func (a *atomicBool) get() bool  { a.mu.Lock(); defer a.mu.Unlock(); return a.v }

func newLabeledBackend(t *testing.T, label string) *labeledBackend {
	t.Helper()
	healthy := &atomicBool{v: true}
	mux := http.NewServeMux()
	// /health reflects the toggle: 200 when healthy, 503 when not.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if healthy.get() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	// Any other path: echo which backend served it, plus a custom header and a
	// 201 status so the proxying test can assert body, header, AND status flow
	// through untouched.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Served-By", label)
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, "served-by=%s path=%s", label, r.URL.Path)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	u, _ := url.Parse(srv.URL)
	b := newBackend(u, 1, func(*Backend) {})
	return &labeledBackend{label: label, server: srv, backend: b, healthy: healthy}
}

func discardLogger() *log.Logger { return log.New(io.Discard, "", 0) }

// ---------------------------------------------------------------------------
// Scheduler: round-robin distributes across backends in order.
// ---------------------------------------------------------------------------

func TestRoundRobinOrder(t *testing.T) {
	backends := []*Backend{
		mustBackend(t, "http://127.0.0.1:1", 1),
		mustBackend(t, "http://127.0.0.1:2", 1),
		mustBackend(t, "http://127.0.0.1:3", 1),
	}
	rr := &RoundRobin{}

	// Six picks over three backends must be: 0,1,2,0,1,2.
	want := []*Backend{backends[0], backends[1], backends[2], backends[0], backends[1], backends[2]}
	for i, w := range want {
		got := rr.Next(backends)
		if got != w {
			t.Fatalf("pick %d: got %s, want %s", i, got.URL, w.URL)
		}
	}
}

func TestRoundRobinEmpty(t *testing.T) {
	rr := &RoundRobin{}
	if got := rr.Next(nil); got != nil {
		t.Fatalf("expected nil for empty backend set, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Scheduler: least-connections prefers the least-busy backend.
// ---------------------------------------------------------------------------

func TestLeastConnPrefersLeastBusy(t *testing.T) {
	b0 := mustBackend(t, "http://127.0.0.1:1", 1)
	b1 := mustBackend(t, "http://127.0.0.1:2", 1)
	b2 := mustBackend(t, "http://127.0.0.1:3", 1)

	// Make b1 the least busy: b0=5 in flight, b1=0, b2=3.
	for i := 0; i < 5; i++ {
		b0.acquire()
	}
	for i := 0; i < 3; i++ {
		b2.acquire()
	}

	lc := &LeastConn{}
	if got := lc.Next([]*Backend{b0, b1, b2}); got != b1 {
		t.Fatalf("least-conn chose %s, want the idle backend %s", got.URL, b1.URL)
	}

	// Now load b1 up past b2; the choice must move to b2.
	for i := 0; i < 4; i++ {
		b1.acquire()
	}
	if got := lc.Next([]*Backend{b0, b1, b2}); got != b2 {
		t.Fatalf("after loading b1, least-conn chose %s, want %s", got.URL, b2.URL)
	}
}

// ---------------------------------------------------------------------------
// Scheduler: weighted round-robin honours weights.
// ---------------------------------------------------------------------------

func TestWeightedRoundRobinDistribution(t *testing.T) {
	heavy := mustBackend(t, "http://127.0.0.1:1", 3)
	light := mustBackend(t, "http://127.0.0.1:2", 1)
	wrr := &WeightedRoundRobin{}

	counts := map[*Backend]int{}
	for i := 0; i < 4; i++ { // one full cycle of total weight (3+1)
		counts[wrr.Next([]*Backend{heavy, light})]++
	}
	if counts[heavy] != 3 || counts[light] != 1 {
		t.Fatalf("weighted split = heavy:%d light:%d, want 3:1", counts[heavy], counts[light])
	}
}

// ---------------------------------------------------------------------------
// End-to-end: requests are proxied correctly (body, header, status).
// ---------------------------------------------------------------------------

func TestProxyingPreservesResponse(t *testing.T) {
	b := newLabeledBackend(t, "A")
	pool := NewPool([]*Backend{b.backend})
	lb := NewLoadBalancer(pool, &RoundRobin{}, discardLogger())

	front := httptest.NewServer(lb)
	t.Cleanup(front.Close)

	resp, err := http.Get(front.URL + "/widgets")
	if err != nil {
		t.Fatalf("request through LB: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201 (proxy must pass the backend's status)", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Served-By"); got != "A" {
		t.Errorf("X-Served-By = %q, want \"A\" (proxy must pass backend headers)", got)
	}
	if want := "served-by=A path=/widgets"; string(body) != want {
		t.Errorf("body = %q, want %q (proxy must pass backend body)", body, want)
	}
}

// ---------------------------------------------------------------------------
// End-to-end: round-robin spreads real requests across all backends in order.
// ---------------------------------------------------------------------------

func TestRoundRobinSpreadsAcrossBackends(t *testing.T) {
	a := newLabeledBackend(t, "A")
	b := newLabeledBackend(t, "B")
	c := newLabeledBackend(t, "C")
	pool := NewPool([]*Backend{a.backend, b.backend, c.backend})
	lb := NewLoadBalancer(pool, &RoundRobin{}, discardLogger())
	front := httptest.NewServer(lb)
	t.Cleanup(front.Close)

	got := make([]string, 0, 6)
	for i := 0; i < 6; i++ {
		resp, err := http.Get(front.URL + "/")
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		got = append(got, resp.Header.Get("X-Served-By"))
		resp.Body.Close()
	}
	want := []string{"A", "B", "C", "A", "B", "C"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("request %d served by %q, want %q (sequence=%v)", i, got[i], want[i], got)
		}
	}
}

// ---------------------------------------------------------------------------
// Health: an unhealthy backend is skipped; a recovered one is reused again.
// ---------------------------------------------------------------------------

func TestUnhealthyBackendSkippedAndRecovered(t *testing.T) {
	a := newLabeledBackend(t, "A")
	b := newLabeledBackend(t, "B")
	pool := NewPool([]*Backend{a.backend, b.backend})
	lb := NewLoadBalancer(pool, &RoundRobin{}, discardLogger())
	front := httptest.NewServer(lb)
	t.Cleanup(front.Close)

	hc := NewHealthChecker(pool, time.Hour, time.Second, "/health", discardLogger())
	ctx := context.Background()

	// Phase 1: both healthy. A probe confirms both up; traffic uses both.
	hc.CheckAll(ctx)
	if !a.backend.IsAlive() || !b.backend.IsAlive() {
		t.Fatalf("expected both backends alive after first probe")
	}

	// Phase 2: B's /health starts failing. After a probe, B is marked DOWN and
	// every request must land on A only.
	b.healthy.set(false)
	hc.CheckAll(ctx)
	if b.backend.IsAlive() {
		t.Fatalf("backend B should be DOWN after failing health probe")
	}
	for i := 0; i < 5; i++ {
		resp, err := http.Get(front.URL + "/")
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		if served := resp.Header.Get("X-Served-By"); served != "A" {
			t.Fatalf("request %d served by %q while B is down; want only A", i, served)
		}
		resp.Body.Close()
	}

	// Phase 3: B recovers. Active health checking is what brings it back —
	// after a probe, B is reused again.
	b.healthy.set(true)
	hc.CheckAll(ctx)
	if !b.backend.IsAlive() {
		t.Fatalf("backend B should be back UP after recovering")
	}
	seen := map[string]bool{}
	for i := 0; i < 6; i++ {
		resp, err := http.Get(front.URL + "/")
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		seen[resp.Header.Get("X-Served-By")] = true
		resp.Body.Close()
	}
	if !seen["A"] || !seen["B"] {
		t.Fatalf("after recovery both A and B should serve traffic; saw %v", seen)
	}
}

// ---------------------------------------------------------------------------
// Resilience: with every backend down, the LB returns 503 rather than hanging.
// ---------------------------------------------------------------------------

func TestNoHealthyBackendReturns503(t *testing.T) {
	a := newLabeledBackend(t, "A")
	a.backend.SetAlive(false)
	pool := NewPool([]*Backend{a.backend})
	lb := NewLoadBalancer(pool, &RoundRobin{}, discardLogger())
	front := httptest.NewServer(lb)
	t.Cleanup(front.Close)

	resp, err := http.Get(front.URL + "/")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when no backend is healthy", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// Passive health: a forward to a dead backend marks it down via ErrorHandler.
// ---------------------------------------------------------------------------

func TestPassiveMarkDownOnTransportError(t *testing.T) {
	// Build a backend whose origin server is already closed, so any forward
	// fails at the transport level and triggers the proxy ErrorHandler.
	dead := newLabeledBackend(t, "DEAD")
	dead.server.Close() // origin is now unreachable

	live := newLabeledBackend(t, "LIVE")
	pool := NewPool([]*Backend{dead.backend, live.backend})
	lb := NewLoadBalancer(pool, &RoundRobin{}, discardLogger())
	front := httptest.NewServer(lb)
	t.Cleanup(front.Close)

	// First request round-robins to the dead backend → 502 + marked down.
	resp, err := http.Get(front.URL + "/")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()
	if dead.backend.IsAlive() {
		t.Fatalf("dead backend should be marked DOWN after a failed forward")
	}
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502 from the failed forward", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// CLI parsing: table-driven backend/scheduler parsing.
// ---------------------------------------------------------------------------

func TestParseBackends(t *testing.T) {
	logger := discardLogger()
	tests := []struct {
		name    string
		csv     string
		wantLen int
		wantErr bool
	}{
		{"single", "http://127.0.0.1:9001", 1, false},
		{"two", "http://a:80,http://b:80", 2, false},
		{"spaces trimmed", " http://a:80 , http://b:80 ", 2, false},
		{"empty entries skipped", "http://a:80,,http://b:80,", 2, false},
		{"with weight", "http://a:80#3,http://b:80", 2, false},
		{"missing scheme", "127.0.0.1:9001", 0, true},
		{"garbage", "::::", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseBackends(tc.csv, logger)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got none", tc.csv)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tc.wantLen {
				t.Fatalf("got %d backends, want %d", len(got), tc.wantLen)
			}
		})
	}

	// weight suffix is parsed onto the backend.
	bs, err := parseBackends("http://a:80#3", logger)
	if err != nil {
		t.Fatalf("parse weighted: %v", err)
	}
	if bs[0].Weight != 3 {
		t.Fatalf("weight = %d, want 3", bs[0].Weight)
	}
}

func TestNewScheduler(t *testing.T) {
	tests := []struct {
		algo    string
		want    string
		wantErr bool
	}{
		{"round-robin", "round-robin", false},
		{"rr", "round-robin", false},
		{"", "round-robin", false},
		{"least-conn", "least-conn", false},
		{"lc", "least-conn", false},
		{"random", "random", false},
		{"weighted", "weighted", false},
		{"bogus", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.algo, func(t *testing.T) {
			s, err := newScheduler(tc.algo)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.algo)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s.Name() != tc.want {
				t.Fatalf("name = %q, want %q", s.Name(), tc.want)
			}
		})
	}
}
