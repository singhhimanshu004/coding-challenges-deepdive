package main

// TTL CACHE — the feature that turns a dumb relay into a useful forwarder.
//
// The whole point of a forwarding resolver is to answer repeat questions
// LOCALLY instead of bothering the upstream every time. DNS records carry a TTL
// (time-to-live, in seconds): the record's author telling us "you may reuse this
// answer for up to N seconds." So we store each upstream reply keyed by the
// question, remember WHEN it should expire, and serve it from memory until then.
//
// Concurrency is the catch. Our server handles every client request on its own
// goroutine (see forwarder.go), so many goroutines read and write this map at
// the SAME TIME. A plain Go map is NOT safe for concurrent use — concurrent
// writes panic. We guard it with a sync.RWMutex:
//
//	🐍➡️🐹 RWMutex is a readers-writer lock (like Python's not-built-in
//	equivalent; closest is a threading.Lock but smarter). MANY goroutines may
//	hold the READ lock at once (RLock); a WRITE lock (Lock) is exclusive. Cache
//	lookups are far more common than inserts, so this lets reads run in parallel.

import (
	"sync"
	"time"
)

// cacheKey is the (QNAME, QTYPE, QCLASS) triple from the question section.
// Using a struct as a map key works in Go as long as every field is comparable
// (strings and integers are) — no manual hashing needed.
//
// 🐍 In Python you'd reach for a tuple key (name, qtype, qclass); a Go struct
// key is the idiomatic, type-safe equivalent.
type cacheKey struct {
	name   string
	qtype  uint16
	qclass uint16
}

// cacheEntry is one stored reply: the raw upstream response bytes plus the
// instant the entry stops being valid. We keep the bytes VERBATIM and only patch
// the 2-byte transaction ID per client when we serve it (see forwarder.go).
type cacheEntry struct {
	response []byte
	expiry   time.Time
}

// cache is a concurrency-safe TTL map.
//
// `now` is an injectable clock: production uses time.Now, but tests swap in a
// controllable clock so they can simulate TTL expiry instantly instead of
// sleeping. Injecting the clock is the same trick as injecting I/O streams —
// it makes time-dependent code deterministically testable.
type cache struct {
	mu  sync.RWMutex
	m   map[cacheKey]cacheEntry
	now func() time.Time
}

// newCache builds an empty cache that reads wall-clock time.
func newCache() *cache {
	return &cache{
		m:   make(map[cacheKey]cacheEntry),
		now: time.Now,
	}
}

// get returns the cached response for k if one exists and has not expired.
//
// We take only the READ lock for the common lookup so concurrent readers don't
// block each other. If we find an EXPIRED entry we upgrade to the write lock to
// delete it ("lazy expiration": entries are reaped on access rather than by a
// background sweeper — simpler, and good enough for a learning forwarder).
func (c *cache) get(k cacheKey) ([]byte, bool) {
	c.mu.RLock()
	e, ok := c.m[k]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if !c.now().Before(e.expiry) {
		// Expired (now >= expiry). Drop it so future lookups are clean misses.
		c.mu.Lock()
		// Re-check under the write lock: another goroutine may have refreshed
		// the entry between our RUnlock and Lock.
		if cur, still := c.m[k]; still && !c.now().Before(cur.expiry) {
			delete(c.m, k)
		}
		c.mu.Unlock()
		return nil, false
	}
	return e.response, true
}

// set stores response under k, valid for ttl seconds from now. A zero TTL means
// "do not reuse," so we skip caching it entirely.
func (c *cache) set(k cacheKey, response []byte, ttl uint32) {
	if ttl == 0 {
		return
	}
	// Copy the bytes: the caller's buffer may be reused for the next datagram.
	stored := make([]byte, len(response))
	copy(stored, response)

	c.mu.Lock()
	c.m[k] = cacheEntry{
		response: stored,
		expiry:   c.now().Add(time.Duration(ttl) * time.Second),
	}
	c.mu.Unlock()
}
