// Command cat concatenates files (or standard input) to standard output.
//
// It mirrors the classic Unix `cat` and the codingchallenges.fyi "build your
// own cat" exercise:
//
//	cat                 copy stdin to stdout
//	cat a.txt b.txt     concatenate two files
//	cat a.txt - c.txt   '-' means "read stdin here", in order
//	cat -n file         number every output line
//	cat -b file         number only non-blank lines (overrides -n)
//	cat -E file         mark the end of each line with '$'
//
// Exit codes follow the repo convention (see huffman/bloom challenges):
//
//	0  success
//	1  a file could not be read (we still process the others)
//	2  usage error (unknown flag)
//
// Go note for a Python dev: a Go program starts at func main() in package
// `main`. There is no `if __name__ == "__main__":` — `main` *is* that entry
// point. We keep main() tiny and push all logic into run() so tests can call
// run() directly with fake streams (see main_test.go).
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
)

func main() {
	// os.Exit sets the process exit status (like sys.exit() in Python).
	// We hand it whatever run() returns. Note: os.Exit skips deferred calls,
	// which is exactly why all our `defer`s live inside run(), not here.
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// options holds the parsed command-line flags.
//
// Go note: a struct is "a class with only fields" — no methods required, no
// inheritance. Field names that start with a capital letter are exported
// (public); lowercase ones are package-private. These are lowercase because
// nothing outside this file needs them.
type options struct {
	numberAll      bool // -n : number every line
	numberNonBlank bool // -b : number only non-blank lines (wins over -n)
	showEnds       bool // -E : print a '$' at the end of each line
}

// run is the real entry point. It takes its streams as parameters
// (dependency injection) so tests can pass bytes.Buffer instead of the real
// os.Stdin/os.Stdout. It returns the process exit code.
//
// Go note: Go functions can return multiple values, but here we return just an
// int. Errors are reported as we go (to stderr) rather than bubbled up, because
// `cat` must keep going after one bad file.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	opts, files, code := parseArgs(args, stderr)
	if code != 0 {
		return code
	}

	// With no file arguments, cat reads standard input — that is what makes
	// `... | cat` and a bare `cat` (type, Ctrl-D to end) work.
	if len(files) == 0 {
		files = []string{"-"}
	}

	// Wrap stdout in a buffered writer so we batch many small writes into a
	// few big syscalls. bufio.Writer is Go's answer to "don't write a byte at
	// a time." We MUST Flush before returning or buffered bytes are lost.
	out := bufio.NewWriter(stdout)
	defer out.Flush()

	// lineNo is shared across every file so numbering is continuous, exactly
	// like real cat: file boundaries do not reset the counter.
	lineNo := 0
	exitCode := 0

	for _, name := range files {
		// Pick the source: '-' (or, by tradition, no name) means stdin;
		// anything else is a path to open.
		var src io.Reader
		if name == "-" {
			src = stdin
		} else {
			f, err := os.Open(name)
			if err != nil {
				// Report and keep going — one missing file must not abort the
				// rest. os.IsNotExist lets us print the familiar message.
				fmt.Fprintf(stderr, "cat: %s: %s\n", name, friendlyErr(err))
				exitCode = 1
				continue
			}
			// Go note: `defer` schedules a call for when the *function*
			// returns, not the loop iteration. With many files that would hold
			// every handle open until the end. So we close explicitly below
			// instead of deferring inside the loop.
			src = f
		}

		err := catStream(src, out, opts, &lineNo)

		// Close real files (stdin is not ours to close).
		if c, ok := src.(io.Closer); ok && name != "-" {
			c.Close()
		}

		if err != nil {
			fmt.Fprintf(stderr, "cat: %s: %s\n", name, friendlyErr(err))
			exitCode = 1
		}
	}

	return exitCode
}

// catStream copies one source to the output. It has two modes:
//
//   - Fast path (no flags): io.Copy streams raw bytes, so the output is a
//     faithful, binary-safe copy of the input — images, executables, anything.
//   - Line mode (any of -n/-b/-E): we read a line at a time so we can prepend
//     line numbers and/or append the '$' end marker.
func catStream(src io.Reader, out *bufio.Writer, opts options, lineNo *int) error {
	// If we don't need to inspect line structure, just shovel bytes through.
	// io.Copy uses an internal 32 KiB buffer and never interprets the data.
	if !opts.numberAll && !opts.numberNonBlank && !opts.showEnds {
		_, err := io.Copy(out, src)
		return err
	}

	// Line mode. bufio.Reader.ReadBytes('\n') returns each line *including* the
	// trailing newline (if present). At EOF it returns the final partial line
	// plus io.EOF, so we must process the bytes before checking the error.
	br := bufio.NewReader(src)
	for {
		line, err := br.ReadBytes('\n')

		if len(line) > 0 {
			hasNewline := line[len(line)-1] == '\n'

			// content is the line without its trailing newline.
			content := line
			if hasNewline {
				content = line[:len(line)-1]
			}
			isBlank := len(content) == 0

			// Numbering. -b means "non-blank only" and, like GNU cat, takes
			// precedence over -n when both are given.
			if opts.numberNonBlank {
				if !isBlank {
					*lineNo++
					fmt.Fprintf(out, "%6d\t", *lineNo)
				}
			} else if opts.numberAll {
				*lineNo++
				fmt.Fprintf(out, "%6d\t", *lineNo)
			}

			if _, werr := out.Write(content); werr != nil {
				return werr
			}
			if opts.showEnds {
				if werr := out.WriteByte('$'); werr != nil {
					return werr
				}
			}
			if hasNewline {
				if werr := out.WriteByte('\n'); werr != nil {
					return werr
				}
			}
		}

		if err != nil {
			if err == io.EOF {
				return nil // clean end of stream
			}
			return err // a real read error
		}
	}
}

// parseArgs splits the argument list into options and file names.
//
// It supports:
//   - short flags -n, -b, -E
//   - bundled short flags, e.g. -nE is the same as -n -E
//   - long flags --number, --number-nonblank, --show-ends
//   - "--" to stop flag parsing (everything after is a file name)
//   - "-" as a file name meaning standard input
//
// On an unknown flag it prints usage to stderr and returns exit code 2.
func parseArgs(args []string, stderr io.Writer) (opts options, files []string, code int) {
	flagsDone := false
	for _, arg := range args {
		switch {
		case flagsDone:
			files = append(files, arg)

		case arg == "--":
			flagsDone = true

		case arg == "-":
			// A lone dash is stdin, not a flag.
			files = append(files, arg)

		case arg == "--number":
			opts.numberAll = true
		case arg == "--number-nonblank":
			opts.numberNonBlank = true
		case arg == "--show-ends":
			opts.showEnds = true

		case len(arg) > 1 && arg[0] == '-' && arg[1] != '-':
			// Bundled short flags: walk each character after the dash.
			for _, c := range arg[1:] {
				switch c {
				case 'n':
					opts.numberAll = true
				case 'b':
					opts.numberNonBlank = true
				case 'E':
					opts.showEnds = true
				default:
					fmt.Fprintf(stderr, "cat: invalid option -- '%c'\n", c)
					fmt.Fprintln(stderr, "usage: cat [-n] [-b] [-E] [file ...]")
					return opts, nil, 2
				}
			}

		default:
			// Anything else (including unknown --long) is treated as a file
			// name only if it doesn't look like a flag; long unknowns error.
			if len(arg) > 2 && arg[0] == '-' && arg[1] == '-' {
				fmt.Fprintf(stderr, "cat: unrecognized option '%s'\n", arg)
				fmt.Fprintln(stderr, "usage: cat [-n] [-b] [-E] [file ...]")
				return opts, nil, 2
			}
			files = append(files, arg)
		}
	}
	return opts, files, 0
}

// friendlyErr turns an os error into a short message close to system cat's.
func friendlyErr(err error) string {
	if os.IsNotExist(err) {
		return "No such file or directory"
	}
	if os.IsPermission(err) {
		return "Permission denied"
	}
	// Fall back to the underlying message.
	return err.Error()
}
