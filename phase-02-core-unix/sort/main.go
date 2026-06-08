// Command sort orders the lines of its input and writes them back out, mirroring
// the classic Unix `sort`. It is Phase 2 / Challenge 11 of the coding-challenges
// curriculum (https://codingchallenges.fyi/challenges/challenge-sort).
//
// Usage:
//
//	sort [-r] [-n] [-u] [-f] [-k FIELD] [-t SEP] [file...]
//
// Flags:
//
//	-r            reverse the result (descending)
//	-n            numeric sort (compare leading numbers, not text)
//	-u            unique: drop lines that compare equal to the one before
//	-f            fold case: treat upper/lower case as equal
//	-k FIELD      sort on a 1-based field instead of the whole line
//	-t SEP        field separator for -k (default: runs of whitespace)
//	--external    force the external (on-disk) merge-sort path
//	--chunk-lines N  lines per in-memory run when sorting externally (default 1000)
//
// With no file operand (or "-") sort reads standard input. All input is read,
// ordered, then written — sort is one of the few Unix filters that is *not*
// streaming, because it cannot emit the first line until it has seen the last.
//
// Exit codes follow the repo convention:
//
//	0  success
//	1  domain failure (read error on a file)
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
// explicit streams so tests can drive it without touching the real os globals.
//
// Go idiom: returning the process exit code as a plain int (instead of calling
// os.Exit deep inside) keeps every code path testable. Only main() actually
// exits.
func cli(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cfg, files, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "sort: %v\n", err)
		usage(stderr)
		return 2
	}

	// Gather the input. sort must read everything before it can output anything,
	// so we either slurp every file into one slice of lines (in-memory path) or,
	// when forced, stream the input through the external merge sort.
	if cfg.external {
		// External path: stream straight from the readers to disk-backed runs.
		readers, closeAll, status := openInputs(files, stdin, stderr)
		defer closeAll()
		if err := externalSort(cfg, io.MultiReader(readers...), stdout); err != nil {
			fmt.Fprintf(stderr, "sort: external sort failed: %v\n", err)
			return 1
		}
		return status
	}

	lines, status := readAllLines(files, stdin, stderr)
	out := sortLines(cfg, lines)
	writeLines(stdout, out)
	return status
}

// openInputs turns the file operands into a slice of io.Readers (using stdin for
// "-" or when there are no operands). It returns a closer that releases every
// opened file and a status code that is non-zero if any file failed to open.
func openInputs(files []string, stdin io.Reader, stderr io.Writer) ([]io.Reader, func(), int) {
	if len(files) == 0 {
		return []io.Reader{stdin}, func() {}, 0
	}
	var readers []io.Reader
	var opened []*os.File
	status := 0
	for _, name := range files {
		if name == "-" {
			readers = append(readers, stdin)
			continue
		}
		f, err := os.Open(name)
		if err != nil {
			fmt.Fprintf(stderr, "sort: %s: %v\n", name, err)
			status = 1
			continue
		}
		readers = append(readers, f)
		opened = append(opened, f)
	}
	closeAll := func() {
		for _, f := range opened {
			f.Close()
		}
	}
	return readers, closeAll, status
}

// usage prints a short reminder of the accepted flags.
func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: sort [-r] [-n] [-u] [-f] [-k FIELD] [-t SEP] [--external] [--chunk-lines N] [file...]")
}
