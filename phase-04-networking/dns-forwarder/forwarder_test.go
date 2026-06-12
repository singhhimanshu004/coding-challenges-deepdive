package main

// Hermetic tests: NO real internet. We stand up a fake upstream resolver (a
// local net.ListenUDP that returns a canned answer and counts how often it is
// asked) and a real UDP client, then assert the forward / cache-hit / TTL-expiry
// behaviour by watching that counter.

import (
	"encoding/binary"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Test helpers: craft DNS queries/responses and a controllable clock.
// ---------------------------------------------------------------------------

// buildAQuery returns the raw bytes of a standard recursive A query for name.
func buildAQuery(id uint16, name string) []byte {
	h := Header{ID: id, Flags: flagRD, QDCount: 1}
	buf := h.pack(nil)
	q := Question{Name: name, Type: TypeA, Class: ClassIN}
	return q.pack(buf)
}

// buildAResponse returns the raw bytes of a response to an A query for name,
// carrying a single A record (ip, four octets) with the given TTL.
func buildAResponse(id uint16, name string, ip [4]byte, ttl uint32) []byte {
	h := Header{ID: id, Flags: flagQR | flagRD, QDCount: 1, ANCount: 1}
	buf := h.pack(nil)

	q := Question{Name: name, Type: TypeA, Class: ClassIN}
	buf = q.pack(buf)

	// Answer RR: NAME TYPE(2) CLASS(2) TTL(4) RDLENGTH(2) RDATA(4)
	buf = encodeName(buf, name)
	var fixed [10]byte
	binary.BigEndian.PutUint16(fixed[0:], TypeA)
	binary.BigEndian.PutUint16(fixed[2:], ClassIN)
	binary.BigEndian.PutUint32(fixed[4:], ttl)
	binary.BigEndian.PutUint16(fixed[8:], 4) // RDLENGTH
	buf = append(buf, fixed[:]...)
	return append(buf, ip[:]...)
}

// fakeClock is a manually advanced clock so TTL expiry is tested without sleeps.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// fakeUpstream is a local UDP server that answers every A query with a canned
// record and counts how many queries it received.
type fakeUpstream struct {
	conn  *net.UDPConn
	count int64 // atomic
	ttl   uint32
	ip    [4]byte
}

// startFakeUpstream binds a local upstream and serves until stop() is called.
func startFakeUpstream(t *testing.T, ip [4]byte, ttl uint32) *fakeUpstream {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("fake upstream listen: %v", err)
	}
	up := &fakeUpstream{conn: conn, ttl: ttl, ip: ip}

	go func() {
		buf := make([]byte, maxUDPMessage)
		for {
			n, client, err := conn.ReadFromUDP(buf)
			if err != nil {
				return // socket closed: stop serving
			}
			atomic.AddInt64(&up.count, 1)

			msg, err := parseMessage(buf[:n])
			if err != nil || len(msg.Questions) == 0 {
				continue
			}
			q := msg.Questions[0]
			resp := buildAResponse(msg.Header.ID, q.Name, up.ip, up.ttl)
			_, _ = conn.WriteToUDP(resp, client)
		}
	}()
	return up
}

func (u *fakeUpstream) addr() string { return u.conn.LocalAddr().String() }
func (u *fakeUpstream) hits() int64  { return atomic.LoadInt64(&u.count) }
func (u *fakeUpstream) close()       { u.conn.Close() }

// startForwarder binds a forwarder pointed at the given upstream and serves it.
// It returns the forwarder's address and its server (so tests can reach the
// cache clock).
func startForwarder(t *testing.T, upstream string) (string, *Server) {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("forwarder listen: %v", err)
	}
	srv := newServer(conn, upstream, false, discardLogger())
	go srv.Serve()
	t.Cleanup(func() { conn.Close() })
	return conn.LocalAddr().String(), srv
}

// ask sends a single A query to addr and returns the parsed reply.
func ask(t *testing.T, addr string, id uint16, name string) *Message {
	t.Helper()
	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		t.Fatalf("resolve %s: %v", addr, err)
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		t.Fatalf("dial forwarder: %v", err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write(buildAQuery(id, name)); err != nil {
		t.Fatalf("write query: %v", err)
	}
	buf := make([]byte, maxUDPMessage)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read reply: %v", err)
	}
	msg, err := parseMessage(buf[:n])
	if err != nil {
		t.Fatalf("parse reply: %v", err)
	}
	return msg
}

// ---------------------------------------------------------------------------
// (a) A query is forwarded and the answer relayed back.
// ---------------------------------------------------------------------------

func TestForwardAndRelay(t *testing.T) {
	up := startFakeUpstream(t, [4]byte{93, 184, 216, 34}, 300)
	defer up.close()
	fwd, _ := startForwarder(t, up.addr())

	reply := ask(t, fwd, 0x1234, "example.com")

	if up.hits() != 1 {
		t.Fatalf("expected upstream to be hit once, got %d", up.hits())
	}
	if reply.Header.ID != 0x1234 {
		t.Errorf("reply ID = %#x, want 0x1234 (must echo the client's ID)", reply.Header.ID)
	}
	if len(reply.Answers) != 1 || reply.Answers[0].Data != "93.184.216.34" {
		t.Errorf("relayed answer = %+v, want the canned 93.184.216.34", reply.Answers)
	}
}

// ---------------------------------------------------------------------------
// (b) A second identical query is served from cache — upstream NOT hit again —
//     and the cached reply still carries the SECOND client's transaction ID.
// ---------------------------------------------------------------------------

func TestSecondQueryServedFromCache(t *testing.T) {
	up := startFakeUpstream(t, [4]byte{1, 2, 3, 4}, 300)
	defer up.close()
	fwd, _ := startForwarder(t, up.addr())

	first := ask(t, fwd, 0x1111, "cached.test")
	if up.hits() != 1 {
		t.Fatalf("after first query upstream hits = %d, want 1", up.hits())
	}
	if first.Answers[0].Data != "1.2.3.4" {
		t.Fatalf("first answer = %q, want 1.2.3.4", first.Answers[0].Data)
	}

	// Same question, DIFFERENT transaction ID.
	second := ask(t, fwd, 0x2222, "cached.test")

	if up.hits() != 1 {
		t.Errorf("second identical query hit upstream again: hits = %d, want 1", up.hits())
	}
	if second.Header.ID != 0x2222 {
		t.Errorf("cached reply ID = %#x, want 0x2222 (must be patched per client)", second.Header.ID)
	}
	if second.Answers[0].Data != "1.2.3.4" {
		t.Errorf("cached answer = %q, want 1.2.3.4", second.Answers[0].Data)
	}
}

// ---------------------------------------------------------------------------
// (c) After the TTL elapses, the entry expires and the upstream is queried
//     again. We drive a fake clock so no real waiting is needed.
// ---------------------------------------------------------------------------

func TestCacheExpiresAfterTTL(t *testing.T) {
	const ttl = 5 // seconds
	up := startFakeUpstream(t, [4]byte{10, 0, 0, 1}, ttl)
	defer up.close()
	fwd, srv := startForwarder(t, up.addr())

	clock := &fakeClock{t: time.Now()}
	srv.cache.now = clock.Now // inject the controllable clock

	ask(t, fwd, 1, "expire.test")
	if up.hits() != 1 {
		t.Fatalf("first query: upstream hits = %d, want 1", up.hits())
	}

	// Still within TTL → served from cache, upstream untouched.
	clock.Advance((ttl - 1) * time.Second)
	ask(t, fwd, 2, "expire.test")
	if up.hits() != 1 {
		t.Fatalf("within TTL: upstream hits = %d, want 1 (should be cache hit)", up.hits())
	}

	// Past TTL → entry expired, upstream queried again.
	clock.Advance(2 * time.Second) // now total = ttl+1 seconds elapsed
	ask(t, fwd, 3, "expire.test")
	if up.hits() != 2 {
		t.Fatalf("after TTL: upstream hits = %d, want 2 (cache should have expired)", up.hits())
	}
}

// ---------------------------------------------------------------------------
// Cache unit test (table-driven): TTL semantics in isolation, no sockets.
// ---------------------------------------------------------------------------

func TestCacheTTL(t *testing.T) {
	key := cacheKey{name: "x.test", qtype: TypeA, qclass: ClassIN}

	tests := []struct {
		name    string
		ttl     uint32
		advance time.Duration
		wantHit bool
	}{
		{"fresh entry hits", 60, 0, true},
		{"just before expiry hits", 60, 59 * time.Second, true},
		{"exactly at expiry misses", 60, 60 * time.Second, false},
		{"after expiry misses", 60, 120 * time.Second, false},
		{"zero TTL never cached", 0, 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clock := &fakeClock{t: time.Now()}
			c := newCache()
			c.now = clock.Now

			c.set(key, []byte("response"), tc.ttl)
			clock.Advance(tc.advance)

			_, ok := c.get(key)
			if ok != tc.wantHit {
				t.Errorf("get hit = %v, want %v", ok, tc.wantHit)
			}
		})
	}
}
