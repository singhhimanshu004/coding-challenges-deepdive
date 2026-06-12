package main

import (
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

// halfCloser is satisfied by *net.TCPConn. A TCP connection has TWO independent
// directions, and you can shut down just the one you're writing to — a
// "half-close." The peer then sees EOF on its read side while WE can still read
// their reply. This is exactly how `echo hi | nc host port` works: stdin hits
// EOF, we half-close, the server notices, replies, and we print the reply.
//
// 🐍 Python analogy: socket.shutdown(socket.SHUT_WR).
type halfCloser interface {
	CloseWrite() error
}

// relay is the heart of netcat. It copies bytes in BOTH directions at once:
//
//	stdin  ──────────►  conn   (what you type goes to the network)
//	stdout ◄──────────  conn   (what the network sends is printed)
//
// Each direction is a blocking io.Copy loop, so we run one of them in a
// goroutine and the other in the foreground. The relay ends when the FOREGROUND
// copy (network → stdout) returns, which happens when the peer closes the
// connection (TCP EOF) or a -w read deadline fires (the usual way a
// connectionless UDP relay stops).
//
// 🐍 Python devs: this is two threads each running
//
//	shutil.copyfileobj(src, dst)
//
// except goroutines are far cheaper than OS threads, and io.Copy is the
// streaming "read a chunk, write a chunk, repeat until EOF" loop. We never load
// the whole stream into memory — perfect for piping gigabytes or a live shell.
func relay(conn io.ReadWriter, stdin io.Reader, stdout io.Writer) error {
	// Direction 1: local input → network, in the background.
	go func() {
		// io.Copy blocks until stdin reports EOF (closed pipe / Ctrl-D) or errors.
		io.Copy(conn, stdin)

		// Local input is exhausted. Tell the peer we're done sending.
		if hc, ok := conn.(halfCloser); ok {
			// TCP: half-close. The socket stays readable so we still get the
			// peer's reply; only OUR write direction is shut.
			hc.CloseWrite()
		}
		// UDP has no half-close (it's connectionless — there is no "connection"
		// to half-shut). For UDP, termination is driven instead by the peer
		// closing (rare) or, more usefully, by the -w read deadline below.
	}()

	// Direction 2: network → local output, in the foreground.
	_, err := io.Copy(stdout, conn)

	// A read deadline expiring is the normal, expected way a UDP relay ends, so
	// don't treat it as a failure.
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return nil
	}
	return err
}

// dialAndRelay implements CONNECT mode (`nc host port`). It opens the socket as
// a client, then hands it to relay.
//
// 🐍 net.Dial("tcp", "host:port") is socket.create_connection(("host", port));
// net.Dial("udp", ...) gives a "connected" UDP socket whose Read/Write are
// already bound to that peer.
func dialAndRelay(network, addr string, timeout time.Duration, stdin io.Reader, stdout io.Writer) error {
	var conn net.Conn
	var err error
	if timeout > 0 {
		conn, err = net.DialTimeout(network, addr, timeout)
	} else {
		conn, err = net.Dial(network, addr)
	}
	if err != nil {
		return err
	}
	defer conn.Close()

	applyTimeout(conn, timeout)
	return relay(conn, stdin, stdout)
}

// listenAndRelay implements LISTEN mode (`nc -l port`). TCP and UDP take
// genuinely different paths, which is the whole connection-oriented vs.
// connectionless lesson made concrete.
func listenAndRelay(network, addr string, timeout time.Duration, stdin io.Reader, stdout io.Writer) error {
	if network == "udp" {
		// UDP: there is nothing to "accept." We just bind a socket and read
		// whatever datagrams arrive, remembering the sender so we can reply.
		uaddr, err := net.ResolveUDPAddr(network, addr)
		if err != nil {
			return err
		}
		pc, err := net.ListenUDP(network, uaddr)
		if err != nil {
			return err
		}
		defer pc.Close()
		return servePacket(pc, timeout, stdin, stdout)
	}

	// TCP: bind, then block in Accept() for the three-way handshake to complete.
	ln, err := net.Listen(network, addr)
	if err != nil {
		return err
	}
	defer ln.Close()
	return serveTCP(ln, timeout, stdin, stdout)
}

// serveTCP accepts exactly one connection from an already-bound listener and
// relays it. Splitting this out (instead of inlining net.Listen) gives tests a
// seam: a test creates its own listener on 127.0.0.1:0, learns the port, then
// calls serveTCP directly — no real network, no fixed ports.
func serveTCP(ln net.Listener, timeout time.Duration, stdin io.Reader, stdout io.Writer) error {
	conn, err := ln.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()

	applyTimeout(conn, timeout)
	return relay(conn, stdin, stdout)
}

// servePacket relays a bound UDP socket. Because UDP is connectionless, we wrap
// the socket so it learns its peer from the first datagram and can reply.
func servePacket(pc *net.UDPConn, timeout time.Duration, stdin io.Reader, stdout io.Writer) error {
	if timeout > 0 {
		pc.SetReadDeadline(time.Now().Add(timeout))
	}
	return relay(&udpListenConn{pc: pc}, stdin, stdout)
}

// applyTimeout arms a read deadline so an idle relay eventually returns instead
// of blocking forever. For UDP this is essentially mandatory: a UDP read never
// produces EOF, so without -w the network→stdout copy would never finish.
func applyTimeout(conn net.Conn, timeout time.Duration) {
	if timeout > 0 {
		conn.SetReadDeadline(time.Now().Add(timeout))
	}
}

// udpListenConn adapts an unconnected UDP listener (*net.UDPConn) to the plain
// io.ReadWriter that relay expects.
//
// The trick: a UDP listener doesn't have a single peer. We discover the peer
// from the first datagram (ReadFromUDP returns the sender's address), remember
// it, and send our replies back there with WriteToUDP. This is the
// connectionless flip side of TCP's accept-then-Read/Write model.
type udpListenConn struct {
	pc   *net.UDPConn
	mu   sync.Mutex
	peer *net.UDPAddr
}

func (c *udpListenConn) Read(p []byte) (int, error) {
	n, addr, err := c.pc.ReadFromUDP(p)
	if addr != nil {
		c.mu.Lock()
		c.peer = addr
		c.mu.Unlock()
	}
	return n, err
}

func (c *udpListenConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	peer := c.peer
	c.mu.Unlock()
	if peer == nil {
		// We were asked to send before any datagram arrived, so we don't yet
		// know where to send. In practice a UDP listener replies to whoever
		// spoke first, so this only happens if local input precedes any packet.
		return 0, errNoPeer
	}
	return c.pc.WriteToUDP(p, peer)
}

var errNoPeer = errors.New("cannot send on UDP listener before a datagram is received")
