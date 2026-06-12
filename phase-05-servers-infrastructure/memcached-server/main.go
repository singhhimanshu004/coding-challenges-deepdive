// Command memcached-server is a from-scratch implementation of the memcached
// TEXT protocol over raw TCP. It speaks the line-based command framing
// (set/get/gets/add/replace/append/prepend/delete/incr/decr/flush_all), keeps an
// in-memory store with per-item expiry, FLAGS and a CAS version token, is
// concurrency-safe (one goroutine per connection guarded by a mutex), and evicts
// the least-recently-used item when a configurable item cap is exceeded.
//
// Usage:
//
//	memcached-server [--addr :11211] [--max-items N] [--verbose]
//
//	--addr        host:port to listen on (default :11211)
//	--max-items   evict LRU items beyond this count; 0 = unlimited (default 0)
//	--verbose     log connections and each received command line to stderr
//
// Exit codes:
//
//	0  clean shutdown
//	1  runtime failure (bind/serve error)
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

// run is the testable entry point: it parses flags, builds the server, and
// serves until the listener is closed. Keeping it separate from main() (and
// taking the log sink as a parameter) is the same dependency-injection trick the
// curl challenge uses — it keeps the program unit-testable.
func run(args []string, logw *os.File) int {
	// 🐍 flag is Go's stdlib argument parser, like argparse. For a server with
	// long --flags it is the idiomatic choice (the Unix-filter challenges in this
	// repo hand-roll their parsers only to get exotic short-flag bundling).
	fs := flag.NewFlagSet("memcached-server", flag.ContinueOnError)
	fs.SetOutput(logw)
	addr := fs.String("addr", ":11211", "host:port to listen on")
	maxItems := fs.Int("max-items", 0, "LRU item cap (0 = unlimited)")
	verbose := fs.Bool("verbose", false, "log connections and commands to stderr")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	logger := log.New(logw, "", log.LstdFlags)
	logf := func(format string, a ...any) { logger.Printf(format, a...) }

	store := NewStore(*maxItems)
	srv := NewServer(store, *verbose, logf)

	if err := srv.Listen(*addr); err != nil {
		fmt.Fprintf(logw, "memcached-server: %v\n", err)
		return 1
	}
	logf("listening on %s (max-items=%d)", srv.Addr(), *maxItems)

	if err := srv.Serve(); err != nil {
		fmt.Fprintf(logw, "memcached-server: %v\n", err)
		return 1
	}
	return 0
}
