package main

import (
	"io"
	"strings"
)

// tokenize reads ALL of r and splits it into the list of "items" that xargs
// will later glue onto command lines.
//
// There are two splitting rules:
//
//   - default (nulDelim == false): items are separated by ANY run of
//     whitespace — spaces, tabs, and newlines all count, and consecutive
//     whitespace collapses. Leading/trailing whitespace and blank lines are
//     ignored, so they never produce empty items. This matches the everyday
//     `... | xargs` behaviour.
//
//   - NUL mode (nulDelim == true, the `-0` flag): items are separated ONLY by
//     the NUL byte ('\x00'). Spaces and newlines inside an item are kept
//     verbatim, which is exactly what you want for filenames that contain
//     spaces. This is the safe pairing for `find . -print0 | xargs -0 ...`.
//
// 🐍 Python analogy: the default path is essentially `data.split()` (split on
// any whitespace, drop empties); the NUL path is `data.split("\x00")` with the
// trailing empty chunk removed.
//
// 🐹 Go idiom: we take an io.Reader (an interface — like Python "any object
// with .read()"). io.ReadAll drains it into a []byte. Returning the error
// instead of panicking is the Go convention; the caller decides what to do.
func tokenize(r io.Reader, nulDelim bool) ([]string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	if !nulDelim {
		// strings.Fields splits on runs of Unicode whitespace and never
		// returns empty strings — precisely the default xargs rule.
		return strings.Fields(string(data)), nil
	}

	// NUL-delimited: split on '\x00' and drop a trailing empty item, which
	// appears when the stream ends with a NUL (as `find -print0` always does).
	parts := strings.Split(string(data), "\x00")
	items := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		items = append(items, p)
	}
	return items, nil
}
