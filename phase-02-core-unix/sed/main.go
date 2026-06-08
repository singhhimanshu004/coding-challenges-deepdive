// Command sed is a stream editor: it reads text from files (or standard input),
// applies an edit *script* to it line by line, and writes the result out. It is
// a faithful subset of the Unix `sed`, built to show off the stream-editing
// execution model rather than to cover every GNU extension.
//
// Usage:
//
//	sed [-n] [-i] SCRIPT [FILE ...]
//
// Flags:
//
//	-n   suppress automatic printing of the pattern space (print only via p)
//	-i   edit files in place (rewrite each FILE instead of writing to stdout)
//
// Supported script commands:
//
//	s/regex/replacement/[g][i][p]   substitute (backrefs \1..\9, & = whole match)
//	p                               print the pattern space
//	d                               delete the pattern space (skip auto-print)
//
// Each command may be preceded by an address that selects which lines it runs on:
//
//	N            a single line number          (e.g. 3d)
//	$            the last line                  (e.g. $p)
//	/regex/      lines matching a pattern       (e.g. /foo/d)
//	addr1,addr2  an inclusive range of lines    (e.g. 2,4d  or  /BEGIN/,/END/p)
//
// Multiple commands are separated by ';' or newlines: `sed 's/a/b/; 2,3d'`.
//
// Exit codes follow the repo convention:
//
//	0  success
//	1  domain failure (I/O error reading or writing)
//	2  usage error (bad flags, missing script, bad script syntax)
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	ised "sed/internal/sed"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run is split out from main so tests can drive it with custom args and streams
// and assert on the exit code without spawning a process.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var suppress, inPlace bool
	var positional []string

	// Hand-rolled flag parser. sed's flags here are all boolean, so we accept
	// them individually (-n, -i) or clustered (-ni). The first non-flag token is
	// the script; everything after it is a file argument.
	parsingFlags := true
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			usage(stdout)
			return 0
		case parsingFlags && a == "--":
			parsingFlags = false
		case parsingFlags && a == "-":
			parsingFlags = false
			positional = append(positional, a)
		case parsingFlags && len(a) > 1 && a[0] == '-':
			for _, f := range a[1:] {
				switch f {
				case 'n':
					suppress = true
				case 'i':
					inPlace = true
				default:
					fmt.Fprintf(stderr, "sed: unknown option -%c\n", f)
					usage(stderr)
					return 2
				}
			}
		default:
			// The script is the first positional; once we have it, stop treating
			// later '-'-prefixed tokens as flags (they are file names).
			parsingFlags = false
			positional = append(positional, a)
		}
	}

	if len(positional) == 0 {
		fmt.Fprintln(stderr, "sed: missing script")
		usage(stderr)
		return 2
	}

	script := positional[0]
	files := positional[1:]

	cmds, err := ised.Parse(script)
	if err != nil {
		fmt.Fprintf(stderr, "sed: %v\n", err)
		return 2
	}

	opts := ised.Options{SuppressAuto: suppress}

	if inPlace {
		if len(files) == 0 {
			fmt.Fprintln(stderr, "sed: -i requires at least one file")
			return 2
		}
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				fmt.Fprintf(stderr, "sed: %v\n", err)
				return 1
			}
			var buf strings.Builder
			if err := ised.Run(cmds, ised.SplitLines(string(data)), &buf, opts); err != nil {
				fmt.Fprintf(stderr, "sed: %v\n", err)
				return 1
			}
			info, _ := os.Stat(f)
			mode := os.FileMode(0o644)
			if info != nil {
				mode = info.Mode()
			}
			if err := os.WriteFile(f, []byte(buf.String()), mode); err != nil {
				fmt.Fprintf(stderr, "sed: %v\n", err)
				return 1
			}
		}
		return 0
	}

	// Not in-place: read stdin or all files into one continuous stream, then
	// write the result to stdout. Concatenating means line numbers and `$`
	// span the whole input, matching GNU sed's default.
	var data strings.Builder
	if len(files) == 0 {
		b, err := io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintf(stderr, "sed: %v\n", err)
			return 1
		}
		data.Write(b)
	} else {
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				fmt.Fprintf(stderr, "sed: %v\n", err)
				return 1
			}
			data.Write(b)
		}
	}

	if err := ised.Run(cmds, ised.SplitLines(data.String()), stdout, opts); err != nil {
		fmt.Fprintf(stderr, "sed: %v\n", err)
		return 1
	}
	return 0
}

func usage(w io.Writer) {
	fmt.Fprint(w, strings.TrimLeft(`
sed — stream editor (files or stdin → stdout)

Usage:
  sed [-n] [-i] SCRIPT [FILE ...]

Flags:
  -n   suppress automatic printing; print only via the p command
  -i   edit files in place

Script commands (each may carry an address or addr1,addr2 range):
  s/regex/replacement/[g][i][p]   substitute; \1..\9 backrefs, & = whole match
  p                               print the pattern space
  d                               delete the pattern space

Addresses:
  N            a line number          $            the last line
  /regex/      matching lines         addr1,addr2  an inclusive range

Examples:
  sed 's/foo/bar/g' file.txt
  sed -n '2,4p' file.txt
  printf 'a\nb\nc\n' | sed '$d'
  sed -i 's/old/new/g' notes.txt
`, "\n"))
}
