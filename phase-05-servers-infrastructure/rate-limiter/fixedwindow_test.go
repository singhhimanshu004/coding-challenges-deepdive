package main

import (
	"testing"
	"time"
)

func TestFixedWindowBasicLimit(t *testing.T) {
	clk := newFakeClock()
	fw := NewFixedWindow(3, time.Second, clk)

	for i := 0; i < 3; i++ {
		if d := fw.Allow("ip"); !d.Allowed {
			t.Fatalf("request %d: want allowed", i+1)
		}
	}
	if d := fw.Allow("ip"); d.Allowed {
		t.Fatalf("4th request in window: want rejected")
	}
}

func TestFixedWindowRollover(t *testing.T) {
	clk := newFakeClock()
	fw := NewFixedWindow(2, time.Second, clk)

	fw.Allow("ip")
	fw.Allow("ip")
	if d := fw.Allow("ip"); d.Allowed {
		t.Fatalf("3rd request in window: want rejected")
	}
	// Cross into the next window: counter resets.
	clk.Advance(time.Second)
	if d := fw.Allow("ip"); !d.Allowed {
		t.Fatalf("request in next window: want allowed (counter reset)")
	}
}

// TestFixedWindowBoundaryBurst demonstrates the fixed-window weakness: up to 2x
// the limit can pass within a single window-length span that straddles the
// boundary between two windows.
func TestFixedWindowBoundaryBurst(t *testing.T) {
	clk := newFakeClock()
	limit := 5
	fw := NewFixedWindow(limit, time.Second, clk)

	// Fire all `limit` requests at the very END of window 1 (t≈0.9s).
	clk.Advance(900 * time.Millisecond)
	for i := 0; i < limit; i++ {
		if d := fw.Allow("ip"); !d.Allowed {
			t.Fatalf("end-of-window request %d: want allowed", i+1)
		}
	}

	// Cross the boundary into window 2 (t≈1.0s) and fire `limit` more.
	clk.Advance(100 * time.Millisecond)
	allowed := 0
	for i := 0; i < limit; i++ {
		if fw.Allow("ip").Allowed {
			allowed++
		}
	}
	if allowed != limit {
		t.Fatalf("start-of-next-window allowed = %d, want %d", allowed, limit)
	}
	// Net effect: 2*limit requests permitted within ~0.2s — the boundary burst.
}

func TestFixedWindowTableDriven(t *testing.T) {
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
		{"still in window", 2, time.Second, 2, 500 * time.Millisecond, false},
		{"new window resets", 2, time.Second, 2, time.Second, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clk := newFakeClock()
			fw := NewFixedWindow(tc.limit, tc.window, clk)
			for i := 0; i < tc.preHits; i++ {
				fw.Allow("ip")
			}
			clk.Advance(tc.advance)
			if got := fw.Allow("ip").Allowed; got != tc.wantAllowed {
				t.Fatalf("Allowed = %v, want %v", got, tc.wantAllowed)
			}
		})
	}
}
