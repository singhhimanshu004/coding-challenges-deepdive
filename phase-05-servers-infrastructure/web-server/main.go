// Command web-server is a from-scratch HTTP/1.1 server built directly on top of
// raw TCP sockets (net.Listen / net.Conn). It does NOT use net/http for the
// core serving path: it runs its own accept loop, parses request bytes by hand,
// and frames response bytes by hand — including the status line, headers, and
// the CRLF framing that separates them. It is the server counterpart to the
// Phase 3 `curl` client, which speaks the same protocol from the other side.
//
// Usage:
//
//	web-server [--addr :8080] [--root ./public] [--verbose]
//
//	--addr      address to listen on (host:port; default ":8080")
//	--root      directory of static files to serve (default "./public")
//	--verbose   log every connection and request
//
// What it does:
//
//   - goroutine-per-connection concurrency via an accept loop
//   - parses the request line, headers, and Content-Length body
//   - routes method+path to handlers, falling back to static file serving
//   - serves static files with the correct Content-Type and 404 for misses
//   - rejects path-traversal attempts (e.g. /../../etc/passwd) with 403
//   - supports HTTP/1.1 persistent connections (keep-alive) with a read timeout
package main

import (
	"flag"
	"log"
	"os"
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on (host:port)")
	root := flag.String("root", "./public", "directory of static files to serve")
	verbose := flag.Bool("verbose", false, "log every connection and request")
	flag.Parse()

	srv := &server{
		addr:    *addr,
		router:  newRouter(*root),
		verbose: *verbose,
		logger:  log.New(os.Stderr, "web-server: ", log.LstdFlags),
	}

	// A demo dynamic route so the router (not just static files) is exercised.
	srv.router.handle("GET", "/hello", helloHandler)

	if err := srv.listenAndServe(); err != nil {
		srv.logger.Fatalf("%v", err)
	}
}
