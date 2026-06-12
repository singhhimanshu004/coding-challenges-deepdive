package main

import (
	"testing"
	"time"
)

func TestTokenBucketBurstThenRefill(t *testing.T) {
	clk := newFakeClock()
	// rate = 2 tokens/sec, burst = 5. A fresh client may fire 5 immediately.
	tb := NewTokenBucket(2, 5, clk)

	for i := 0; i < 5; i++ {
		if d := tb.Allow("ip"); !d.Allowed {
			t.Fatalf("burst request %d: want allowed, got rejected", i+1)
		}
	}

	// 6th request with an empty bucket and no time elapsed must be rejected.
	if d := tb.Allow("ip"); d.Allowed {
		t.Fatalf("request after exhausting burst: want rejected, got allowed")
	}

	// After 1s at 2 tokens/sec, exactly 2 more tokens have accrued.
	clk.Advance(time.Second)
	for i := 0; i < 2; i++ {
		if d := tb.Allow("ip"); !d.Allowed {
			t.Fatalf("refilled request %d: want allowed, got rejected", i+1)
		}
	}
	if d := tb.Allow("ip"); d.Allowed {
		t.Fatalf("third request after 1s refill: want rejected, got allowed")
	}
}

func TestTokenBucketRetryAfter(t *testing.T) {
	clk := newFakeClock()
	tb := NewTokenBucket(2, 1, clk) // 1 token capacity, 2/sec refill

	if d := tb.Allow("ip"); !d.Allowed {
		t.Fatalf("first request: want allowed")
	}
	d := tb.Allow("ip")
	if d.Allowed {
		t.Fatalf("second request: want rejected")
	}
	// Need 1 token at 2/sec => 0.5s.
	if want := 500 * time.Millisecond; d.RetryAfter != want {
		t.Fatalf("RetryAfter = %v, want %v", d.RetryAfter, want)
	}
}

func TestTokenBucketPerKeyIsolation(t *testing.T) {
	clk := newFakeClock()
	tb := NewTokenBucket(1, 1, clk)

	if d := tb.Allow("a"); !d.Allowed {
		t.Fatalf("key a first request: want allowed")
	}
	// Key b has its own full bucket and must not be affected by key a.
	if d := tb.Allow("b"); !d.Allowed {
		t.Fatalf("key b first request: want allowed (independent bucket)")
	}
	if d := tb.Allow("a"); d.Allowed {
		t.Fatalf("key a second request: want rejected")
	}
}

func TestTokenBucketTableDriven(t *testing.T) {
	tests := []struct {
		name        string
		rate, burst float64
		advance     time.Duration // time to pass before the probe request
		preDrain    int           // requests to consume first (at t0)
		wantAllowed bool
	}{
		{"fresh bucket allows", 1, 3, 0, 0, true},
		{"empty bucket rejects", 1, 3, 0, 3, false},
		{"partial refill not enough", 1, 3, 500 * time.Millisecond, 3, false},
		{"full refill of one token", 1, 3, time.Second, 3, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clk := newFakeClock()
			tb := NewTokenBucket(tc.rate, tc.burst, clk)
			for i := 0; i < tc.preDrain; i++ {
				tb.Allow("ip")
			}
			clk.Advance(tc.advance)
			if got := tb.Allow("ip").Allowed; got != tc.wantAllowed {
				t.Fatalf("Allowed = %v, want %v", got, tc.wantAllowed)
			}
		})
	}
}
