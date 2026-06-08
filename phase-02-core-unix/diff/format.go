package main

import (
	"fmt"
	"strings"
)

// This file renders an edit script into human/tool readable text. Three formats
// are supported:
//
//	normal   — the classic terse diff (1,3c1,3 with < and > lines)
//	unified  — `diff -u`: @@ hunks with +/- and surrounding context
//	context  — `diff -c`: the older *** / --- block format (bonus)
//
// Go idiom note for a Python dev:
//   - strings.Builder is Go's StringIO/"".join — an efficient, mutable text
//     buffer. We Fprintf into it instead of concatenating with +, which would
//     allocate a fresh string every time.

// ---------------------------------------------------------------------------
// Normal format (the default, like GNU `diff` with no flags)
// ---------------------------------------------------------------------------

// normalDiff renders the edit script in the classic format. Consecutive
// non-equal edits are grouped into one "change block" and labelled with an
// a(dd) / d(elete) / c(hange) command.
func normalDiff(edits []edit) string {
	var out strings.Builder

	i := 0
	for i < len(edits) {
		// Skip equal lines; they produce no output in normal format.
		if edits[i].kind == opEqual {
			i++
			continue
		}

		// Gather one maximal run of changes.
		start := i
		for i < len(edits) && edits[i].kind != opEqual {
			i++
		}
		block := edits[start:i]

		var dels, adds []edit
		for _, e := range block {
			if e.kind == opDelete {
				dels = append(dels, e)
			} else {
				adds = append(adds, e)
			}
		}

		switch {
		case len(dels) > 0 && len(adds) > 0: // change
			fmt.Fprintf(&out, "%sc%s\n", aRangeNormal(dels), bRangeNormal(adds))
			writeLines(&out, "< ", dels)
			out.WriteString("---\n")
			writeLines(&out, "> ", adds)
		case len(dels) > 0: // pure deletion
			// Right-hand reference is the B line preceding the deletion.
			fmt.Fprintf(&out, "%sd%d\n", aRangeNormal(dels), dels[0].bIndex)
			writeLines(&out, "< ", dels)
		default: // pure addition
			// Left-hand reference is the A line preceding the insertion.
			fmt.Fprintf(&out, "%da%s\n", adds[0].aIndex, bRangeNormal(adds))
			writeLines(&out, "> ", adds)
		}
	}

	return out.String()
}

// aRangeNormal formats the A-side line range of a set of deletes as "n" or
// "n,m" using 1-based line numbers.
func aRangeNormal(dels []edit) string {
	start := dels[0].aIndex + 1
	end := dels[len(dels)-1].aIndex + 1
	return rangeNM(start, end)
}

// bRangeNormal formats the B-side line range of a set of inserts.
func bRangeNormal(adds []edit) string {
	start := adds[0].bIndex + 1
	end := adds[len(adds)-1].bIndex + 1
	return rangeNM(start, end)
}

func rangeNM(start, end int) string {
	if start == end {
		return fmt.Sprintf("%d", start)
	}
	return fmt.Sprintf("%d,%d", start, end)
}

func writeLines(out *strings.Builder, prefix string, edits []edit) {
	for _, e := range edits {
		out.WriteString(prefix)
		out.WriteString(e.text)
		out.WriteByte('\n')
	}
}

// ---------------------------------------------------------------------------
// Unified format (`diff -u`)
// ---------------------------------------------------------------------------

// hunkRange is the half-open [start,end) span of edit indices that make up one
// unified hunk, including its surrounding context lines.
type hunkRange struct {
	start int
	end   int
}

// unifiedDiff renders the edit script as a unified diff. ctx is the number of
// context lines to keep around each change (3 is the conventional default).
// aHeader / bHeader are the "--- a" / "+++ b" lines (filename plus an optional
// trailing timestamp) printed once at the top.
func unifiedDiff(edits []edit, aHeader, bHeader string, ctx int) string {
	hunks := groupHunks(edits, ctx)
	if len(hunks) == 0 {
		return ""
	}

	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n", aHeader)
	fmt.Fprintf(&out, "+++ %s\n", bHeader)

	for _, h := range hunks {
		seg := edits[h.start:h.end]
		aStart, aLen := sideSpan(seg, true)
		bStart, bLen := sideSpan(seg, false)

		fmt.Fprintf(&out, "@@ -%s +%s @@\n",
			unifiedRange(aStart, aLen), unifiedRange(bStart, bLen))

		for _, e := range seg {
			switch e.kind {
			case opEqual:
				out.WriteByte(' ')
			case opDelete:
				out.WriteByte('-')
			case opInsert:
				out.WriteByte('+')
			}
			out.WriteString(e.text)
			out.WriteByte('\n')
		}
	}

	return out.String()
}

// groupHunks walks the edit script and clusters changes into hunks. Each change
// pulls in up to ctx equal lines on either side; clusters whose context windows
// touch or overlap are merged into a single hunk (exactly what GNU diff does).
func groupHunks(edits []edit, ctx int) []hunkRange {
	var changes []int
	for idx, e := range edits {
		if e.kind != opEqual {
			changes = append(changes, idx)
		}
	}
	if len(changes) == 0 {
		return nil
	}

	var hunks []hunkRange
	hs := clampLow(changes[0] - ctx)
	he := clampHigh(changes[0]+ctx+1, len(edits))

	for _, c := range changes[1:] {
		cs := clampLow(c - ctx)
		ce := clampHigh(c+ctx+1, len(edits))
		if cs <= he {
			// Context windows touch or overlap: extend the current hunk.
			he = ce
		} else {
			hunks = append(hunks, hunkRange{hs, he})
			hs, he = cs, ce
		}
	}
	hunks = append(hunks, hunkRange{hs, he})
	return hunks
}

// sideSpan computes the 1-based starting line and the length of one hunk on
// either the A side (aSide=true) or the B side. The A side counts equal+delete
// lines; the B side counts equal+insert lines.
func sideSpan(seg []edit, aSide bool) (start, length int) {
	for _, e := range seg {
		counts := (aSide && (e.kind == opEqual || e.kind == opDelete)) ||
			(!aSide && (e.kind == opEqual || e.kind == opInsert))
		if !counts {
			continue
		}
		if length == 0 {
			if aSide {
				start = e.aIndex + 1
			} else {
				start = e.bIndex + 1
			}
		}
		length++
	}

	// A zero-length side (pure insertion or pure deletion hunk) is reported with
	// the line number *before* the change, matching GNU diff's "-l,0" form.
	if length == 0 {
		if aSide {
			start = seg[0].aIndex
		} else {
			start = seg[0].bIndex
		}
	}
	return start, length
}

// unifiedRange formats a hunk side as "start,len", collapsing to just "start"
// when the side spans exactly one line (the conventional shorthand).
func unifiedRange(start, length int) string {
	if length == 1 {
		return fmt.Sprintf("%d", start)
	}
	return fmt.Sprintf("%d,%d", start, length)
}

func clampLow(x int) int {
	if x < 0 {
		return 0
	}
	return x
}

func clampHigh(x, hi int) int {
	if x > hi {
		return hi
	}
	return x
}

// ---------------------------------------------------------------------------
// Context format (`diff -c`) — bonus
// ---------------------------------------------------------------------------

// contextDiff renders the older context format: a "***" block showing the A
// side and a "---" block showing the B side for each hunk.
func contextDiff(edits []edit, aHeader, bHeader string, ctx int) string {
	hunks := groupHunks(edits, ctx)
	if len(hunks) == 0 {
		return ""
	}

	var out strings.Builder
	fmt.Fprintf(&out, "*** %s\n", aHeader)
	fmt.Fprintf(&out, "--- %s\n", bHeader)

	for _, h := range hunks {
		seg := edits[h.start:h.end]
		aStart, aLen := sideSpan(seg, true)
		bStart, bLen := sideSpan(seg, false)

		out.WriteString("***************\n")

		fmt.Fprintf(&out, "*** %s ****\n", contextRange(aStart, aLen))
		if hasKind(seg, opDelete) || hasKind(seg, opInsert) {
			for _, e := range seg {
				switch e.kind {
				case opEqual:
					fmt.Fprintf(&out, "  %s\n", e.text)
				case opDelete:
					if hasKind(seg, opInsert) {
						fmt.Fprintf(&out, "! %s\n", e.text)
					} else {
						fmt.Fprintf(&out, "- %s\n", e.text)
					}
				}
			}
		}

		fmt.Fprintf(&out, "--- %s ----\n", contextRange(bStart, bLen))
		for _, e := range seg {
			switch e.kind {
			case opEqual:
				fmt.Fprintf(&out, "  %s\n", e.text)
			case opInsert:
				if hasKind(seg, opDelete) {
					fmt.Fprintf(&out, "! %s\n", e.text)
				} else {
					fmt.Fprintf(&out, "+ %s\n", e.text)
				}
			}
		}
	}

	return out.String()
}

func contextRange(start, length int) string {
	if length == 0 {
		return fmt.Sprintf("%d", start)
	}
	if length == 1 {
		return fmt.Sprintf("%d", start)
	}
	return fmt.Sprintf("%d,%d", start, start+length-1)
}

func hasKind(seg []edit, k opKind) bool {
	for _, e := range seg {
		if e.kind == k {
			return true
		}
	}
	return false
}
