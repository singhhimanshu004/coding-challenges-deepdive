package huffman

import "container/heap"

// priorityQueue is a min-heap of *node ordered by frequency, with a stable
// tie-break on insertion order. It implements heap.Interface so we can lean on
// the standard library's container/heap rather than hand-rolling sift-up/down.
//
// Why a min-heap? Huffman repeatedly needs the TWO smallest frequencies. A heap
// gives us O(log n) extract-min and insert, so building the tree over n distinct
// symbols costs O(n log n) — far better than re-scanning a list each merge.
type priorityQueue struct {
	items []*node
}

func (pq *priorityQueue) Len() int { return len(pq.items) }

func (pq *priorityQueue) Less(i, j int) bool {
	a, b := pq.items[i], pq.items[j]
	if a.freq != b.freq {
		return a.freq < b.freq
	}
	// Deterministic tie-break: lower insertion order wins. This keeps the tree
	// identical across encode and decode runs.
	return a.order < b.order
}

func (pq *priorityQueue) Swap(i, j int) {
	pq.items[i], pq.items[j] = pq.items[j], pq.items[i]
}

func (pq *priorityQueue) Push(x any) {
	pq.items = append(pq.items, x.(*node))
}

func (pq *priorityQueue) Pop() any {
	old := pq.items
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	pq.items = old[:n-1]
	return item
}

// Thin helpers so the tree-building code reads naturally.

func initHeap(pq *priorityQueue) { heap.Init(pq) }

func pushNode(pq *priorityQueue, n *node) { heap.Push(pq, n) }

func popNode(pq *priorityQueue) *node { return heap.Pop(pq).(*node) }
