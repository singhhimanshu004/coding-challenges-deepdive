package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"time"
)

// dialTimeout bounds how long we wait to establish the TCP (and TLS) connection.
const dialTimeout = 30 * time.Second

// dial opens a byte pipe to the server described by t and hands it back as a
// net.Conn.
//
// THE BIG IDEA OF THIS WHOLE CHALLENGE LIVES HERE:
//
//	A net.Conn is just a bidirectional stream of bytes — Read() pulls bytes the
//	server sent, Write() pushes bytes to the server. HTTP is NOTHING more than
//	an agreement about what text to write into that stream and how to interpret
//	the text that comes back. There is no magic "http" object; we are going to
//	type the protocol out by hand.
//
// 🐍 Python analogy: this is socket.create_connection((host, port)) followed by
// ssl.wrap_socket() for https. net.Conn is the duck-typed "thing with read and
// write" — Go's io.Reader + io.Writer interfaces, checked at compile time.
func dial(t target) (net.Conn, error) {
	// Step 1: the TCP three-way handshake. net.Dial("tcp", host:port) performs
	// SYN / SYN-ACK / ACK and returns once the socket is open. After this we
	// have a raw, UNENCRYPTED byte pipe.
	tcp, err := net.DialTimeout("tcp", t.hostport(), dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("could not connect to %s: %w", t.hostport(), err)
	}

	// Plain HTTP: the TCP socket itself is our HTTP transport. Done.
	if t.scheme == "http" {
		return tcp, nil
	}

	// Step 2 (https only): the TLS handshake, layered ON TOP of the TCP socket.
	//
	// tls.Client wraps the existing connection. When we make the first Read or
	// Write (or call Handshake explicitly, as we do below) the library:
	//   1. sends a ClientHello advertising supported cipher suites/versions,
	//   2. receives the server's certificate chain,
	//   3. VERIFIES that chain against the OS trust store AND checks the
	//      certificate's names cover ServerName (this is why ServerName must be
	//      the real hostname, not host:port),
	//   4. performs a key exchange so both sides derive the same symmetric keys.
	//
	// From then on every Write is transparently encrypted and every Read is
	// transparently decrypted — so the HTTP code above this layer is byte-for-
	// byte identical whether the scheme is http or https. That clean seam is
	// the payoff of putting TLS in its own wrapper.
	tlsConn := tls.Client(tcp, &tls.Config{
		ServerName: t.host, // MUST be the bare hostname for SNI + cert validation
		MinVersion: tls.VersionTLS12,
	})

	// Drive the handshake now so a TLS failure surfaces here with a clear
	// message rather than later, buried inside the response read.
	if err := tlsConn.Handshake(); err != nil {
		tcp.Close()
		return nil, fmt.Errorf("TLS handshake with %s failed: %w", t.host, err)
	}

	return tlsConn, nil
}
