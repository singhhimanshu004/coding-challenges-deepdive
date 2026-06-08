// Package main — ranges.go holds the reusable "range list" parser that powers
// both -f (fields) and -c (characters).
//
// A LIST in cut looks like:  1,3      2-4      -3      2-      1,4-6,9
// i.e. a comma-separated set of items, where each item is one of:
//
//	N      a single 1-based column         -> rng{lo:N,  hi:N}
//	N-M    a closed range, N..M inclusive   -> rng{lo:N,  hi:M}
//	-M     "from the start through M"        -> rng{lo:1,  hi:M}
//	N-     "from N through the end"          -> rng{lo:N,  hi:0}  (hi==0 means open)
//
// Factoring this out keeps main.go small and means -f and -c share exactly the
// same selection semantics — only what they slice (whole fields vs. runes)
// differs.
package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// rng is one inclusive interval of 1-based positions.
//
// Go idiom (for a Python dev): this is a struct — think of it as a tiny class
// with only data and no __init__. Fields starting with a lowercase letter are
// "unexported" (package-private), the rough equivalent of a leading underscore
// in Python, except Go actually enforces it.
type rng struct {
	lo int // first position, always >= 1
	hi int // last position; hi == 0 is the sentinel for "open to end of line"
}

// contains reports whether 1-based position p falls inside this interval.
func (r rng) contains(p int) bool {
	if p < r.lo {
		return false
	}
	if r.hi == 0 { // open-ended range "N-"
		return true
	}
	return p <= r.hi
}

// Selector is an ordered set of ranges, e.g. the parsed form of "1,4-6,9".
//
// Go idiom: a named type backed by a slice. `[]rng` is "a Python list of rng".
// Methods can hang off any named type, not just structs, so Selector gets a
// contains() method below.
type Selector []rng

// contains reports whether position p (1-based) is selected by ANY range.
//
// Note we test membership rather than expanding ranges into a set. That matters
// because real `cut` emits selected columns in *input* order with no
// duplicates: `cut -f3,1` prints field 1 then field 3. By walking the line's
// columns in order and asking contains() for each, we get that behaviour for
// free — order of the spec is irrelevant.
func (s Selector) contains(p int) bool {
	for _, r := range s {
		if r.contains(p) {
			return true
		}
	}
	return false
}

// ParseList turns a LIST string ("1,4-6,9") into a Selector.
//
// Go idiom: the multiple-return-value pattern `(T, error)`. There are no
// exceptions in Go; functions that can fail return an error as their last
// value and the caller checks `if err != nil`. This is the equivalent of
// Python's `raise ValueError(...)`, just made explicit at every call site.
func ParseList(list string) (Selector, error) {
	if strings.TrimSpace(list) == "" {
		return nil, errors.New("empty field/character list")
	}

	var sel Selector
	for _, item := range strings.Split(list, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, fmt.Errorf("empty range in list %q", list)
		}

		r, err := parseItem(item)
		if err != nil {
			return nil, err
		}
		sel = append(sel, r)
	}
	return sel, nil
}

// parseItem parses a single LIST item (the part between commas).
func parseItem(item string) (rng, error) {
	// A range if it contains a dash that isn't a leading "-M" minus... actually
	// in cut the dash is always the range operator, so we split on the first
	// '-' and treat empty sides as open ends.
	if !strings.Contains(item, "-") {
		// Single position: "N".
		n, err := parsePos(item)
		if err != nil {
			return rng{}, err
		}
		return rng{lo: n, hi: n}, nil
	}

	parts := strings.SplitN(item, "-", 2)
	loStr, hiStr := parts[0], parts[1]

	if strings.Contains(hiStr, "-") {
		return rng{}, fmt.Errorf("invalid range %q (too many dashes)", item)
	}

	switch {
	case loStr == "" && hiStr == "": // bare "-"
		return rng{}, fmt.Errorf("invalid range %q", item)

	case loStr == "": // "-M" => 1..M
		hi, err := parsePos(hiStr)
		if err != nil {
			return rng{}, err
		}
		return rng{lo: 1, hi: hi}, nil

	case hiStr == "": // "N-" => N..end (hi==0 sentinel)
		lo, err := parsePos(loStr)
		if err != nil {
			return rng{}, err
		}
		return rng{lo: lo, hi: 0}, nil

	default: // "N-M"
		lo, err := parsePos(loStr)
		if err != nil {
			return rng{}, err
		}
		hi, err := parsePos(hiStr)
		if err != nil {
			return rng{}, err
		}
		if lo > hi {
			return rng{}, fmt.Errorf("invalid decreasing range %q", item)
		}
		return rng{lo: lo, hi: hi}, nil
	}
}

// parsePos parses a single 1-based position, rejecting 0 and negatives the way
// GNU cut does ("fields are numbered from 1").
func parsePos(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q", s)
	}
	if n < 1 {
		return 0, fmt.Errorf("position %d is not allowed; positions are numbered from 1", n)
	}
	return n, nil
}
