// memsort.go — the in-memory sort path plus the shared line I/O helpers.
//
// For inputs that comfortably fit in RAM this is all you need: read every line
// into a slice, hand it to Go's standard sort, then optionally de-duplicate.
// The external path (external.go) only exists for inputs too big for this.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
)

// readAllLines reads every line from the file operands (or stdin) into one
// slice. The returned status is non-zero if any file could not be read.
func readAllLines(files []string, stdin io.Reader, stderr io.Writer) ([]string, int) {
	var lines []string
	status := 0

	appendFrom := func(r io.Reader, name string) {
		ls, err := scanLines(r)
		lines = append(lines, ls...)
		if err != nil {
			fmt.Fprintf(stderr, "sort: %s: %v\n", name, err)
			status = 1
		}
	}

	if len(files) == 0 {
		appendFrom(stdin, "stdin")
		return lines, status
	}

	for _, name := range files {
		if name == "-" {
			appendFrom(stdin, "stdin")
			continue
		}
		f, err := os.Open(name)
		if err != nil {
			fmt.Fprintf(stderr, "sort: %s: %v\n", name, err)
			status = 1
			continue
		}
		appendFrom(f, name)
		f.Close()
	}
	return lines, status
}

// scanLines reads all newline-terminated lines from r. A trailing line without a
// newline is still returned (matching how sort treats a final unterminated line).
func scanLines(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	// Lines can be long; lift the per-token cap above the 64 KiB default.
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// sortLines orders a slice of lines using the configured comparator and applies
// -u if requested.
//
// We use sort.SliceStable so that lines which compare *equal* keep their input
// order. Stability is what makes a keyed sort predictable: with `-k2`, two rows
// that share field 2 stay in the order you fed them. (sort.Slice is faster but
// may reorder equal elements — we trade a little speed for a faithful result.)
func sortLines(cfg config, lines []string) []string {
	sort.SliceStable(lines, func(i, j int) bool {
		return cfg.less(lines[i], lines[j])
	})

	if cfg.unique {
		lines = dedupeAdjacent(cfg, lines)
	}
	return lines
}

// dedupeAdjacent collapses runs of lines that compare equal. Because the slice
// is already sorted, equal lines are neighbours, so one linear pass suffices —
// the same trick `uniq` uses. Note -u dedupes on the *comparison key*, so under
// `-f` "Foo" and "foo" are duplicates and only the first survives.
func dedupeAdjacent(cfg config, lines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	out := lines[:1]
	for _, line := range lines[1:] {
		if cfg.compare(out[len(out)-1], line) != 0 {
			out = append(out, line)
		}
	}
	return out
}

// writeLines prints each line followed by a newline through a buffered writer.
func writeLines(w io.Writer, lines []string) {
	out := bufio.NewWriter(w)
	defer out.Flush() // Go idiom: defer runs at function exit, like `finally`.
	for _, line := range lines {
		fmt.Fprintln(out, line)
	}
}
