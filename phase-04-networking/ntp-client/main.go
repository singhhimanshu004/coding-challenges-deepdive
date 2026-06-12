package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"
)

// ntpVersion is the protocol version we advertise in the request. 4 is the
// current version (RFC 5905); 3 is also widely accepted. Either works.
const ntpVersion uint8 = 4

// queryTimeout bounds the whole UDP round trip. UDP has no connection and no
// retransmission, so if a packet is lost we'd otherwise block forever — the
// deadline is what turns "lost packet" into a clean error.
const queryTimeout = 5 * time.Second

// Result bundles everything we learned from one NTP transaction.
type Result struct {
	ServerTime time.Time     // T3, the server's transmit timestamp, as a real time
	Offset     time.Duration // how far the local clock is from the server
	Delay      time.Duration // round-trip network delay
	Stratum    uint8         // distance from a reference clock (1 = atomic/GPS source)
}

// query performs one NTP request/response over UDP and computes the clock
// offset and round-trip delay.
//
// The flow is the classic four-timestamp dance:
//
//	t1 := now()         // T1 originate
//	send request
//	read response       // carries server's T2 (receive) and T3 (transmit)
//	t4 := now()         // T4 destination
func query(server string, port int, version uint8, timeout time.Duration) (*Result, error) {
	// net.JoinHostPort handles IPv6 bracketing for us; never build "host:port"
	// with string concatenation.
	addr := net.JoinHostPort(server, strconv.Itoa(port))

	// 🐍 net.Dial with "udp" gives us a connected UDP socket. There's no
	// handshake — "connected" here just means the OS remembers the peer so we
	// can use plain Write/Read instead of WriteTo/ReadFrom.
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("dialing %s: %w", addr, err)
	}
	defer conn.Close()

	// One deadline covers both the write and the read.
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, fmt.Errorf("setting deadline: %w", err)
	}

	req := buildRequest(version)

	// T1: record the local clock immediately before sending.
	t1 := time.Now()
	if _, err := conn.Write(req); err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}

	resp := make([]byte, packetSize)
	n, err := conn.Read(resp)
	// T4: record the local clock immediately after the reply arrives.
	t4 := time.Now()
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	p, err := parseResponse(resp[:n])
	if err != nil {
		return nil, err
	}

	// T2 and T3 come straight out of the server's reply.
	t2 := p.RecvTimestamp.toTime()
	t3 := p.TxTimestamp.toTime()

	offset, delay := clockMetrics(t1, t2, t3, t4)

	return &Result{
		ServerTime: t3,
		Offset:     offset,
		Delay:      delay,
		Stratum:    p.Stratum,
	}, nil
}

func main() {
	server := flag.String("server", "pool.ntp.org", "NTP server hostname")
	port := flag.Int("port", 123, "NTP server UDP port")
	flag.Parse()

	res, err := query(*server, *port, ntpVersion, queryTimeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ntp-client: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Server:      %s:%d\n", *server, *port)
	fmt.Printf("Stratum:     %d\n", res.Stratum)
	fmt.Printf("Server time: %s\n", res.ServerTime.Format(time.RFC3339Nano))
	fmt.Printf("Local time:  %s\n", time.Now().UTC().Format(time.RFC3339Nano))
	fmt.Printf("Offset:      %v\n", res.Offset)
	fmt.Printf("Delay:       %v\n", res.Delay)
}
