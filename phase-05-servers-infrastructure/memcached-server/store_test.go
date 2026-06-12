package main

import (
	"testing"
	"time"
)

// fakeClock lets tests control "now" so expiry is deterministic and instant —
// no time.Sleep, no flakiness. We just move the clock forward by assignment.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestStore(maxItems int) (*Store, *fakeClock) {
	clk := &fakeClock{t: time.Unix(1_000_000_000, 0)}
	s := NewStore(maxItems)
	s.now = clk.now
	return s, clk
}

func TestSetGet(t *testing.T) {
	s, _ := newTestStore(0)
	if got := s.Set("foo", 7, 0, []byte("bar")); got != Stored {
		t.Fatalf("Set = %v, want Stored", got)
	}
	res := s.Get([]string{"foo"})
	if len(res) != 1 {
		t.Fatalf("Get returned %d results, want 1", len(res))
	}
	if string(res[0].Value) != "bar" || res[0].Flags != 7 {
		t.Fatalf("Get = %q flags=%d, want %q flags=7", res[0].Value, res[0].Flags, "bar")
	}
}

func TestGetMissingReturnsNothing(t *testing.T) {
	s, _ := newTestStore(0)
	if res := s.Get([]string{"nope"}); len(res) != 0 {
		t.Fatalf("Get(missing) = %v, want empty", res)
	}
}

func TestAddReplaceSemantics(t *testing.T) {
	s, _ := newTestStore(0)
	if got := s.Add("k", 0, 0, []byte("1")); got != Stored {
		t.Fatalf("Add new = %v, want Stored", got)
	}
	if got := s.Add("k", 0, 0, []byte("2")); got != NotStored {
		t.Fatalf("Add existing = %v, want NotStored", got)
	}
	if got := s.Replace("k", 0, 0, []byte("3")); got != Stored {
		t.Fatalf("Replace existing = %v, want Stored", got)
	}
	if got := s.Replace("missing", 0, 0, []byte("x")); got != NotStored {
		t.Fatalf("Replace missing = %v, want NotStored", got)
	}
}

func TestAppendPrepend(t *testing.T) {
	s, _ := newTestStore(0)
	s.Set("k", 0, 0, []byte("middle"))
	s.Append("k", []byte("-end"))
	s.Prepend("k", []byte("start-"))
	got := string(s.Get([]string{"k"})[0].Value)
	if got != "start-middle-end" {
		t.Fatalf("append/prepend = %q, want %q", got, "start-middle-end")
	}
	if s.Append("missing", []byte("x")) != NotStored {
		t.Fatalf("append missing should be NotStored")
	}
}

func TestCAS(t *testing.T) {
	s, _ := newTestStore(0)
	s.Set("k", 0, 0, []byte("v1"))
	tok := s.Get([]string{"k"})[0].CAS

	if got := s.CAS("k", 0, 0, tok+999, []byte("v2")); got != Exists {
		t.Fatalf("CAS wrong token = %v, want Exists", got)
	}
	if got := s.CAS("k", 0, 0, tok, []byte("v2")); got != Stored {
		t.Fatalf("CAS right token = %v, want Stored", got)
	}
	// The old token must no longer work after a successful swap.
	if got := s.CAS("k", 0, 0, tok, []byte("v3")); got != Exists {
		t.Fatalf("CAS reused token = %v, want Exists", got)
	}
	if got := s.CAS("missing", 0, 0, 1, []byte("x")); got != NotFound {
		t.Fatalf("CAS missing = %v, want NotFound", got)
	}
}

func TestExpiryWithInjectableClock(t *testing.T) {
	s, clk := newTestStore(0)
	s.Set("k", 0, 2, []byte("v")) // expires 2 seconds from "now"

	if len(s.Get([]string{"k"})) != 1 {
		t.Fatalf("item should be live before expiry")
	}
	clk.advance(2 * time.Second) // now == expiresAt => expired
	if res := s.Get([]string{"k"}); len(res) != 0 {
		t.Fatalf("item should be gone after expiry, got %v", res)
	}
	// Lazy expiration should also have reaped it from the map.
	if s.Len() != 0 {
		t.Fatalf("expired item not reaped, Len=%d", s.Len())
	}
}

func TestNegativeExptimeIsImmediatelyDead(t *testing.T) {
	s, _ := newTestStore(0)
	s.Set("k", 0, -1, []byte("v"))
	if res := s.Get([]string{"k"}); len(res) != 0 {
		t.Fatalf("negative exptime should be dead on arrival, got %v", res)
	}
}

func TestIncrDecr(t *testing.T) {
	s, _ := newTestStore(0)
	s.Set("n", 0, 0, []byte("10"))

	v, found, err := s.IncrDecr("n", 5, false)
	if err != nil || !found || v != 15 {
		t.Fatalf("incr = (%d,%v,%v), want (15,true,nil)", v, found, err)
	}
	v, _, _ = s.IncrDecr("n", 20, true) // decr below zero floors at 0
	if v != 0 {
		t.Fatalf("decr floor = %d, want 0", v)
	}
	if _, found, _ := s.IncrDecr("missing", 1, false); found {
		t.Fatalf("incr missing should report not found")
	}
	s.Set("word", 0, 0, []byte("abc"))
	if _, _, err := s.IncrDecr("word", 1, false); err != errNotNumeric {
		t.Fatalf("incr non-numeric err = %v, want errNotNumeric", err)
	}
}

func TestDeleteAndFlush(t *testing.T) {
	s, _ := newTestStore(0)
	s.Set("a", 0, 0, []byte("1"))
	if !s.Delete("a") {
		t.Fatalf("delete existing should return true")
	}
	if s.Delete("a") {
		t.Fatalf("delete missing should return false")
	}
	s.Set("b", 0, 0, []byte("2"))
	s.Set("c", 0, 0, []byte("3"))
	s.FlushAll()
	if s.Len() != 0 {
		t.Fatalf("FlushAll should empty the store, Len=%d", s.Len())
	}
}

func TestLRUEviction(t *testing.T) {
	s, _ := newTestStore(2) // cap of 2 items
	s.Set("a", 0, 0, []byte("1"))
	s.Set("b", 0, 0, []byte("2"))

	// Touch "a" so "b" becomes the least-recently-used.
	s.Get([]string{"a"})

	// Inserting "c" overflows the cap; the coldest ("b") must be evicted.
	s.Set("c", 0, 0, []byte("3"))

	if s.Len() != 2 {
		t.Fatalf("Len after eviction = %d, want 2", s.Len())
	}
	if len(s.Get([]string{"b"})) != 0 {
		t.Fatalf("b should have been evicted as LRU")
	}
	if len(s.Get([]string{"a"})) != 1 || len(s.Get([]string{"c"})) != 1 {
		t.Fatalf("a and c should both survive")
	}
}

func TestLRUEvictsOldestWhenNoTouch(t *testing.T) {
	s, _ := newTestStore(3)
	for _, k := range []string{"k1", "k2", "k3"} {
		s.Set(k, 0, 0, []byte(k))
	}
	// No touches: k1 is the oldest. Adding k4 should evict k1.
	s.Set("k4", 0, 0, []byte("k4"))
	if len(s.Get([]string{"k1"})) != 0 {
		t.Fatalf("k1 (oldest) should be evicted")
	}
	if s.Len() != 3 {
		t.Fatalf("Len = %d, want 3", s.Len())
	}
}
