package main

// main.go — the command-line front end. It wires everything together:
//
//	read input → ParseJSONStream → ParseFilter → eval(filter, value) → encode
//
// 🐍 Python analogy: the same testability trick as a well-structured Python CLI
// — `main()` is a thin shell around `run(args, stdin, stdout, stderr) int`, so
// tests can drive the whole program with in-memory buffers and assert on the
// exact bytes produced, with no subprocess and no temp files.
//
// Exit codes (repo convention, also close to real jq):
//
//	0  success
//	1  runtime/domain error (e.g. indexing a number)
//	2  usage error (bad flags, bad filter, unreadable file)

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// cliOptions holds the parsed command-line configuration.
type cliOptions struct {
	enc      encodeOptions
	rawOut   bool   // -r : print string results without surrounding quotes
	filter   string // the jq program (first positional argument)
	files    []string
	hasColor bool
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	opts, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "jq: usage error: %v\n", err)
		return 2
	}

	// Compile the filter once; reused for every input value.
	program, err := ParseFilter(opts.filter)
	if err != nil {
		fmt.Fprintf(stderr, "jq: error: invalid filter: %v\n", err)
		return 2
	}

	// Gather the raw JSON text from files (concatenated) or stdin.
	rawInput, err := readInput(opts.files, stdin)
	if err != nil {
		fmt.Fprintf(stderr, "jq: error: %v\n", err)
		return 2
	}

	values, err := ParseJSONStream(rawInput)
	if err != nil {
		fmt.Fprintf(stderr, "jq: error: invalid JSON: %v\n", err)
		return 2
	}

	// Apply the filter to each input value in turn, printing every result.
	for _, v := range values {
		results, err := eval(program, v)
		if err != nil {
			fmt.Fprintf(stderr, "jq: error: %v\n", err)
			return 1
		}
		for _, r := range results {
			writeResult(stdout, r, opts)
		}
	}
	return 0
}

// writeResult prints one filter output, honouring -r (raw strings).
func writeResult(stdout io.Writer, r any, opts cliOptions) {
	if opts.rawOut {
		if s, ok := r.(string); ok {
			fmt.Fprintln(stdout, s)
			return
		}
	}
	fmt.Fprintln(stdout, encodeValue(r, opts.enc))
}

// parseArgs is a small hand-rolled flag parser (same approach as the Phase-2
// Unix tools): it supports bundled short flags, a `--` terminator, then treats
// the first remaining argument as the filter and the rest as file paths.
func parseArgs(args []string) (cliOptions, error) {
	var opts cliOptions
	var positionals []string
	flagsDone := false

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case flagsDone || a == "-" || !strings.HasPrefix(a, "-"):
			positionals = append(positionals, a)
		case a == "--":
			flagsDone = true
		case strings.HasPrefix(a, "--"):
			switch a {
			case "--compact-output":
				opts.enc.compact = true
			case "--raw-output":
				opts.rawOut = true
			case "--sort-keys":
				opts.enc.sortKeys = true
			case "--color-output":
				opts.enc.color = true
			case "--monochrome-output":
				opts.enc.color = false
			default:
				return opts, fmt.Errorf("unknown option %q", a)
			}
		default:
			// Bundled short flags, e.g. -cr.
			for j := 1; j < len(a); j++ {
				switch a[j] {
				case 'c':
					opts.enc.compact = true
				case 'r':
					opts.rawOut = true
				case 'S':
					opts.enc.sortKeys = true
				case 'C':
					opts.enc.color = true
				case 'M':
					opts.enc.color = false
				default:
					return opts, fmt.Errorf("unknown flag -%c", a[j])
				}
			}
		}
	}

	if len(positionals) == 0 {
		return opts, fmt.Errorf("no filter given")
	}
	opts.filter = positionals[0]
	opts.files = positionals[1:]
	return opts, nil
}

// readInput returns the JSON source text: every file concatenated, or stdin when
// no files are given.
func readInput(files []string, stdin io.Reader) (string, error) {
	if len(files) == 0 {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		return string(data), nil
	}
	var sb strings.Builder
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return "", fmt.Errorf("cannot read %s: %w", f, err)
		}
		sb.Write(data)
		sb.WriteByte('\n')
	}
	return sb.String(), nil
}
