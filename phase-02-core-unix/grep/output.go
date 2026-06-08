// output.go — the reporting engine: it takes already-loaded Sources and a
// Config, runs the Matcher over every line, and writes grep-style output.
//
// This is where the three "summary" modes (-c count, -l files-with-matches) and
// the line-by-line mode (with -n line numbers and -A/-B/-C context) all live,
// kept apart from both pattern matching (matcher.go) and input gathering
// (walker.go). That clean split is the whole point of the design.
package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Config is the fully-parsed request handed to the engine.
type Config struct {
	matcher          *Matcher
	lineNum          bool // -n : prefix each printed line with its 1-based number
	count            bool // -c : print only a count of matching lines per source
	filesWithMatches bool // -l : print only the names of sources that matched
	before           int  // -B : lines of leading context
	after            int  // -A : lines of trailing context
	showName         bool // prefix output with the source name (multi-file / -r)
}

// Run drives every Source through the Config and reports whether *any* line
// matched anywhere — which main turns into the process exit code (0 vs 1).
//
// Go idiom: we write through a single buffered writer and `defer` its Flush.
// Without buffering, each Fprintln would be its own syscall; bufio batches them
// and the deferred Flush guarantees the tail is pushed out on every return path.
func Run(cfg *Config, sources []Source, w io.Writer) bool {
	out := bufio.NewWriter(w)
	defer out.Flush()

	anyMatch := false
	groupPrinted := false // shared across files so "--" lands between groups

	for _, src := range sources {
		switch {
		case cfg.count:
			n := countMatches(cfg, src)
			fmt.Fprintln(out, cfg.summaryPrefix(src.Name)+strconv.Itoa(n))
			if n > 0 {
				anyMatch = true
			}
		case cfg.filesWithMatches:
			if countMatches(cfg, src) > 0 {
				anyMatch = true
				fmt.Fprintln(out, src.Name)
			}
		default:
			if printMatches(cfg, src, out, &groupPrinted) {
				anyMatch = true
			}
		}
	}

	return anyMatch
}

// countMatches returns how many lines of a source the matcher accepts.
func countMatches(cfg *Config, src Source) int {
	n := 0
	for _, line := range src.Lines {
		if cfg.matcher.Match(line) {
			n++
		}
	}
	return n
}

// summaryPrefix produces the "name:" prefix used by -c when names are shown.
func (cfg *Config) summaryPrefix(name string) string {
	if cfg.showName {
		return name + ":"
	}
	return ""
}

// lineRange is an inclusive [start, end] span of line indices to print as one
// uninterrupted block. Context turns each match into a span; overlapping or
// touching spans get merged so we never print a line twice or split a run.
type lineRange struct{ start, end int }

// printMatches handles the default, line-by-line output mode (including -n and
// the -A/-B/-C context flags). It returns whether this source had any match.
func printMatches(cfg *Config, src Source, out io.Writer, groupPrinted *bool) bool {
	n := len(src.Lines)

	// First pass: find the matching line indices.
	var hits []int
	for i, line := range src.Lines {
		if cfg.matcher.Match(line) {
			hits = append(hits, i)
		}
	}
	if len(hits) == 0 {
		return false
	}

	// Second pass: expand each hit into a context span and merge the spans.
	ranges := mergeRanges(hits, cfg.before, cfg.after, n)
	hasContext := cfg.before > 0 || cfg.after > 0

	matched := make([]bool, n)
	for _, i := range hits {
		matched[i] = true
	}

	for _, r := range ranges {
		// GNU grep separates non-adjacent context blocks with a literal "--".
		// We only do this once context is in play and only *between* blocks.
		if hasContext && *groupPrinted {
			fmt.Fprintln(out, "--")
		}
		for i := r.start; i <= r.end; i++ {
			fmt.Fprintln(out, cfg.formatLine(src.Name, i+1, src.Lines[i], matched[i]))
		}
		*groupPrinted = true
	}

	return true
}

// mergeRanges turns a sorted list of hit indices into merged context spans,
// clamped to [0, n-1]. Two spans are merged when they overlap OR merely touch
// (end+1 == next start), so adjacent context never produces a stray separator.
func mergeRanges(hits []int, before, after, n int) []lineRange {
	var ranges []lineRange
	for _, i := range hits {
		start := i - before
		if start < 0 {
			start = 0
		}
		end := i + after
		if end > n-1 {
			end = n - 1
		}

		if len(ranges) > 0 && start <= ranges[len(ranges)-1].end+1 {
			// Overlaps or abuts the previous span — extend it instead.
			if end > ranges[len(ranges)-1].end {
				ranges[len(ranges)-1].end = end
			}
			continue
		}
		ranges = append(ranges, lineRange{start, end})
	}
	return ranges
}

// formatLine renders one output line. Matching lines use ":" between the
// prefix fields; pure context lines use "-", exactly like GNU grep, so you can
// tell at a glance which lines actually matched.
func (cfg *Config) formatLine(name string, lineNo int, text string, isMatch bool) string {
	sep := ":"
	if !isMatch {
		sep = "-"
	}

	var b strings.Builder
	if cfg.showName {
		b.WriteString(name)
		b.WriteString(sep)
	}
	if cfg.lineNum {
		b.WriteString(strconv.Itoa(lineNo))
		b.WriteString(sep)
	}
	b.WriteString(text)
	return b.String()
}
