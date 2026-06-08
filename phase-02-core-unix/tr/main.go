// Command tr is a pure stream filter that translates, deletes, or squeezes
// characters read from standard input and writes the result to standard
// output — a faithful subset of the Unix `tr`.
//
// Usage:
//
//	tr [-c] [-d] [-s] SET1 [SET2]
//
// Flags:
//
//	-c   complement SET1 (operate on every rune NOT in SET1)
//	-d   delete runes in SET1
//	-s   squeeze repeated runes in the relevant set into one
//
// Like the real tr, this program takes no file arguments: it is a filter that
// always reads stdin and writes stdout, so you compose it with pipes.
//
// Exit codes follow the repo convention:
//
//	0  success
//	1  domain failure (I/O error mid-stream)
//	2  usage error (bad flags / wrong number of SETs)
package main

import (
	"fmt"
	"os"
	"strings"

	"tr/internal/translate"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout))
}

// run is split out from main so tests can drive it with custom args/streams
// and assert on the exit code without spawning a process.
func run(args []string, stdin *os.File, stdout *os.File) int {
	spec := translate.Spec{}
	var sets []string

	// Tiny hand-rolled flag parser. We accept clustered short flags (-ds),
	// long-ish equivalents, and stop treating "-" specially once we hit a
	// non-flag token. tr's flags are all boolean, which keeps this simple.
	for _, a := range args {
		switch {
		case a == "-h" || a == "--help":
			usage(stdout)
			return 0
		case a == "--":
			// explicit end-of-flags marker; ignore.
		case len(a) > 1 && a[0] == '-' && !isNegativeSetLiteral(a):
			for _, f := range a[1:] {
				switch f {
				case 'c', 'C':
					spec.Complement = true
				case 'd':
					spec.Delete = true
				case 's':
					spec.Squeeze = true
				default:
					fmt.Fprintf(os.Stderr, "tr: unknown option -%c\n", f)
					usage(os.Stderr)
					return 2
				}
			}
		default:
			sets = append(sets, a)
		}
	}

	if len(sets) == 0 {
		fmt.Fprintln(os.Stderr, "tr: missing operand")
		usage(os.Stderr)
		return 2
	}
	if len(sets) > 2 {
		fmt.Fprintf(os.Stderr, "tr: extra operand %q\n", sets[2])
		return 2
	}
	spec.Set1 = sets[0]
	if len(sets) == 2 {
		spec.Set2 = sets[1]
	}

	t, err := translate.New(spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tr: %v\n", err)
		return 2
	}

	if err := t.Run(stdin, stdout); err != nil {
		fmt.Fprintf(os.Stderr, "tr: %v\n", err)
		return 1
	}
	return 0
}

// isNegativeSetLiteral guards against treating a lone "-" (sometimes used to
// mean stdin) as a flag bundle. A bare "-" is not a valid tr flag, so we let
// it fall through to the SET operands rather than erroring.
func isNegativeSetLiteral(a string) bool {
	return a == "-"
}

func usage(w *os.File) {
	fmt.Fprint(w, strings.TrimLeft(`
tr — translate, delete, or squeeze characters (stdin → stdout)

Usage:
  tr [-c] [-d] [-s] SET1 [SET2]

Flags:
  -c   complement SET1 (operate on runes NOT in SET1)
  -d   delete runes in SET1
  -s   squeeze repeats of the relevant set into a single rune

SETs support ranges (a-z), classes ([:alpha:] [:digit:] [:space:]
[:upper:] [:lower:]), and backslash escapes (\n \t \\).

Examples:
  echo hello | tr a-z A-Z          # HELLO
  echo 'a1b2' | tr -d [:digit:]    # ab
  echo aaabbb | tr -s a-z          # ab
  echo 'a1b2' | tr -cd [:digit:]   # 12
`, "\n"))
}
