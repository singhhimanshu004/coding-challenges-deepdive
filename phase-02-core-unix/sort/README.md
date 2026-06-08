# sort

> **Phase:** 2 — Core Unix: Text Processing
> **Difficulty:** 🔵
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** M

**Status:** ✅ Done

> 🆕 **New to Go?** Read the project's [Go Quickstart for a Python Developer](../../docs/go-quickstart.md)
> first — it explains slices, structs, `error` returns, interfaces, `defer`, and
> the `container/heap` pattern by mapping each one to the Python you already know.
> Every Go idiom used below is cross-referenced there.

---

## 🎯 What We're Building

`sort` reads lines of text and writes them back **in order**. That sounds
trivial until you notice all the questions hiding in the word "order":

- Order by *text* or by *number*? (`"10"` comes before `"2"` as text, but after
  it as a number.)
- Ascending or descending? (`-r`)
- Keep duplicates or collapse them? (`-u`)
- Case-sensitive or not? (`-f`)
- Order by the whole line or by one *column*? (`-k` / `-t`)
- And the big one: **what if the file is larger than your RAM?**

```console
$ printf 'banana\napple\ncherry\n' | sort
apple
banana
cherry

$ printf '10\n2\n100\n21\n' | sort -n        # numeric, not text
2
10
21
100

$ printf 'alice 30\nbob 25\ncarol 40\n' | sort -k2 -n   # sort on column 2
bob 25
alice 30
carol 40
```

If you come from Python, the in-memory core is essentially
`sorted(lines, key=..., reverse=...)`. The interesting part — the part worth
building yourself — is everything *around* that: faithful comparators and an
**external merge sort** for inputs too big to fit in `sorted()` at all.

Supported flags: `-r` (reverse), `-n` (numeric), `-u` (unique), `-f` (fold
case), `-k FIELD` / `-t SEP` (key field + delimiter), plus two teaching knobs —
`--external` (force the on-disk path) and `--chunk-lines N` (tune run size).

## 📚 Core Concepts

### 1. A sort is just a comparator applied many times

Every sorting algorithm needs exactly one primitive: **"does line *a* come
before line *b*?"** Get that one function right and quicksort, mergesort, and a
heap merge all produce the same order. So the whole tool's *behaviour* lives in
one place — [`compare.go`](compare.go) — and the algorithms are
interchangeable plumbing.

Our comparator returns `-1 / 0 / +1` (the ascending comparison). From that:

- `-r` just **flips the sign**.
- `-u` uses the **`0` case**: two lines are duplicates when they compare equal.
- `-n` changes how the key is **interpreted** (number vs text).
- `-k` changes **which substring** is the key.

### 2. Numeric vs lexicographic

Text comparison is character-by-character: `"100" < "2"` because `'1' < '2'`.
Numeric comparison parses the leading number and compares values, so
`100 > 2`. Our `leadingNumber` parser is deliberately *forgiving* — like real
`sort`, `"3.5kg"` parses as `3.5` and `"abc"` parses as `0` — so a numeric sort
never blows up on messy data.

### 3. Stable vs unstable sorting

A sort is **stable** if elements that compare *equal* keep their original input
order. This matters the moment you sort on a *key*: if you sort `alice 30` and
`carol 30` by column 2, both have key `30` — a stable sort leaves them in input
order; an unstable one may swap them for no reason.

We use Go's `sort.SliceStable` (not `sort.Slice`) precisely so keyed and folded
sorts are predictable. The external path then goes to extra lengths (a run-index
tie-break in the merge heap) to stay stable too, so **both paths produce
byte-identical output**.

### 4. Why `sort` is *not* a streaming filter

`wc`, `cat`, and `cut` emit output as they read — they never need the whole
file. `sort` is different: it **cannot print the first line until it has seen
the last one**, because the last line read might belong at the very top. That
single fact is what forces it to buffer all input, and what makes the
"bigger-than-RAM" problem real.

## 🏗️ Architecture & Design

The code splits along the seams above — comparators, in-memory sort, and the
external path never bleed into each other:

| File | Responsibility |
| --- | --- |
| [`main.go`](main.go) | CLI wiring: read operands/stdin, pick a path, set exit codes. |
| [`args.go`](args.go) | Hand-rolled flag parsing (bundled short flags, `-k`/`-t` values, long flags). |
| [`compare.go`](compare.go) | **The comparator** — key extraction, numeric/text/fold logic. Shared by *both* paths. |
| [`memsort.go`](memsort.go) | In-memory path + line I/O helpers (read, stable sort, dedupe, write). |
| [`external.go`](external.go) | **External merge sort**: split → sort runs → k-way heap merge. |

```
                ┌───────────────┐
   argv ──────▶ │   args.go     │ ── config + files
                └───────────────┘
                        │
                        ▼
   stdin/files ─▶ ┌───────────┐   small?   ┌──────────────┐
                  │  main.go  │ ─────────▶ │  memsort.go  │ ─▶ stdout
                  └───────────┘            └──────────────┘
                        │  --external / too big      ▲
                        ▼                             │ same comparator
                  ┌──────────────┐                    │
                  │ external.go  │ ───────────────────┘
                  └──────────────┘
```

## 🌍 The Star of the Show: External Merge Sort

This is the concept the challenge is really about, so it gets its own section.

### The problem

You have a **100 GB** log file and a **16 GB** machine. `sorted(open(file))`
(or our in-memory path) tries to hold every line at once and dies with an
out-of-memory error. You physically cannot fit the data in RAM — yet the file
*does* fit on disk. External sorting is the family of algorithms that sort data
**too big for memory** by using the disk as scratch space. It's exactly what
databases do for a huge `ORDER BY`, and what GNU `sort` itself does under the
hood.

### The algorithm: split → sort → merge

```
        INPUT (too big for RAM)
        ┌───────────────────────────┐
        │ 8 3 9 1 5 2 7 0 4 6 ...    │
        └───────────────────────────┘
                    │  (1) SPLIT into chunks that DO fit in RAM
                    ▼
        ┌────────┐ ┌────────┐ ┌────────┐
        │ 8 3 9  │ │ 1 5 2  │ │ 7 0 4  │   each chunk = chunkLines lines
        └────────┘ └────────┘ └────────┘
                    │  (2) SORT each chunk in memory, write to a temp "run"
                    ▼
   disk:  run0:[3 8 9]   run1:[1 2 5]   run2:[0 4 7]   ← each run sorted
                    │  (3) MERGE all runs at once (k-way merge)
                    ▼
        ┌───────────────────────────┐
        │ 0 1 2 3 4 5 7 8 9 ...      │   OUTPUT, fully sorted
        └───────────────────────────┘
```

1. **Split.** Read the input `chunkLines` at a time. Each chunk is small enough
   to fit in memory. (See `splitIntoSortedRuns` in `external.go`.)
2. **Sort each run.** Sort the chunk with the *same* in-memory comparator and
   write it to a temporary file on disk. Each run is now internally sorted.
3. **K-way merge.** Open **all** the runs at once. Repeatedly take the smallest
   line across the *fronts* of the runs and emit it; when you take a line from a
   run, pull that run's next line into the contest. Only **one line per run** is
   in memory at any moment, so peak memory is *O(number of runs)*, not
   *O(total lines)*. That is the whole trick.

### Why a min-heap (`container/heap`)?

The merge step constantly asks "which of these *k* candidate lines is the
smallest?" A naive scan of all *k* fronts is *O(k)* per output line. A
**min-heap** answers it in *O(log k)*, so merging *N* lines costs
*O(N log k)* — the same asymptotics as a normal mergesort.

We hand-roll the heap with Go's `container/heap` (see `mergeHeap` in
`external.go`) rather than using `sort` for the merge, because the data never
lives in one slice — it trickles in from *k* files. Two Go-specific notes for a
Python reader:

- `container/heap` is **not generic**. You implement the `heap.Interface`
  methods (`Len`, `Less`, `Swap`, `Push`, `Pop`) on your own slice type and the
  package supplies the sift-up/sift-down logic. (Python's `heapq` instead takes
  a plain list and tuples for ordering — same idea, different ergonomics.)
- Our heap's `Less` reuses the **exact same comparator** as the in-memory sort,
  so `-r`, `-n`, and `-k` "just work" in the merge — for a reverse sort, the
  "heap minimum" is simply the largest line. A tie-break on run index keeps the
  merge **stable**, matching the in-memory path line-for-line.

### When does it actually matter?

| Situation | Path |
| --- | --- |
| File fits comfortably in RAM | In-memory (`sort.SliceStable`) — simpler and faster. |
| File is larger than RAM (or you want bounded memory) | External merge sort. |

Real `sort` decides automatically from a memory budget (its `-S` option). Here
we expose `--external` and `--chunk-lines N` so you can **force and shrink** the
external path and watch it produce many runs — which is exactly what the
forced-external test does with `--chunk-lines 4` over 50 numbers.

## 🔨 Step-by-Step Implementation

1. **Parse flags** (`args.go`): support bundled short flags (`-rn`), `-k`/`-t`
   values (attached `-k2` or separated `-k 2`), and the long teaching flags.
2. **Build the comparator** (`compare.go`): key extraction → numeric/text →
   fold → reverse. This is the only file that knows about *order*.
3. **In-memory path** (`memsort.go`): read all lines, `sort.SliceStable`,
   dedupe adjacent equals for `-u`, write out.
4. **External path** (`external.go`): split into sorted runs on disk, then
   k-way merge them with a `container/heap` min-heap, deduping across run
   boundaries for `-u`.
5. **Wire it up** (`main.go`): choose the path, handle file operands / stdin /
   `-`, and return Unix-style exit codes (`0` ok, `1` read error, `2` usage).

## 🧪 Testing Strategy

Run the suite and the static checker:

```console
$ go test ./...
ok      ccsort
$ go vet ./...
```

What the tests in [`sort_test.go`](sort_test.go) cover:

- **Lexicographic** default, **`-r`**, **`-n`**, **`-n -r`**, **`-u`**, **`-f`**,
  **`-f -u`**, and **`-k`/`-t`** key fields.
- **stdin** fallback and **empty input** (must succeed and print nothing).
- A **forced external sort** with a tiny `--chunk-lines 4` so dozens of runs
  flow through the k-way merge, checked against an independently computed order.
- A **cross-check** that the external path equals the in-memory path across
  many flag combinations — the strongest possible correctness guarantee.
- The forgiving **numeric prefix parser** (`"3.5kg"` → `3.5`, `"abc"` → `0`).

And the real-world check — diffing against the system tool, including numeric
input and the external path:

```console
$ diff <(./ccsort -n big.txt) <(LC_ALL=C sort -n big.txt) && echo MATCH
MATCH
$ diff <(./ccsort -n --external --chunk-lines 50 big.txt) <(LC_ALL=C sort -n big.txt) && echo MATCH
MATCH
```

> 💡 Compare against `LC_ALL=C sort`. Without `LC_ALL=C`, GNU `sort` uses
> locale-aware collation (where case and punctuation are reordered), so a plain
> byte-wise comparator like ours would "disagree" for reasons that have nothing
> to do with our logic.

## 💡 Key Takeaways

- **One comparator, many algorithms.** Isolating "does a come before b?" lets
  the in-memory and external paths share *all* the ordering logic and guarantees
  they agree.
- **Numeric ≠ lexicographic**, and a robust numeric sort parses *forgivingly*.
- **Stability is a feature**, not an accident — it's what makes keyed sorts
  predictable. We chose `sort.SliceStable` and made the merge stable to match.
- **External merge sort** turns "too big for RAM" into "fits on disk": split
  into sorted runs, then k-way merge with a min-heap so peak memory is *O(k)*.
- **`container/heap`** is Go's manual-but-fast priority queue; implementing
  `heap.Interface` is the idiomatic way to get a min-heap.
- `sort` is the rare Unix filter that **must buffer** — it can't stream because
  the last line read can belong first.

## 📖 Further Reading

- Challenge spec — [Build Your Own sort](https://codingchallenges.fyi/challenges/challenge-sort)
- Knuth, *TAOCP Vol. 3* — the canonical treatment of external sorting and merging.
- Go docs — [`container/heap`](https://pkg.go.dev/container/heap) and [`sort`](https://pkg.go.dev/sort)
- GNU coreutils — [`sort` manual](https://www.gnu.org/software/coreutils/manual/html_node/sort-invocation.html) (see `-S`, `-T` for its real external-sort knobs)
- Project primer — [Go Quickstart for a Python Developer](../../docs/go-quickstart.md)
