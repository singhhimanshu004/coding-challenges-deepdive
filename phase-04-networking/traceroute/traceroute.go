package main

// traceroute.go holds the path-discovery engine. It is split into two layers:
//
//  1. The TTL-iteration / hop-aggregation logic (runTrace + the `prober`
//     interface). This is pure control flow over an injectable dependency, so
//     tests can drive it with a fake prober — no sockets, no root, no network.
//  2. The real ICMP prober (icmpProber) that actually opens an unprivileged
//     datagram socket, sets the per-packet TTL, sends an echo request, and
//     waits for a reply.

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// probeResult is the outcome of a single probe (one packet sent at one TTL).
type probeResult struct {
	from    net.Addr      // responding router/host, or nil on timeout
	rtt     time.Duration // round-trip time, valid only when from != nil
	kind    replyKind     // what the reply meant
	timeout bool          // true if we gave up waiting
}

// prober is the seam between "decide what to send and aggregate answers" and
// "actually talk to the network". runTrace depends only on this interface, so a
// test can supply a scripted fake.
//
// 🐍➡️🐹 This is classic dependency injection. In Python you might pass a mock
// object; in Go you define a small interface and accept it as a parameter.
// Interfaces in Go are satisfied implicitly — a type that has a `probe` method
// with this signature *is* a prober, no "implements" keyword needed.
type prober interface {
	// probe sends one probe with the given IP TTL and sequence number and waits
	// up to its configured timeout for a reply.
	probe(ttl, seq int) probeResult
}

// hop is the aggregated result for one TTL value (one row of traceroute output).
type hop struct {
	ttl     int
	results []probeResult
}

// reachedDest reports whether any probe in this hop hit the destination.
func (h hop) reachedDest() bool {
	for _, r := range h.results {
		if r.kind.terminal() {
			return true
		}
	}
	return false
}

// runTrace is the core algorithm, free of any networking. It walks the TTL from
// 1 upward; for each TTL it fires `probes` probes through the injected prober,
// collects the answers, reports the hop via the callback, and stops as soon as a
// probe reaches the destination or maxHops is exhausted.
//
// The `report` callback lets the caller stream each finished hop (e.g. print it)
// without runTrace knowing anything about formatting or I/O.
func runTrace(p prober, maxHops, probes int, report func(hop)) []hop {
	var hops []hop
	seq := 0
	for ttl := 1; ttl <= maxHops; ttl++ {
		h := hop{ttl: ttl}
		for i := 0; i < probes; i++ {
			seq++
			h.results = append(h.results, p.probe(ttl, seq))
		}
		hops = append(hops, h)
		if report != nil {
			report(h)
		}
		if h.reachedDest() {
			break
		}
	}
	return hops
}

// formatHop renders one hop as a traceroute-style line, e.g.
//
//	" 3  router.example.net (203.0.113.1)  1.204ms  1.118ms  1.301ms"
//
// Probes that timed out show as "*". Consecutive probes that answered from the
// same address collapse to a single address label, matching system traceroute.
//
// resolve toggles reverse-DNS lookups (slow), so tests keep it off.
func formatHop(h hop, resolve bool) string {
	line := fmt.Sprintf("%2d ", h.ttl)
	var lastAddr string
	for _, r := range h.results {
		if r.timeout || r.from == nil {
			line += " *"
			continue
		}
		addr := addrIP(r.from)
		if addr != lastAddr {
			label := addr
			if resolve {
				if name := reverseDNS(addr); name != "" {
					label = fmt.Sprintf("%s (%s)", name, addr)
				}
			}
			line += " " + label
			lastAddr = addr
		}
		line += fmt.Sprintf("  %s", r.rtt.Round(time.Microsecond))
	}
	return line
}

// addrIP extracts the bare IP string from a net.Addr, dropping any port. On a
// "udp4" ICMP socket the peer comes back as a *net.UDPAddr whose String() is
// "1.2.3.4:0" — the :0 is meaningless for ICMP, so we strip it for display.
func addrIP(a net.Addr) string {
	switch v := a.(type) {
	case *net.UDPAddr:
		return v.IP.String()
	case *net.IPAddr:
		return v.IP.String()
	default:
		return a.String()
	}
}

// reverseDNS does a best-effort PTR lookup, returning "" on failure so callers
// can fall back to the bare IP. Errors are intentionally swallowed: a missing
// reverse record is normal, not a failure of the trace.
func reverseDNS(ip string) string {
	names, err := net.LookupAddr(ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	// Trim the trailing dot DNS returns on FQDNs.
	name := names[0]
	if len(name) > 0 && name[len(name)-1] == '.' {
		name = name[:len(name)-1]
	}
	return name
}

// icmpProber is the production prober. It owns one unprivileged ICMP datagram
// socket and the ipv4 control wrapper used to set the TTL per packet.
type icmpProber struct {
	conn    *icmp.PacketConn // the datagram socket ("udp4" => no root needed)
	pkt     *ipv4.PacketConn // lets us SetTTL on each outgoing packet
	dst     net.Addr         // resolved destination address
	id      int              // echo identifier for this run
	timeout time.Duration    // how long to wait for each reply
}

// newICMPProber resolves the host and opens the socket.
//
// 🔑 The privilege trick: icmp.ListenPacket("udp4", ...) asks the OS for a
// DATAGRAM ICMP socket (SOCK_DGRAM, IPPROTO_ICMP) rather than a RAW one. macOS
// and Linux both allow this WITHOUT root — the kernel writes/checks the ICMP id
// for us. A raw socket ("ip4:icmp") would need root / CAP_NET_RAW.
//
// 🐍 icmp.ListenPacket is the rough analogue of socket.socket(AF_INET,
// SOCK_DGRAM, IPPROTO_ICMP) followed by bind to 0.0.0.0.
func newICMPProber(host string, timeout time.Duration) (*icmpProber, error) {
	ipAddr, err := net.ResolveIPAddr("ip4", host)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", host, err)
	}

	conn, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		return nil, fmt.Errorf("open icmp socket (try running with network access; raw sockets need root): %w", err)
	}

	return &icmpProber{
		conn: conn,
		pkt:  conn.IPv4PacketConn(),
		// For a "udp4" ICMP socket the destination is given as a *net.UDPAddr;
		// the kernel maps it to the right ICMP datagram.
		dst:     &net.UDPAddr{IP: ipAddr.IP},
		id:      os.Getpid() & 0xffff,
		timeout: timeout,
	}, nil
}

func (p *icmpProber) Close() error { return p.conn.Close() }

// probe implements the prober interface: set TTL, send one echo request, wait
// for a reply, classify it, and time the round trip.
func (p *icmpProber) probe(ttl, seq int) probeResult {
	if err := p.pkt.SetTTL(ttl); err != nil {
		return probeResult{timeout: true}
	}

	payload := []byte("traceroute-probe")
	msg, err := buildEchoRequest(p.id, seq, payload)
	if err != nil {
		return probeResult{timeout: true}
	}

	start := time.Now()
	if _, err := p.conn.WriteTo(msg, p.dst); err != nil {
		return probeResult{timeout: true}
	}

	// A read deadline is how Go enforces a timeout on a blocking socket read.
	// 🐍 This is the equivalent of sock.settimeout(p.timeout) before recvfrom.
	_ = p.conn.SetReadDeadline(time.Now().Add(p.timeout))

	buf := make([]byte, 1500) // one Ethernet MTU is plenty for an ICMP reply
	for {
		n, peer, err := p.conn.ReadFrom(buf)
		if err != nil {
			// A timeout (deadline) shows up as a net.Error with Timeout()==true.
			var nerr net.Error
			if errors.As(err, &nerr) && nerr.Timeout() {
				return probeResult{timeout: true}
			}
			return probeResult{timeout: true}
		}

		kind, _, perr := parseICMPReply(buf[:n])
		if perr != nil || kind == replyOther {
			// Not a packet we care about (could be a reply to another probe in
			// flight). Keep reading until the deadline.
			continue
		}
		return probeResult{
			from: peer,
			rtt:  time.Since(start),
			kind: kind,
		}
	}
}

// Ensure the real prober satisfies the interface at compile time.
// 🐍➡️🐹 This blank-assignment idiom is a static "does X implement Y?" check —
// the compiler errors here (not deep in some call site) if the method set drifts.
var _ prober = (*icmpProber)(nil)

// trace wires a real ICMP prober to runTrace and streams hops to out. It is the
// thin glue between the network layer and the pure algorithm.
func trace(host string, maxHops, probes int, timeout time.Duration, resolve bool, out io.Writer) error {
	p, err := newICMPProber(host, timeout)
	if err != nil {
		return err
	}
	defer p.Close()

	fmt.Fprintf(out, "traceroute to %s, %d hops max, %d probes per hop\n", host, maxHops, probes)
	runTrace(p, maxHops, probes, func(h hop) {
		fmt.Fprintln(out, formatHop(h, resolve))
	})
	return nil
}
