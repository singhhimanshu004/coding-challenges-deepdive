// Command http-forward-proxy is a from-scratch HTTP/HTTPS forward proxy — the
// Phase 4 capstone. It ties together everything the networking phase taught:
// raw TCP sockets (netcat), the HTTP/1.1 wire format (curl), TLS, and
// goroutine-per-connection concurrency.
//
// A forward proxy sits between a client (your browser, curl, an http.Client)
// and the wider internet. The client is *configured* to send its requests to
// the proxy, and the proxy fetches the resource on the client's behalf. This is
// what corporate egress proxies, content filters, and caching proxies all do.
//
// Two request styles arrive on the same listening port:
//
//	GET http://host/path HTTP/1.1      ← PLAIN HTTP: we parse, rewrite, forward,
//	                                     and relay the response (we can SEE it).
//	CONNECT host:443 HTTP/1.1          ← HTTPS: we open a raw byte TUNNEL and
//	                                     relay opaque bytes (we CANNOT see it).
//
// Usage:
//
//	http-forward-proxy [--listen :8080] [--verbose]
//
// Then point a client at it:
//
//	curl -x http://127.0.0.1:8080 http://example.com      # plain HTTP
//	curl -x http://127.0.0.1:8080 https://example.com     # CONNECT tunnel
//
// Exit codes:
//
//	0  clean shutdown
//	1  runtime failure (could not bind the listening socket)
//	2  usage error (bad flags)
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

// run is the real entry point. main() only wires os.Args / os.Stderr / os.Exit
// to it, so a test can call run() with its own args and an in-memory log sink.
//
// 🐍 Python devs: this is the classic `def main(argv): ... ` split. Keeping the
// process-y bits (os.Exit, os.Args) in main() leaves run() pure and testable.
func run(args []string, stderr io.Writer) int {
	// flag is Go's stdlib argument parser — the rough equivalent of argparse.
	// We give it our OWN FlagSet (instead of the global one) so tests can call
	// run() repeatedly without "flag redefined" panics.
	fs := flag.NewFlagSet("http-forward-proxy", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", ":8080", "address to listen on, e.g. :8080 or 127.0.0.1:8080")
	verbose := fs.Bool("verbose", false, "log every proxied request and tunnel")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	// net.Listen binds a TCP socket and starts the kernel listen queue. This is
	// the server side of the handshake — the moment the OS will accept inbound
	// connections on this port.
	//
	// 🐍 Equivalent to socket(); bind(); listen() rolled into one call.
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Fprintf(stderr, "http-forward-proxy: %v\n", err)
		return 1
	}
	defer ln.Close()

	// logf is a tiny logging closure. When --verbose is off it's a no-op, so the
	// hot path pays nothing. Passing it into the Proxy keeps logging decoupled
	// from the proxy logic and trivially mockable in tests.
	logger := log.New(stderr, "", log.LstdFlags)
	logf := func(format string, a ...any) {}
	if *verbose {
		logf = func(format string, a ...any) { logger.Printf(format, a...) }
	}

	logf("listening on %s", ln.Addr())
	p := &Proxy{Logf: logf, DialTimeout: 10 * time.Second}
	if err := p.Serve(ln); err != nil {
		fmt.Fprintf(stderr, "http-forward-proxy: %v\n", err)
		return 1
	}
	return 0
}
