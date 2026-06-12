package main

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"sync/atomic"
)

// Scheduler decides which healthy backend should serve the next request.
//
// 🐍 This is the classic "Strategy pattern", expressed the Go way: instead of an
// abstract base class with subclasses, we define a small *interface* (just one
// method that matters: Next) and provide several concrete types that satisfy it.
// Any value with a `Next([]*Backend) *Backend` method IS a Scheduler — there is
// no "implements" keyword, the match is structural. The load balancer holds a
// Scheduler and never cares which algorithm is plugged in.
//
// Next receives the already-filtered slice of HEALTHY backends and returns the
// chosen one, or nil if the slice is empty (no healthy backends right now).
type Scheduler interface {
	Next(backends []*Backend) *Backend
	Name() string
}

// ---------------------------------------------------------------------------
// Round-robin: hand each request to the next backend in turn, cycling forever.
// ---------------------------------------------------------------------------

// RoundRobin spreads requests evenly by walking the backend list in order:
// backend[0], backend[1], ..., wrapping back to [0]. It ignores how busy each
// backend is — every backend gets the same number of requests over time.
type RoundRobin struct {
	// counter is monotonically increasing; we take it modulo the number of
	// healthy backends to pick an index. atomic.Uint64 lets many request
	// goroutines advance it without a lock or a torn read.
	counter atomic.Uint64
}

func (s *RoundRobin) Name() string { return "round-robin" }

func (s *RoundRobin) Next(backends []*Backend) *Backend {
	n := uint64(len(backends))
	if n == 0 {
		return nil
	}
	// Add(1) returns the NEW value, so subtract 1 to get the slot this call
	// "owns". Two concurrent calls get two different indices — that is the
	// whole reason for an atomic counter rather than `s.counter++`.
	i := s.counter.Add(1) - 1
	return backends[i%n]
}

// ---------------------------------------------------------------------------
// Least-connections: send each request to whichever backend has the fewest
// in-flight requests right now. Good when request durations vary a lot.
// ---------------------------------------------------------------------------

// LeastConn picks the backend with the smallest ActiveConnections count. Unlike
// round-robin it adapts to real load: a backend stuck on a slow request stops
// receiving new ones until it catches up.
type LeastConn struct{}

func (s *LeastConn) Name() string { return "least-conn" }

func (s *LeastConn) Next(backends []*Backend) *Backend {
	var best *Backend
	var min int64
	for _, b := range backends {
		a := b.ActiveConnections()
		if best == nil || a < min {
			best, min = b, a
		}
	}
	return best
}

// ---------------------------------------------------------------------------
// Random: pick a uniformly random healthy backend. Stateless and, with enough
// requests, statistically even. A surprisingly strong baseline in practice.
// ---------------------------------------------------------------------------

// Random selects a backend uniformly at random.
type Random struct {
	// intn lets tests inject a deterministic chooser; production uses
	// math/rand/v2's concurrency-safe top-level generator.
	intn func(n int) int
}

// NewRandom builds a Random scheduler backed by the global, concurrency-safe
// math/rand/v2 source.
func NewRandom() *Random { return &Random{intn: rand.IntN} }

func (s *Random) Name() string { return "random" }

func (s *Random) Next(backends []*Backend) *Backend {
	n := len(backends)
	if n == 0 {
		return nil
	}
	return backends[s.intn(n)]
}

// ---------------------------------------------------------------------------
// Weighted round-robin: like round-robin, but a backend with weight W appears W
// times as often. Use it when backends have unequal capacity (a big box and a
// small box behind the same VIP).
// ---------------------------------------------------------------------------

// WeightedRoundRobin distributes requests in proportion to each backend's
// weight. We implement the simple "expansion" form: a backend of weight 3 takes
// 3 of every (sum-of-weights) slots, still cycling deterministically.
type WeightedRoundRobin struct {
	counter atomic.Uint64
}

func (s *WeightedRoundRobin) Name() string { return "weighted" }

func (s *WeightedRoundRobin) Next(backends []*Backend) *Backend {
	if len(backends) == 0 {
		return nil
	}
	total := 0
	for _, b := range backends {
		total += b.effectiveWeight()
	}
	// slot is a value in [0, total); walk the cumulative weights to find which
	// backend's band it falls into.
	slot := int(s.counter.Add(1)-1) % total
	for _, b := range backends {
		slot -= b.effectiveWeight()
		if slot < 0 {
			return b
		}
	}
	return backends[len(backends)-1] // unreachable; defensive
}

// newScheduler maps a CLI --algo string to a concrete Scheduler.
func newScheduler(algo string) (Scheduler, error) {
	switch strings.ToLower(strings.TrimSpace(algo)) {
	case "round-robin", "rr", "":
		return &RoundRobin{}, nil
	case "least-conn", "least-connections", "lc":
		return &LeastConn{}, nil
	case "random":
		return NewRandom(), nil
	case "weighted", "weighted-round-robin", "wrr":
		return &WeightedRoundRobin{}, nil
	default:
		return nil, fmt.Errorf("unknown algorithm %q (want round-robin, least-conn, random, or weighted)", algo)
	}
}
