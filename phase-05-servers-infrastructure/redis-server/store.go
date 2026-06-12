package main

// store.go — the concurrency-safe, in-memory key/value store with key expiry.
//
// This is the "database" behind the server. It is deliberately independent of the
// network and protocol code so it can be unit-tested directly and reused.
//
// 🐍 For a Python dev: this is a `dict[str, str]` guarded by a lock, plus a parallel
// notion of "when does this key expire". Go has no GIL, so goroutines genuinely run
// in parallel and we MUST guard shared maps ourselves — a concurrent map write in
// Go is a hard crash, not just a race.

import (
	"sort"
	"strconv"
	"sync"
	"time"
)

// entry is a single stored value plus its optional expiry deadline.
type entry struct {
	value    string
	expireAt time.Time // zero value (IsZero) means "never expires"
}

// Store is the shared key/value space. A single sync.RWMutex protects the map.
//
// Why RWMutex and not Mutex? Read-heavy commands (GET, EXISTS, KEYS, TTL) can hold
// a *read* lock and run concurrently with each other; only writers (SET, DEL, …)
// and the lazy-delete path need the exclusive *write* lock. A production Redis-like
// server would shard the keyspace into N independently-locked maps to cut
// contention further — noted in the README.
type Store struct {
	mu   sync.RWMutex
	data map[string]entry

	// now is injected so tests can control time deterministically instead of
	// sleeping. Defaults to time.Now in NewStore.
	now func() time.Time
}

// NewStore returns an empty store using the real wall clock.
func NewStore() *Store {
	return &Store{
		data: make(map[string]entry),
		now:  time.Now,
	}
}

// isExpired reports whether e is past its deadline as of the store's clock.
func (s *Store) isExpired(e entry) bool {
	return !e.expireAt.IsZero() && !s.now().Before(e.expireAt)
}

// getLive looks up a key, applying LAZY EXPIRY: if the key exists but is past its
// deadline, we delete it on the spot and report a miss. This is one of Redis's two
// expiry strategies — "delete it the moment someone touches it".
//
// Because it may delete, it takes the write lock.
func (s *Store) getLive(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data[key]
	if !ok {
		return "", false
	}
	if s.isExpired(e) {
		delete(s.data, key)
		return "", false
	}
	return e.value, true
}

// Get returns the value for key, or ok=false if absent/expired.
func (s *Store) Get(key string) (string, bool) {
	return s.getLive(key)
}

// SetOptions carries the optional modifiers of the SET command.
type SetOptions struct {
	ttl   time.Duration // 0 => no expiry
	hasEx bool          // an EX/PX option was supplied
	nx    bool          // only set if the key does NOT already exist
	xx    bool          // only set if the key already exists
}

// Set writes key=value subject to NX/XX guards and optional TTL. It returns
// ok=false only when an NX/XX guard prevented the write.
func (s *Store) Set(key, value string, opts SetOptions) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, present := s.data[key]
	if present && s.isExpired(existing) {
		// A logically-expired key counts as absent for NX/XX purposes.
		delete(s.data, key)
		present = false
	}
	if opts.nx && present {
		return false
	}
	if opts.xx && !present {
		return false
	}

	e := entry{value: value}
	if opts.hasEx {
		e.expireAt = s.now().Add(opts.ttl)
	}
	s.data[key] = e
	return true
}

// Del removes the given keys and returns how many actually existed.
func (s *Store) Del(keys ...string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var removed int64
	for _, k := range keys {
		if e, ok := s.data[k]; ok {
			if s.isExpired(e) {
				delete(s.data, k)
				continue // expired keys don't count toward the DEL total
			}
			delete(s.data, k)
			removed++
		}
	}
	return removed
}

// Exists returns how many of the given keys currently exist (live).
func (s *Store) Exists(keys ...string) int64 {
	var count int64
	for _, k := range keys {
		if _, ok := s.getLive(k); ok {
			count++
		}
	}
	return count
}

// Expire sets a TTL (in seconds) on an existing key. Returns false if the key
// does not exist.
func (s *Store) Expire(key string, seconds int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data[key]
	if !ok || s.isExpired(e) {
		delete(s.data, key)
		return false
	}
	e.expireAt = s.now().Add(time.Duration(seconds) * time.Second)
	s.data[key] = e
	return true
}

// TTL returns the remaining life of a key in seconds. Following Redis semantics:
//
//	-2 => key does not exist
//	-1 => key exists but has no expiry
//	>=0 => seconds remaining (rounded up)
func (s *Store) TTL(key string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.data[key]
	if !ok || s.isExpired(e) {
		return -2
	}
	if e.expireAt.IsZero() {
		return -1
	}
	remaining := e.expireAt.Sub(s.now())
	// Round up so a 999ms remainder still reports 1 second, matching Redis.
	return int64((remaining + time.Second - 1) / time.Second)
}

// addBy implements the shared core of INCR/DECR/INCRBY. The stored value is parsed
// as a base-10 integer, mutated, and written back as its decimal string.
func (s *Store) addBy(key string, delta int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var current int64
	if e, ok := s.data[key]; ok && !s.isExpired(e) {
		n, err := strconv.ParseInt(e.value, 10, 64)
		if err != nil {
			return 0, errNotInteger
		}
		current = n
	}
	current += delta
	// INCR/DECR preserve any existing TTL in real Redis; we keep it simple and
	// also preserve the deadline if the key already had one.
	prev := s.data[key]
	s.data[key] = entry{value: strconv.FormatInt(current, 10), expireAt: prev.expireAt}
	return current, nil
}

// Incr / Decr are the public wrappers over addBy.
func (s *Store) Incr(key string) (int64, error) { return s.addBy(key, 1) }
func (s *Store) Decr(key string) (int64, error) { return s.addBy(key, -1) }

// Append concatenates value onto the key (creating it if absent) and returns the
// new length, mirroring Redis APPEND.
func (s *Store) Append(key, value string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.data[key]
	if ok && !s.isExpired(e) {
		e.value += value
	} else {
		e = entry{value: value} // fresh key has no TTL
	}
	s.data[key] = e
	return int64(len(e.value))
}

// GetSet atomically sets a new value and returns the previous one (or ok=false if
// there was no live previous value). Setting a value clears any prior TTL.
func (s *Store) GetSet(key, value string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.data[key]
	wasLive := ok && !s.isExpired(old)
	s.data[key] = entry{value: value}
	if wasLive {
		return old.value, true
	}
	return "", false
}

// MSet sets many key/value pairs atomically (all under one lock).
func (s *Store) MSet(pairs map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range pairs {
		s.data[k] = entry{value: v}
	}
}

// Keys returns the live keys matching a glob-ish pattern. We support the common
// "*" = match-all and otherwise exact match — enough to demonstrate KEYS without
// reimplementing Redis's full glob matcher.
//
// Note this is a pure read: it uses RLock and does NOT delete expired keys (it
// just skips them). The background sweeper reclaims them — a nice illustration of
// reads staying cheap while cleanup happens elsewhere.
func (s *Store) Keys(pattern string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.data))
	for k, e := range s.data {
		if s.isExpired(e) {
			continue
		}
		if pattern == "*" || pattern == k {
			out = append(out, k)
		}
	}
	sort.Strings(out) // stable, testable ordering (Redis returns arbitrary order)
	return out
}

// SweepExpired implements ACTIVE EXPIRY: it scans the keyspace and removes every
// key already past its deadline, returning how many it reclaimed. The server calls
// this on a timer from a background goroutine. Lazy expiry alone would leak memory
// for keys that are set-with-TTL and then never read again; the sweep bounds that.
func (s *Store) SweepExpired() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int
	for k, e := range s.data {
		if s.isExpired(e) {
			delete(s.data, k)
			n++
		}
	}
	return n
}

// snapshot returns a copy of every live entry, used by the persistence layer.
func (s *Store) snapshot() map[string]entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]entry, len(s.data))
	for k, e := range s.data {
		if s.isExpired(e) {
			continue
		}
		out[k] = e
	}
	return out
}

// load replaces the store's contents with the given entries (used at startup).
func (s *Store) load(entries map[string]entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = make(map[string]entry, len(entries))
	for k, e := range entries {
		if s.isExpired(e) {
			continue // don't resurrect already-expired keys from disk
		}
		s.data[k] = e
	}
}
