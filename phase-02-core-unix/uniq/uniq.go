package main

import (
	"bufio"
	"fmt"
	"io"
)

// uniqStream is the heart of the tool. It reads text from r line by line and
// writes the de-duplicated result to w according to opt.
//
// THE CORE ALGORITHM — "carry one line of state across the stream":
//
// We never load the whole file into memory. Instead we remember just ONE
// thing: the previous line and how many times we've seen it in a row (the
// current "run" or "group"). When the next line differs from what we're
// holding, the group is finished — we emit it, then start a new group with the
// new line. This is a classic streaming / run-length pattern and it's exactly
// why uniq only collapses *adjacent* duplicates: our memory is one line deep.
//
//	prev = first line, count = 1
//	for each next line:
//	    if line == prev:        count++          (group grows)
//	    else:                   emit(prev,count) (group ended)
//	                            prev = line, count = 1
//	after the loop:             emit(prev,count) (flush the final group)
func uniqStream(r io.Reader, w io.Writer, opt options) error {
	// bufio.Scanner streams the input one line at a time and strips the
	// trailing '\n' for us. This is the streaming equivalent of Python's
	// `for line in file:` — it never holds more than a buffer in memory, so it
	// works on files far larger than RAM.
	scanner := bufio.NewScanner(r)

	// bufio.Writer batches small writes into one big buffered flush, so we
	// aren't doing a syscall per line. We MUST Flush before returning, hence
	// the deferred flush below.
	bw := bufio.NewWriter(w)
	defer bw.Flush()

	var prev string    // the line we're currently holding (the open group)
	var count int      // how many times prev has appeared in a row
	haveGroup := false // false until we've read the very first line

	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case !haveGroup:
			// First line of the whole stream: open the first group.
			prev, count, haveGroup = line, 1, true
		case line == prev:
			// Same as the line we're holding: the run just got longer.
			count++
		default:
			// A different line arrived, so the previous group is complete.
			// Emit it, then begin a fresh group with this new line.
			if err := emit(bw, prev, count, opt); err != nil {
				return err
			}
			prev, count = line, 1
		}
	}
	// Scanner.Err() surfaces read errors (Scan() returns false on both EOF and
	// error, so we have to check explicitly — a common Go gotcha).
	if err := scanner.Err(); err != nil {
		return err
	}

	// Flush the final open group. (If the input was empty, haveGroup is still
	// false and there is nothing to emit.)
	if haveGroup {
		if err := emit(bw, prev, count, opt); err != nil {
			return err
		}
	}
	return nil
}

// emit writes a single finished group, honoring the -d/-u filters and the -c
// count prefix. `line` is the group's text and `count` is how many adjacent
// copies it had.
func emit(w io.Writer, line string, count int, opt options) error {
	// -d: keep only groups that were duplicated (seen 2 or more times).
	if opt.onlyDup && count < 2 {
		return nil
	}
	// -u: keep only groups that were truly unique (seen exactly once).
	if opt.onlyUni && count != 1 {
		return nil
	}

	if opt.count {
		// Match BSD/macOS uniq -c formatting: a right-justified count in a
		// 4-wide field, a space, then the line. (GNU uniq uses a 7-wide field;
		// pick whichever matches the `uniq` on your system.)
		_, err := fmt.Fprintf(w, "%4d %s\n", count, line)
		return err
	}
	_, err := fmt.Fprintf(w, "%s\n", line)
	return err
}
