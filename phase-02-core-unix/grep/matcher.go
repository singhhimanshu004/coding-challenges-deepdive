// matcher.go — the pattern-matching core of grep.
//
// A Matcher wraps a compiled regular expression plus the two flags that change
// what "a match" means: case-insensitivity (-i) and inversion (-v). Whole-word
// matching (-w) is folded into the regex itself at compile time.
package main

import (
	"fmt"
	"regexp"
)

// Matcher decides, for a single line of text, whether grep should consider it a
// hit. Everything that can fail (an invalid pattern) happens once in
// NewMatcher; the per-line Match call can never error, which keeps the hot loop
// simple.
type Matcher struct {
	re     *regexp.Regexp
	invert bool // -v : report the lines that DON'T match
}

// NewMatcher compiles a user pattern into a Matcher.
//
// Go idiom: constructors are just plain functions named `NewX` that return the
// value (or a pointer to it) and an `error`. There are no exceptions in Go — a
// bad pattern comes back as a second return value the caller must check, the
// same way `re.compile` raising in Python forces a try/except.
//
// The flags are applied by rewriting the pattern text *before* compilation:
//
//   - word (-w): wrap the pattern in `\b(?:...)\b` so it only matches when
//     bounded by word boundaries. `(?:...)` is a non-capturing group — it keeps
//     our boundaries glued to the whole user pattern even if the user wrote an
//     alternation like `foo|bar`.
//   - ignoreCase (-i): prepend the `(?i)` inline flag, RE2's portable way to
//     ask for case-insensitive matching for the rest of the expression.
func NewMatcher(pattern string, ignoreCase, invert, word bool) (*Matcher, error) {
	if word {
		pattern = `\b(?:` + pattern + `)\b`
	}
	if ignoreCase {
		pattern = `(?i)` + pattern
	}

	// regexp.Compile uses Go's RE2 engine. Unlike PCRE/Perl/Python's `re`, RE2
	// guarantees the match runs in linear time in the length of the input and
	// can never "catastrophically backtrack" — see the README for why that
	// matters and what it costs (no backreferences, no lookaround).
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern %q: %w", pattern, err)
	}

	return &Matcher{re: re, invert: invert}, nil
}

// Match reports whether a line counts as a hit, honouring -v.
//
// Go idiom: methods hang off a receiver declared in parentheses before the
// name — `(m *Matcher)` is roughly Python's `self`, except you name the type
// explicitly and choose a pointer receiver to avoid copying the struct.
func (m *Matcher) Match(line string) bool {
	hit := m.re.MatchString(line)
	if m.invert {
		return !hit
	}
	return hit
}
