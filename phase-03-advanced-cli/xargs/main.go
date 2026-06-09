// Command xargs reads items from standard input and uses them to build and run
// commands — a from-scratch, teaching-sized clone of the Unix `xargs`.
//
// The whole tool is three stages glued together by run():
//
//	stdin ──tokenize──▶ items ──buildJobs──▶ []job ──runJobs──▶ exit code
//	      (tokenize.go)        (build.go)            (run.go, parallel)
//
// Usage:
//
//	xargs [-0] [-t] [-n N] [-I R] [-P N] [command [initial-args...]]
//
// Flags:
//
//	-0     items are NUL-delimited (pairs with `find -print0`)
//	-t     echo each command to stderr before running it
//	-n N   put at most N items on each command line (batch size)
//	-I R   replace mode: run once per item, substituting R with the item
//	-P N   run up to N commands in parallel (bounded parallelism)
//
// If no command is given, the default is `echo`.
//
// Exit codes:
//
//	0    success
//	123  one or more invocations exited 1–125
//	126  a command was found but could not be executed
//	127  a command could not be found
//	2    usage error (bad flags)
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// options holds the parsed flag values.
type options struct {
	nulDelim bool
	echo     bool
	maxItems int
	parallel int
	replace  string
}

// run is split out from main so tests can drive it with custom args/streams and
// assert on the exit code without spawning the whole binary. It returns the
// process exit code.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	opts, command, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "xargs: %v\n", err)
		return 2
	}

	items, err := tokenize(stdin, opts.nulDelim)
	if err != nil {
		fmt.Fprintf(stderr, "xargs: %v\n", err)
		return 1
	}

	// Default command is echo, just like real xargs.
	if len(command) == 0 {
		command = []string{"echo"}
	}

	jobs := buildJobs(command, items, opts.replace, opts.maxItems)
	exec := execRunner(stdout, stderr)
	return runJobs(jobs, opts.parallel, opts.echo, stderr, exec)
}

// parseArgs is a small hand-rolled flag parser. We roll our own (instead of the
// stdlib `flag` package) for authentic xargs ergonomics: flags only appear
// BEFORE the command, the first non-flag token starts the command, and the rest
// of the line — including any dashes — belongs to that command, untouched.
//
// Option values may be glued (`-n2`) or separated (`-n 2`).
func parseArgs(args []string) (options, []string, error) {
	opts := options{parallel: 1} // default: one child at a time

	i := 0
	for ; i < len(args); i++ {
		a := args[i]

		// "-" alone, or any token not starting with '-', is the command.
		if a == "-" || len(a) == 0 || a[0] != '-' {
			break
		}
		if a == "--" { // explicit end-of-flags marker
			i++
			break
		}

		switch {
		case a == "-0":
			opts.nulDelim = true
		case a == "-t":
			opts.echo = true
		case a[:2] == "-n":
			v, err := flagValue(a, args, &i)
			if err != nil {
				return opts, nil, err
			}
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				return opts, nil, fmt.Errorf("-n requires a positive integer, got %q", v)
			}
			opts.maxItems = n
		case a[:2] == "-P":
			v, err := flagValue(a, args, &i)
			if err != nil {
				return opts, nil, err
			}
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				return opts, nil, fmt.Errorf("-P requires a positive integer, got %q", v)
			}
			opts.parallel = n
		case a[:2] == "-I":
			v, err := flagValue(a, args, &i)
			if err != nil {
				return opts, nil, err
			}
			opts.replace = v
		default:
			return opts, nil, fmt.Errorf("unknown flag %q", a)
		}
	}

	return opts, args[i:], nil
}

// flagValue returns the value for a value-taking flag, supporting both the
// glued form (`-n2`, a is longer than two chars) and the separated form
// (`-n 2`, value is the next argument). It advances *i past a consumed value.
func flagValue(a string, args []string, i *int) (string, error) {
	if len(a) > 2 {
		return a[2:], nil // glued: -n2, -IR, -P4
	}
	if *i+1 < len(args) {
		*i++
		return args[*i], nil // separated: -n 2
	}
	return "", fmt.Errorf("option %s requires a value", a)
}
