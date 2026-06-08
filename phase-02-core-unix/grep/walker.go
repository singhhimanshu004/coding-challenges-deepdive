// walker.go — turning command-line operands into a stream of named "sources".
//
// grep can read from three kinds of place: real files, standard input ("-" or
// no operand at all), and — with -r — every regular file underneath a
// directory. This file hides all three behind one idea: a Source, which is just
// a display name plus the lines it contains.
package main

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// stdinName is the label GNU grep prints for standard input.
const stdinName = "(standard input)"

// Source is one named unit of input. We read each file fully into memory up
// front because the context flags (-A/-B/-C) need to look both backwards and
// forwards around a match; a slice of lines makes that trivial. The tradeoff
// (you hold a whole file in RAM) is discussed in the README.
type Source struct {
	Name  string
	Lines []string
}

// readLines slurps a reader into a slice of lines, dropping the trailing
// newline on each (Scanner does this for us).
//
// Go idiom: bufio.Scanner is the standard line reader. Its default token buffer
// caps at 64 KiB, which silently breaks on very long lines — so we raise the
// cap, exactly as the other tools in this repo do.
func readLines(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// gatherSources expands the operand list into concrete Sources.
//
// Rules (mirroring GNU grep):
//   - no operands           -> read standard input
//   - "-"                   -> read standard input
//   - a regular file        -> read that file
//   - a directory + -r      -> walk it, reading every regular file inside
//   - a directory without -r-> a warning, skipped (and the run is "errored")
//
// It returns the sources plus a flag saying whether any operand-level error
// occurred, so main can map that onto grep's exit code 2.
func gatherSources(operands []string, recursive bool, stdin io.Reader, stderr io.Writer) ([]Source, bool) {
	var sources []Source
	errored := false

	// No file operands: the classic `… | grep pat` pipe case.
	if len(operands) == 0 {
		lines, err := readLines(stdin)
		if err != nil {
			fmt.Fprintf(stderr, "grep: (standard input): %v\n", err)
			return sources, true
		}
		return []Source{{Name: stdinName, Lines: lines}}, false
	}

	for _, op := range operands {
		if op == "-" {
			lines, err := readLines(stdin)
			if err != nil {
				fmt.Fprintf(stderr, "grep: (standard input): %v\n", err)
				errored = true
				continue
			}
			sources = append(sources, Source{Name: stdinName, Lines: lines})
			continue
		}

		info, err := os.Stat(op)
		if err != nil {
			fmt.Fprintf(stderr, "grep: %s: %v\n", op, err)
			errored = true
			continue
		}

		if info.IsDir() {
			if !recursive {
				fmt.Fprintf(stderr, "grep: %s: Is a directory\n", op)
				errored = true
				continue
			}
			walked, walkErrored := walkDir(op, stderr)
			sources = append(sources, walked...)
			errored = errored || walkErrored
			continue
		}

		lines, err := readFile(op)
		if err != nil {
			fmt.Fprintf(stderr, "grep: %s: %v\n", op, err)
			errored = true
			continue
		}
		sources = append(sources, Source{Name: op, Lines: lines})
	}

	return sources, errored
}

// readFile opens a path and reads it to completion, always closing the handle.
//
// Go idiom: `defer f.Close()` schedules the close to run when the function
// returns, no matter which return path is taken — Go's answer to a `with` block
// or try/finally.
func readFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readLines(f)
}

// walkDir implements -r using filepath.WalkDir, the modern (Go 1.16+) directory
// walker. It visits every entry in lexical order, recursing into subdirectories
// for us.
//
// Go idiom: WalkDir takes a callback `fn(path, d, err)`. Returning nil keeps
// going; returning fs.SkipDir prunes a subtree; returning any other error
// aborts the whole walk. The fs.DirEntry it hands us is cheaper than a full
// os.Stat because it reuses what the directory read already revealed.
func walkDir(root string, stderr io.Writer) ([]Source, bool) {
	var sources []Source
	errored := false

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Fprintf(stderr, "grep: %s: %v\n", path, err)
			errored = true
			return nil // report and keep walking the rest of the tree
		}
		if d.IsDir() {
			return nil // descend into it; nothing to read for the dir itself
		}
		if !d.Type().IsRegular() {
			return nil // skip symlinks, sockets, devices, …
		}

		lines, readErr := readFile(path)
		if readErr != nil {
			fmt.Fprintf(stderr, "grep: %s: %v\n", path, readErr)
			errored = true
			return nil
		}
		sources = append(sources, Source{Name: path, Lines: lines})
		return nil
	})
	if err != nil {
		fmt.Fprintf(stderr, "grep: %s: %v\n", root, err)
		errored = true
	}

	return sources, errored
}
