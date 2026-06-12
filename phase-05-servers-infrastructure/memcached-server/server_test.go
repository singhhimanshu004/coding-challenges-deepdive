package main

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// testClient is a raw TCP client that speaks the memcached text protocol by
// hand — exactly how a real memcached client (or `telnet`) would. We use it to
// exercise the full accept -> parse -> store -> reply path over a real socket.
type testClient struct {
	t    *testing.T
	conn net.Conn
	r    *bufio.Reader
}

// startTestServer boots a server on 127.0.0.1:0 (kernel-chosen free port) and
// returns the server plus its dial address. The optional store lets a test
// inject a fake clock; pass nil for a default store.
func startTestServer(t *testing.T, store *Store) (*Server, string) {
	t.Helper()
	if store == nil {
		store = NewStore(0)
	}
	srv := NewServer(store, false, nil)
	if err := srv.Listen("127.0.0.1:0"); err != nil {
		t.Fatalf("listen: %v", err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })
	return srv, srv.Addr().String()
}

func dialTestClient(t *testing.T, addr string) *testClient {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return &testClient{t: t, conn: conn, r: bufio.NewReader(conn)}
}

// send writes a raw protocol line (CRLF is appended for you).
func (c *testClient) send(line string) {
	c.t.Helper()
	c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if _, err := fmt.Fprintf(c.conn, "%s\r\n", line); err != nil {
		c.t.Fatalf("write %q: %v", line, err)
	}
}

// sendRaw writes bytes verbatim (used for data blocks).
func (c *testClient) sendRaw(s string) {
	c.t.Helper()
	c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.conn.Write([]byte(s)); err != nil {
		c.t.Fatalf("write raw: %v", err)
	}
}

// readLine reads one CRLF-terminated reply line (CRLF trimmed).
func (c *testClient) readLine() string {
	c.t.Helper()
	c.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	s, err := c.r.ReadString('\n')
	if err != nil {
		c.t.Fatalf("read: %v (got %q)", err, s)
	}
	return strings.TrimRight(s, "\r\n")
}

// expect reads one line and asserts it equals want.
func (c *testClient) expect(want string) {
	c.t.Helper()
	if got := c.readLine(); got != want {
		c.t.Fatalf("reply = %q, want %q", got, want)
	}
}

// set is a convenience for the most common storage command.
func (c *testClient) set(key string, flags uint32, exptime int64, value string) {
	c.t.Helper()
	c.send(fmt.Sprintf("set %s %d %d %d", key, flags, exptime, len(value)))
	c.sendRaw(value + "\r\n")
	c.expect("STORED")
}

func TestProtocolSetGet(t *testing.T) {
	_, addr := startTestServer(t, nil)
	c := dialTestClient(t, addr)

	c.set("foo", 42, 0, "hello")

	c.send("get foo")
	c.expect("VALUE foo 42 5") // key, flags, byte length
	c.expect("hello")
	c.expect("END")
}

func TestProtocolGetMissingReturnsEnd(t *testing.T) {
	_, addr := startTestServer(t, nil)
	c := dialTestClient(t, addr)

	c.send("get does-not-exist")
	c.expect("END") // no VALUE lines, just the END terminator
}

func TestProtocolGets(t *testing.T) {
	_, addr := startTestServer(t, nil)
	c := dialTestClient(t, addr)
	c.set("k", 0, 0, "v")

	c.send("gets k")
	line := c.readLine()
	// gets adds a trailing CAS token: "VALUE k 0 1 <cas>"
	if !strings.HasPrefix(line, "VALUE k 0 1 ") {
		t.Fatalf("gets header = %q, want VALUE k 0 1 <cas>", line)
	}
	c.expect("v")
	c.expect("END")
}

func TestProtocolAddReplace(t *testing.T) {
	_, addr := startTestServer(t, nil)
	c := dialTestClient(t, addr)

	c.send("add a 0 0 1")
	c.sendRaw("1\r\n")
	c.expect("STORED")

	c.send("add a 0 0 1")
	c.sendRaw("2\r\n")
	c.expect("NOT_STORED") // already exists

	c.send("replace missing 0 0 1")
	c.sendRaw("x\r\n")
	c.expect("NOT_STORED") // does not exist
}

func TestProtocolAppendPrepend(t *testing.T) {
	_, addr := startTestServer(t, nil)
	c := dialTestClient(t, addr)
	c.set("k", 0, 0, "B")

	c.send("append k 0 0 1")
	c.sendRaw("C\r\n")
	c.expect("STORED")
	c.send("prepend k 0 0 1")
	c.sendRaw("A\r\n")
	c.expect("STORED")

	c.send("get k")
	c.expect("VALUE k 0 3")
	c.expect("ABC")
	c.expect("END")
}

func TestProtocolCAS(t *testing.T) {
	_, addr := startTestServer(t, nil)
	c := dialTestClient(t, addr)
	c.set("k", 0, 0, "v1")

	c.send("gets k")
	header := c.readLine()
	c.expect("v1")
	c.expect("END")
	parts := strings.Fields(header)
	cas := parts[len(parts)-1]

	// Stale token loses.
	c.send("cas k 0 0 2 " + "999999")
	c.sendRaw("v2\r\n")
	c.expect("EXISTS")

	// Correct token wins.
	c.send(fmt.Sprintf("cas k 0 0 2 %s", cas))
	c.sendRaw("v2\r\n")
	c.expect("STORED")
}

func TestProtocolDelete(t *testing.T) {
	_, addr := startTestServer(t, nil)
	c := dialTestClient(t, addr)
	c.set("k", 0, 0, "v")

	c.send("delete k")
	c.expect("DELETED")
	c.send("delete k")
	c.expect("NOT_FOUND")
}

func TestProtocolIncrDecr(t *testing.T) {
	_, addr := startTestServer(t, nil)
	c := dialTestClient(t, addr)
	c.set("n", 0, 0, "10")

	c.send("incr n 5")
	c.expect("15")
	c.send("decr n 100") // floors at 0
	c.expect("0")
	c.send("incr missing 1")
	c.expect("NOT_FOUND")
}

func TestProtocolFlushAll(t *testing.T) {
	_, addr := startTestServer(t, nil)
	c := dialTestClient(t, addr)
	c.set("k", 0, 0, "v")

	c.send("flush_all")
	c.expect("OK")
	c.send("get k")
	c.expect("END") // gone
}

func TestProtocolUnknownCommand(t *testing.T) {
	_, addr := startTestServer(t, nil)
	c := dialTestClient(t, addr)
	c.send("frobnicate x")
	c.expect("ERROR")
}

func TestProtocolNoreply(t *testing.T) {
	_, addr := startTestServer(t, nil)
	c := dialTestClient(t, addr)

	c.send("set k 0 0 1 noreply")
	c.sendRaw("v\r\n")
	// No STORED reply expected; the next get proves the noreply set worked.
	c.send("get k")
	c.expect("VALUE k 0 1")
	c.expect("v")
	c.expect("END")
}

func TestProtocolExpiryWithInjectedClock(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_000_000_000, 0)}
	store := NewStore(0)
	store.now = clk.now
	_, addr := startTestServer(t, store)
	c := dialTestClient(t, addr)

	c.send("set k 0 1 1") // 1-second TTL
	c.sendRaw("v\r\n")
	c.expect("STORED")

	c.send("get k")
	c.expect("VALUE k 0 1")
	c.expect("v")
	c.expect("END")

	clk.advance(2 * time.Second) // jump past the TTL

	c.send("get k")
	c.expect("END") // expired and gone
}

func TestProtocolLRUEviction(t *testing.T) {
	store := NewStore(2) // cap of 2 items
	_, addr := startTestServer(t, store)
	c := dialTestClient(t, addr)

	c.set("a", 0, 0, "1")
	c.set("b", 0, 0, "2")
	// Touch "a" so "b" is the coldest.
	c.send("get a")
	c.expect("VALUE a 0 1")
	c.expect("1")
	c.expect("END")

	c.set("c", 0, 0, "3") // overflow -> evict "b"

	c.send("get b")
	c.expect("END") // evicted
	c.send("get a")
	c.expect("VALUE a 0 1")
	c.expect("1")
	c.expect("END")
}
