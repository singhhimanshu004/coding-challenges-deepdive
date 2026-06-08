package main

// This file implements the heart of diff: the Longest Common Subsequence (LCS)
// of two sequences of lines, computed with dynamic programming — no external
// library. Once we know the LCS we can read off exactly which lines were added,
// removed, or kept unchanged.
//
// Go idiom note for a Python dev:
//   - There is no built-in 2D list. We build a "matrix" as a [][]int — a slice
//     whose elements are themselves slices. Python's `[[0]*w for _ in range(h)]`
//     becomes the explicit loop in newMatrix below.
//   - Slices are reference types (like Python lists), so passing them to a
//     function does NOT copy the backing data — handy and cheap.

// lcsTable builds the classic LCS dynamic-programming table for sequences a and
// b. table[i][j] holds the length of the LCS of the first i lines of a and the
// first j lines of b.
//
// Recurrence (the whole algorithm in three lines):
//
//	if a[i-1] == b[j-1]:  table[i][j] = table[i-1][j-1] + 1
//	else:                 table[i][j] = max(table[i-1][j], table[i][j-1])
//
// Row 0 and column 0 stay 0 (an empty sequence shares nothing), which is why we
// size the table (len(a)+1) x (len(b)+1) and index the inputs with i-1 / j-1.
func lcsTable(a, b []string) [][]int {
	n, m := len(a), len(b)

	// (n+1) x (m+1) grid initialised to zero. Go zeroes memory for us, so we
	// only need to allocate — no explicit fill of the first row/column.
	table := make([][]int, n+1)
	for i := range table {
		table[i] = make([]int, m+1)
	}

	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			if a[i-1] == b[j-1] {
				table[i][j] = table[i-1][j-1] + 1
			} else {
				// Go has no ternary operator; a small helper keeps this readable.
				table[i][j] = max2(table[i-1][j], table[i][j-1])
			}
		}
	}
	return table
}

// max2 returns the larger of two ints. (Go 1.21+ ships a built-in `max`, but we
// spell it out so the algorithm is self-contained and obvious.)
func max2(x, y int) int {
	if x > y {
		return x
	}
	return y
}
