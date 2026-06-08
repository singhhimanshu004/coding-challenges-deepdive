// cut.go — the line-processing engine. Given a parsed configuration it reads
// input line by line and writes the selected columns, mirroring the semantics
// of the Unix `cut` utility.
package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// mode distinguishes the two ways cut slices a line.
//
// Go idiom: there is no `enum` keyword. The idiomatic substitute is a small
// named integer type plus `iota`, which auto-numbers the constants 0, 1, 2...
// Read `modeFields` as "the first member of an enum".
type mode int

const (
	modeFields mode = iota // -f : split on a delimiter, keep whole fields
	modeChars              // -c : index into the line's characters (runes)
)

// config is the fully-parsed request: what to select and how.
type config struct {
	mode     mode
	sel      Selector
	delim    string // field delimiter (-d); default is a single TAB
	suppress bool   // -s : drop lines that contain no delimiter (field mode only)
}

// run streams every line of r through the configured cut and writes results to
// w. It returns an error only for I/O problems; selection itself never fails
// once the config has been validated.
//
// Go idiom: we take io.Reader / io.Writer interfaces, not concrete files. This
// is duck typing made explicit — a file, an os.Stdin, or a bytes.Buffer all
// satisfy these interfaces, which is exactly what makes the tests able to feed
// in-memory strings instead of real files.
func run(r io.Reader, w io.Writer, cfg config) error {
	scanner := bufio.NewScanner(r)
	// Lines can be long; raise the per-token cap well above the 64KiB default
	// so a wide CSV row doesn't trip bufio.ErrTooLong.
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	out := bufio.NewWriter(w)
	defer out.Flush() // Go idiom: defer runs at function exit, like a `finally`.

	for scanner.Scan() {
		line := scanner.Text()
		processed, emit := cfg.processLine(line)
		if !emit {
			continue
		}
		if _, err := fmt.Fprintln(out, processed); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// processLine applies the configuration to a single line. The second return
// value reports whether the line should be emitted at all (it is false only
// when -s suppresses a delimiter-less line in field mode).
func (cfg config) processLine(line string) (string, bool) {
	if cfg.mode == modeChars {
		return cfg.cutChars(line), true
	}
	return cfg.cutFields(line)
}

// cutChars selects characters by 1-based position.
//
// We convert to []rune first so multi-byte UTF-8 characters count as one
// position each — `cut -c` is character-oriented, not byte-oriented. (A Python
// `str` already indexes by code point; in Go a plain string indexes by byte, so
// the explicit []rune conversion is how we get Python-like behaviour.)
func (cfg config) cutChars(line string) string {
	runes := []rune(line)
	var b strings.Builder
	for i, ch := range runes {
		if cfg.sel.contains(i + 1) { // positions are 1-based
			b.WriteRune(ch)
		}
	}
	return b.String()
}

// cutFields splits the line on the delimiter and keeps the selected fields,
// re-joined with the same delimiter.
//
// Behaviour for a line with no delimiter matches GNU cut: print the line
// unchanged, unless -s ("suppress") was requested, in which case skip it.
func (cfg config) cutFields(line string) (string, bool) {
	if !strings.Contains(line, cfg.delim) {
		if cfg.suppress {
			return "", false
		}
		return line, true
	}

	fields := strings.Split(line, cfg.delim)
	var kept []string
	for i, f := range fields {
		if cfg.sel.contains(i + 1) { // 1-based field numbers
			kept = append(kept, f)
		}
	}
	return strings.Join(kept, cfg.delim), true
}
