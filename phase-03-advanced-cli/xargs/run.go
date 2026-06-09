package main

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// runner runs one argv and returns the child's exit code. Making this a
// function type (instead of hard-coding os/exec everywhere) is the key
// testability seam: production code injects execRunner, while tests inject a
// fake runner to exercise the worker pool WITHOUT spawning real processes.
//
// 🐍 Python analogy: passing a callable around, like handing a function object
// to another function. 🐹 In Go, functions are first-class values too, and a
// named func type documents the contract.
type runner func(argv []string) int

// execRunner is the real, process-spawning runner. It fork/execs argv[0] with
// argv[1:] as arguments and wires the child's stdout/stderr straight to ours.
//
// 🐹 Go idiom: exec.Command builds a *exec.Cmd; cmd.Run() does the
// fork + exec + wait. We type-assert the error to *exec.ExitError to read the
// child's real exit code (err != nil but the program *did* run and returned
// non-zero). A different error type means we never launched it (command not
// found / not executable) — xargs reports that as 127.
func execRunner(stdout, stderr io.Writer) runner {
	return func(argv []string) int {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		// xargs does NOT feed stdin to children (stdin is the item source),
		// so we leave cmd.Stdin nil → /dev/null for the child.
		err := cmd.Run()
		if err == nil {
			return 0
		}
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		// Could not start the process at all (not found / not executable).
		return 127
	}
}

// runJobs is the "parallel runner": it executes jobs with BOUNDED PARALLELISM,
// running at most `parallelism` children at any instant, and returns one
// aggregate exit code.
//
// The concurrency engine is the classic Go worker-pool / semaphore pattern:
//
//	sem := make(chan struct{}, parallelism)   // P permits
//	sem <- struct{}{}                          // acquire (blocks once P are busy)
//	... do work ...
//	<-sem                                      // release (frees a permit)
//
// A buffered channel of capacity P holds at most P tokens, so the (P+1)-th
// goroutine blocks on send until someone receives — that send/receive pair IS
// the throttle. A sync.WaitGroup lets us wait for every goroutine to finish.
//
// 🐍 Python analogy: `sem` is like `threading.BoundedSemaphore(P)` and the
// WaitGroup is like joining a list of threads — but goroutines are far cheaper
// than OS threads, so spawning one per job is normal and idiomatic in Go.
//
// Exit-status propagation follows xargs conventions, in priority order:
//
//	127 — a command could not be run (not found)
//	126 — a command was found but could not be executed
//	123 — one or more invocations exited with status 1–125
//	  0 — everything succeeded
func runJobs(jobs []job, parallelism int, echo bool, echoOut io.Writer, run runner) int {
	if parallelism < 1 {
		parallelism = 1
	}

	sem := make(chan struct{}, parallelism) // counting semaphore: P permits
	var wg sync.WaitGroup
	var mu sync.Mutex // guards `codes` and serialises -t echo lines
	var codes []int

	for _, j := range jobs {
		j := j // capture loop variable (pre-1.22 habit; harmless and clear)
		wg.Add(1)
		sem <- struct{}{} // ACQUIRE a permit — blocks while P are in flight
		go func() {
			defer wg.Done()
			defer func() { <-sem }() // RELEASE the permit when this job ends

			if echo {
				// -t: print the command to stderr before running it.
				// Lock so concurrent goroutines don't interleave the line.
				mu.Lock()
				fmt.Fprintln(echoOut, strings.Join(j.argv, " "))
				mu.Unlock()
			}

			code := run(j.argv)

			mu.Lock()
			codes = append(codes, code)
			mu.Unlock()
		}()
	}

	wg.Wait()
	return aggregateExit(codes)
}

// aggregateExit collapses every child's exit code into xargs' single exit code.
func aggregateExit(codes []int) int {
	final := 0
	for _, c := range codes {
		switch {
		case c == 127:
			return 127 // highest priority: short-circuit
		case c == 126:
			final = 126
		case c != 0 && final != 126:
			final = 123
		}
	}
	return final
}
