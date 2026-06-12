package main

import (
	"sync"
	"time"
)

// FixedWindow implements a fixed-window-counter rate limiter.
//
// Mental model: time is chopped into back-to-back windows of length `window`
// (e.g. every 60s). Each key has a counter that increments per request and
// resets to zero at the start of the next window. Allow while count < limit.
//
// This is the simplest and cheapest limiter (one integer + one timestamp per
// key), but it has a well-known flaw: the BOUNDARY BURST. A client can send
// `limit` requests at the very end of one window and another `limit` at the
// very start of the next — up to 2x the limit within a single `window`-length
// span straddling the boundary. The sliding-window algorithms exist precisely
// to fix this. (We demonstrate the boundary behaviour directly in the tests.)
type FixedWindow struct {
	limit  int
	window time.Duration
	clock  Clock

	mu       sync.Mutex
	counters map[string]*windowState
}

type windowState struct {
	count int
	start time.Time // start of the current window for this key
}

// NewFixedWindow builds a fixed-window limiter allowing `limit` requests per
// `window`.
func NewFixedWindow(limit int, window time.Duration, clock Clock) *FixedWindow {
	return &FixedWindow{
		limit:    limit,
		window:   window,
		clock:    clock,
		counters: make(map[string]*windowState),
	}
}

// Allow records a request for key within the current fixed window.
func (f *FixedWindow) Allow(key string) Decision {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := f.clock.Now()
	// Align windows to fixed boundaries (e.g. each whole second/minute) by
	// truncating "now" to a multiple of the window length. This is the classic
	// fixed-window-counter behaviour and is what produces the boundary-burst
	// effect: two adjacent aligned windows each independently allow `limit`.
	windowStart := now.Truncate(f.window)

	w, ok := f.counters[key]
	if !ok {
		w = &windowState{start: windowStart}
		f.counters[key] = w
	}

	// If we have moved into a new aligned window, reset the counter.
	if !w.start.Equal(windowStart) {
		w.start = windowStart
		w.count = 0
	}

	d := Decision{Limit: f.limit}
	resetAfter := w.start.Add(f.window).Sub(now)

	if w.count < f.limit {
		w.count++
		d.Allowed = true
		d.Remaining = f.limit - w.count
		d.ResetAfter = resetAfter
		return d
	}

	// Window is full; the client must wait for the next window to begin.
	d.Allowed = false
	d.Remaining = 0
	d.RetryAfter = resetAfter
	d.ResetAfter = resetAfter
	return d
}
