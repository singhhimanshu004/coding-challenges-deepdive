package main

import "time"

// Clock is our injectable time source. Every algorithm reads "now" through this
// interface instead of calling time.Now() directly. In production we pass a
// realClock; in tests we pass a fake clock we can move forward by hand, so the
// algorithms become deterministic (no real sleeps, no flaky timing).
//
// 🐍 Python analogy: this is the same trick as passing a `now=datetime.now`
// callable (or monkeypatching `time.time`) into a function so tests control the
// clock. Go just expresses it as a one-method interface, checked at compile time.
type Clock interface {
	Now() time.Time
}

// realClock is the production implementation: it simply reports the wall clock.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }
