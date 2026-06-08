// compare.go — the heart of sort: the comparator. Everything about *order* lives
// here, isolated from how we read input or whether we sort in memory or on disk.
// Both the in-memory path (memsort.go) and the external path (external.go) call
// the exact same comparator, which is why their results are guaranteed to agree.
//
// A comparator answers one question: "does line a come before line b?". From
// that single primitive you get ascending/descending, numeric/text, and
// field-keyed ordering just by changing how the key is extracted and compared.
package main

import (
	"strconv"
	"strings"
	"unicode"
)

// extractKey pulls out the substring that should drive the comparison for one
// line. With no -k it is the whole line; with -k N it is the Nth field.
//
// Field splitting mirrors sort's default: when no -t separator is given, fields
// are separated by runs of whitespace (so "  a   b " has fields "a","b"). With
// -t, the separator is a single literal character and empty fields count.
func (cfg config) extractKey(line string) string {
	if cfg.keyField == 0 {
		return line
	}

	var fields []string
	if cfg.sep == "" {
		// strings.Fields collapses runs of whitespace — exactly the default
		// field model of sort. (Python equivalent: "  a  b ".split() .)
		fields = strings.Fields(line)
	} else {
		// A fixed separator keeps empty fields, like "a,,b".split(",").
		fields = strings.Split(line, cfg.sep)
	}

	idx := cfg.keyField - 1 // -k is 1-based
	if idx >= len(fields) {
		return "" // missing field sorts as empty, matching sort
	}
	// A keyed sort compares from the chosen field to the end of the line, which
	// is what sort does for "-k N" without an explicit end field.
	return strings.Join(fields[idx:], sepOrSpace(cfg.sep))
}

func sepOrSpace(sep string) string {
	if sep == "" {
		return " "
	}
	return sep
}

// compare returns -1, 0 or +1 for (a before b), (a equal b), (a after b) in the
// *ascending* sense, before -r is applied. Equality (0) is what -u uses to
// decide two lines are duplicates.
//
// Go idiom: a method on config keeps all the knobs (numeric, foldCase, ...) in
// scope without threading them through as parameters — this is Go's stand-in for
// a closure-capturing comparator.
func (cfg config) compare(a, b string) int {
	ka := cfg.extractKey(a)
	kb := cfg.extractKey(b)

	if cfg.numeric {
		na := leadingNumber(ka)
		nb := leadingNumber(kb)
		switch {
		case na < nb:
			return -1
		case na > nb:
			return 1
		default:
			return 0
		}
	}

	if cfg.foldCase {
		ka = strings.ToLower(ka)
		kb = strings.ToLower(kb)
	}

	return strings.Compare(ka, kb)
}

// less is the boolean form the sort package wants. It folds in -r by flipping
// the sign of the ascending comparison.
func (cfg config) less(a, b string) bool {
	c := cfg.compare(a, b)
	if cfg.reverse {
		return c > 0
	}
	return c < 0
}

// leadingNumber parses the leading numeric value of s, the way `sort -n` does.
// Non-numeric text (e.g. "abc") parses as 0, and trailing junk is ignored
// ("10kg" -> 10), so the comparison never errors out on messy real-world data.
//
// We parse by hand rather than calling strconv.ParseFloat so that the "ignore
// trailing junk" rule is explicit and matches sort's forgiving behaviour.
func leadingNumber(s string) float64 {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)

	i := 0
	n := len(s)

	// Optional leading sign.
	if i < n && (s[i] == '+' || s[i] == '-') {
		i++
	}

	sawDigit := false
	for i < n && s[i] >= '0' && s[i] <= '9' {
		i++
		sawDigit = true
	}
	// Optional fractional part.
	if i < n && s[i] == '.' {
		i++
		for i < n && s[i] >= '0' && s[i] <= '9' {
			i++
			sawDigit = true
		}
	}

	if !sawDigit {
		return 0
	}

	// strconv.ParseFloat does the actual string->float64 conversion on the clean
	// numeric prefix we isolated above.
	f, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0
	}
	return f
}
