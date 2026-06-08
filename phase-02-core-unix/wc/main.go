// Command wc counts lines, words, characters and bytes — a from-scratch Go
// rebuild of the classic Unix `wc`, following codingchallenges.fyi.
//
// Usage:
//
//	wc [-c] [-l] [-w] [-m] [file ...]
//
// Flags (bundling like `-lw` and long forms like `--lines` are supported):
//
//	-c, --bytes  print the byte count
//	-l, --lines  print the newline count
//	-w, --words  print the word count
//	-m, --chars  print the character (rune) count
//
// With no flags, wc prints lines, words and bytes (the traditional default).
// With no file arguments — or with "-" — it reads standard input. Multiple
// files each get their own line plus a final "total" line.
//
// Exit codes follow the repo convention:
//
//	0  success
//	1  a file could not be read (matches real wc)
//	2  usage error (unknown flag)
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func main() {
	// All real work lives in run() so it returns an int we can test directly;
	// only main() is allowed to call os.Exit. This keeps the logic unit-testable.
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// options records which columns the user asked for. The zero value means
// "no flags given", which we later expand into the traditional default.
type options struct {
	lines, words, chars, bytes bool
}

// any reports whether at least one column flag was selected.
func (o options) any() bool {
	return o.lines || o.words || o.chars || o.bytes
}

// run is the real entry point. Passing in the streams (in/out/err) as plain
// io interfaces instead of touching os.Stdin/Stdout directly is a Go testing
// idiom: tests can feed a bytes.Buffer and assert on the output without
// spawning a process.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	opts, files, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "wc: %v\n", err)
		fmt.Fprintln(stderr, "usage: wc [-clwm] [file ...]")
		return 2
	}

	// No flags → the historical default of lines, words, bytes.
	if !opts.any() {
		opts.lines, opts.words, opts.bytes = true, true, true
	}

	// No file arguments → read standard input, labelled with an empty name
	// so we print counts with no trailing filename, exactly like real wc.
	if len(files) == 0 {
		files = []string{""}
	}

	exit := 0
	var total counts
	results := make([]lineResult, 0, len(files))

	for _, name := range files {
		c, err := countNamed(name, stdin)
		if err != nil {
			fmt.Fprintf(stderr, "wc: %s: %v\n", name, err)
			exit = 1 // real wc reports 1 when a file can't be read, but keeps going
			continue
		}
		total.add(c)
		results = append(results, lineResult{name: name, c: c})
	}

	// wc right-aligns every column to a shared width. The width is driven by the
	// largest number we will print anywhere (including the total row), so all
	// rows line up cleanly. We compute it up front, then format every row.
	width := columnWidth(results, total, opts, len(results) > 1)

	for _, r := range results {
		fmt.Fprintln(stdout, formatLine(r.c, r.name, opts, width))
	}
	if len(results) > 1 {
		fmt.Fprintln(stdout, formatLine(total, "total", opts, width))
	}

	return exit
}

// lineResult pairs a file's name with its counts for the output phase.
type lineResult struct {
	name string
	c    counts
}

// countNamed counts a single input: standard input when name is "" or "-",
// otherwise the named file. The file handle is always closed via defer.
func countNamed(name string, stdin io.Reader) (counts, error) {
	if name == "" || name == "-" {
		return count(stdin)
	}

	f, err := os.Open(name)
	if err != nil {
		return counts{}, err
	}
	defer f.Close() // defer runs on return — Go's tidy answer to try/finally.

	return count(f)
}

// parseArgs hand-parses the command line so we can support short-flag bundling
// (-lw == -l -w) and "--" to stop flag parsing, which Go's standard flag package
// does not do. Anything that is not a flag is treated as a filename.
func parseArgs(args []string) (options, []string, error) {
	var opts options
	var files []string
	flagsDone := false

	for _, a := range args {
		switch {
		case flagsDone:
			files = append(files, a)

		case a == "--":
			flagsDone = true // everything after "--" is a filename

		case a == "-":
			files = append(files, a) // "-" means stdin, not a flag

		case strings.HasPrefix(a, "--"):
			if err := applyLongFlag(&opts, a); err != nil {
				return opts, nil, err
			}

		case strings.HasPrefix(a, "-") && len(a) > 1:
			// A bundle like "-lwc": every character is its own flag.
			for _, ch := range a[1:] {
				if err := applyShortFlag(&opts, ch); err != nil {
					return opts, nil, err
				}
			}

		default:
			files = append(files, a)
		}
	}

	return opts, files, nil
}

func applyShortFlag(o *options, ch rune) error {
	switch ch {
	case 'l':
		o.lines = true
	case 'w':
		o.words = true
	case 'm':
		o.chars = true
	case 'c':
		o.bytes = true
	default:
		return fmt.Errorf("invalid option -- '%c'", ch)
	}
	return nil
}

func applyLongFlag(o *options, flag string) error {
	switch flag {
	case "--lines":
		o.lines = true
	case "--words":
		o.words = true
	case "--chars":
		o.chars = true
	case "--bytes":
		o.bytes = true
	default:
		return fmt.Errorf("unrecognized option '%s'", flag)
	}
	return nil
}

// columnWidth finds the field width that makes every printed number line up.
// We scan only the columns that will actually be shown and take the widest
// number string. A small minimum keeps single-digit output looking like wc.
func columnWidth(results []lineResult, total counts, opts options, hasTotal bool) int {
	width := 1
	consider := func(c counts) {
		for _, n := range selected(c, opts) {
			if d := len(strconv.Itoa(n)); d > width {
				width = d
			}
		}
	}
	for _, r := range results {
		consider(r.c)
	}
	if hasTotal {
		consider(total)
	}
	return width
}

// selected returns the counts for the chosen columns, in wc's fixed order:
// lines, words, chars, bytes.
func selected(c counts, opts options) []int {
	var out []int
	if opts.lines {
		out = append(out, c.lines)
	}
	if opts.words {
		out = append(out, c.words)
	}
	if opts.chars {
		out = append(out, c.chars)
	}
	if opts.bytes {
		out = append(out, c.bytes)
	}
	return out
}

// formatLine renders one output row: right-aligned numbers joined by single
// spaces, followed by the filename (if any).
func formatLine(c counts, name string, opts options, width int) string {
	nums := selected(c, opts)
	parts := make([]string, len(nums))
	for i, n := range nums {
		parts[i] = fmt.Sprintf("%*d", width, n) // %*d = right-align in `width` cols
	}
	line := strings.Join(parts, " ")
	if name != "" {
		line += " " + name
	}
	return line
}
