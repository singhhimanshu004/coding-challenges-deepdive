// server.go owns the TCP side: it opens a net.Listener, accepts connections, and
// spawns one goroutine per connection (the classic Go server model). The actual
// protocol parsing lives in conn.go.
package main

import (
	"fmt"
	"io"
	"net"
	"sync"
)

// Server ties a listener to a Store.
type Server struct {
	store   *Store
	verbose bool
	logf    func(string, ...any) // where verbose lines go (os.Stderr in prod)

	mu      sync.Mutex
	ln      net.Listener
	conns   map[net.Conn]struct{} // tracked so Close() can drop them
	closing bool
}

// NewServer builds a server backed by the given store.
func NewServer(store *Store, verbose bool, logf func(string, ...any)) *Server {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Server{
		store:   store,
		verbose: verbose,
		logf:    logf,
		conns:   make(map[net.Conn]struct{}),
	}
}

// Listen binds the address but does not start serving yet. Returning the bound
// address lets tests pass "127.0.0.1:0" (kernel picks a free port) and then
// discover the real port via Addr().
//
// 🐍 net.Listen("tcp", addr) is socket() + bind() + listen() rolled into one,
// like Python's socketserver setting up a listening socket.
func (s *Server) Listen(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.ln = ln
	s.mu.Unlock()
	return nil
}

// Addr returns the actual listening address (host:port), or nil before Listen.
func (s *Server) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return nil
	}
	return s.ln.Addr()
}

// Serve runs the accept loop until the listener is closed. Each accepted
// connection is handled in its own goroutine — that is what lets many clients
// talk to the store concurrently (the Store's mutex keeps that safe).
func (s *Server) Serve() error {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			s.mu.Lock()
			closing := s.closing
			s.mu.Unlock()
			if closing {
				return nil // clean shutdown, not a real error
			}
			return err
		}

		s.mu.Lock()
		s.conns[conn] = struct{}{}
		s.mu.Unlock()

		// 🐍 `go f()` launches a goroutine — like starting a thread, but far
		// cheaper, so "one per connection" scales to thousands.
		go s.handle(conn)
	}
}

// handle serves a single connection then cleans it up.
func (s *Server) handle(conn net.Conn) {
	defer func() {
		conn.Close()
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
	}()

	if s.verbose {
		s.logf("connection opened: %s", conn.RemoteAddr())
	}
	if err := serveConn(conn, conn, s.store, s.verbose, s.logf); err != nil && err != io.EOF {
		if s.verbose {
			s.logf("connection error %s: %v", conn.RemoteAddr(), err)
		}
	}
	if s.verbose {
		s.logf("connection closed: %s", conn.RemoteAddr())
	}
}

// Close stops accepting and drops all live connections.
func (s *Server) Close() error {
	s.mu.Lock()
	s.closing = true
	ln := s.ln
	conns := make([]net.Conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()

	var firstErr error
	if ln != nil {
		if err := ln.Close(); err != nil {
			firstErr = err
		}
	}
	for _, c := range conns {
		c.Close()
	}
	return firstErr
}

// ListenAndServe is the production convenience: bind, then serve.
func (s *Server) ListenAndServe(addr string) error {
	if err := s.Listen(addr); err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	return s.Serve()
}
