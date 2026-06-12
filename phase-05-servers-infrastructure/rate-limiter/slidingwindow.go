package main

import (
	"sync"
	"time"
)

// SlidingWindowLog implements a sliding-window-log rate limiter.
//
// Mental model: for each key we keep the timestamp of every request made in the
// last `window`. To decide a new request, we discard timestamps older than
// `now - window`, then allow the request only if fewer than `limit` remain. The
// "window" therefore slides continuously with the current time.
//
// Trade-off vs. fixed window: the sliding log is exact — there is no boundary
// burst — because the window moves smoothly instead of snapping to fixed edges.
// The cost is memory: it stores one timestamp per request in the window, so a
// noisy key with a large limit can use a lot of RAM. (A sliding-window *counter*
// trades a little accuracy for O(1) memory; see the README.)
type SlidingWindowLog struct {
	limit  int
	window time.Duration
	clock  Clock

	mu   sync.Mutex
	hits map[string][]time.Time // per-key list of recent request timestamps
}

// NewSlidingWindowLog builds a sliding-window-log limiter allowing `limit`
// requests within any rolling `window`.
func NewSlidingWindowLog(limit int, window time.Duration, clock Clock) *SlidingWindowLog {
	return &SlidingWindowLog{
		limit:  limit,
		window: window,
		clock:  clock,
		hits:   make(map[string][]time.Time),
	}
}

// Allow records a request for key and reports whether it stays within the limit.
func (s *SlidingWindowLog) Allow(key string) Decision {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.clock.Now()
	cutoff := now.Add(-s.window)

	// Drop every timestamp that has slid out of the window. We compact in place
	// so the backing slice is reused rather than reallocated each call.
	times := s.hits[key]
	kept := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}

	d := Decision{Limit: s.limit}

	if len(kept) < s.limit {
		kept = append(kept, now)
		s.hits[key] = kept
		d.Allowed = true
		d.Remaining = s.limit - len(kept)
		// The window clears when the oldest surviving hit ages out.
		d.ResetAfter = kept[0].Add(s.window).Sub(now)
		return d
	}

	// At the limit: the next slot frees up when the OLDEST request leaves the
	// window, so that is exactly how long the client should wait.
	s.hits[key] = kept
	d.Allowed = false
	d.Remaining = 0
	d.RetryAfter = kept[0].Add(s.window).Sub(now)
	d.ResetAfter = d.RetryAfter
	return d
}
