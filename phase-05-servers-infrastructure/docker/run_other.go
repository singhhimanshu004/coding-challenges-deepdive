//go:build !linux

package main

// run_other.go is compiled on EVERY non-Linux platform (macOS, Windows, BSD…).
// Its only job is to make `go build`, `go vet`, and `go test` succeed on those
// machines while making it crystal-clear at runtime that container isolation is
// a Linux kernel feature we cannot fake.
//
// 🐍 The `//go:build !linux` line at the very top is a BUILD TAG. Think of it as
// a compile-time `if platform != "linux"`. The Go toolchain includes this file
// only when NOT building for Linux. Its Linux twin (run_linux.go) carries
// `//go:build linux` and provides the real implementations of these same two
// functions. Exactly one of the two files is ever compiled, so the symbols
// `runParent` and `runChild` are always defined exactly once.

import (
	"fmt"
	"io"
	"runtime"
)

func runParent(cfg *Config, stdout, stderr io.Writer) error {
	return errLinuxOnly()
}

func runChild(cfg *Config, stdout, stderr io.Writer) error {
	// This can't normally be reached on non-Linux (nobody re-execs us in
	// child mode here), but we keep the symbol defined and behaving sanely.
	return errLinuxOnly()
}

func errLinuxOnly() error {
	return fmt.Errorf(
		"this container runtime only runs on Linux — namespaces, cgroups and "+
			"pivot_root are Linux-only kernel features; you are on %s/%s. "+
			"Build and run it inside a Linux VM, container, or host (see README.md)",
		runtime.GOOS, runtime.GOARCH,
	)
}
