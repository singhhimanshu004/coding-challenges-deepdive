// Command head prints the beginning of files (or standard input).
//
// It mirrors the classic Unix `head` tool from codingchallenges.fyi:
//
//	head [-n N] [-c N] [file ...]
//	    -n N   print the first N lines  (default: 10)
//	    -c N   print the first N bytes  (overrides -n)
//
// With no file arguments (or with "-"), head reads from standard input.
// When more than one file is given, each file's output is preceded by a
//
//	==> filename <==
//
// header, exactly like GNU head.
//
// The defining trait of head is EARLY TERMINATION: it stops reading as soon
// as it has produced N lines (or N bytes). It never slurps a whole file into
// memory, so `head -n 5` on a 10 GB log file is instant and cheap.
//
// Exit codes follow the repo convention:
//
//	0  success
//	1  a file could not be opened / read (other files are still processed)
//	2  usage error (bad flag or bad numeric argument)
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
)

// In Go, program execution begins at main(). We keep main() tiny: it just
// translates the integer result of run() into a process exit code. Splitting
// the real logic into run([]string) int (instead of calling os.Exit deep
// inside helpers) is a common Go testing idiom — tests can call run() directly
// and assert on the returned code without the test process actually exiting.
func main() {
	os.Exit(run(os.Args[1:]))
}

// config holds the parsed command-line options.
//
// Go has no classes; we group related values in a struct. byteMode tells us
// whether the user asked for bytes (-c) instead of lines (-n). We track it
// separately because -c with a value of 0 is still "byte mode", and a plain
// zero count can't distinguish the two on its own.
type config struct {
	count    int  // how many lines or bytes to print
	byteMode bool // true => -c (bytes); false => -n (lines)
	files    []string
}

// run is the real entry point. It returns an exit code instead of calling
// os.Exit so it stays unit-testable.
func run(args []string) int {
	cfg, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "head: %v\n", err)
		fmt.Fprintln(os.Stderr, "usage: head [-n lines | -c bytes] [file ...]")
		return 2
	}

	// No files named => read stdin. This is the Unix "filter" convention:
	// a tool with no file args behaves as a pipe stage.
	if len(cfg.files) == 0 {
		if err := headStream(os.Stdout, os.Stdin, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "head: stdin: %v\n", err)
			return 1
		}
		return 0
	}

	exitCode := 0
	multiple := len(cfg.files) > 1

	// `for i, name := range slice` is Go's standard loop over a slice; i is the
	// index, name is the element. We need the index to decide when to print a
	// blank separator line between file sections (GNU head prints one before
	// every header except the first).
	for i, name := range cfg.files {
		var src io.Reader

		if name == "-" {
			// "-" is the conventional placeholder for stdin even when other
			// real files are listed alongside it.
			src = os.Stdin
		} else {
			f, err := os.Open(name)
			if err != nil {
				// os.Open's error already reads like "open NAME: no such file
				// or directory", so we don't re-add the filename.
				fmt.Fprintf(os.Stderr, "head: %v\n", err)
				exitCode = 1
				continue
			}
			// `defer f.Close()` schedules the close to run when the surrounding
			// FUNCTION returns — not at the end of this loop iteration. With
			// many files that would hold every handle open until the end, so we
			// instead close explicitly below. (Deferring inside a loop is a
			// classic Go gotcha worth calling out for a Python dev used to
			// `with open(...)` scoping to the block.)
			src = f
		}

		if multiple {
			printHeader(os.Stdout, name, i)
		}

		if err := headStream(os.Stdout, src, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "head: %s: %v\n", name, err)
			exitCode = 1
		}

		// Close the file now (not via defer) so we never accumulate open
		// descriptors across a long file list. Type assertion `src.(io.Closer)`
		// checks at runtime whether src implements Close — os.Stdin does not get
		// closed here because we only close real files.
		if name != "-" {
			if c, ok := src.(io.Closer); ok {
				c.Close()
			}
		}
	}

	return exitCode
}

// parseArgs is a tiny hand-rolled flag parser. We avoid the standard `flag`
// package on purpose: real head accepts `-n5`, `-n 5`, and stops treating
// things as flags after the first non-flag argument — behaviour the stdlib
// `flag` package doesn't model. Writing it by hand also makes the parsing
// rules visible, which is the whole point of this learning repo.
func parseArgs(args []string) (config, error) {
	cfg := config{count: 10} // default: first 10 lines

	i := 0
	for i < len(args) {
		arg := args[i]

		// A bare "-" means stdin and is NOT a flag; "--" ends flag parsing.
		// Anything not starting with '-' is a filename, and once we hit the
		// first filename every later argument is a filename too.
		if arg == "-" || arg == "--" || len(arg) == 0 || arg[0] != '-' {
			if arg == "--" {
				cfg.files = append(cfg.files, args[i+1:]...)
				break
			}
			cfg.files = append(cfg.files, args[i:]...)
			break
		}

		switch {
		case arg == "-n" || arg == "-c":
			// Value is the NEXT argument: `-n 5`.
			if i+1 >= len(args) {
				return cfg, fmt.Errorf("option %s requires an argument", arg)
			}
			n, err := parseCount(args[i+1])
			if err != nil {
				return cfg, err
			}
			cfg.count = n
			cfg.byteMode = arg == "-c"
			i += 2

		case len(arg) > 2 && (arg[1] == 'n' || arg[1] == 'c'):
			// Value is glued on: `-n5` / `-c100`.
			n, err := parseCount(arg[2:])
			if err != nil {
				return cfg, err
			}
			cfg.count = n
			cfg.byteMode = arg[1] == 'c'
			i++

		case arg == "-h" || arg == "--help":
			return cfg, fmt.Errorf("help requested")

		default:
			return cfg, fmt.Errorf("invalid option %q", arg)
		}
	}

	return cfg, nil
}

// parseCount converts a count argument to an int and rejects negatives.
// strconv.Atoi returns (int, error); Go forces us to handle the error, which
// is how the language nudges you toward robust input validation.
func parseCount(s string) (int, error) {
	n := 0
	if len(s) == 0 {
		return 0, fmt.Errorf("invalid count %q", s)
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid count %q", s)
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

// printHeader writes the GNU-style "==> name <==" banner. A blank line
// separates sections, but not before the very first one (index 0).
func printHeader(w io.Writer, name string, index int) {
	if index > 0 {
		fmt.Fprintln(w)
	}
	display := name
	if name == "-" {
		display = "standard input"
	}
	fmt.Fprintf(w, "==> %s <==\n", display)
}

// headStream copies the first N lines or N bytes from r to w and then STOPS.
// This is the heart of head: we never read past what we need.
//
// bufio.Reader wraps any io.Reader with an in-memory buffer so we can read
// efficiently in line- or chunk-sized pieces without a syscall per byte.
func headStream(w io.Writer, r io.Reader, cfg config) error {
	br := bufio.NewReader(r)
	if cfg.byteMode {
		return headBytes(w, br, cfg.count)
	}
	return headLines(w, br, cfg.count)
}

// headLines emits the first n lines. ReadBytes('\n') returns everything up to
// and including the next newline, so line terminators are preserved exactly
// (important when diffing against system head). We stop the moment we've
// written n lines — that's the early termination that makes head cheap.
func headLines(w io.Writer, br *bufio.Reader, n int) error {
	if n <= 0 {
		return nil
	}
	written := 0
	for written < n {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			if _, werr := w.Write(line); werr != nil {
				return werr
			}
			// A final line counts even if the file didn't end in '\n'. We only
			// increment when the chunk actually ends in a newline OR when EOF
			// makes it the last (unterminated) line.
			if line[len(line)-1] == '\n' {
				written++
			} else {
				written++ // last line, no trailing newline
			}
		}
		if err != nil {
			// io.EOF is the normal "stream finished" signal, not a failure.
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
	return nil
}

// headBytes emits the first n bytes. We copy in bounded chunks rather than
// byte-by-byte: io.CopyN copies exactly n bytes (or until EOF) and is itself
// implemented on top of buffered reads, so this stays both correct and fast.
func headBytes(w io.Writer, br *bufio.Reader, n int) error {
	if n <= 0 {
		return nil
	}
	// io.CopyN returns io.EOF if the source had fewer than n bytes — that's
	// fine (printing the whole short file is correct), so we swallow EOF.
	_, err := io.CopyN(w, br, int64(n))
	if err != nil && err != io.EOF {
		return err
	}
	return nil
}
