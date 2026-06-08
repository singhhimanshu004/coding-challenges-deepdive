package main

import (
	"bufio"
	"io"
	"unicode"
)

// counts holds the four quantities wc can report for a single stream.
//
// Go idiom: a small "plain data" struct like this is the equivalent of a Python
// dataclass or a tuple with named fields. There is no constructor and no
// __init__ — the zero value (all fields 0) is already a valid, usable counts.
// That "the zero value is useful" philosophy is why we can just write
// `var c counts` and start adding to it.
type counts struct {
	lines int // number of '\n' bytes seen
	words int // maximal runs of non-whitespace, like wc -w
	chars int // number of Unicode runes (wc -m)
	bytes int // number of raw bytes (wc -c)
}

// add accumulates another counts into this one. We use a pointer receiver
// (*counts) because we are mutating the receiver in place — the Go equivalent
// of a method that modifies `self`. A value receiver would mutate a copy and
// the change would be lost.
func (c *counts) add(o counts) {
	c.lines += o.lines
	c.words += o.words
	c.chars += o.chars
	c.bytes += o.bytes
}

// count streams every rune from r and tallies lines, words, characters and
// bytes in a single pass. It never loads the whole input into memory, so it
// works the same on a 10-byte string or a 10-GB file.
//
// Why bufio? A raw io.Reader.Read can issue one syscall per call. bufio.Reader
// wraps it with an in-memory buffer (4 KiB by default) and hands us runes from
// that buffer, turning thousands of tiny reads into a handful of big ones. This
// is the streaming/buffered I/O pattern the README explains in depth.
func count(r io.Reader) (counts, error) {
	br := bufio.NewReader(r)

	var c counts
	inWord := false // are we currently inside a run of non-whitespace?

	for {
		// ReadRune decodes one UTF-8 rune and tells us how many bytes it spanned.
		// For a multibyte character (é, 世, 😀) size is 2–4; for ASCII it is 1.
		// On invalid UTF-8 it returns the replacement rune with size 1, which
		// still lets us advance one byte at a time without getting stuck.
		ru, size, err := br.ReadRune()

		if size > 0 {
			c.bytes += size // -c: every raw byte, regardless of encoding
			c.chars++       // -m: one rune == one character

			if ru == '\n' {
				c.lines++ // -l: wc counts newlines, not "visual lines"
			}

			// Word boundaries: a new word starts on the first non-space rune
			// after one or more spaces (or at the start of input).
			if unicode.IsSpace(ru) {
				inWord = false
			} else if !inWord {
				inWord = true
				c.words++
			}
		}

		if err != nil {
			// io.EOF is the normal, expected end of stream — not a real error.
			// Any other error (e.g. a disk read failure) is surfaced to the caller.
			if err == io.EOF {
				return c, nil
			}
			return c, err
		}
	}
}
