package main

// THE SERVER. dns-resolver was a CLIENT (it asked questions). dns-forwarder is a
// SERVER that also asks questions: clients send it queries, and for each one it
// either answers from cache or FORWARDS the query to an upstream resolver and
// relays the reply back.
//
//	          query                    cache miss → forward
//	client ──────────► dns-forwarder ──────────────────────► upstream (8.8.8.8:53)
//	       ◄──────────              ◄──────────────────────
//	          reply                    relay reply + cache it
//
// 🐍➡️🐹 net.UDPConn is the same object on both sides of the story, because UDP
// has no "connection". The server obtains its socket with net.ListenUDP and
// then uses ReadFromUDP / WriteToUDP (which carry the peer's address, since
// there's no fixed peer). Python: sock.recvfrom() / sock.sendto().

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"time"
)

// upstreamTimeout bounds how long we wait for the upstream's reply before
// giving up on a single forwarded query.
const upstreamTimeout = 5 * time.Second

// maxUDPMessage is a generous receive buffer. Classic DNS-over-UDP caps at 512
// bytes; EDNS0 allows more. 65535 is the max possible UDP payload, so one
// ReadFromUDP always returns a whole datagram with room to spare.
const maxUDPMessage = 65535

// Server is a forwarding DNS resolver listening on a UDP socket.
type Server struct {
	conn     *net.UDPConn // the listening socket (also used to reply)
	upstream string       // "ip:port" of the resolver we forward to
	verbose  bool         // log cache hits/misses and forwards
	cache    *cache
	logger   *log.Logger

	// dialUpstream opens a fresh UDP socket to the upstream. It's a field so
	// tests can leave it as the default; production never overrides it.
	dialUpstream func(upstream string) (*net.UDPConn, error)
}

// newServer wires up a Server around an already-bound listening socket.
func newServer(conn *net.UDPConn, upstream string, verbose bool, logger *log.Logger) *Server {
	return &Server{
		conn:         conn,
		upstream:     upstream,
		verbose:      verbose,
		cache:        newCache(),
		logger:       logger,
		dialUpstream: defaultDialUpstream,
	}
}

// defaultDialUpstream resolves "ip:port" and opens a connected UDP socket.
func defaultDialUpstream(upstream string) (*net.UDPConn, error) {
	raddr, err := net.ResolveUDPAddr("udp", upstream)
	if err != nil {
		return nil, fmt.Errorf("bad upstream address %q: %w", upstream, err)
	}
	// nil local address → the OS picks an ephemeral source port.
	return net.DialUDP("udp", nil, raddr)
}

// Serve runs the accept loop until the socket is closed. It is the heart of any
// UDP server: read a datagram, hand it to a worker, repeat.
//
// THE GOROUTINE-PER-REQUEST IDIOM. Each datagram is processed on its own
// goroutine so a slow upstream for one client never stalls everyone else.
//
//	🐍 A goroutine is a function scheduled by the Go runtime onto a small pool of
//	OS threads — like asyncio tasks, but you don't await them and there's no event
//	loop in your code. `go s.handle(...)` is "fire and forget."
//
// THE COPY THAT EVERYONE FORGETS: `buf` is reused on the next ReadFromUDP, so we
// must copy the bytes before handing them to a goroutine — otherwise the next
// datagram would overwrite the slice the worker is still reading.
func (s *Server) Serve() error {
	buf := make([]byte, maxUDPMessage)
	for {
		n, client, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			// A closed socket surfaces here as a read error: clean shutdown.
			return err
		}
		packet := make([]byte, n)
		copy(packet, buf[:n])
		go s.handle(packet, client)
	}
}

// handle processes one client query end-to-end: parse the question, try the
// cache, forward on a miss, cache the reply, and relay it to the client.
func (s *Server) handle(query []byte, client *net.UDPAddr) {
	msg, err := parseMessage(query)
	if err != nil || len(msg.Questions) == 0 {
		// Not something we understand well enough to cache. Forward it blindly
		// so odd-but-valid traffic still works, but don't try to key a cache.
		if resp, ferr := s.forward(query); ferr == nil {
			s.reply(resp, client)
		} else {
			s.logf("drop unparseable query from %s: %v", client, err)
		}
		return
	}

	q := msg.Questions[0]
	key := cacheKey{name: q.Name, qtype: q.Type, qclass: q.Class}
	label := fmt.Sprintf("%s %s", q.Name, typeName(q.Type))

	// ── Cache HIT ──────────────────────────────────────────────────────────
	if cached, ok := s.cache.get(key); ok {
		s.logf("cache HIT  %-30s served from cache", label)
		// The cached bytes carry the transaction ID of the ORIGINAL query that
		// first populated the cache. This new client used a different ID, so we
		// overwrite the first two header bytes with THIS query's ID before
		// replying — otherwise the client rejects the answer as unsolicited.
		resp := make([]byte, len(cached))
		copy(resp, cached)
		binary.BigEndian.PutUint16(resp[0:], msg.Header.ID)
		s.reply(resp, client)
		return
	}

	// ── Cache MISS → forward upstream ────────────────────────────────────────
	s.logf("cache MISS %-30s forwarding to %s", label, s.upstream)
	resp, err := s.forward(query)
	if err != nil {
		s.logf("upstream error for %s: %v", label, err)
		return
	}

	// Cache the reply for the minimum answer TTL (the most conservative choice:
	// the whole answer set is only valid as long as its shortest-lived record).
	if parsed, perr := parseMessage(resp); perr == nil {
		if ttl, cacheable := parsed.minTTL(); cacheable {
			s.cache.set(key, resp, ttl)
			s.logf("cached     %-30s ttl=%ds", label, ttl)
		}
	}

	s.reply(resp, client)
}

// forward sends the raw query to the upstream resolver over a fresh UDP socket
// and returns the raw reply bytes. We relay bytes VERBATIM — we don't rebuild
// the message — so every record type works even though we only parse a subset.
func (s *Server) forward(query []byte) ([]byte, error) {
	conn, err := s.dialUpstream(s.upstream)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(upstreamTimeout))

	if _, err := conn.Write(query); err != nil {
		return nil, fmt.Errorf("send to upstream: %w", err)
	}

	buf := make([]byte, maxUDPMessage)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("read from upstream: %w", err)
	}
	return buf[:n], nil
}

// reply sends resp back to the client on the listening socket.
func (s *Server) reply(resp []byte, client *net.UDPAddr) {
	if _, err := s.conn.WriteToUDP(resp, client); err != nil {
		s.logf("reply to %s failed: %v", client, err)
	}
}

// logf logs only when --verbose is on. Routing through one helper keeps the hot
// path quiet by default.
func (s *Server) logf(format string, args ...any) {
	if s.verbose && s.logger != nil {
		s.logger.Printf(format, args...)
	}
}

// discardLogger is a no-op logger used when verbose output is disabled.
func discardLogger() *log.Logger { return log.New(io.Discard, "", 0) }
