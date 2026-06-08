// Package huffman implements Huffman coding: frequency analysis, greedy tree
// construction via a min-heap priority queue, code-table derivation, and the
// compress/decompress round-trip with a self-describing file header.
//
// The big idea: assign SHORT bit-codes to FREQUENT symbols and LONG codes to
// rare ones. Because the codes form a prefix code (no code is a prefix of
// another), the packed bit-stream decodes unambiguously without separators.
// Huffman's greedy "merge the two least-frequent nodes" strategy is provably
// optimal for symbol-by-symbol coding.
package huffman

// node is one vertex of the Huffman tree. Leaves carry a real byte value;
// internal nodes only carry the combined frequency of their subtree.
type node struct {
	symbol byte // valid only when leaf == true
	freq   uint64
	leaf   bool
	left   *node
	right  *node

	// order is a stable, monotonically increasing insertion id used purely as
	// a deterministic tie-breaker in the heap. Without it, two nodes of equal
	// frequency could pop in an unspecified order, producing different (though
	// still valid) trees on encode vs. decode. Since we rebuild the tree from
	// the frequency table on decode, both sides MUST agree exactly — a stable
	// tie-break guarantees that.
	order int
}

// CountFrequencies tallies how often each byte value appears in data.
// The returned map only contains symbols that actually occur.
func CountFrequencies(data []byte) map[byte]uint64 {
	freqs := make(map[byte]uint64)
	for _, b := range data {
		freqs[b]++
	}
	return freqs
}

// buildTree constructs the Huffman tree from a frequency table using a min-heap
// priority queue. It repeatedly removes the two lowest-frequency nodes and
// merges them under a new parent, until a single root remains.
//
// Special cases:
//   - 0 symbols (empty input)  -> returns nil; there is nothing to encode.
//   - 1 distinct symbol        -> a lone leaf. Callers must give it a 1-bit
//     code by convention, since a tree of depth 0 yields an empty code.
func buildTree(freqs map[byte]uint64) *node {
	if len(freqs) == 0 {
		return nil
	}

	pq := &priorityQueue{}
	// Leaf tie-break order MUST be independent of Go's randomized map iteration
	// order, otherwise encode and decode could build different (valid) trees and
	// fail to round-trip. We key leaves on their byte value (0..255). Internal
	// nodes then take ids starting at 256, in their deterministic creation order.
	for sym, f := range freqs {
		pq.items = append(pq.items, &node{symbol: sym, freq: f, leaf: true, order: int(sym)})
	}
	order := 256
	// Heapify the leaves, then merge pairs until one root remains.
	initHeap(pq)

	if pq.Len() == 1 {
		return popNode(pq)
	}

	for pq.Len() > 1 {
		a := popNode(pq)
		b := popNode(pq)
		parent := &node{
			freq:  a.freq + b.freq,
			left:  a,
			right: b,
			order: order,
		}
		order++
		pushNode(pq, parent)
	}
	return popNode(pq)
}

// buildCodes walks the tree assigning '0' for left edges and '1' for right
// edges, producing a symbol -> bit-string table. The single-symbol tree is
// handled specially: its sole leaf gets the code "0".
func buildCodes(root *node) map[byte]string {
	codes := make(map[byte]string)
	if root == nil {
		return codes
	}
	if root.leaf {
		codes[root.symbol] = "0"
		return codes
	}
	var walk func(n *node, prefix string)
	walk = func(n *node, prefix string) {
		if n.leaf {
			codes[n.symbol] = prefix
			return
		}
		walk(n.left, prefix+"0")
		walk(n.right, prefix+"1")
	}
	walk(root, "")
	return codes
}
