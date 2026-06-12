package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"time"
)

// keepAliveTimeout bounds how long an idle kept-alive connection may wait for
// the next request before we close it. Without this, every client that opens a
// connection and goes quiet would hold a goroutine and a file descriptor open
// forever — a trivial way to exhaust the server. Real servers all set one
// (nginx's keepalive_timeout defaults to ~75s).
const keepAliveTimeout = 10 * time.Second

// server holds the runtime configuration and the router.
type server struct {
	addr    string
	router  *router
	verbose bool
	logger  *log.Logger
}

// listenAndServe binds the TCP socket and runs the accept loop until ln errors.
//
// 🐍 net.Listen("tcp", addr) is like Python's socket(); s.bind(); s.listen().
// ln.Accept() blocks until a client connects and then returns a net.Conn — the
// same bidirectional byte pipe the curl challenge dialed from the client side.
// A server is just the other end of that pipe.
func (s *server) listenAndServe() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.addr, err)
	}
	return s.serve(ln)
}

// serve runs the accept loop on an already-bound listener. Splitting this out
// from listenAndServe lets the tests bind their own 127.0.0.1:0 listener (so
// the OS picks a free port), learn its address via ln.Addr(), and drive real
// requests through the server — no fixed port, no external network.
//
// 🐍 net.Listen("tcp", addr) is like Python's socket(); s.bind(); s.listen().
// ln.Accept() blocks until a client connects and then returns a net.Conn — the
// same bidirectional byte pipe the curl challenge dialed from the client side.
// A server is just the other end of that pipe.
func (s *server) serve(ln net.Listener) error {
	defer ln.Close()
	s.logger.Printf("listening on %s (serving %s)", ln.Addr(), s.describeRoot())

	// THE ACCEPT LOOP — the heart of every server.
	//
	// We Accept a connection, then hand it to a NEW GOROUTINE and immediately
	// loop back to Accept the next one. That `go` keyword is the whole
	// concurrency model: each connection is served independently and in
	// parallel, and a slow client can never block other clients.
	//
	// 🐍 In Python you'd reach for threads, asyncio, or a process pool to get
	// this. In Go a goroutine is so cheap (a few KB of stack, scheduled by the
	// runtime onto OS threads) that "one goroutine per connection" is the
	// normal, idiomatic design rather than a thing to optimise away.
	for {
		conn, err := ln.Accept()
		if err != nil {
			return fmt.Errorf("accept: %w", err)
		}
		go s.handleConn(conn)
	}
}

// handleConn serves every request that arrives on a single TCP connection.
//
// This loop is where HTTP/1.1 KEEP-ALIVE lives. In HTTP/1.0 the model was "one
// request per TCP connection": connect, ask, get answer, disconnect — paying a
// full TCP (and for HTTPS, TLS) handshake EVERY time. HTTP/1.1 made persistent
// connections the default: after we send a response we loop back and read the
// NEXT request on the same socket, amortising that handshake cost across many
// requests. We stop looping when:
//   - the client asked us to (Connection: close), or
//   - the client sent HTTP/1.0 without opting in to keep-alive, or
//   - the idle read timeout fires, or
//   - the connection errors / the client closes its end.
func (s *server) handleConn(conn net.Conn) {
	defer conn.Close()
	remote := conn.RemoteAddr()

	// One buffered reader for the whole connection's lifetime. Re-creating it
	// per request would discard bytes the client may have already pipelined
	// into the buffer.
	br := bufio.NewReader(conn)

	for {
		// Arm the idle/read timeout before each request. SetReadDeadline makes
		// the next Read fail if no bytes arrive in time, which is how we reclaim
		// connections from clients that connect and then go silent.
		conn.SetReadDeadline(time.Now().Add(keepAliveTimeout))

		req, err := parseRequest(br)
		if err != nil {
			// A clean EOF means the client closed a kept-alive connection with
			// no further request — that's expected, not an error to report.
			if errors.Is(err, io.EOF) {
				return
			}
			// A timeout means the connection went idle; just close it quietly.
			var nerr net.Error
			if errors.As(err, &nerr) && nerr.Timeout() {
				return
			}
			// Anything else is a malformed request: answer 400 and close.
			s.logf("%s -> 400 (%v)", remote, err)
			resp := textResponse(400, "text/plain; charset=utf-8", "400 Bad Request\n")
			resp.setHeader("Connection", "close")
			resp.write(conn)
			return
		}

		resp := s.router.route(req)

		// Decide whether this connection survives to serve another request.
		// Both sides must agree to keep it alive; we honour the client's wish
		// and advertise our decision back via the Connection header so the
		// client knows whether to reuse the socket.
		keepAlive := req.wantsKeepAlive()
		if keepAlive {
			resp.setHeader("Connection", "keep-alive")
			resp.setHeader("Keep-Alive", fmt.Sprintf("timeout=%d", int(keepAliveTimeout.Seconds())))
		} else {
			resp.setHeader("Connection", "close")
		}

		if err := resp.write(conn); err != nil {
			s.logf("%s write error: %v", remote, err)
			return
		}
		s.logf("%s %q %s -> %s", remote, req.method+" "+req.path, req.version, statusString(resp.statusCode))

		if !keepAlive {
			return
		}
	}
}

// logf writes a line only when verbose logging is enabled.
func (s *server) logf(format string, args ...any) {
	if s.verbose {
		s.logger.Printf(format, args...)
	}
}

// describeRoot is a human-friendly description of the static root for startup
// logging.
func (s *server) describeRoot() string {
	if s.router.files == nil {
		return "dynamic routes only"
	}
	return "static root " + s.router.files.root
}
