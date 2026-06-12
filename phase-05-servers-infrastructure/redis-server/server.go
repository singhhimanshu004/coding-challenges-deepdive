package main

// server.go — the TCP layer: accept connections, parse a stream of RESP commands
// per connection, dispatch each, and write the reply.
//
// Concurrency model: goroutine-per-connection. Every accepted socket gets its own
// goroutine running handleConn; they all share one *Store, whose internal mutex
// makes concurrent access safe. A separate background goroutine runs active expiry.
//
// 🐍 For a Python dev: `go s.handleConn(conn)` is like spawning a thread per client,
// except goroutines are far cheaper (thousands are routine). There is no GIL, so
// the shared store really is touched in parallel — hence the lock inside Store.

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

// Server owns the listener, the shared store, and configuration.
type Server struct {
	addr    string // requested listen address, e.g. ":6379"
	rdbPath string // snapshot file path; "" disables persistence

	store *Store

	ln     net.Listener
	logger *log.Logger

	// sweepInterval controls how often active expiry runs. Configurable so tests
	// can speed it up.
	sweepInterval time.Duration

	quit      chan struct{} // closed by Close to stop the accept loop & sweeper
	closeOnce sync.Once     // makes Close idempotent
	wg        sync.WaitGroup
}

// NewServer constructs a server. Call Listen then Serve to run it.
func NewServer(addr, rdbPath string, logger *log.Logger) *Server {
	return &Server{
		addr:          addr,
		rdbPath:       rdbPath,
		store:         NewStore(),
		logger:        logger,
		sweepInterval: time.Second,
		quit:          make(chan struct{}),
	}
}

// Listen binds the TCP socket and, if a snapshot path is configured, loads it.
// It is split from Serve so tests can bind to 127.0.0.1:0 and then read back the
// OS-assigned port via Addr() before any client connects.
func (s *Server) Listen() error {
	if s.rdbPath != "" {
		entries, err := Load(s.rdbPath)
		if err != nil {
			return fmt.Errorf("loading snapshot: %w", err)
		}
		s.store.load(entries)
		s.logger.Printf("loaded %d keys from %s", len(entries), s.rdbPath)
	}

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.ln = ln
	return nil
}

// Addr returns the actual bound address (useful when the port was 0/auto).
func (s *Server) Addr() string {
	if s.ln == nil {
		return s.addr
	}
	return s.ln.Addr().String()
}

// Serve runs the accept loop and the background expiry sweeper until Close. It
// blocks, so callers typically run it in its own goroutine (main does exactly
// that and waits on a signal).
func (s *Server) Serve() {
	s.wg.Add(1)
	go s.activeExpiryLoop()

	for {
		conn, err := s.ln.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return // Close() was called; a closed listener errors here.
			default:
				s.logger.Printf("accept error: %v", err)
				continue
			}
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(conn)
		}()
	}
}

// activeExpiryLoop is the background sweeper: on every tick it asks the store to
// drop already-expired keys. This complements lazy expiry so that keys nobody
// ever reads again don't pin memory forever.
func (s *Server) activeExpiryLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.quit:
			return
		case <-ticker.C:
			if n := s.store.SweepExpired(); n > 0 {
				s.logger.Printf("active expiry reclaimed %d keys", n)
			}
		}
	}
}

// handleConn serves one client connection: it reads RESP requests in a loop until
// the client disconnects or sends something malformed.
func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	// One buffered reader per connection. The RESP decoder pulls bytes from here;
	// a single client request may span multiple TCP segments, and bufio hides that.
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	for {
		request, err := DecodeValue(reader)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return // client closed the connection — normal.
			}
			// Protocol error: report it and close, mirroring Redis behaviour.
			writer.Write(ErrorVal("ERR Protocol error: " + err.Error()).Marshal())
			writer.Flush()
			return
		}

		reply := dispatch(s, request)
		if _, err := writer.Write(reply.Marshal()); err != nil {
			return
		}
		// Flush after each reply so the client isn't left waiting. (A throughput-
		// oriented server would flush only when the read buffer drains.)
		if err := writer.Flush(); err != nil {
			return
		}
	}
}

// Close stops accepting connections, stops the sweeper, and waits for in-flight
// goroutines to finish. Safe to call multiple times (sync.Once guards the teardown).
func (s *Server) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.quit)
		if s.ln != nil {
			err = s.ln.Close()
		}
		s.wg.Wait()
	})
	return err
}
