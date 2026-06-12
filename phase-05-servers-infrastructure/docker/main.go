// Command gocker is a tiny, educational container runtime: a "Docker" you can
// read in an afternoon. It shows what a container ACTUALLY is — not a VM, but a
// normal Linux process that the kernel has been told to view the world
// differently (its own hostname, process tree, mounts, network, and resource
// limits).
//
// This file (main.go) is PLATFORM-NEUTRAL: it only does argument parsing and
// dispatch, so it compiles and its logic is unit-testable on macOS, Windows, or
// Linux. The actual isolation machinery lives in build-tagged files:
//
//	run_linux.go     //go:build linux   — the real namespaces/cgroups/pivot_root
//	run_other.go     //go:build !linux  — a stub that prints "Linux only"
//
// 🐍 For a Python dev: think of this as `if __name__ == "__main__": main()`.
// `os.Args[1:]` is Python's `sys.argv[1:]`. Go has no exceptions, so functions
// return an `error` value that we check explicitly instead of try/except.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

func main() {
	// We pass os.Stdout/os.Stderr in explicitly rather than letting deep code
	// reach for globals. That dependency-injection style is what makes run()
	// testable: a test can pass a bytes.Buffer and assert on the output.
	if err := dispatch(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "gocker:", err)
		os.Exit(1)
	}
}

// dispatch routes a CLI invocation to the right handler.
//
// There are two "modes" the same binary runs in. This is the famous container
// RE-EXEC TRICK and it is worth understanding before reading run_linux.go:
//
//  1. "run"   — what the USER types. It creates new namespaces and then
//     re-launches THIS binary as `/proc/self/exe child ...`.
//  2. "child" — the re-executed copy of ourselves, now living INSIDE the new
//     namespaces. It finishes setup (hostname, cgroups, pivot_root, /proc) and
//     finally exec's the user's command.
//
// Why two modes? Some setup (like setting the hostname or mounting a private
// /proc) MUST happen from inside the new namespaces, but Go can't fork() safely
// the way C can (the Go runtime is multi-threaded). Re-executing ourselves with
// the right clone flags is the idiomatic Go workaround.
func dispatch(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return errors.New("no command given")
	}

	switch args[0] {
	case "run":
		cfg, err := parseRunArgs(args[1:], stderr)
		if err != nil {
			return err
		}
		return runParent(cfg, stdout, stderr) // platform-specific

	case "child":
		cfg, err := parseChildArgs(args[1:], stderr)
		if err != nil {
			return err
		}
		return runChild(cfg, stdout, stderr) // platform-specific

	case "-h", "--help", "help":
		printUsage(stdout)
		return nil

	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `gocker — a minimal Linux container runtime (educational)

USAGE:
    gocker run [flags] <rootfs-dir> <command> [args...]

FLAGS:
    --mem <size>       memory limit, e.g. 100m, 512k, 1g  (0/empty = unlimited)
    --pids <n>         max number of processes             (0 = unlimited)
    --hostname <name>  hostname inside the container        (default "container")

EXAMPLES:
    sudo ./gocker run --mem 100m --pids 50 /path/to/rootfs /bin/sh
    sudo ./gocker run /path/to/rootfs /bin/echo hello

NOTE:
    This is LINUX-ONLY. Namespaces, cgroups and pivot_root are Linux kernel
    features. On macOS/Windows the binary still builds, but "run" will tell you
    to use a Linux machine. See README.md.
`)
}
