// Command cut selects portions of each line of its input — either whole fields
// delimited by a character (-f) or individual characters (-c). It mirrors the
// classic Unix `cut`, built as Phase 2 / Challenge 8 of the coding-challenges
// curriculum.
//
// Usage:
//
//	cut -f LIST [-d DELIM] [-s] [file...]
//	cut -c LIST           [file...]
//
// LIST is a comma-separated set of 1-based positions and ranges:
//
//	1,3     fields 1 and 3
//	2-4     fields 2 through 4
//	-3      fields 1 through 3
//	2-      field 2 through end of line
//
// With no file (or "-") cut reads standard input.
//
// Exit codes follow the repo convention:
//
//	0  success
//	1  domain failure (read error on a file)
//	2  usage error (bad flags, bad LIST)
package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	os.Exit(cli(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// cli is the testable entry point: it takes argv (without the program name) and
// explicit streams, so tests can drive it without touching real os globals.
func cli(args []string, stdin *os.File, stdout, stderr *os.File) int {
	cfg, files, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "cut: %v\n", err)
		usage(stderr)
		return 2
	}

	status := 0

	// No file operands -> read stdin once. Otherwise process each file in turn,
	// treating "-" as stdin (standard Unix convention).
	if len(files) == 0 {
		if err := run(stdin, stdout, cfg); err != nil {
			fmt.Fprintf(stderr, "cut: stdin: %v\n", err)
			status = 1
		}
		return status
	}

	for _, name := range files {
		if name == "-" {
			if err := run(stdin, stdout, cfg); err != nil {
				fmt.Fprintf(stderr, "cut: stdin: %v\n", err)
				status = 1
			}
			continue
		}

		f, err := os.Open(name)
		if err != nil {
			fmt.Fprintf(stderr, "cut: %s: %v\n", name, err)
			status = 1
			continue
		}
		if err := run(f, stdout, cfg); err != nil {
			fmt.Fprintf(stderr, "cut: %s: %v\n", name, err)
			status = 1
		}
		f.Close()
	}
	return status
}

// parseArgs hand-rolls flag parsing so we can accept both the attached form
// (-f1,3, -d,) and the separated form (-f 1,3, -d ,) the way real cut does.
// Go's standard `flag` package can't do attached short flags, hence the manual
// loop.
func parseArgs(args []string) (config, []string, error) {
	var (
		cfg       config
		files     []string
		fieldList string
		charList  string
		haveDelim bool
	)
	cfg.delim = "\t" // default delimiter is a single TAB

	// takeValue returns the value for a flag, supporting both "-fVAL" (attached,
	// passed as rest) and "-f VAL" (separated, consumes the next arg).
	for i := 0; i < len(args); i++ {
		arg := args[i]

		// A lone "-" or any non-flag token is a file operand.
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			files = append(files, arg)
			continue
		}

		// "--" ends flag parsing; everything after is a file operand.
		if arg == "--" {
			files = append(files, args[i+1:]...)
			break
		}

		flag := arg[:2] // e.g. "-f"
		rest := arg[2:] // attached value, possibly empty
		needVal := func() (string, error) {
			if rest != "" {
				return rest, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("option %s requires an argument", flag)
			}
			i++
			return args[i], nil
		}

		switch flag {
		case "-f":
			v, err := needVal()
			if err != nil {
				return cfg, nil, err
			}
			fieldList = v
		case "-c":
			v, err := needVal()
			if err != nil {
				return cfg, nil, err
			}
			charList = v
		case "-d":
			v, err := needVal()
			if err != nil {
				return cfg, nil, err
			}
			if len([]rune(v)) != 1 {
				return cfg, nil, fmt.Errorf("the delimiter must be a single character")
			}
			cfg.delim = v
			haveDelim = true
		case "-s":
			cfg.suppress = true
		default:
			return cfg, nil, fmt.Errorf("unknown option %q", arg)
		}
	}

	// Exactly one of -f / -c is required and they are mutually exclusive.
	switch {
	case fieldList != "" && charList != "":
		return cfg, nil, fmt.Errorf("only one of -f or -c may be used")
	case fieldList != "":
		cfg.mode = modeFields
		sel, err := ParseList(fieldList)
		if err != nil {
			return cfg, nil, err
		}
		cfg.sel = sel
	case charList != "":
		cfg.mode = modeChars
		if haveDelim {
			return cfg, nil, fmt.Errorf("-d may only be used with -f")
		}
		if cfg.suppress {
			return cfg, nil, fmt.Errorf("-s may only be used with -f")
		}
		sel, err := ParseList(charList)
		if err != nil {
			return cfg, nil, err
		}
		cfg.sel = sel
	default:
		return cfg, nil, fmt.Errorf("you must specify a list of fields (-f) or characters (-c)")
	}

	return cfg, files, nil
}

func usage(w *os.File) {
	fmt.Fprintln(w, "usage: cut -f LIST [-d DELIM] [-s] [file...]")
	fmt.Fprintln(w, "       cut -c LIST [file...]")
}
