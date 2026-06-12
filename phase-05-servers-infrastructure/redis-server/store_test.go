package main

import (
	"testing"
	"time"
)

// newTestStore returns a store with a mock clock the test controls, so expiry is
// instant and deterministic — no time.Sleep, no flakiness.
func newTestStore() (*Store, *time.Time) {
	now := time.Unix(1_700_000_000, 0)
	s := NewStore()
	s.now = func() time.Time { return now }
	return s, &now
}

func TestStoreSetGetDel(t *testing.T) {
	s, _ := newTestStore()
	if ok := s.Set("k", "v", SetOptions{}); !ok {
		t.Fatal("Set returned false")
	}
	if got, ok := s.Get("k"); !ok || got != "v" {
		t.Fatalf("Get = %q,%v; want v,true", got, ok)
	}
	if n := s.Del("k", "missing"); n != 1 {
		t.Fatalf("Del = %d; want 1", n)
	}
	if _, ok := s.Get("k"); ok {
		t.Fatal("key still present after Del")
	}
}

func TestStoreNXXX(t *testing.T) {
	s, _ := newTestStore()
	if ok := s.Set("k", "1", SetOptions{xx: true}); ok {
		t.Fatal("XX should fail on missing key")
	}
	if ok := s.Set("k", "1", SetOptions{nx: true}); !ok {
		t.Fatal("NX should succeed on missing key")
	}
	if ok := s.Set("k", "2", SetOptions{nx: true}); ok {
		t.Fatal("NX should fail on existing key")
	}
	if ok := s.Set("k", "2", SetOptions{xx: true}); !ok {
		t.Fatal("XX should succeed on existing key")
	}
	if got, _ := s.Get("k"); got != "2" {
		t.Fatalf("value = %q; want 2", got)
	}
}

func TestStoreLazyExpiryAndTTL(t *testing.T) {
	s, now := newTestStore()
	s.Set("k", "v", SetOptions{hasEx: true, ttl: 10 * time.Second})

	if ttl := s.TTL("k"); ttl != 10 {
		t.Fatalf("TTL = %d; want 10", ttl)
	}
	// Advance the clock past the deadline.
	*now = now.Add(11 * time.Second)

	if ttl := s.TTL("k"); ttl != -2 {
		t.Fatalf("TTL after expiry = %d; want -2", ttl)
	}
	if _, ok := s.Get("k"); ok {
		t.Fatal("expired key should be a miss (lazy expiry)")
	}
}

func TestStoreTTLNoExpiry(t *testing.T) {
	s, _ := newTestStore()
	s.Set("k", "v", SetOptions{})
	if ttl := s.TTL("k"); ttl != -1 {
		t.Fatalf("TTL = %d; want -1 (no expiry)", ttl)
	}
	if ttl := s.TTL("missing"); ttl != -2 {
		t.Fatalf("TTL of missing = %d; want -2", ttl)
	}
}

func TestStoreExpireCommand(t *testing.T) {
	s, now := newTestStore()
	s.Set("k", "v", SetOptions{})
	if ok := s.Expire("missing", 5); ok {
		t.Fatal("Expire on missing key should return false")
	}
	if ok := s.Expire("k", 5); !ok {
		t.Fatal("Expire on existing key should return true")
	}
	if ttl := s.TTL("k"); ttl != 5 {
		t.Fatalf("TTL = %d; want 5", ttl)
	}
	*now = now.Add(6 * time.Second)
	if _, ok := s.Get("k"); ok {
		t.Fatal("key should have expired")
	}
}

func TestStoreActiveSweep(t *testing.T) {
	s, now := newTestStore()
	s.Set("a", "1", SetOptions{hasEx: true, ttl: 1 * time.Second})
	s.Set("b", "2", SetOptions{}) // no expiry
	*now = now.Add(2 * time.Second)

	if n := s.SweepExpired(); n != 1 {
		t.Fatalf("SweepExpired reclaimed %d; want 1", n)
	}
	// "b" must survive; "a" must be gone.
	if _, ok := s.Get("b"); !ok {
		t.Fatal("non-expiring key was swept")
	}
	if keys := s.Keys("*"); len(keys) != 1 || keys[0] != "b" {
		t.Fatalf("Keys = %v; want [b]", keys)
	}
}

func TestStoreIncrDecr(t *testing.T) {
	s, _ := newTestStore()
	n, err := s.Incr("counter")
	if err != nil || n != 1 {
		t.Fatalf("Incr = %d,%v; want 1,nil", n, err)
	}
	n, _ = s.Incr("counter")
	if n != 2 {
		t.Fatalf("Incr = %d; want 2", n)
	}
	n, _ = s.Decr("counter")
	if n != 1 {
		t.Fatalf("Decr = %d; want 1", n)
	}
	s.Set("text", "abc", SetOptions{})
	if _, err := s.Incr("text"); err == nil {
		t.Fatal("Incr on non-integer should error")
	}
}

func TestStoreAppendGetSetMGet(t *testing.T) {
	s, _ := newTestStore()
	if n := s.Append("k", "foo"); n != 3 {
		t.Fatalf("Append = %d; want 3", n)
	}
	if n := s.Append("k", "bar"); n != 6 {
		t.Fatalf("Append = %d; want 6", n)
	}
	old, had := s.GetSet("k", "new")
	if !had || old != "foobar" {
		t.Fatalf("GetSet = %q,%v; want foobar,true", old, had)
	}
	s.MSet(map[string]string{"x": "1", "y": "2"})
	if v, _ := s.Get("x"); v != "1" {
		t.Fatalf("MSet/Get x = %q; want 1", v)
	}
}
