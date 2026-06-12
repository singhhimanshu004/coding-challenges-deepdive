package main

import (
	"math"
	"sync"
	"time"
)

// TokenBucket implements the classic token-bucket algorithm.
//
// Mental model: a bucket holds up to `burst` tokens and is refilled at a steady
// `rate` tokens per second (up to the cap). Every request must take one token;
// if the bucket is empty the request is rejected. Because the bucket can be
// full, a client that has been idle may fire a *burst* of up to `burst`
// requests instantly, then is throttled to the steady refill rate. This "save
// up then spend" behaviour is what makes token bucket the most popular limiter:
// it tolerates spiky-but-bounded traffic.
type TokenBucket struct {
	rate  float64 // tokens added per second (the sustained allowed rate)
	burst float64 // bucket capacity (the maximum instantaneous burst)
	clock Clock

	mu      sync.Mutex
	buckets map[string]*tokenState // one bucket per client key
}

// tokenState is the per-key bucket. We store tokens as a float so partial
// refills accumulate correctly between requests.
type tokenState struct {
	tokens float64   // current number of available tokens
	last   time.Time // last time we refilled this bucket
}

// NewTokenBucket builds a token-bucket limiter. rate is tokens/second and burst
// is the bucket capacity (the largest instantaneous spike allowed).
func NewTokenBucket(rate, burst float64, clock Clock) *TokenBucket {
	return &TokenBucket{
		rate:    rate,
		burst:   burst,
		clock:   clock,
		buckets: make(map[string]*tokenState),
	}
}

// Allow takes one token for key if available.
func (tb *TokenBucket) Allow(key string) Decision {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := tb.clock.Now()

	b, ok := tb.buckets[key]
	if !ok {
		// A brand-new client starts with a full bucket so it can burst.
		b = &tokenState{tokens: tb.burst, last: now}
		tb.buckets[key] = b
	}

	// Lazily refill: add the tokens that would have accrued since `last`. We do
	// this on access rather than with a background goroutine/timer — it is exact
	// and costs nothing while a key is idle.
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens = math.Min(tb.burst, b.tokens+elapsed*tb.rate)
		b.last = now
	}

	d := Decision{Limit: int(tb.burst)}

	if b.tokens >= 1 {
		b.tokens--
		d.Allowed = true
		d.Remaining = int(b.tokens)
		// Time until the bucket is full again.
		if tb.rate > 0 {
			d.ResetAfter = time.Duration((tb.burst - b.tokens) / tb.rate * float64(time.Second))
		}
		return d
	}

	// Rejected: report how long until at least one token is available.
	d.Allowed = false
	d.Remaining = 0
	if tb.rate > 0 {
		need := 1 - b.tokens
		d.RetryAfter = time.Duration(need / tb.rate * float64(time.Second))
		d.ResetAfter = time.Duration((tb.burst - b.tokens) / tb.rate * float64(time.Second))
	}
	return d
}
