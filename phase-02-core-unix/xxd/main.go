// Command xxd makes a hex dump of its input, or — with -r — turns a hex dump
// back into the original bytes. It mirrors the classic Unix `xxd`, built as
// Phase 2 / Challenge 15 of the coding-challenges curriculum.
//
// Usage:
//
//	xxd [-l len] [-c cols] [-s seek] [-g group] [file]
//	xxd -r [file]                # reverse: hex dump -> bytes
//
// Flags:
//
//	-l len    stop after dumping `len` bytes
//	-c cols   bytes shown per output line (default 16)
//	-s seek   skip `seek` bytes of input before dumping
//	-g group  number of bytes per space-separated group (default 2)
//	-r        reverse — read a hex dump and write the original bytes
//
// With no file operand (or "-") xxd reads standard input. All I/O is
// binary-safe: bytes are never interpreted as text on the dumping path.
//
// Exit codes follow the repo convention:
//
//	0  success
//	1  domain failure (read/parse error)
//	2  usage error (bad flags)
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(cli(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// cli is the testable entry point: it takes argv (without the program name) and
// explicit streams, so tests can drive it with in-memory buffers instead of the
// real os globals.
//
// Go idiom: the streams are typed as the io.Reader / io.Writer interfaces, not
// *os.File. A real file, os.Stdin, or a bytes.Buffer all satisfy them — that
// duck-typing is what makes the function trivially testable.
func cli(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cfg, file, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "xxd: %v\n", err)
		usage(stderr)
		return 2
	}

	// Resolve the input source: a named file, or stdin for "" / "-".
	in := stdin
	if file != "" && file != "-" {
		f, err := os.Open(file)
		if err != nil {
			fmt.Fprintf(stderr, "xxd: %s: %v\n", file, err)
			return 1
		}
		// Go idiom: defer runs at function return, like a `finally` block, so
		// the handle is always closed regardless of which branch we exit from.
		defer f.Close()
		in = f
	}

	if cfg.reverse {
		if err := reverse(in, stdout); err != nil {
			fmt.Fprintf(stderr, "xxd: %v\n", err)
			return 1
		}
		return 0
	}

	if err := dump(in, stdout, cfg); err != nil {
		fmt.Fprintf(stderr, "xxd: %v\n", err)
		return 1
	}
	return 0
}

// parseArgs hand-rolls flag parsing so we can accept both the attached form
// (-c16) and the separated form (-c 16) exactly the way real xxd does. Go's
// standard `flag` package cannot parse attached short flags, hence the manual
// loop.
//
// Go idiom: Go has no default-argument syntax, so we seed the config struct
// with the defaults up front and let any flags overwrite them.
func parseArgs(args []string) (config, string, error) {
	cfg := config{cols: 16, group: 2} // defaults match GNU/BSD xxd
	var file string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		// A lone "-" or any non-flag token is the file operand.
		if arg == "-" || len(arg) == 0 || arg[0] != '-' {
			if file != "" {
				return cfg, "", fmt.Errorf("only one input file is supported")
			}
			file = arg
			continue
		}

		flag := arg[:2] // e.g. "-c"
		rest := arg[2:] // attached value, possibly empty

		// needVal returns the value for a flag, supporting both "-cVAL"
		// (attached, already in rest) and "-c VAL" (separated, consumes the
		// next argv token).
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
		case "-r":
			cfg.reverse = true
		case "-l":
			v, err := needVal()
			if err != nil {
				return cfg, "", err
			}
			n, err := parseUint(v)
			if err != nil {
				return cfg, "", fmt.Errorf("invalid -l value %q", v)
			}
			cfg.length = n
			cfg.hasLength = true
		case "-s":
			v, err := needVal()
			if err != nil {
				return cfg, "", err
			}
			n, err := parseUint(v)
			if err != nil {
				return cfg, "", fmt.Errorf("invalid -s value %q", v)
			}
			cfg.seek = n
		case "-c":
			v, err := needVal()
			if err != nil {
				return cfg, "", err
			}
			n, err := parseUint(v)
			if err != nil || n == 0 {
				return cfg, "", fmt.Errorf("invalid -c value %q", v)
			}
			cfg.cols = int(n)
		case "-g":
			v, err := needVal()
			if err != nil {
				return cfg, "", err
			}
			n, err := parseUint(v)
			if err != nil {
				return cfg, "", fmt.Errorf("invalid -g value %q", v)
			}
			cfg.group = int(n)
		default:
			return cfg, "", fmt.Errorf("unknown option %q", arg)
		}
	}

	// A group size of 0 means "one big group" — no internal spaces.
	if cfg.group == 0 {
		cfg.group = cfg.cols
	}
	return cfg, file, nil
}

// parseUint reads a base-10 non-negative integer without pulling in strconv's
// signed handling — it keeps the intent (offsets/lengths are never negative)
// explicit and rejects junk input.
func parseUint(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty number")
	}
	var n int64
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("not a number")
		}
		n = n*10 + int64(r-'0')
	}
	return n, nil
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: xxd [-l len] [-c cols] [-s seek] [-g group] [file]")
	fmt.Fprintln(w, "       xxd -r [file]")
}
