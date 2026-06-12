package main

import (
	"testing"
	"time"
)

func TestLeakyBucketCapacity(t *testing.T) {
	clk := newFakeClock()
	// Drains 1/sec, holds 3. Three quick requests fill it; the 4th overflows.
	lb := NewLeakyBucket(1, 3, clk)

	for i := 0; i < 3; i++ {
		if d := lb.Allow("ip"); !d.Allowed {
			t.Fatalf("request %d: want allowed", i+1)
		}
	}
	if d := lb.Allow("ip"); d.Allowed {
		t.Fatalf("4th request: want rejected (bucket full)")
	}
}

func TestLeakyBucketDrains(t *testing.T) {
	clk := newFakeClock()
	lb := NewLeakyBucket(1, 2, clk)

	lb.Allow("ip")
	lb.Allow("ip")
	if d := lb.Allow("ip"); d.Allowed {
		t.Fatalf("3rd request: want rejected")
	}
	// After 1s, exactly 1 unit drains, freeing one slot.
	clk.Advance(time.Second)
	if d := lb.Allow("ip"); !d.Allowed {
		t.Fatalf("request after 1s drain: want allowed")
	}
	// But not two slots.
	if d := lb.Allow("ip"); d.Allowed {
		t.Fatalf("second request after 1s drain: want rejected")
	}
}

func TestLeakyBucketRetryAfter(t *testing.T) {
	clk := newFakeClock()
	lb := NewLeakyBucket(2, 1, clk) // drains 2/sec, capacity 1

	if d := lb.Allow("ip"); !d.Allowed {
		t.Fatalf("first request: want allowed")
	}
	d := lb.Allow("ip")
	if d.Allowed {
		t.Fatalf("second request: want rejected")
	}
	// Need 1 unit to drain at 2/sec => 0.5s.
	if want := 500 * time.Millisecond; d.RetryAfter != want {
		t.Fatalf("RetryAfter = %v, want %v", d.RetryAfter, want)
	}
}
