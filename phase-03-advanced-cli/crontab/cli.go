package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// ----------------------------------------------------------------------------
// The command-line interface.
//
// We follow the same shape used across the repo's Go challenges: a thin main()
// (in main.go) that just calls run() and exits, while run() takes its output
// streams as parameters so tests can drive it with bytes.Buffers — no
// subprocess, no temp files.
//
// Exit-code convention (repo-wide):
//   0  success
//   2  usage / parse error
// ----------------------------------------------------------------------------

const usage = `crontab — parse, explain, and schedule cron expressions

USAGE:
  crontab [options] "<cron expression>"

OPTIONS:
  -n N        print the next N run times (default 5)
  -from TIME  reference time as RFC3339 (default: now)
  -explain    print a human-readable breakdown of the expression
  -run        sleep until the next scheduled minute, then print it and exit
  -h, --help  show this help

EXAMPLES:
  crontab "*/15 9-17 * * 1-5"
  crontab -n 3 -explain "0 0 * * 0"
  crontab @daily
  crontab -from 2026-06-09T00:00:00Z "0 0 13 * 5"
`

// options holds the parsed command-line configuration.
type options struct {
	n       int
	from    time.Time
	explain bool
	runMode bool
	help    bool
	expr    string
}

// run is the testable entry point. It parses args, then prints the explanation
// and/or the next N run times. It returns the process exit code.
func run(args []string, stdout, stderr io.Writer) int {
	opts, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n\n%s", err, usage)
		return 2
	}
	if opts.help {
		fmt.Fprint(stdout, usage)
		return 0
	}

	sched, err := Parse(opts.expr)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}

	if opts.explain {
		fmt.Fprint(stdout, sched.Explain())
		fmt.Fprintln(stdout)
	}

	if opts.runMode {
		next, ok := sched.Next(opts.from)
		if !ok {
			fmt.Fprintln(stderr, "error: this expression has no upcoming run time")
			return 2
		}
		fmt.Fprintf(stdout, "Sleeping until %s ...\n", next.Format(time.RFC1123))
		time.Sleep(time.Until(next))
		fmt.Fprintf(stdout, "Tick: %s\n", next.Format(time.RFC1123))
		return 0
	}

	runs := sched.NextN(opts.from, opts.n)
	if len(runs) == 0 {
		fmt.Fprintln(stderr, "error: this expression has no upcoming run time")
		return 2
	}
	fmt.Fprintf(stdout, "Next %d run(s) after %s:\n", len(runs), opts.from.Format(time.RFC1123))
	for _, t := range runs {
		fmt.Fprintf(stdout, "  %s\n", t.Format(time.RFC1123))
	}
	return 0
}

// parseArgs is a small hand-rolled flag parser (matching the repo's other Go
// tools) so a glued or separated form both work and the cron expression — which
// itself contains spaces and asterisks — is taken as a single quoted argument.
func parseArgs(args []string) (options, error) {
	opts := options{n: 5, from: time.Now()}
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			opts.help = true
			return opts, nil
		case arg == "-explain" || arg == "--explain":
			opts.explain = true
		case arg == "-run" || arg == "--run":
			opts.runMode = true
		case arg == "-n" || arg == "--n":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("-n requires a number")
			}
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 1 {
				return opts, fmt.Errorf("-n requires a positive integer, got %q", args[i])
			}
			opts.n = n
		case strings.HasPrefix(arg, "-n"):
			// glued form, e.g. -n3
			n, err := strconv.Atoi(arg[2:])
			if err != nil || n < 1 {
				return opts, fmt.Errorf("-n requires a positive integer, got %q", arg[2:])
			}
			opts.n = n
		case arg == "-from" || arg == "--from":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("-from requires an RFC3339 time")
			}
			t, err := time.Parse(time.RFC3339, args[i])
			if err != nil {
				return opts, fmt.Errorf("-from: %v", err)
			}
			opts.from = t
		default:
			positional = append(positional, arg)
		}
	}

	if len(positional) == 0 {
		return opts, fmt.Errorf("no cron expression provided")
	}
	// Join positionals so an UNQUOTED expression ("*/15 9-17 * * 1-5" passed as
	// five separate shell words) still works as a convenience.
	opts.expr = strings.Join(positional, " ")
	return opts, nil
}
