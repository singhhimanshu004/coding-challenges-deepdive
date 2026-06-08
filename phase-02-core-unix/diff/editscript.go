package main

// This file turns the LCS table into an *edit script*: an ordered list of
// operations that transform file A into file B. Each operation is one of:
//
//	opEqual   — a line present in both files (context)
//	opDelete  — a line in A but not in B (printed with a leading '-')
//	opInsert  — a line in B but not in A (printed with a leading '+')
//
// Go idiom note for a Python dev:
//   - We model the operation kind as a small integer "enum" using `iota`.
//     Python would reach for an Enum or string constants; Go's idiomatic choice
//     is a named int type plus iota-generated constants.
//   - A struct groups the fields of one edit, like a dataclass without the
//     decorator.

// opKind enumerates the three edit operations.
type opKind int

const (
	opEqual  opKind = iota // line unchanged between A and B
	opDelete               // line removed from A
	opInsert               // line added in B
)

// edit is a single step in the script. aIndex / bIndex are 0-based line numbers
// in the respective files (one of them is unused for pure insert/delete, but we
// keep both so the formatters can compute hunk ranges easily).
type edit struct {
	kind   opKind
	text   string
	aIndex int
	bIndex int
}

// buildEditScript walks the LCS table *backwards* from the bottom-right corner
// to reconstruct the diff. This backtracking is the mirror image of how the
// table was filled:
//
//   - If the current A and B lines are equal, they belong to the LCS: emit an
//     "equal" edit and step diagonally up-left (i-1, j-1).
//   - Otherwise we moved into this cell from whichever neighbour had the larger
//     LCS length. Preferring an "up" move (deletion) when table[i-1][j] >=
//     table[i][j-1] yields the same bias GNU diff uses, so deletions are listed
//     before insertions in a changed region.
//
// Because we walk backwards we collect edits in reverse, then flip the slice so
// the caller receives them in file order.
func buildEditScript(a, b []string) []edit {
	table := lcsTable(a, b)

	var rev []edit
	i, j := len(a), len(b)

	for i > 0 || j > 0 {
		switch {
		case i > 0 && j > 0 && a[i-1] == b[j-1]:
			rev = append(rev, edit{kind: opEqual, text: a[i-1], aIndex: i - 1, bIndex: j - 1})
			i--
			j--
		case j > 0 && (i == 0 || table[i][j-1] >= table[i-1][j]):
			// Came from the left: line b[j-1] was inserted.
			rev = append(rev, edit{kind: opInsert, text: b[j-1], aIndex: i, bIndex: j - 1})
			j--
		default:
			// Came from above: line a[i-1] was deleted.
			rev = append(rev, edit{kind: opDelete, text: a[i-1], aIndex: i - 1, bIndex: j})
			i--
		}
	}

	// Reverse in place to restore forward file order.
	for l, r := 0, len(rev)-1; l < r; l, r = l+1, r-1 {
		rev[l], rev[r] = rev[r], rev[l]
	}
	return rev
}

// hasChanges reports whether an edit script contains any insert or delete. If
// not, the two files are identical and diff should exit 0 with no output.
func hasChanges(edits []edit) bool {
	for _, e := range edits {
		if e.kind != opEqual {
			return true
		}
	}
	return false
}
