// Package sed implements the core of a stream editor: a tiny interpreter that
// parses a sed *script* into a list of commands and then runs those commands
// against an input stream, one line at a time.
//
// The mental model has three moving parts that mirror real sed:
//
//   - The *pattern space*: a one-line scratch buffer. sed reads a line into it,
//     runs every command against it, and (unless -n) prints it at the end.
//   - *Addressing*: each command may be gated by an address (a line number, the
//     last-line marker `$`, or a `/regex/`) or an address *range* `addr1,addr2`.
//     A command only fires on lines its address selects.
//   - *Commands*: the verbs. Here we implement `s` (substitute), `p` (print),
//     and `d` (delete) — enough to show the whole execution model.
//
// This file defines the data model — addresses and commands — plus the two
// behaviours that operate on a single line: deciding whether a command
// *applies*, and performing a substitution.
package sed

import (
	"regexp"
	"strings"
)

// addrKind enumerates the three kinds of address sed understands.
//
// Go idiom (🐍→🐹): this is the standard "enum" pattern. There is no `enum`
// keyword like in some languages; instead we declare a named integer type and
// a block of `iota` constants. `iota` auto-increments (0, 1, 2, 3) so we never
// hand-number them. Think of it as a typed set of named ints.
type addrKind int

const (
	addrLine  addrKind = iota // a literal line number, e.g. `3`
	addrLast                  // the `$` marker: matches only the final line
	addrRegex                 // a `/regex/`: matches lines the pattern hits
)

// address is one parsed address. Only the field relevant to `kind` is used; the
// others sit at their zero values. (Go zero-values every field for free, so an
// addrLine address simply leaves `re` as nil.)
type address struct {
	kind addrKind
	line int            // used when kind == addrLine
	re   *regexp.Regexp // used when kind == addrRegex
}

// matches reports whether this single address selects the current line.
//
// `isLast` is supplied by the executor because a line cannot know on its own
// that it is the final one — that is a property of the whole stream.
func (a *address) matches(lineNum int, line string, isLast bool) bool {
	switch a.kind {
	case addrLine:
		return lineNum == a.line
	case addrLast:
		return isLast
	case addrRegex:
		return a.re.MatchString(line)
	}
	return false
}

// command is one parsed instruction: an optional address (or address range)
// plus the verb to run and its arguments.
//
// We use a single struct for all command kinds and switch on `kind` rather than
// defining an interface per command. With only three verbs that is simpler and
// keeps the executor's hot loop flat and easy to read.
type command struct {
	a1, a2 *address // a1==nil: run on every line. a2!=nil: a1,a2 is a range.
	active bool     // range state: are we currently inside an a1,a2 range?

	kind byte // 's', 'p', or 'd'

	// Fields below are only populated for the 's' (substitute) command.
	re           *regexp.Regexp // the compiled search pattern
	replTemplate string         // replacement rewritten in Go's $-expansion form
	global       bool           // the `g` flag: replace every match, not just the first
	printFlag    bool           // the `p` flag: print the line if a substitution happened
}

// applies decides whether this command should fire on the current line. It also
// advances the range state machine, so it must be called exactly once per line
// per command, in order.
//
// The three cases:
//
//	a1 == nil            → no address, fire on every line
//	a2 == nil            → single address, fire when a1 matches this line
//	a1 != nil, a2 != nil → range, fire for every line from the one that matches
//	                       a1 through the one that matches a2 (inclusive)
func (c *command) applies(lineNum int, line string, isLast bool) bool {
	if c.a1 == nil {
		return true
	}
	if c.a2 == nil {
		return c.a1.matches(lineNum, line, isLast)
	}

	// Range handling. `active` carries state *between lines*, which is exactly
	// how sed ranges work: addr1 turns the range on, addr2 turns it off.
	if !c.active {
		if c.a1.matches(lineNum, line, isLast) {
			c.active = true
			// A numeric end address that is already at or behind the start
			// line closes the range immediately — the range is just this line.
			if c.a2.kind == addrLine && c.a2.line <= lineNum {
				c.active = false
			}
			return true
		}
		return false
	}

	// We are inside the range: this line is included. Decide whether the range
	// closes *after* it.
	if c.a2.matches(lineNum, line, isLast) {
		c.active = false
	} else if c.a2.kind == addrLine && lineNum >= c.a2.line {
		// Safety net: a numeric end we have already passed (e.g. the exact line
		// was deleted upstream) still closes the range.
		c.active = false
	}
	return true
}

// substitute applies an `s///` command to one line and returns the rewritten
// line plus whether any replacement occurred.
//
// We do the matching by hand instead of using regexp.ReplaceAllString because
// sed's default (no `g` flag) replaces only the *first* match per line, which
// the standard library helper does not offer directly.
func (c *command) substitute(s string) (string, bool) {
	// FindAllStringSubmatchIndex returns, for every match, a flat slice of
	// start/end byte offsets: [m0s, m0e, g1s, g1e, ...]. That index form is what
	// ExpandString consumes to resolve $1, ${2}, $0 backreferences.
	matches := c.re.FindAllStringSubmatchIndex(s, -1)
	if matches == nil {
		return s, false
	}
	if !c.global {
		matches = matches[:1] // first match only
	}

	var b strings.Builder
	last := 0
	for _, m := range matches {
		b.WriteString(s[last:m[0]])                           // text before the match
		b.Write(c.re.ExpandString(nil, c.replTemplate, s, m)) // expanded replacement
		last = m[1]                                           // resume after the match
	}
	b.WriteString(s[last:]) // trailing text after the final match
	return b.String(), true
}
