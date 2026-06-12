package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Proxy is the forward proxy. It holds only its dependencies (a logger and a
// dial timeout); all per-connection state lives on the goroutine stack. That
// statelessness is what lets us fan out to thousands of concurrent clients.
type Proxy struct {
	// Logf logs a line. main() injects either a real logger or a no-op.
	Logf func(format string, a ...any)
	// DialTimeout caps how long we wait when opening the origin connection so a
	// slow/dead upstream can't pin a goroutine forever.
	DialTimeout time.Duration
}

// Serve runs the accept loop: block in Accept(), and for every client that
// completes the TCP handshake, spin up a goroutine to handle it.
//
// 🐍 This is the `while True: conn, _ = sock.accept(); Thread(...).start()`
// pattern — except a goroutine costs a couple of KB, not an OS thread, so
// "one goroutine per connection" scales to tens of thousands of clients.
func (p *Proxy) Serve(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			// A closed listener (e.g. test teardown) lands here; treat it as a
			// clean stop rather than a crash.
			return nil
		}
		go p.handleConn(conn)
	}
}

// handleConn owns a single client connection from accept to close. It reads ONE
// request, decides plain-HTTP vs CONNECT, and dispatches. (A production proxy
// would loop for HTTP keep-alive; we close after each request — Connection:
// close — to keep the teaching version's framing dead simple.)
func (p *Proxy) handleConn(conn net.Conn) {
	defer conn.Close()

	// Wrap the raw socket in a buffered reader. http.ReadRequest needs a
	// *bufio.Reader, and — crucially for CONNECT — the SAME reader carries any
	// bytes that arrived in the same packet as the request line.
	br := bufio.NewReader(conn)

	// http.ReadRequest parses the request line + headers off the wire for us.
	// We lean on the stdlib HERE (parsing is fiddly and not the lesson), but the
	// CONNECT tunnel below is hand-rolled raw byte relay — see the README's
	// "hand-rolled vs library" note.
	//
	// 🐍 Like http.server's BaseHTTPRequestHandler parsing the request line and
	// headers, but returning a plain struct you inspect yourself.
	req, err := http.ReadRequest(br)
	if err != nil {
		if err != io.EOF {
			p.Logf("read request: %v", err)
		}
		return
	}

	if req.Method == http.MethodConnect {
		p.handleConnect(conn, br, req)
		return
	}
	p.handlePlainHTTP(conn, br, req)
}

// handlePlainHTTP proxies an unencrypted request. The client sent us the
// request in ABSOLUTE-FORM (the whole URL on the request line), which is the
// signal "I'm talking to a proxy, please fetch this for me":
//
//	GET http://example.com/path HTTP/1.1      ← absolute-form (to a proxy)
//	GET /path HTTP/1.1                         ← origin-form (to the server)
//	Host: example.com
//
// Our job: dial the origin, REWRITE the request to origin-form, strip the
// hop-by-hop headers that must not be forwarded, send it, and relay the reply.
func (p *Proxy) handlePlainHTTP(client net.Conn, br *bufio.Reader, req *http.Request) {
	// req.URL.Host is "example.com" or "example.com:8080". Default to port 80.
	target := req.URL.Host
	if target == "" {
		target = req.Host
	}
	target = ensurePort(target, "80")

	p.Logf("HTTP  %s %s", req.Method, req.URL)

	origin, err := net.DialTimeout("tcp", target, p.DialTimeout)
	if err != nil {
		// Speak HTTP back to the client so it sees a real error, not a dead socket.
		writeError(client, http.StatusBadGateway, "cannot reach origin: "+err.Error())
		return
	}
	defer origin.Close()

	// --- Request rewriting: absolute-form → origin-form ---------------------
	// The origin server is NOT a proxy; it expects just the path on the request
	// line. req.URL.RequestURI() gives us exactly "/path?query".
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s HTTP/1.1\r\n", req.Method, req.URL.RequestURI())

	// The Host header is mandatory in HTTP/1.1 and identifies the virtual host.
	fmt.Fprintf(&sb, "Host: %s\r\n", req.Host)

	// Forward the client's headers EXCEPT hop-by-hop ones (see isHopByHop). Those
	// describe the single client↔proxy hop and are meaningless — even harmful —
	// to the proxy↔origin hop.
	for name, values := range req.Header {
		if isHopByHop(name) {
			continue
		}
		for _, v := range values {
			fmt.Fprintf(&sb, "%s: %s\r\n", name, v)
		}
	}

	// We close the origin connection after one exchange, which also gives us a
	// dead-simple way to know the response ended: read until EOF.
	sb.WriteString("Connection: close\r\n\r\n")

	if _, err := io.WriteString(origin, sb.String()); err != nil {
		p.Logf("write request head: %v", err)
		return
	}
	// Stream the request body (POST/PUT data), if any, straight through.
	if req.Body != nil {
		io.Copy(origin, req.Body)
		req.Body.Close()
	}

	// --- Relay the response verbatim ---------------------------------------
	// We don't parse the response at all — the origin frames it correctly, and
	// because we asked for Connection: close, EOF marks the end. Just copy the
	// bytes back to the client. (br has no leftover bytes for plain HTTP.)
	if _, err := io.Copy(client, origin); err != nil {
		p.Logf("relay response: %v", err)
	}
}

// handleConnect implements HTTPS proxying via TUNNELLING. The client asks:
//
//	CONNECT example.com:443 HTTP/1.1
//
// meaning "open a raw pipe to this host:port and get out of my way." We dial the
// origin, reply "200 Connection Established", and from then on relay bytes
// blindly in BOTH directions.
//
// 🔐 THE KEY TEACHING POINT: after the 200, the client performs its TLS
// handshake *with the origin*, through us. The encryption keys are negotiated
// end-to-end; the proxy never has them. So everything we relay is opaque
// ciphertext — we literally cannot read or modify the HTTPS request or
// response. That opacity is the whole security guarantee of HTTPS-over-proxy.
func (p *Proxy) handleConnect(client net.Conn, br *bufio.Reader, req *http.Request) {
	// For CONNECT the authority is in req.Host (e.g. "example.com:443").
	target := ensurePort(req.Host, "443")
	p.Logf("CONNECT %s", target)

	origin, err := net.DialTimeout("tcp", target, p.DialTimeout)
	if err != nil {
		writeError(client, http.StatusBadGateway, "cannot reach origin: "+err.Error())
		return
	}
	defer origin.Close()

	// Tell the client the pipe is up. After these bytes, NOTHING we send is HTTP
	// any more — it's raw tunnel traffic (the client's TLS ClientHello comes next).
	if _, err := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		p.Logf("write CONNECT 200: %v", err)
		return
	}

	// Hand both sockets to the byte relay. We read the client side through `br`,
	// not `client`, because the buffered reader MIGHT already hold the first
	// tunnel bytes the client pipelined right after CONNECT.
	tunnel(client, br, origin)
}

// tunnel relays bytes between the client and the origin in both directions at
// once, until either side closes. This is the same bidirectional-copy idea as
// netcat's relay, generalised to two network sockets.
//
//	client ──► origin   (the client's encrypted request)
//	client ◄── origin   (the origin's encrypted response)
//
// 🐍 Two threads each doing shutil.copyfileobj(src, dst) — but goroutines are
// cheap and io.Copy streams in fixed-size chunks, so a tunnel uses almost no
// memory no matter how much data flows.
func tunnel(client net.Conn, clientSrc io.Reader, origin net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	// client → origin
	go func() {
		defer wg.Done()
		io.Copy(origin, clientSrc)
		// Signal EOF to the origin so its read side unblocks and it can finish.
		if cw, ok := origin.(closeWriter); ok {
			cw.CloseWrite()
		}
	}()

	// origin → client
	go func() {
		defer wg.Done()
		io.Copy(client, origin)
		if cw, ok := client.(closeWriter); ok {
			cw.CloseWrite()
		}
	}()

	// Block until BOTH directions have drained. Returning here triggers the
	// deferred Close() on both sockets in the callers.
	wg.Wait()
}

// closeWriter is satisfied by *net.TCPConn. Half-closing only our write side
// sends the peer a TCP FIN ("I'm done sending") while we can still read their
// remaining bytes — the clean way to tear down one direction of a tunnel.
//
// 🐍 socket.shutdown(socket.SHUT_WR).
type closeWriter interface {
	CloseWrite() error
}

// hopByHopHeaders are connection-scoped: they govern the single TCP hop they
// arrive on and must NOT be passed to the next hop (RFC 7230 §6.1). A proxy that
// blindly forwarded "Connection" or "Proxy-Connection" could wedge keep-alive
// state or leak its own presence.
var hopByHopHeaders = map[string]bool{
	"Connection":          true,
	"Proxy-Connection":    true, // non-standard but common; strip it
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailer":             true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}

// isHopByHop reports whether a header name is connection-scoped. http.Header
// stores names in canonical form (e.g. "Proxy-Connection"), so a map lookup is
// enough.
func isHopByHop(name string) bool {
	return hopByHopHeaders[http.CanonicalHeaderKey(name)]
}

// ensurePort appends ":defaultPort" if addr has no port. The proxy's request
// line/authority may omit the port for the well-known scheme default (80 for
// http, 443 for https), but net.Dial always needs host:port.
func ensurePort(addr, defaultPort string) string {
	if addr == "" {
		return addr
	}
	// SplitHostPort errors when there's no port — that's our cue to add one.
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return net.JoinHostPort(addr, defaultPort)
	}
	return addr
}

// writeError sends a minimal HTTP/1.1 error response to the client. Used when
// we fail before any origin bytes flow (e.g. the upstream is unreachable).
func writeError(w io.Writer, code int, msg string) {
	body := msg + "\n"
	fmt.Fprintf(w, "HTTP/1.1 %d %s\r\n", code, http.StatusText(code))
	fmt.Fprintf(w, "Content-Type: text/plain; charset=utf-8\r\n")
	fmt.Fprintf(w, "Content-Length: %d\r\n", len(body))
	fmt.Fprintf(w, "Connection: close\r\n\r\n")
	io.WriteString(w, body)
}
