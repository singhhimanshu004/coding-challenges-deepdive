package main

import (
	"fmt"
	"net"
	"sort"
	"sync"
	"time"
)

// result is the outcome of probing a single port.
type result struct {
	port    int
	open    bool
	service string // well-known name, "" if unknown
}

// scanPort performs a single TCP connect probe against host:port.
//
// This is the heart of a "connect scan": we ask the OS to complete a full TCP
// three-way handshake (SYN → SYN/ACK → ACK) via net.DialTimeout. The result is
// interpreted simply:
//
//   - the dial SUCCEEDS  → something accepted the connection → the port is OPEN.
//     We immediately Close() it; we only cared whether the handshake completed.
//   - the dial FAILS     → the port is closed (we got a TCP RST) OR filtered
//     (a firewall dropped our packets and we hit the timeout). A connect scan
//     can't reliably tell "closed" from "filtered" apart, so we bucket both as
//     "not open".
//
// The timeout is essential: an unreachable/filtered port produces NO reply at
// all, so without a deadline the dial would block until the OS's (very long)
// default TCP timeout. The timeout is what keeps a scan of thousands of ports
// fast and bounded.
func scanPort(host string, port int, timeout time.Duration) result {
	address := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		// Closed, filtered, or timed out — all "not open" for a connect scan.
		return result{port: port, open: false}
	}
	// Open. We don't need the connection for anything; release it at once so we
	// don't exhaust file descriptors while scanning thousands of ports.
	_ = conn.Close()

	return result{port: port, open: true, service: serviceName(port)}
}

// scan probes every port in `ports` against `host` using a fixed-size pool of
// `workers` goroutines, each connection bounded by `timeout`. It returns the
// OPEN ports, sorted ascending.
//
// ── The worker-pool concurrency pattern (the lesson of this challenge) ────────
//
// Scanning is "embarrassingly parallel": each port probe is independent and
// spends almost all its time WAITING on the network, not using CPU. The naive
// answer is "one goroutine per port", but firing 65,535 simultaneous dials
// would exhaust file descriptors and swamp the network. We want bounded
// concurrency: at most N probes in flight at once. That is exactly a WORKER
// POOL, and Go expresses it with two primitives — channels and a WaitGroup:
//
//		          jobs channel (ports to scan)
//		            │   │   │   │   │
//		            ▼   ▼   ▼   ▼   ▼          a fixed number (N) of worker
//		         ┌────┐┌────┐┌────┐  ...      goroutines, each looping:
//		         │ w1 ││ w2 ││ w3 │           "take a port, probe it, send result"
//		         └─┬──┘└─┬──┘└─┬──┘
//		            │   │   │   │   │
//		            ▼   ▼   ▼   ▼   ▼
//		          results channel (open/closed)
//
//	  - `jobs` is a channel of ports. We push every port onto it, then close it.
//	    Closing signals "no more work"; a `for port := range jobs` loop in each
//	    worker drains the channel and then exits cleanly when it's closed.
//	  - Each worker sends its findings on `results`.
//	  - A sync.WaitGroup counts the live workers so we know when every worker has
//	    finished — at which point it's safe to close `results` and stop reading.
//
// 🐍 For a Python dev:
//   - A goroutine is a function scheduled by the Go runtime, far cheaper than an
//     OS thread (a few KB of stack). 100+ are routine. `go f()` ≈ scheduling a
//     coroutine, but you do NOT need async/await — blocking calls like
//     DialTimeout are fine because the runtime parks the goroutine and runs
//     another on the same OS thread. No GIL: goroutines run on multiple cores.
//   - A channel is a typed, thread-safe queue (like queue.Queue) that ALSO
//     does the synchronisation for you. `range`-ing a channel until it's closed
//     is the idiomatic "consume until the producer is done".
//   - This is the same shape as Python's concurrent.futures.ThreadPoolExecutor
//     with max_workers=N — bounded fan-out — but built from language primitives.
func scan(host string, ports []int, workers int, timeout time.Duration) []result {
	if workers < 1 {
		workers = 1
	}

	jobs := make(chan int, len(ports))
	results := make(chan result, len(ports))

	// Launch the fixed pool of workers. Each one loops over `jobs` until the
	// channel is closed and drained, then returns.
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for port := range jobs {
				results <- scanPort(host, port, timeout)
			}
		}()
	}

	// Feed every port to the workers, then close `jobs` so the `range` loops
	// above terminate once the queue is empty.
	for _, p := range ports {
		jobs <- p
	}
	close(jobs)

	// Close `results` exactly once all workers have returned. Doing this in its
	// own goroutine lets us start reading results below without deadlocking.
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect. The loop ends naturally when `results` is closed and drained.
	var open []result
	for r := range results {
		if r.open {
			open = append(open, r)
		}
	}

	sort.Slice(open, func(i, j int) bool { return open[i].port < open[j].port })
	return open
}
