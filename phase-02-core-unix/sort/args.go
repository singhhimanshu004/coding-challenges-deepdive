// args.go — hand-rolled flag parsing. We accept the classic short flags (-r, -n,
// -u, -f), the value-taking -k / -t, and two long flags (--external,
// --chunk-lines) that exist mainly so tests and curious humans can force and
// tune the external merge-sort path.
//
// Go's standard `flag` package would work for most of this, but rolling it by
// hand keeps the parsing rules explicit and lets us bundle short flags the way
// real sort does (e.g. `-rn` == `-r -n`).
package main

import (
	"fmt"
	"strconv"
	"strings"
)

// config is the fully-parsed request: what to compare and how.
type config struct {
	reverse  bool   // -r : descending order
	numeric  bool   // -n : compare leading numeric value, not text
	unique   bool   // -u : drop adjacent equal lines (under the comparator)
	foldCase bool   // -f : case-insensitive comparison
	keyField int    // -k : 1-based field to sort on; 0 means "whole line"
	sep      string // -t : field separator for -k; "" means runs of whitespace

	// External merge-sort controls (teaching/testing knobs).
	external   bool // --external      : force the on-disk path
	chunkLines int  // --chunk-lines N : lines per in-memory run
}

const defaultChunkLines = 1000

// parseArgs converts argv into a config plus the list of file operands.
func parseArgs(args []string) (config, []string, error) {
	cfg := config{chunkLines: defaultChunkLines}
	var files []string

	for i := 0; i < len(args); i++ {
		arg := args[i]

		// A lone "-" is the stdin operand; any non-flag token is a file.
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			files = append(files, arg)
			continue
		}

		// "--" ends flag parsing; the rest are file operands.
		if arg == "--" {
			files = append(files, args[i+1:]...)
			break
		}

		// Long flags first.
		if strings.HasPrefix(arg, "--") {
			switch arg {
			case "--external":
				cfg.external = true
			case "--chunk-lines":
				if i+1 >= len(args) {
					return cfg, nil, fmt.Errorf("option --chunk-lines requires an argument")
				}
				i++
				n, err := strconv.Atoi(args[i])
				if err != nil || n <= 0 {
					return cfg, nil, fmt.Errorf("invalid --chunk-lines value %q", args[i])
				}
				cfg.chunkLines = n
			default:
				return cfg, nil, fmt.Errorf("unknown option %q", arg)
			}
			continue
		}

		// Short flags. A single token may bundle several boolean flags, and a
		// value-taking flag (-k/-t) may attach its value (-k2) or take the next
		// token (-k 2). We walk the characters after the leading '-'.
		chars := arg[1:]
		for j := 0; j < len(chars); j++ {
			switch chars[j] {
			case 'r':
				cfg.reverse = true
			case 'n':
				cfg.numeric = true
			case 'u':
				cfg.unique = true
			case 'f':
				cfg.foldCase = true
			case 'k', 't':
				flag := chars[j]
				// The value is the rest of this token, or the next argument.
				val := chars[j+1:]
				if val == "" {
					if i+1 >= len(args) {
						return cfg, nil, fmt.Errorf("option -%c requires an argument", flag)
					}
					i++
					val = args[i]
				}
				if flag == 'k' {
					n, err := strconv.Atoi(val)
					if err != nil || n <= 0 {
						return cfg, nil, fmt.Errorf("invalid field number %q for -k", val)
					}
					cfg.keyField = n
				} else {
					cfg.sep = val
				}
				j = len(chars) // value consumed the rest of the token
			default:
				return cfg, nil, fmt.Errorf("unknown option -%c", chars[j])
			}
		}
	}

	return cfg, files, nil
}
