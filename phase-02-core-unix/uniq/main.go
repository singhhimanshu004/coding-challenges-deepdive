// Command uniq filters ADJACENT duplicate lines from a stream of text.
//
// Usage:
//
//	uniq [-c] [-d] [-u] [input [output]]
//
//	-c   prefix each line with the number of times it occurred (count)
//	-d   only print lines that were duplicated (a group of 2 or more)
//	-u   only print lines that were unique (a group of exactly 1)
//
// If no input file is given (or it is "-"), uniq reads from stdin.
// If no output file is given, uniq writes to stdout.
//
// THE KEY INSIGHT: uniq only compares lines that are *next to each other*.
// "a / a / b / a" collapses to "a / b / a" — the two a's separated by b are
// NOT merged, because they are not adjacent. That is why you almost always run
// `sort` first: sorting brings all equal lines together so uniq can see them
// as one group. See the README for the full story.
//
// Exit codes follow the repo convention:
//
//	0  success
//	2  usage / IO error (bad args, file not found)
package main

import (
	"fmt"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// options holds the three mode flags. In Go we group related flags into a
// small struct and pass it by value — cheap to copy, and it keeps the core
// streaming function's signature tidy (Python dev note: think of this like a
// little @dataclass of booleans).
type options struct {
	count   bool // -c : prefix each emitted line with its occurrence count
	onlyDup bool // -d : emit only groups seen 2+ times
	onlyUni bool // -u : emit only groups seen exactly once
}

// run is the real entry point. Keeping main() trivial and putting the logic in
// run(args) []int -> int makes the program testable: a test can call run with
// fake args and assert on the exit code, without spawning a process.
func run(args []string) int {
	var opt options
	var positional []string

	// Manual flag parsing. We avoid the stdlib `flag` package here on purpose:
	// uniq's real CLI allows the flags to appear in any order and lets a lone
	// "-" mean stdin, which is awkward for `flag`. Walking the args by hand is
	// only a few lines and mirrors how the system tool behaves.
	for _, a := range args {
		switch a {
		case "-c", "--count":
			opt.count = true
		case "-d", "--repeated":
			opt.onlyDup = true
		case "-u", "--unique":
			opt.onlyUni = true
		case "-h", "--help":
			usage()
			return 0
		default:
			if len(a) > 1 && a[0] == '-' && a != "-" {
				// An unknown -x flag. Report and bail with a usage error.
				fmt.Fprintf(os.Stderr, "uniq: unknown option %q\n", a)
				usage()
				return 2
			}
			// Not a flag: treat as a positional path ("-" included, it means stdin).
			positional = append(positional, a)
		}
	}

	// At most two positional args: input then output. More is a usage error.
	if len(positional) > 2 {
		fmt.Fprintln(os.Stderr, "uniq: too many file arguments")
		usage()
		return 2
	}

	// Resolve the input reader: a named file, or stdin when omitted / "-".
	in := os.Stdin
	if len(positional) >= 1 && positional[0] != "-" {
		f, err := os.Open(positional[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "uniq: %v\n", err)
			return 2
		}
		// defer schedules Close to run when run() returns — Go's answer to a
		// Python `with open(...)` block, but deferred to function exit.
		defer f.Close()
		in = f
	}

	// Resolve the output writer: a named file, or stdout when omitted.
	out := os.Stdout
	if len(positional) == 2 {
		f, err := os.Create(positional[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "uniq: %v\n", err)
			return 2
		}
		defer f.Close()
		out = f
	}

	if err := uniqStream(in, out, opt); err != nil {
		fmt.Fprintf(os.Stderr, "uniq: %v\n", err)
		return 2
	}
	return 0
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: uniq [-c] [-d] [-u] [input [output]]")
}
