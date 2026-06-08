package sed

import (
	"bufio"
	"io"
	"strings"
)

// Options holds the run-time switches that change how the executor behaves —
// the parts controlled by command-line flags rather than the script itself.
type Options struct {
	SuppressAuto bool // -n: do NOT auto-print the pattern space after each line
}

// Run is the line-by-line execution loop — the "interpreter" half of sed.
//
// For each input line it:
//  1. loads the line into the pattern space,
//  2. walks every command in order, firing those whose address applies,
//  3. unless -n (or the line was deleted), auto-prints the pattern space.
//
// This is the classic read → execute → auto-print cycle that defines sed.
func Run(cmds []*command, lines []string, out io.Writer, opts Options) error {
	// Reset range state so the same compiled command list can be reused across
	// multiple files (e.g. one Run per file for in-place editing).
	for _, c := range cmds {
		c.active = false
	}

	w := bufio.NewWriter(out)
	defer w.Flush() // Go's try/finally: always flush, even on early return

	total := len(lines)
	for i, line := range lines {
		lineNum := i + 1
		isLast := i == total-1

		patternSpace := line
		deleted := false

		for _, c := range cmds {
			if !c.applies(lineNum, patternSpace, isLast) {
				continue
			}
			switch c.kind {
			case 's':
				newPS, changed := c.substitute(patternSpace)
				patternSpace = newPS
				if changed && c.printFlag {
					writeLine(w, patternSpace)
				}
			case 'p':
				writeLine(w, patternSpace)
			case 'd':
				// `d` clears the pattern space, skips any remaining commands,
				// and suppresses the auto-print for this line.
				deleted = true
			}
			if deleted {
				break
			}
		}

		if !deleted && !opts.SuppressAuto {
			writeLine(w, patternSpace)
		}
	}
	return nil
}

func writeLine(w *bufio.Writer, s string) {
	w.WriteString(s)
	w.WriteByte('\n')
}

// SplitLines breaks raw input into the lines sed will iterate over, dropping a
// single trailing newline so a file ending in "\n" does not yield a spurious
// empty final line. Line endings are normalised back to "\n" on output.
func SplitLines(data string) []string {
	if data == "" {
		return nil
	}
	data = strings.TrimSuffix(data, "\n")
	return strings.Split(data, "\n")
}
