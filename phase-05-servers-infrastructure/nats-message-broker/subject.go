package main

import "strings"

// NATS subjects are hierarchical names made of dot-separated *tokens*:
//
//	time.us.east        -> ["time", "us", "east"]
//	foo.bar             -> ["foo", "bar"]
//
// Subscriptions may use two wildcards in their subject *pattern* (a PUBlished
// subject is always concrete — wildcards only ever appear on the SUB side):
//
//	*   matches exactly ONE token at that position   (foo.*   matches foo.bar)
//	>   matches one or MORE trailing tokens; must be  (foo.>   matches foo.bar.baz)
//	    the final token in the pattern
//
// This file isolates the matching algorithm so it can be unit-tested on its
// own, away from any networking. It is the conceptual heart of the broker.

// tokenize splits a subject (or pattern) into its dot-delimited tokens.
//
// 🐍 Python note: strings.Split is Go's "subject".split("."). Unlike Python,
// Go has no separate "list" type — this returns a []string slice.
func tokenize(subject string) []string {
	return strings.Split(subject, ".")
}

// matchSubject reports whether the concrete published subject matches the
// subscription pattern. Both are dot-separated token strings.
//
// The algorithm walks the pattern tokens left to right and compares each
// against the subject token at the same position:
//
//   - ">"  : only valid as the LAST pattern token. Matches all remaining
//     subject tokens, and there must be at least one of them.
//   - "*"  : matches whatever single token sits at this position.
//   - else : must match the subject token literally.
//
// If we exhaust the pattern, it is a match only when we also exhausted the
// subject (equal length) — otherwise "foo" should not match "foo.bar".
func matchSubject(pattern, subject string) bool {
	pt := tokenize(pattern)
	st := tokenize(subject)

	for i, token := range pt {
		if token == ">" {
			// '>' is a tail wildcard: it must be the final pattern token and
			// it consumes one-or-more remaining subject tokens.
			return i == len(pt)-1 && i < len(st)
		}
		if i >= len(st) {
			// Pattern still has tokens but the subject ran out: no match.
			return false
		}
		if token == "*" {
			continue // single-token wildcard: accept any token here.
		}
		if token != st[i] {
			return false // literal token mismatch.
		}
	}

	// Pattern fully consumed: match iff the subject was fully consumed too.
	return len(pt) == len(st)
}
