package main

// main.go — the command-line entry point.
//
// Usage:
//
//	redis-server [--addr :6379] [--rdb dump.rdb]
//
// On start it (optionally) loads a snapshot; on Ctrl-C / SIGTERM it (optionally)
// saves one back, giving us load-on-start + save-on-shutdown for free in addition
// to the explicit SAVE/BGSAVE commands.
//
// 🐍 For a Python dev: Go's stdlib `flag` package is `argparse`-lite. `main` stays
// thin and delegates to `run`, which is easy to reason about. Signal handling uses
// a channel — Go's concurrency primitive — instead of a callback.

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

// run wires up the server from CLI args and blocks until a shutdown signal. It is
// separated from main (which only calls os.Exit) so it could be driven from a test
// if desired; the integration tests use NewServer directly for finer control.
func run(args []string, logOut *os.File) int {
	fs := flag.NewFlagSet("redis-server", flag.ContinueOnError)
	addr := fs.String("addr", ":6379", "TCP address to listen on (host:port)")
	rdb := fs.String("rdb", "", "snapshot file path; enables load-on-start and save-on-shutdown")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	logger := log.New(logOut, "redis-server ", log.LstdFlags)
	srv := NewServer(*addr, *rdb, logger)

	if err := srv.Listen(); err != nil {
		fmt.Fprintf(logOut, "fatal: %v\n", err)
		return 1
	}
	logger.Printf("listening on %s", srv.Addr())

	// Run the accept loop in the background so main can wait for a signal.
	go srv.Serve()

	// Block until SIGINT/SIGTERM arrives.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	logger.Printf("shutting down")

	if *rdb != "" {
		if err := Save(srv.store, *rdb); err != nil {
			logger.Printf("save on shutdown failed: %v", err)
		} else {
			logger.Printf("saved snapshot to %s", *rdb)
		}
	}
	srv.Close()
	return 0
}
