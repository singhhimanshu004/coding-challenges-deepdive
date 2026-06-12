package main

import (
	"testing"
	"time"
)

func TestSlidingWindowCountsWithinWindow(t *testing.T) {
	clk := newFakeClock()
	// 3 requests per rolling second.
	sw := NewSlidingWindowLog(3, time.Second, clk)

	for i := 0; i < 3; i++ {
		if d := sw.Allow("ip"); !d.Allowed {
			t.Fatalf("request %d within window: want allowed", i+1)
		}
	}
	// 4th within the same window must be rejected.
	if d := sw.Allow("ip"); d.Allowed {
		t.Fatalf("4th request within window: want rejected")
	}
}

func TestSlidingWindowSlidesForward(t *testing.T) {
	clk := newFakeClock()
	sw := NewSlidingWindowLog(2, time.Second, clk)

	sw.Allow("ip") // t=0
	clk.Advance(400 * time.Millisecond)
	sw.Allow("ip") // t=0.4, now at limit (2 in window)

	if d := sw.Allow("ip"); d.Allowed {
		t.Fatalf("3rd request at t=0.4: want rejected (2 already in window)")
	}

	// Advance so the FIRST hit (t=0) slides out, but the second (t=0.4) remains.
	clk.Advance(700 * time.Millisecond) // t=1.1: t=0 expired, t=0.4 still alive
	if d := sw.Allow("ip"); !d.Allowed {
		t.Fatalf("request at t=1.1: want allowed (oldest hit slid out)")
	}
}

func TestSlidingWindowRetryAfter(t *testing.T) {
	clk := newFakeClock()
	sw := NewSlidingWindowLog(1, time.Second, clk)

	sw.Allow("ip") // t=0
	clk.Advance(300 * time.Millisecond)
	d := sw.Allow("ip") // t=0.3, rejected
	if d.Allowed {
		t.Fatalf("want rejected at t=0.3")
	}
	// Oldest hit (t=0) leaves the window at t=1.0, i.e. 0.7s from now.
	if want := 700 * time.Millisecond; d.RetryAfter != want {
		t.Fatalf("RetryAfter = %v, want %v", d.RetryAfter, want)
	}
}

func TestSlidingWindowTableDriven(t *testing.T) {
	tests := []struct {
		name        string
		limit       int
		window      time.Duration
		preHits     int
		advance     time.Duration
		wantAllowed bool
	}{
		{"under limit", 3, time.Second, 1, 0, true},
		{"at limit", 3, time.Second, 3, 0, false},
		{"window not yet cleared", 2, time.Second, 2, 500 * time.Millisecond, false},
		{"window fully cleared", 2, time.Second, 2, time.Second, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clk := newFakeClock()
			sw := NewSlidingWindowLog(tc.limit, tc.window, clk)
			for i := 0; i < tc.preHits; i++ {
				sw.Allow("ip")
			}
			clk.Advance(tc.advance)
			if got := sw.Allow("ip").Allowed; got != tc.wantAllowed {
				t.Fatalf("Allowed = %v, want %v", got, tc.wantAllowed)
			}
		})
	}
}
