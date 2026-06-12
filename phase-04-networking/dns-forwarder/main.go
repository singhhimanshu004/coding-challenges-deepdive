// Command dns-forwarder is a caching, forwarding DNS server. It listens on a
// UDP socket, forwards client queries to an upstream recursive resolver, relays
// the answers back, and CACHES each answer for the duration of its TTL so repeat
// questions are served locally without touching the upstream.
//
// It builds directly on the sibling dns-resolver challenge (#23): that was a DNS
// CLIENT; this is the same wire format turned into a SERVER.
//
// Usage:
//
//	dns-forwarder [--listen :1053] [--upstream 8.8.8.8:53] [--verbose]
//
//	--listen ADDR     UDP address to listen on (default :1053). Use :53 for the
//	                  real DNS port, which requires elevated privileges (see the
//	                  README); :1053 avoids needing root for local experiments.
//	--upstream ADDR   resolver to forward to (default 8.8.8.8:53). A :port is
//	                  assumed to be 53 if omitted.
//	--verbose         log cache hits/misses and forwards to stderr.
//
// Exit codes:
//
//	0  clean shutdown
//	1  runtime failure (e.g. cannot bind the listen socket)
//	2  usage error (bad flags)
package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

// options is the parsed command line. 🐍 think of it as a small @dataclass.
type options struct {
	listen   string
	upstream string
	verbose  bool
}

// run is the testable entry point. It binds the socket and serves until the
// process is interrupted (Ctrl-C closes the socket and Serve returns).
func run(args []string, stderr *os.File) int {
	opt, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "dns-forwarder: %v\n", err)
		return 2
	}

	addr, err := net.ResolveUDPAddr("udp", opt.listen)
	if err != nil {
		fmt.Fprintf(stderr, "dns-forwarder: bad listen address %q: %v\n", opt.listen, err)
		return 2
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Fprintf(stderr, "dns-forwarder: cannot listen on %s: %v\n", opt.listen, err)
		return 1
	}
	defer conn.Close()

	logger := log.New(stderr, "", log.LstdFlags)
	logger.Printf("dns-forwarder listening on %s, forwarding to %s", conn.LocalAddr(), opt.upstream)

	srv := newServer(conn, opt.upstream, opt.verbose, logger)
	if err := srv.Serve(); err != nil {
		// A closed socket is the normal shutdown path; report anything else.
		if !strings.Contains(err.Error(), "use of closed network connection") {
			fmt.Fprintf(stderr, "dns-forwarder: %v\n", err)
			return 1
		}
	}
	return 0
}

// parseArgs is a small hand-rolled flag parser, consistent with the other tools
// in this repo.
func parseArgs(args []string) (options, error) {
	opt := options{listen: ":1053", upstream: "8.8.8.8:53"}

	i := 0
	next := func(flag string) (string, error) {
		i++
		if i >= len(args) {
			return "", fmt.Errorf("option %s requires a value", flag)
		}
		return args[i], nil
	}

	for ; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--listen", "-l":
			v, err := next(a)
			if err != nil {
				return opt, err
			}
			opt.listen = v
		case "--upstream", "-u":
			v, err := next(a)
			if err != nil {
				return opt, err
			}
			opt.upstream = v
		case "--verbose", "-v":
			opt.verbose = true
		default:
			return opt, fmt.Errorf("unknown option %q\nusage: dns-forwarder [--listen :1053] [--upstream 8.8.8.8:53] [--verbose]", a)
		}
	}

	// Default the upstream port to 53 if the user didn't give one.
	if !strings.Contains(opt.upstream, ":") {
		opt.upstream += ":53"
	}
	return opt, nil
}
