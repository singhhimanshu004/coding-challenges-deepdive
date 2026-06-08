// Command bloom is a Bloom-filter-backed spell checker.
//
// It works in two phases:
//
//	bloom build [-p rate] [-o filter.bf] <wordlist>
//	    Read a dictionary (one word per line), size an optimal Bloom filter for
//	    the target false-positive rate, insert every word, and serialize the
//	    filter to disk.
//
//	bloom check [-f filter.bf] [words...]
//	    Load a saved filter and report, for each input word (CLI args, or stdin
//	    if none given), whether it is "probably present" (a known word) or
//	    "definitely not present" (a likely misspelling).
//
// Exit codes follow the repo convention:
//
//	0  success (and, for check, every word was probably present)
//	1  domain signal (corrupt filter file, or at least one word flagged absent)
//	2  usage / IO error (bad args, file not found)
package main

import (
	"bufio"
	"fmt"
	"os"

	"bloom/internal/bloom"
	"bloom/internal/codec"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) < 1 {
		usage()
		return 2
	}

	switch args[0] {
	case "build", "b":
		return doBuild(args[1:])
	case "check", "c":
		return doCheck(args[1:])
	case "-h", "--help", "help":
		usage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "error: unknown command %q\n", args[0])
		usage()
		return 2
	}
}

// doBuild handles: bloom build [-p rate] [-o out] <wordlist>
func doBuild(args []string) int {
	p := 0.01 // default target false-positive rate: 1%
	outPath := ""
	inPath := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-p", "--rate":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: -p requires a value")
				return 2
			}
			i++
			if _, err := fmt.Sscanf(args[i], "%g", &p); err != nil {
				fmt.Fprintf(os.Stderr, "error: invalid rate %q\n", args[i])
				return 2
			}
		case "-o", "--output":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: -o requires a value")
				return 2
			}
			i++
			outPath = args[i]
		default:
			if inPath != "" {
				fmt.Fprintf(os.Stderr, "error: unexpected argument %q\n", args[i])
				return 2
			}
			inPath = args[i]
		}
	}

	if inPath == "" {
		fmt.Fprintln(os.Stderr, "error: missing <wordlist>")
		return 2
	}
	if outPath == "" {
		outPath = inPath + ".bf"
	}

	words, err := readWords(inPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	if len(words) == 0 {
		fmt.Fprintln(os.Stderr, "error: dictionary is empty")
		return 2
	}

	f, err := bloom.New(uint64(len(words)), p)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	for _, w := range words {
		f.AddString(w)
	}

	out, err := os.Create(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	defer out.Close()
	if err := codec.Save(out, f); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}

	fmt.Printf("built filter from %s: %d words\n", inPath, len(words))
	fmt.Printf("  m = %d bits (%.1f KB), k = %d hashes\n",
		f.M(), float64((f.M()+7)/8)/1024, f.K())
	fmt.Printf("  target false-positive rate: %.4g\n", p)
	fmt.Printf("  estimated false-positive rate at this fill: %.4g\n", f.EstimatedFalsePositiveRate())
	fmt.Printf("  saved to %s\n", outPath)
	return 0
}

// doCheck handles: bloom check [-f filter] [words...]
func doCheck(args []string) int {
	filterPath := ""
	var words []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-f", "--filter":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: -f requires a value")
				return 2
			}
			i++
			filterPath = args[i]
		default:
			words = append(words, args[i])
		}
	}

	if filterPath == "" {
		fmt.Fprintln(os.Stderr, "error: -f <filter> is required")
		return 2
	}

	in, err := os.Open(filterPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	defer in.Close()
	f, err := codec.Load(in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	// No words on the command line → read them from stdin, one per line.
	if len(words) == 0 {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			if w := normalize(sc.Text()); w != "" {
				words = append(words, w)
			}
		}
		if err := sc.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 2
		}
	}

	missing := 0
	for _, w := range words {
		if f.ContainsString(normalize(w)) {
			fmt.Printf("%-20s probably present\n", w)
		} else {
			fmt.Printf("%-20s MISSPELLED (definitely not in dictionary)\n", w)
			missing++
		}
	}

	if missing > 0 {
		// Signal "found likely misspellings" via exit code 1 so the command is
		// scriptable (e.g. fail a commit hook if any word is unknown).
		return 1
	}
	return 0
}

// readWords loads a dictionary file, one word per line, normalized and
// de-duplicated implicitly by the filter (duplicates just re-set the same bits).
func readWords(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var words []string
	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if w := normalize(sc.Text()); w != "" {
			words = append(words, w)
		}
	}
	return words, sc.Err()
}

// normalize lower-cases and trims a word so "The", "the", and " the " all map to
// the same dictionary entry. Spell checking is case-insensitive by convention.
func normalize(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			continue
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out = append(out, c)
	}
	return string(out)
}

func usage() {
	fmt.Fprint(os.Stderr, `bloom — Bloom-filter spell checker

Usage:
  bloom build [-p rate] [-o filter.bf] <wordlist>   # alias: b
  bloom check  -f filter.bf [words...]              # alias: c  (reads stdin if no words)

Options:
  -p, --rate    target false-positive rate for build (default 0.01 = 1%)
  -o, --output  output filter path (default <wordlist>.bf)
  -f, --filter  filter file to load for check

Examples:
  bloom build -p 0.01 -o words.bf /usr/share/dict/words
  bloom check -f words.bf recieve receive
  echo "teh cat sat" | tr ' ' '\n' | bloom check -f words.bf
`)
}
