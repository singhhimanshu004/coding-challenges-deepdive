package main

import (
	"math"
	"sync"
	"time"
)

// LeakyBucket implements a leaky-bucket (as a meter) rate limiter.
//
// Mental model: imagine a bucket with a hole. Each request pours one unit of
// water in; water leaks out steadily at `leakRate` units per second. If adding a
// request would overflow the bucket's `capacity`, the request is rejected.
//
// Contrast with token bucket: token bucket lets you SPEND a saved-up burst all
// at once (bursty output, bounded average). Leaky bucket instead enforces a
// SMOOTH output rate — it drains at a constant rate regardless of how spiky the
// input is, so it is the right choice when a downstream system needs an even,
// shaped flow rather than occasional bursts. The capacity only sets how much
// backlog you tolerate before shedding load.
type LeakyBucket struct {
	leakRate float64 // units drained per second (the smoothed output rate)
	capacity float64 // bucket size (how much burst/backlog is tolerated)
	clock    Clock

	mu      sync.Mutex
	buckets map[string]*leakState
}

type leakState struct {
	level float64   // current water level
	last  time.Time // last time we drained this bucket
}

// NewLeakyBucket builds a leaky-bucket limiter that drains at leakRate units/sec
// and holds at most capacity units.
func NewLeakyBucket(leakRate, capacity float64, clock Clock) *LeakyBucket {
	return &LeakyBucket{
		leakRate: leakRate,
		capacity: capacity,
		clock:    clock,
		buckets:  make(map[string]*leakState),
	}
}

// Allow pours one unit into key's bucket unless that would overflow it.
func (lb *LeakyBucket) Allow(key string) Decision {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	now := lb.clock.Now()

	b, ok := lb.buckets[key]
	if !ok {
		b = &leakState{level: 0, last: now}
		lb.buckets[key] = b
	}

	// Lazily leak: subtract whatever drained since we last looked.
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.level = math.Max(0, b.level-elapsed*lb.leakRate)
		b.last = now
	}

	d := Decision{Limit: int(lb.capacity)}

	if b.level+1 <= lb.capacity {
		b.level++
		d.Allowed = true
		d.Remaining = int(lb.capacity - b.level)
		if lb.leakRate > 0 {
			d.ResetAfter = time.Duration(b.level / lb.leakRate * float64(time.Second))
		}
		return d
	}

	// Bucket is full; the client must wait for enough to drain to fit one unit.
	d.Allowed = false
	d.Remaining = 0
	if lb.leakRate > 0 {
		need := b.level + 1 - lb.capacity
		d.RetryAfter = time.Duration(need / lb.leakRate * float64(time.Second))
		d.ResetAfter = time.Duration(b.level / lb.leakRate * float64(time.Second))
	}
	return d
}
