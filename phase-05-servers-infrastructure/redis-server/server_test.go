package main

import (
	"bufio"
	"io"
	"log"
	"net"
	"path/filepath"
	"testing"
	"time"
)

// testClient is a minimal RESP client used only by the tests. It deliberately
// reuses this package's own encoder/decoder (no external redis library) so the
// tests stay self-contained.
type testClient struct {
	conn   net.Conn
	reader *bufio.Reader
}

func dial(t *testing.T, addr string) *testClient {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return &testClient{conn: conn, reader: bufio.NewReader(conn)}
}

func (c *testClient) close() { c.conn.Close() }

// do sends a command (as a RESP array of bulk strings) and returns the reply.
func (c *testClient) do(t *testing.T, parts ...string) Value {
	t.Helper()
	args := make([]Value, len(parts))
	for i, p := range parts {
		args[i] = BulkString(p)
	}
	if _, err := c.conn.Write(Array(args...).Marshal()); err != nil {
		t.Fatalf("write: %v", err)
	}
	c.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	reply, err := DecodeValue(c.reader)
	if err != nil {
		t.Fatalf("read reply for %v: %v", parts, err)
	}
	return reply
}

// startServer spins up a server on an OS-assigned localhost port and returns it
// plus its address. It registers cleanup automatically.
func startServer(t *testing.T, rdbPath string) (*Server, string) {
	t.Helper()
	logger := log.New(io.Discard, "", 0)
	srv := NewServer("127.0.0.1:0", rdbPath, logger)
	srv.sweepInterval = 20 * time.Millisecond // fast active expiry for the test
	if err := srv.Listen(); err != nil {
		t.Fatalf("listen: %v", err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })
	return srv, srv.Addr()
}

func assertSimple(t *testing.T, v Value, want string) {
	t.Helper()
	if v.typ != typeSimpleString || v.str != want {
		t.Fatalf("got %#v; want simple string %q", v, want)
	}
}

func assertBulk(t *testing.T, v Value, want string) {
	t.Helper()
	if v.typ != typeBulkString || v.null || v.str != want {
		t.Fatalf("got %#v; want bulk %q", v, want)
	}
}

func assertInt(t *testing.T, v Value, want int64) {
	t.Helper()
	if v.typ != typeInteger || v.num != want {
		t.Fatalf("got %#v; want integer %d", v, want)
	}
}

func assertNullBulk(t *testing.T, v Value) {
	t.Helper()
	if v.typ != typeBulkString || !v.null {
		t.Fatalf("got %#v; want null bulk", v)
	}
}

func TestServerBasicCommands(t *testing.T) {
	_, addr := startServer(t, "")
	c := dial(t, addr)
	defer c.close()

	assertSimple(t, c.do(t, "PING"), "PONG")
	assertBulk(t, c.do(t, "ECHO", "hi there"), "hi there")

	assertSimple(t, c.do(t, "SET", "name", "malcolm"), "OK")
	assertBulk(t, c.do(t, "GET", "name"), "malcolm")
	assertNullBulk(t, c.do(t, "GET", "missing"))

	assertInt(t, c.do(t, "EXISTS", "name", "missing"), 1)
	assertInt(t, c.do(t, "DEL", "name"), 1)
	assertNullBulk(t, c.do(t, "GET", "name"))
}

func TestServerIncrDecr(t *testing.T) {
	_, addr := startServer(t, "")
	c := dial(t, addr)
	defer c.close()

	assertInt(t, c.do(t, "INCR", "n"), 1)
	assertInt(t, c.do(t, "INCR", "n"), 2)
	assertInt(t, c.do(t, "DECR", "n"), 1)

	c.do(t, "SET", "word", "abc")
	if reply := c.do(t, "INCR", "word"); reply.typ != typeError {
		t.Fatalf("INCR on non-integer: got %#v; want error", reply)
	}
}

func TestServerExpireTTL(t *testing.T) {
	_, addr := startServer(t, "")
	c := dial(t, addr)
	defer c.close()

	c.do(t, "SET", "temp", "v")
	assertInt(t, c.do(t, "TTL", "temp"), -1) // exists, no expiry
	assertInt(t, c.do(t, "EXPIRE", "temp", "100"), 1)
	if v := c.do(t, "TTL", "temp"); v.typ != typeInteger || v.num <= 0 || v.num > 100 {
		t.Fatalf("TTL after EXPIRE = %#v; want 1..100", v)
	}
	assertInt(t, c.do(t, "TTL", "missing"), -2)
}

func TestServerExpiryViaPX(t *testing.T) {
	_, addr := startServer(t, "")
	c := dial(t, addr)
	defer c.close()

	// Set a key that lives 40ms, then wait for the background sweeper to reap it.
	assertSimple(t, c.do(t, "SET", "blink", "x", "PX", "40"), "OK")
	assertBulk(t, c.do(t, "GET", "blink"), "x")

	time.Sleep(150 * time.Millisecond)
	assertNullBulk(t, c.do(t, "GET", "blink"))
}

func TestServerMGetMSetAppendGetSet(t *testing.T) {
	_, addr := startServer(t, "")
	c := dial(t, addr)
	defer c.close()

	assertSimple(t, c.do(t, "MSET", "a", "1", "b", "2"), "OK")
	reply := c.do(t, "MGET", "a", "missing", "b")
	if reply.typ != typeArray || len(reply.array) != 3 {
		t.Fatalf("MGET reply = %#v", reply)
	}
	assertBulk(t, reply.array[0], "1")
	assertNullBulk(t, reply.array[1])
	assertBulk(t, reply.array[2], "2")

	assertInt(t, c.do(t, "APPEND", "a", "23"), 3) // "1" + "23" = "123"
	assertBulk(t, c.do(t, "GET", "a"), "123")
	assertBulk(t, c.do(t, "GETSET", "a", "new"), "123")
	assertBulk(t, c.do(t, "GET", "a"), "new")
}

// TestServerSaveAndReload is the persistence round-trip: write data, SAVE, shut the
// server down, then start a BRAND NEW server pointed at the same file and assert
// every key (including a TTL) came back.
func TestServerSaveAndReload(t *testing.T) {
	rdbPath := filepath.Join(t.TempDir(), "dump.rdb")

	// --- first server: populate and persist ---
	srv1, addr1 := startServer(t, rdbPath)
	c1 := dial(t, addr1)
	c1.do(t, "SET", "user:1", "alice")
	c1.do(t, "SET", "user:2", "bob")
	c1.do(t, "SET", "session", "tok", "EX", "1000")
	assertSimple(t, c1.do(t, "SAVE"), "OK")
	c1.close()
	srv1.Close() // stop it so there's no ambiguity about who owns the data

	// --- second server: load from the same file ---
	_, addr2 := startServer(t, rdbPath)
	c2 := dial(t, addr2)
	defer c2.close()

	assertBulk(t, c2.do(t, "GET", "user:1"), "alice")
	assertBulk(t, c2.do(t, "GET", "user:2"), "bob")
	assertBulk(t, c2.do(t, "GET", "session"), "tok")
	// The TTL must survive the round-trip (allowing for a little elapsed time).
	if v := c2.do(t, "TTL", "session"); v.typ != typeInteger || v.num <= 0 || v.num > 1000 {
		t.Fatalf("restored TTL = %#v; want 1..1000", v)
	}
}
