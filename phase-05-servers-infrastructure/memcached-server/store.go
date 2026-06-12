// store.go holds the in-memory key/value store that backs the server. It is the
// heart of the challenge: it owns expiry semantics, the CAS (compare-and-swap)
// token, FLAGS, concurrency safety, and LRU eviction.
//
// 🐍 For a Python dev: think of Store as a thread-safe dict whose values carry a
// little metadata (flags, an expiry timestamp, a version number), plus a
// doubly-linked list that remembers "which key was touched least recently" so we
// can throw the coldest one out when we run out of room.
package main

import (
	"container/list"
	"errors"
	"strconv"
	"sync"
	"time"
)

// thirtyDays is the memcached cutoff between "exptime is a relative number of
// seconds from now" and "exptime is an absolute Unix timestamp". This is a real
// quirk of the protocol, not something we invented — see normalizeExpiry.
const thirtyDays = 60 * 60 * 24 * 30

// errNotNumeric is returned by Incr/Decr when the stored value is not a base-10
// unsigned integer. The protocol layer turns this into a CLIENT_ERROR line.
var errNotNumeric = errors.New("cannot increment or decrement non-numeric value")

// item is a single stored entry plus its metadata.
//
// 🐍 A struct is just a class with fields and no inheritance. The `elem` field is
// a back-pointer into the LRU linked list so we can move/remove this item in
// O(1) without scanning the list.
type item struct {
	key       string
	value     []byte
	flags     uint32
	cas       uint64        // version token, bumped on every write; powers gets/cas
	expiresAt time.Time     // zero value == "never expires"
	elem      *list.Element // this item's node in the LRU list
}

// Store is the concurrency-safe in-memory store.
//
// 🐍 sync.Mutex is like threading.Lock(). We take it at the top of every public
// method and release it with `defer` so the dict and the LRU list always move in
// lock-step — they must never be observed out of sync by another goroutine.
type Store struct {
	mu       sync.Mutex
	items    map[string]*item // the actual key -> item index, O(1) lookup
	lru      *list.List       // front = most-recently-used, back = coldest
	maxItems int              // 0 == unlimited; otherwise the eviction cap
	casCount uint64           // monotonic source of CAS tokens

	// now is injectable so tests can control time without sleeping. Production
	// code leaves it as time.Now; a test can swap in a fake clock and "advance"
	// it to prove that expiry works.
	now func() time.Time
}

// NewStore builds a store. maxItems <= 0 means "no eviction cap".
func NewStore(maxItems int) *Store {
	return &Store{
		items:    make(map[string]*item),
		lru:      list.New(),
		maxItems: maxItems,
		now:      time.Now,
	}
}

// nextCAS returns a fresh, ever-increasing version token. Callers already hold mu.
func (s *Store) nextCAS() uint64 {
	s.casCount++
	return s.casCount
}

// normalizeExpiry converts the wire-format exptime into an absolute deadline.
//
// memcached's exptime rules (faithfully reproduced):
//   - 0           -> never expires (we return the zero time.Time)
//   - negative    -> already expired ("delete on the spot" semantics)
//   - 1..2592000  -> that many SECONDS from now (relative)
//   - > 2592000   -> an absolute Unix timestamp (seconds since the epoch)
func (s *Store) normalizeExpiry(exptime int64) time.Time {
	switch {
	case exptime == 0:
		return time.Time{} // zero value => never
	case exptime < 0:
		return s.now().Add(-time.Second) // in the past => immediately dead
	case exptime <= thirtyDays:
		return s.now().Add(time.Duration(exptime) * time.Second)
	default:
		return time.Unix(exptime, 0)
	}
}

// expired reports whether it has passed its deadline as of `at`.
func expired(it *item, at time.Time) bool {
	return !it.expiresAt.IsZero() && !at.Before(it.expiresAt)
}

// liveLocked fetches a non-expired item, lazily evicting it if it has expired.
// Callers must already hold s.mu. Returns (nil, false) if missing or expired.
//
// This is "lazy expiration": memcached does not run a background reaper for
// every key; it simply notices an entry is stale the next time someone touches
// it, and drops it then. LRU eviction reclaims anything that is never touched.
func (s *Store) liveLocked(key string) (*item, bool) {
	it, ok := s.items[key]
	if !ok {
		return nil, false
	}
	if expired(it, s.now()) {
		s.removeLocked(it)
		return nil, false
	}
	return it, true
}

// removeLocked deletes an item from both the map and the LRU list.
func (s *Store) removeLocked(it *item) {
	delete(s.items, it.key)
	if it.elem != nil {
		s.lru.Remove(it.elem)
		it.elem = nil
	}
}

// touchLocked marks an item as most-recently-used by moving it to the front.
func (s *Store) touchLocked(it *item) {
	if it.elem != nil {
		s.lru.MoveToFront(it.elem)
	}
}

// insertLocked adds a brand new item and enforces the LRU cap afterwards.
func (s *Store) insertLocked(it *item) {
	it.elem = s.lru.PushFront(it)
	s.items[it.key] = it
	s.evictIfNeededLocked()
}

// evictIfNeededLocked drops the coldest items until we are back under the cap.
//
// 🐍 This is the LRU policy: the back of the list is the least-recently-used
// item, so we keep removing from the back until len(items) <= maxItems. Both the
// map delete and the list remove are O(1), so an eviction is cheap.
func (s *Store) evictIfNeededLocked() {
	if s.maxItems <= 0 {
		return
	}
	for len(s.items) > s.maxItems {
		back := s.lru.Back()
		if back == nil {
			return
		}
		s.removeLocked(back.Value.(*item))
	}
}

// --- Public API (each method locks; the protocol layer calls these) ---

// StoreResult mirrors the protocol's storage replies.
type StoreResult int

const (
	Stored StoreResult = iota
	NotStored
	Exists
	NotFound
)

// Set unconditionally stores key=value, replacing any existing item.
func (s *Store) Set(key string, flags uint32, exptime int64, value []byte) StoreResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	if it, ok := s.items[key]; ok { // overwrite in place keeps the LRU node
		it.value = value
		it.flags = flags
		it.expiresAt = s.normalizeExpiry(exptime)
		it.cas = s.nextCAS()
		s.touchLocked(it)
		return Stored
	}
	s.insertLocked(&item{
		key:       key,
		value:     value,
		flags:     flags,
		expiresAt: s.normalizeExpiry(exptime),
		cas:       s.nextCAS(),
	})
	return Stored
}

// Add stores only if the key does NOT already exist (live). Else NOT_STORED.
func (s *Store) Add(key string, flags uint32, exptime int64, value []byte) StoreResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.liveLocked(key); ok {
		return NotStored
	}
	s.insertLocked(&item{
		key:       key,
		value:     value,
		flags:     flags,
		expiresAt: s.normalizeExpiry(exptime),
		cas:       s.nextCAS(),
	})
	return Stored
}

// Replace stores only if the key already exists (live). Else NOT_STORED.
func (s *Store) Replace(key string, flags uint32, exptime int64, value []byte) StoreResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	it, ok := s.liveLocked(key)
	if !ok {
		return NotStored
	}
	it.value = value
	it.flags = flags
	it.expiresAt = s.normalizeExpiry(exptime)
	it.cas = s.nextCAS()
	s.touchLocked(it)
	return Stored
}

// Append/Prepend concatenate data onto an existing value, keeping its flags and
// expiry. Memcached ignores the flags/exptime sent with these commands.
func (s *Store) concat(key string, value []byte, prepend bool) StoreResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	it, ok := s.liveLocked(key)
	if !ok {
		return NotStored
	}
	if prepend {
		it.value = append(append([]byte{}, value...), it.value...)
	} else {
		it.value = append(it.value, value...)
	}
	it.cas = s.nextCAS()
	s.touchLocked(it)
	return Stored
}

func (s *Store) Append(key string, value []byte) StoreResult  { return s.concat(key, value, false) }
func (s *Store) Prepend(key string, value []byte) StoreResult { return s.concat(key, value, true) }

// CAS (compare-and-swap) stores only if casUnique matches the item's current
// version. Returns:
//   - Stored   on a successful swap
//   - Exists   if the item changed since the client last read it (token mismatch)
//   - NotFound if the key is gone
func (s *Store) CAS(key string, flags uint32, exptime int64, casUnique uint64, value []byte) StoreResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	it, ok := s.liveLocked(key)
	if !ok {
		return NotFound
	}
	if it.cas != casUnique {
		return Exists
	}
	it.value = value
	it.flags = flags
	it.expiresAt = s.normalizeExpiry(exptime)
	it.cas = s.nextCAS()
	s.touchLocked(it)
	return Stored
}

// GetResult is one VALUE line's worth of data returned by Get/Gets.
type GetResult struct {
	Key   string
	Flags uint32
	Value []byte
	CAS   uint64
}

// Get returns the live items for the requested keys (missing keys are skipped),
// preserving request order. A successful read counts as a "use" for the LRU.
func (s *Store) Get(keys []string) []GetResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]GetResult, 0, len(keys))
	for _, k := range keys {
		it, ok := s.liveLocked(k)
		if !ok {
			continue
		}
		s.touchLocked(it)
		out = append(out, GetResult{Key: k, Flags: it.flags, Value: it.value, CAS: it.cas})
	}
	return out
}

// Delete removes a key. Returns true if it existed (and was live).
func (s *Store) Delete(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	it, ok := s.liveLocked(key)
	if !ok {
		return false
	}
	s.removeLocked(it)
	return true
}

// IncrDecr adds (or subtracts) delta from a numeric value, in place.
//
// memcached treats the stored value as a 64-bit unsigned base-10 integer:
//   - incr overflow wraps around (uint64 arithmetic).
//   - decr never goes below zero (it floors at 0).
//   - a non-numeric stored value is a client error.
//
// Returns (newValue, found, err).
func (s *Store) IncrDecr(key string, delta uint64, decr bool) (uint64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	it, ok := s.liveLocked(key)
	if !ok {
		return 0, false, nil
	}
	cur, perr := strconv.ParseUint(string(it.value), 10, 64)
	if perr != nil {
		return 0, true, errNotNumeric
	}
	var next uint64
	if decr {
		if delta > cur {
			next = 0 // floor at zero, never wrap negative
		} else {
			next = cur - delta
		}
	} else {
		next = cur + delta // unsigned overflow wraps, matching memcached
	}
	it.value = []byte(strconv.FormatUint(next, 10))
	it.cas = s.nextCAS()
	s.touchLocked(it)
	return next, true, nil
}

// FlushAll empties the entire store immediately.
func (s *Store) FlushAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = make(map[string]*item)
	s.lru.Init()
}

// Len returns the current live-ish item count (may include not-yet-reaped
// expired items; primarily used by tests and stats).
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items)
}
