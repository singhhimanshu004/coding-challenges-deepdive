package main

import (
	"flag"
	"log"
)

// nats-broker — a from-scratch NATS-style publish/subscribe message broker.
//
// Usage:
//
//	nats-broker [--addr :4222] [--verbose]
//
// It speaks the core NATS text protocol over TCP: CONNECT, PING/PONG, PUB, SUB,
// UNSUB in; INFO, MSG, +OK, -ERR, PONG out. See README.md for the full protocol
// walkthrough and the subject-routing lesson.
func main() {
	addr := flag.String("addr", ":4222", "TCP address to listen on (NATS default port is 4222)")
	verbose := flag.Bool("verbose", false, "log connection events and send +OK acknowledgements")
	flag.Parse()

	srv := NewServer(*addr, *verbose)
	if err := srv.Listen(); err != nil {
		log.Fatalf("nats-broker: %v", err)
	}
	log.Printf("nats-broker listening on %s", srv.Addr())
	srv.Serve()
}
