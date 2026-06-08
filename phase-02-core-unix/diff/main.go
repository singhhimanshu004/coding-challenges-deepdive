// Command diff compares two text files line by line and prints the edits that
// turn the first into the second. It mirrors the classic Unix `diff`, built as
// Phase 2 / Challenge 14 (the phase capstone) of the coding-challenges
// curriculum.
//
// The comparison engine computes the Longest Common Subsequence (LCS) of the
// two files with dynamic programming — from scratch, no diff library — then
// backtracks through the DP table to recover an edit script. See lcs.go and
// editscript.go for the algorithm and format.go for the renderers.
//
// Usage:
//
//	diff [-u] [-c] [-U n] FILE1 FILE2
//
//	-u       unified format (@@ hunks with +/- and context lines)
//	-c       context format (older *** / --- block format)
//	-U n     unified format with n lines of context (implies -u)
//	(none)   classic "normal" format (1,3c1,3 with < and > lines)
//
// Either FILE may be "-" to read standard input.
//
// Exit codes follow both GNU diff and the repo convention:
//
//	0  files are identical
//	1  files differ
//	2  trouble (bad flags, unreadable file)
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

// config holds the parsed command-line options.
type config struct {
	format  formatKind
	context int
}

type formatKind int

const (
	formatNormal formatKind = iota
	formatUnified
	formatContext
)

// cli is the testable entry point: it takes argv (without the program name) and
// explicit streams so tests can drive it without touching the real os globals.
func cli(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cfg, files, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "diff: %v\n", err)
		usage(stderr)
		return 2
	}
	if len(files) != 2 {
		fmt.Fprintf(stderr, "diff: need exactly two file operands\n")
		usage(stderr)
		return 2
	}

	aLines, aErr := readLines(files[0], stdin)
	if aErr != nil {
		fmt.Fprintf(stderr, "diff: %s: %v\n", files[0], aErr)
		return 2
	}
	bLines, bErr := readLines(files[1], stdin)
	if bErr != nil {
		fmt.Fprintf(stderr, "diff: %s: %v\n", files[1], bErr)
		return 2
	}

	edits := buildEditScript(aLines, bLines)
	if !hasChanges(edits) {
		return 0 // identical
	}

	switch cfg.format {
	case formatUnified:
		ah := header(files[0])
		bh := header(files[1])
		io.WriteString(stdout, unifiedDiff(edits, ah, bh, cfg.context))
	case formatContext:
		ah := header(files[0])
		bh := header(files[1])
		io.WriteString(stdout, contextDiff(edits, ah, bh, cfg.context))
	default:
		io.WriteString(stdout, normalDiff(edits))
	}

	return 1 // differences found
}

// parseArgs turns argv into a config plus the list of file operands.
func parseArgs(args []string) (config, []string, error) {
	cfg := config{format: formatNormal, context: 3}
	var files []string

	i := 0
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "--":
			files = append(files, args[i+1:]...)
			i = len(args)
		case arg == "-u":
			cfg.format = formatUnified
			i++
		case arg == "-c":
			cfg.format = formatContext
			i++
		case arg == "-U":
			if i+1 >= len(args) {
				return cfg, nil, fmt.Errorf("option -U requires an argument")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 0 {
				return cfg, nil, fmt.Errorf("invalid context length %q", args[i+1])
			}
			cfg.format = formatUnified
			cfg.context = n
			i += 2
		case strings.HasPrefix(arg, "-U"):
			// Combined form, e.g. -U5.
			n, err := strconv.Atoi(arg[2:])
			if err != nil || n < 0 {
				return cfg, nil, fmt.Errorf("invalid context length %q", arg[2:])
			}
			cfg.format = formatUnified
			cfg.context = n
			i++
		case arg == "-":
			files = append(files, arg)
			i++
		case strings.HasPrefix(arg, "-") && len(arg) > 1:
			return cfg, nil, fmt.Errorf("unknown option %q", arg)
		default:
			files = append(files, arg)
			i++
		}
	}

	return cfg, files, nil
}

// readLines reads a file (or stdin when name == "-") and splits it into lines,
// dropping the trailing newline of each. A trailing newline at end of file does
// not create a spurious empty final line.
func readLines(name string, stdin io.Reader) ([]string, error) {
	var r io.Reader
	if name == "-" {
		r = stdin
	} else {
		f, err := os.Open(name)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return splitLines(string(data)), nil
}

// splitLines splits text on '\n'. An empty input yields no lines, and a final
// newline does not produce a trailing empty element.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	text = strings.TrimSuffix(text, "\n")
	return strings.Split(text, "\n")
}

// header builds the "--- name" / "+++ name" label. Real diff appends an mtime;
// we keep just the path so output is deterministic and easy to test.
func header(name string) string {
	if name == "-" {
		return "stdin"
	}
	return name
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: diff [-u] [-c] [-U n] FILE1 FILE2")
}
