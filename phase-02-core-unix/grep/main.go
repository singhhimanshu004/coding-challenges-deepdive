// Command grep searches its input line by line for a regular-expression pattern
// and prints the lines that match. It is Phase 2 / Challenge 12 of the
// coding-challenges curriculum, a reimplementation of the classic Unix `grep`.
//
// Usage:
//
//	grep [flags] PATTERN [file...]
//
// Flags:
//
//	-i        ignore case
//	-v        invert: print lines that DON'T match
//	-n        prefix each line with its 1-based line number
//	-c        print only a count of matching lines per file
//	-w        match only whole words
//	-r        recurse into directories
//	-l        print only the names of files that contain a match
//	-A N      print N lines of context After each match
//	-B N      print N lines of context Before each match
//	-C N      print N lines of context around each match (= -A N -B N)
//
// With no file operand (or "-") grep reads standard input.
//
// Exit codes follow the grep convention (and the repo convention):
//
//	0  at least one line matched
//	1  no lines matched
//	2  an error occurred (bad flags, bad pattern, unreadable file)
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func main() {
	os.Exit(cli(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// options is the raw, pre-validation result of parsing argv.
type options struct {
	ignoreCase bool
	invert     bool
	lineNum    bool
	count      bool
	word       bool
	recursive  bool
	listFiles  bool
	after      int
	before     int
	pattern    string
	files      []string
}

// cli is the testable entry point. Like the other tools in this repo it takes
// argv (without the program name) and explicit streams so tests can drive it
// with in-memory readers/writers and assert on the exit code.
func cli(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	opt, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "grep: %v\n", err)
		usage(stderr)
		return 2
	}

	matcher, err := NewMatcher(opt.pattern, opt.ignoreCase, opt.invert, opt.word)
	if err != nil {
		fmt.Fprintf(stderr, "grep: %v\n", err)
		return 2
	}

	sources, errored := gatherSources(opt.files, opt.recursive, stdin, stderr)

	cfg := &Config{
		matcher:          matcher,
		lineNum:          opt.lineNum,
		count:            opt.count,
		filesWithMatches: opt.listFiles,
		before:           opt.before,
		after:            opt.after,
		// Show the filename when there's ambiguity: more than one file operand,
		// or a recursive walk that can fan out into many files.
		showName: opt.recursive || len(opt.files) > 1,
	}

	anyMatch := Run(cfg, sources, stdout)

	// Exit code precedence: an operand error is code 2 regardless of matches;
	// otherwise 0 if something matched, 1 if nothing did.
	if errored {
		return 2
	}
	if anyMatch {
		return 0
	}
	return 1
}

// parseArgs hand-rolls flag parsing (like the sibling `cut` tool) so we can
// accept the things Go's `flag` package can't: bundled short flags (`-in`),
// attached values (`-A3`), and the "first non-flag operand is the PATTERN"
// rule that grep uses.
func parseArgs(args []string) (options, error) {
	var opt options
	havePattern := false

	for i := 0; i < len(args); i++ {
		arg := args[i]

		// "-" alone and anything not starting with "-" is a positional operand:
		// the first such token is the PATTERN, the rest are file names.
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			if !havePattern {
				opt.pattern = arg
				havePattern = true
			} else {
				opt.files = append(opt.files, arg)
			}
			continue
		}

		// "--" ends flag parsing; everything after is an operand.
		if arg == "--" {
			for _, rest := range args[i+1:] {
				if !havePattern {
					opt.pattern = rest
					havePattern = true
				} else {
					opt.files = append(opt.files, rest)
				}
			}
			break
		}

		// A bundle of short flags, e.g. "-ivn" or "-rnA3". We walk it character
		// by character; the context flags A/B/C consume a numeric value that is
		// either the rest of this token or the next argument.
		body := arg[1:]
		for j := 0; j < len(body); j++ {
			switch c := body[j]; c {
			case 'i':
				opt.ignoreCase = true
			case 'v':
				opt.invert = true
			case 'n':
				opt.lineNum = true
			case 'c':
				opt.count = true
			case 'w':
				opt.word = true
			case 'r', 'R':
				opt.recursive = true
			case 'l':
				opt.listFiles = true
			case 'A', 'B', 'C':
				val, consumedRest, err := takeNumber(body[j+1:], args, &i)
				if err != nil {
					return opt, fmt.Errorf("option -%c: %v", c, err)
				}
				switch c {
				case 'A':
					opt.after = val
				case 'B':
					opt.before = val
				case 'C':
					opt.after, opt.before = val, val
				}
				if consumedRest {
					j = len(body) // the rest of the token was the number
				}
			default:
				return opt, fmt.Errorf("invalid option -- '%c'", c)
			}
		}
	}

	if !havePattern {
		return opt, fmt.Errorf("no pattern given")
	}
	return opt, nil
}

// takeNumber extracts the numeric argument for a context flag. `rest` is the
// remainder of the current token after the flag letter; if it is empty we
// consume the next argv element instead (and bump the caller's index i).
func takeNumber(rest string, args []string, i *int) (n int, consumedRest bool, err error) {
	if rest != "" {
		v, convErr := strconv.Atoi(rest)
		if convErr != nil {
			return 0, false, fmt.Errorf("invalid context length %q", rest)
		}
		if v < 0 {
			return 0, false, fmt.Errorf("context length cannot be negative")
		}
		return v, true, nil
	}
	if *i+1 >= len(args) {
		return 0, false, fmt.Errorf("requires a numeric argument")
	}
	*i++
	v, convErr := strconv.Atoi(args[*i])
	if convErr != nil {
		return 0, false, fmt.Errorf("invalid context length %q", args[*i])
	}
	if v < 0 {
		return 0, false, fmt.Errorf("context length cannot be negative")
	}
	return v, false, nil
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: grep [-ivncwrl] [-A N] [-B N] [-C N] PATTERN [file...]")
}
