// external.go — the external (on-disk) merge-sort path.
//
// Why this exists
// ---------------
// The in-memory path loads every line into one slice. That is fine until the
// input is bigger than RAM — sorting a 100 GB log on a 16 GB machine cannot hold
// all the lines at once. External merge sort is the classic answer, and the same
// algorithm a database uses for huge ORDER BYs:
//
//  1. SPLIT  — read the input in bounded chunks (chunkLines at a time).
//
//  2. SORT   — sort each chunk in memory (cheap, it fits) and write it to a
//     temporary "run" file on disk. Each run is individually sorted.
//
//  3. MERGE  — open all the runs at once and do a k-way merge: repeatedly take
//     the smallest line across the run fronts and emit it. Only one line
//     per run is in memory at a time, so peak memory is O(number of
//     runs), not O(total lines).
//
//     input            sorted runs on disk            merged output
//     ┌────────┐        ┌──────┐ ┌──────┐ ┌──────┐
//     │ 8 3 9  │  split │ run0 │ │ run1 │ │ run2 │   k-way merge
//     │ 1 5 2  │ ─────▶ │ 3 8 9│ │ 1 2 5│ │ 0 4 7│ ───────────────▶ 0 1 2 3 4 5 ...
//     │ 7 0 4  │  +sort └──────┘ └──────┘ └──────┘   (min-heap picks
//     └────────┘                                       the next line)
//
// The merge step uses a min-heap (container/heap) keyed by the same comparator
// as everything else, so reverse / numeric / keyed sorts merge correctly too —
// for a reverse sort the "heap minimum" is simply the largest line.
package main

import (
	"bufio"
	"container/heap"
	"fmt"
	"io"
	"os"
)

// externalSort streams r through the split → sort → merge pipeline and writes
// the fully ordered result to w. It never holds more than chunkLines lines in
// memory at once (plus one line per open run during the merge).
func externalSort(cfg config, r io.Reader, w io.Writer) error {
	runs, err := splitIntoSortedRuns(cfg, r)
	// Always clean up the temp files, even on error.
	defer func() {
		for _, name := range runs {
			os.Remove(name)
		}
	}()
	if err != nil {
		return err
	}

	return mergeRuns(cfg, runs, w)
}

// splitIntoSortedRuns reads r in chunks of cfg.chunkLines, sorts each chunk in
// memory, and writes it to a temporary run file. It returns the run file paths.
func splitIntoSortedRuns(cfg config, r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var runs []string
	chunk := make([]string, 0, cfg.chunkLines)

	flush := func() error {
		if len(chunk) == 0 {
			return nil
		}
		// Reuse the in-memory comparator so every run is sorted identically.
		sorted := sortLines(cfg, chunk)
		name, err := writeRun(sorted)
		if err != nil {
			return err
		}
		runs = append(runs, name)
		chunk = make([]string, 0, cfg.chunkLines)
		return nil
	}

	for scanner.Scan() {
		chunk = append(chunk, scanner.Text())
		if len(chunk) >= cfg.chunkLines {
			if err := flush(); err != nil {
				return runs, err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return runs, err
	}
	if err := flush(); err != nil {
		return runs, err
	}
	return runs, nil
}

// writeRun creates a temp file in the working directory and writes the sorted
// lines to it, one per line. We keep scratch files local (not the system temp
// dir) so they are easy to spot and are covered by .gitignore.
func writeRun(lines []string) (string, error) {
	f, err := os.CreateTemp(".", "sort-run-*.tmp")
	if err != nil {
		return "", err
	}
	defer f.Close()

	bw := bufio.NewWriter(f)
	for _, line := range lines {
		if _, err := fmt.Fprintln(bw, line); err != nil {
			return f.Name(), err
		}
	}
	if err := bw.Flush(); err != nil {
		return f.Name(), err
	}
	return f.Name(), nil
}

// runReader wraps an open run file with a buffered scanner so we can peek one
// line at a time during the merge.
type runReader struct {
	file    *os.File
	scanner *bufio.Scanner
}

func openRun(name string) (*runReader, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	return &runReader{file: f, scanner: sc}, nil
}

// next returns the next line and whether one was available.
func (rr *runReader) next() (string, bool) {
	if rr.scanner.Scan() {
		return rr.scanner.Text(), true
	}
	return "", false
}

func (rr *runReader) close() { rr.file.Close() }

// mergeRuns performs the k-way merge of the sorted run files into w, applying -u
// across run boundaries if requested.
func mergeRuns(cfg config, runs []string, w io.Writer) error {
	// Degenerate cases: nothing to merge, or a single run is already sorted.
	if len(runs) == 0 {
		return nil
	}

	readers := make([]*runReader, 0, len(runs))
	defer func() {
		for _, rr := range readers {
			rr.close()
		}
	}()

	// Seed the heap with the first line of every run.
	h := &mergeHeap{cfg: cfg}
	for ri, name := range runs {
		rr, err := openRun(name)
		if err != nil {
			return err
		}
		readers = append(readers, rr)
		if line, ok := rr.next(); ok {
			h.items = append(h.items, mergeItem{line: line, run: ri})
		}
	}
	heap.Init(h)

	out := bufio.NewWriter(w)
	defer out.Flush()

	havePrev := false
	var prev string
	for h.Len() > 0 {
		// Pop the smallest line across all run fronts.
		item := heap.Pop(h).(mergeItem)

		// -u: skip a line that compares equal to the one we just emitted.
		emit := true
		if cfg.unique && havePrev && cfg.compare(prev, item.line) == 0 {
			emit = false
		}
		if emit {
			if _, err := fmt.Fprintln(out, item.line); err != nil {
				return err
			}
			prev = item.line
			havePrev = true
		}

		// Refill the heap from the run we just drew from.
		if line, ok := readers[item.run].next(); ok {
			heap.Push(h, mergeItem{line: line, run: item.run})
		}
	}
	return nil
}

// mergeItem is one entry in the merge heap: a line plus the run it came from
// (so we know which run to pull the next line from after popping).
type mergeItem struct {
	line string
	run  int
}

// mergeHeap is a min-heap of mergeItems ordered by the sort comparator.
//
// Go idiom: container/heap is not generic — you implement heap.Interface
// (sort.Interface plus Push/Pop) on your own slice type, and the package
// supplies the sift-up/sift-down logic. Holding the whole config lets us reuse
// the same comparator the rest of sort uses instead of hard-coding string order.
type mergeHeap struct {
	items []mergeItem
	cfg   config
}

func (h mergeHeap) Len() int { return len(h.items) }

// Less keeps the merge globally stable. When two lines compare equal, we break
// the tie by run index: runs are produced in input order, so the earlier run
// holds the earlier input line. Without this tie-break the heap could emit equal
// lines in run-arrival order and disagree with the stable in-memory sort.
func (h mergeHeap) Less(i, j int) bool {
	a, b := h.items[i], h.items[j]
	c := h.cfg.compare(a.line, b.line)
	if h.cfg.reverse {
		c = -c
	}
	if c == 0 {
		return a.run < b.run
	}
	return c < 0
}

func (h mergeHeap) Swap(i, j int) { h.items[i], h.items[j] = h.items[j], h.items[i] }

// Push/Pop operate on the slice's tail; heap.Push/heap.Pop wrap these with the
// sift logic that keeps the heap invariant. Note the pointer receiver: these
// mutate the slice, so they must be methods on *mergeHeap.
func (h *mergeHeap) Push(x any) {
	h.items = append(h.items, x.(mergeItem))
}

func (h *mergeHeap) Pop() any {
	old := h.items
	n := len(old)
	item := old[n-1]
	h.items = old[:n-1]
	return item
}
