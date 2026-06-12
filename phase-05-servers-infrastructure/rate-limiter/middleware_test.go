package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// okHandler is the protected "business" handler used in the middleware tests.
func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func TestMiddlewareAllowsUnderLimitThenBlocks(t *testing.T) {
	clk := newFakeClock()
	// 2 requests/sec, capacity 2 — so 2 pass, the 3rd is limited.
	limiter := NewTokenBucket(2, 2, clk)
	h := Middleware(limiter, clientIPKey)(okHandler())

	newReq := func() *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "203.0.113.7:54321"
		return r
	}

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, newReq())
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200", i+1, rec.Code)
		}
	}

	// Third request is over the limit -> 429 with a Retry-After header.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newReq())
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("over-limit status = %d, want 429", rec.Code)
	}
	if ra := rec.Header().Get("Retry-After"); ra == "" {
		t.Fatalf("over-limit response missing Retry-After header")
	}
	if rec.Header().Get("X-RateLimit-Limit") != "2" {
		t.Fatalf("X-RateLimit-Limit = %q, want 2", rec.Header().Get("X-RateLimit-Limit"))
	}
}

func TestMiddlewarePerClientIsolation(t *testing.T) {
	clk := newFakeClock()
	limiter := NewTokenBucket(1, 1, clk)
	h := Middleware(limiter, clientIPKey)(okHandler())

	req := func(ip string) int {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = ip + ":1234"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code
	}

	if code := req("198.51.100.1"); code != http.StatusOK {
		t.Fatalf("client A first request: status = %d, want 200", code)
	}
	// Different client must still be allowed (separate bucket).
	if code := req("198.51.100.2"); code != http.StatusOK {
		t.Fatalf("client B first request: status = %d, want 200", code)
	}
	// Client A is now over its own limit.
	if code := req("198.51.100.1"); code != http.StatusTooManyRequests {
		t.Fatalf("client A second request: status = %d, want 429", code)
	}
}

func TestMiddlewareRecoversAfterRefill(t *testing.T) {
	clk := newFakeClock()
	limiter := NewTokenBucket(1, 1, clk)
	h := Middleware(limiter, clientIPKey)(okHandler())

	req := func() int {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "192.0.2.5:9999"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code
	}

	if code := req(); code != http.StatusOK {
		t.Fatalf("first request: status = %d, want 200", code)
	}
	if code := req(); code != http.StatusTooManyRequests {
		t.Fatalf("second request: status = %d, want 429", code)
	}
	// Wait for a token to refill, then it should pass again.
	clk.Advance(time.Second)
	if code := req(); code != http.StatusOK {
		t.Fatalf("request after refill: status = %d, want 200", code)
	}
}
