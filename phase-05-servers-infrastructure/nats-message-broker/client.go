package main

import (
	"bufio"
	"net"
	"sync"
)

// client represents one connected TCP client (a publisher, a subscriber, or
// both — NATS connections are bidirectional). Every client gets its own
// goroutine to READ commands and a second goroutine to WRITE outbound frames.
//
// Why a dedicated writer goroutine + channel? Multiple publishers can deliver
// messages to the same subscriber *concurrently*. A net.Conn is not safe for
// concurrent writes, so instead of locking the socket we funnel every outbound
// frame through one buffered channel (`out`) that a single writer goroutine
// drains. This is the idiomatic Go pattern: "don't share memory by locking the
// socket — share the socket by communicating over a channel."
//
// 🐍 Python note: a channel is like a thread-safe queue.Queue, and a goroutine
// is like a very cheap thread. `out` is the queue; writeLoop is the consumer.
type client struct {
	id      uint64
	conn    net.Conn
	srv     *Server
	out     chan []byte   // outbound frames waiting to hit the socket
	quit    chan struct{} // closed once, signals reader/writer to stop
	once    sync.Once     // guarantees close() runs exactly once
	verbose bool          // whether to echo +OK (set by the CONNECT verb)
}

// enqueue hands an outbound frame to the writer goroutine.
//
// It selects on `quit` so that if the client has already disconnected we drop
// the frame instead of blocking forever (or panicking on a closed channel).
// Dropping is intentional: NATS core is *at-most-once* with no persistence.
func (c *client) enqueue(frame []byte) {
	select {
	case c.out <- frame:
	case <-c.quit:
	}
}

// writeLoop is the single owner of the socket's write side. It serialises every
// outbound frame so concurrent publishers never interleave bytes on the wire.
func (c *client) writeLoop() {
	w := bufio.NewWriter(c.conn)
	for {
		select {
		case frame := <-c.out:
			if _, err := w.Write(frame); err != nil {
				c.close()
				return
			}
			if err := w.Flush(); err != nil {
				c.close()
				return
			}
		case <-c.quit:
			return
		}
	}
}

// close tears the client down exactly once: it signals the goroutines to stop
// (via the quit channel) and closes the underlying socket. sync.Once makes this
// safe to call from both the reader and the writer without a double-close.
func (c *client) close() {
	c.once.Do(func() {
		close(c.quit)
		c.conn.Close()
	})
}

// ok writes a +OK acknowledgement, but only when this client asked for verbose
// acknowledgements in its CONNECT. Real NATS clients default verbose to false
// and rely on the absence of -ERR to mean success.
func (c *client) ok() {
	if c.verbose {
		c.enqueue([]byte("+OK\r\n"))
	}
}

// err writes a protocol error frame. Errors are non-fatal for the connection
// (the client may keep issuing commands), matching real NATS behaviour.
func (c *client) err(msg string) {
	c.enqueue([]byte("-ERR '" + msg + "'\r\n"))
}
