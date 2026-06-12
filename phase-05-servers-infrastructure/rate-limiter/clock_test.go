package main

import (
	"sync"
	"time"
)

// fakeClock is a manually-advanced Clock for deterministic tests. Instead of
// sleeping for real time, tests call Advance() to move time forward by an exact
// amount, so every algorithm's time-based behaviour is reproducible and fast.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// newFakeClock starts the clock at a fixed, arbitrary instant.
func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Advance moves the clock forward by d.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
