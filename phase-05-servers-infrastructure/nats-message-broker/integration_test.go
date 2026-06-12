package main

import (
	"bufio"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

// These tests are fully self-contained: they start the broker on 127.0.0.1:0
// (an OS-assigned free port) and talk to it with raw TCP clients that speak the
// NATS text protocol by hand. No external libraries, no real nats.go.

// startBroker boots a broker on an ephemeral port and returns its address.
func startBroker(t *testing.T) string {
	t.Helper()
	srv := NewServer("127.0.0.1:0", false)
	if err := srv.Listen(); err != nil {
		t.Fatalf("listen: %v", err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })
	return srv.Addr()
}

// testClient is a raw-TCP NATS client used only by the tests.
type testClient struct {
	t    *testing.T
	conn net.Conn
	r    *bufio.Reader
}

func dial(t *testing.T, addr string) *testClient {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	c := &testClient{t: t, conn: conn, r: bufio.NewReader(conn)}

	// Every connection is greeted with an INFO line first.
	line, err := c.r.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "INFO") {
		t.Fatalf("expected INFO greeting, got %q (err %v)", line, err)
	}
	return c
}

func (c *testClient) send(s string) {
	c.t.Helper()
	if _, err := c.conn.Write([]byte(s)); err != nil {
		c.t.Fatalf("write: %v", err)
	}
}

func (c *testClient) sub(subject, sid string)  { c.send("SUB " + subject + " " + sid + "\r\n") }
func (c *testClient) subQ(subj, q, sid string) { c.send("SUB " + subj + " " + q + " " + sid + "\r\n") }
func (c *testClient) unsub(sid string)         { c.send("UNSUB " + sid + "\r\n") }

func (c *testClient) pub(subject, payload string) {
	c.send("PUB " + subject + " " + strconv.Itoa(len(payload)) + "\r\n" + payload + "\r\n")
}

// flush issues a PING and waits for the PONG. Because the broker processes each
// connection's commands strictly in order, a returned PONG proves that every
// command sent earlier on this connection (e.g. a SUB) has been applied.
func (c *testClient) flush() {
	c.t.Helper()
	c.send("PING\r\n")
	c.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		line, err := c.r.ReadString('\n')
		if err != nil {
			c.t.Fatalf("flush: %v", err)
		}
		if strings.TrimRight(line, "\r\n") == "PONG" {
			return
		}
	}
}

type message struct {
	subject, sid, reply, payload string
}

// readMsg waits up to timeout for a MSG frame. It returns an error (a timeout)
// if none arrives — which is exactly how the "does NOT deliver" tests assert a
// non-delivery.
func (c *testClient) readMsg(timeout time.Duration) (message, error) {
	c.conn.SetReadDeadline(time.Now().Add(timeout))
	for {
		line, err := c.r.ReadString('\n')
		if err != nil {
			return message{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if !strings.HasPrefix(line, "MSG") {
			continue // skip PONG/+OK/etc.
		}
		f := strings.Fields(line)
		var m message
		var size int
		switch len(f) {
		case 4: // MSG subject sid size
			m.subject, m.sid = f[1], f[2]
			size, _ = strconv.Atoi(f[3])
		case 5: // MSG subject sid reply size
			m.subject, m.sid, m.reply = f[1], f[2], f[3]
			size, _ = strconv.Atoi(f[4])
		default:
			c.t.Fatalf("malformed MSG line: %q", line)
		}
		buf := make([]byte, size+2) // payload + trailing CRLF
		if _, err := io.ReadFull(c.r, buf); err != nil {
			return message{}, err
		}
		m.payload = string(buf[:size])
		return m, nil
	}
}

func TestSingleTokenWildcardDelivers(t *testing.T) {
	addr := startBroker(t)
	sub := dial(t, addr)
	pub := dial(t, addr)

	sub.sub("foo.*", "1")
	sub.flush()

	pub.pub("foo.bar", "hello")

	m, err := sub.readMsg(time.Second)
	if err != nil {
		t.Fatalf("expected a message, got error: %v", err)
	}
	if m.subject != "foo.bar" || m.sid != "1" || m.payload != "hello" {
		t.Fatalf("unexpected message: %+v", m)
	}
}

func TestNonMatchingSubjectNotDelivered(t *testing.T) {
	addr := startBroker(t)
	sub := dial(t, addr)
	pub := dial(t, addr)

	sub.sub("foo.*", "1")
	sub.flush()

	// Neither of these matches "foo.*": different head, and too many tokens.
	pub.pub("bar.baz", "nope")
	pub.pub("foo.bar.baz", "also nope")

	if m, err := sub.readMsg(300 * time.Millisecond); err == nil {
		t.Fatalf("expected no delivery, but received: %+v", m)
	}
}

func TestTailWildcardMatchesMultipleTokens(t *testing.T) {
	addr := startBroker(t)
	sub := dial(t, addr)
	pub := dial(t, addr)

	sub.sub("foo.>", "1")
	sub.flush()

	pub.pub("foo.bar.baz", "deep")

	m, err := sub.readMsg(time.Second)
	if err != nil {
		t.Fatalf("expected a message, got error: %v", err)
	}
	if m.subject != "foo.bar.baz" || m.payload != "deep" {
		t.Fatalf("unexpected message: %+v", m)
	}
}

func TestQueueGroupDeliversToExactlyOne(t *testing.T) {
	addr := startBroker(t)
	a := dial(t, addr)
	b := dial(t, addr)
	pub := dial(t, addr)

	a.subQ("foo", "workers", "1")
	b.subQ("foo", "workers", "2")
	a.flush()
	b.flush()

	pub.pub("foo", "task")

	got := 0
	if _, err := a.readMsg(400 * time.Millisecond); err == nil {
		got++
	}
	if _, err := b.readMsg(400 * time.Millisecond); err == nil {
		got++
	}
	if got != 1 {
		t.Fatalf("queue group should deliver to exactly one member, got %d", got)
	}
}

func TestUnsubStopsDelivery(t *testing.T) {
	addr := startBroker(t)
	sub := dial(t, addr)
	pub := dial(t, addr)

	sub.sub("foo", "1")
	sub.flush()

	// First publish should arrive.
	pub.pub("foo", "first")
	if _, err := sub.readMsg(time.Second); err != nil {
		t.Fatalf("expected first message, got error: %v", err)
	}

	// After UNSUB, further publishes must not arrive.
	sub.unsub("1")
	sub.flush()
	pub.pub("foo", "second")
	if m, err := sub.readMsg(300 * time.Millisecond); err == nil {
		t.Fatalf("expected no delivery after UNSUB, got: %+v", m)
	}
}

func TestPingPong(t *testing.T) {
	addr := startBroker(t)
	c := dial(t, addr)
	c.send("PING\r\n")
	c.conn.SetReadDeadline(time.Now().Add(time.Second))
	line, err := c.r.ReadString('\n')
	if err != nil || strings.TrimRight(line, "\r\n") != "PONG" {
		t.Fatalf("expected PONG, got %q (err %v)", line, err)
	}
}
