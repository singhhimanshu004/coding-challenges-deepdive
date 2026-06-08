# diff

> **Phase:** 2 — Core Unix: Text Processing
> **Difficulty:** 🔵
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** M

**Status:** ✅ Done

> 🆕 **New to Go?** Read the project's [Go Quickstart for a Python Developer](../../docs/go-quickstart.md)
> first — it explains slices, structs, `iota` enums, `error` returns, and
> `strings.Builder` by mapping each one to the Python you already know. Every Go
> idiom used below is cross-referenced there.

> 🏁 **Phase capstone.** Together with `grep`, this is the conceptual peak of
> Phase 2: `grep` is pattern matching, `diff` is the **Longest Common
> Subsequence** algorithm. If you understand the dynamic-programming table below,
> you understand the engine behind `git diff`, code review tools, and `patch`.

---

## 🎯 What We're Building

`diff` compares two text files **line by line** and prints the smallest set of
edits that turns the first file into the second. It is the tool behind every
code review, every `git diff`, and every `patch`.

```console
$ diff -u old.txt new.txt
--- old.txt
+++ new.txt
@@ -1,7 +1,7 @@
 apple
-banana
+blueberry
 cherry
 date
 elder
 fig
-grape
+kiwi
```

A line prefixed with `-` was **removed**, `+` was **added**, and a leading space
means the line is **unchanged context**. Our implementation supports three
output styles:

| Flag      | Format    | Looks like                          |
| --------- | --------- | ----------------------------------- |
| (none)    | normal    | `2c2` / `< old` / `---` / `> new`   |
| `-u`      | unified   | `@@ -l,s +l,s @@` with `+`/`-`/space |
| `-c`      | context   | `*** / ---` blocks with `!` markers |
| `-U n`    | unified   | unified with `n` context lines      |

If you come from Python, think of the input as `text.split("\n")` and the job as
finding the cheapest list of insert/delete operations between two lists.

## 📚 Core Concepts

### The real question: what stayed the same?

The naive idea — "compare line 1 to line 1, line 2 to line 2" — falls apart the
moment a single line is inserted, because **everything after it shifts down** and
now mismatches. The trick the real `diff` uses is to flip the question around:

> Find the **longest sequence of lines that appears, in order, in both files.**
> Everything in that sequence is "unchanged". Everything *not* in it must be a
> deletion (from A) or an insertion (into B).

That maximal shared backbone is the **Longest Common Subsequence (LCS)**. A
*subsequence* keeps order but need not be contiguous: the LCS of `A B C D` and
`A C D` is `A C D`, so the only edit is "delete `B`".

> **Subsequence vs. substring:** a substring is contiguous (`B C`); a
> subsequence just preserves order (`A C D`). diff wants the longest
> *subsequence*.

### Computing the LCS with dynamic programming

We build a table `L`, sized `(len(A)+1) × (len(B)+1)`, where `L[i][j]` is the
length of the LCS of the first `i` lines of A and the first `j` lines of B. The
extra zero row/column represents "compared against nothing".

The recurrence is just three rules:

```
L[i][j] = 0                                  if i == 0 or j == 0
L[i][j] = L[i-1][j-1] + 1                     if A[i-1] == B[j-1]
L[i][j] = max(L[i-1][j], L[i][j-1])           otherwise
```

In words: if the two current lines match, the LCS grows by one and we look at
the smaller sub-problem diagonally up-left. If they don't match, the best we can
do is the better of "ignore A's line" (look up) or "ignore B's line" (look left).

### A worked matrix

Let `A = [A, B, C, D, E]` and `B = [A, C, E]`. Filling the table row by row:

```
            ""   A    C    E       (B, the columns)
       ""    0   0    0    0
        A    0   1    1    1
        B    0   1    1    1
        C    0   1    2    2
        D    0   1    2    2
        E    0   1    2    3
      (A, the rows)
```

The bottom-right cell is **3** — the LCS is `A C E`, length 3. Notice how a `+1`
only appears on a diagonal step where the labels match (A=A, C=C, E=E).

### Backtracking the table into edits

Knowing the *length* isn't enough; we need the actual edits. So we walk the
table **backwards** from the bottom-right corner, retracing the choice each cell
made:

```
start at L[5][3]
  A[i-1]==B[j-1]?  → diagonal step, emit  =  (unchanged line)
  else if L[i][j-1] >= L[i-1][j] → left step, emit + (B[j-1] inserted)
  else                            → up step,   emit - (A[i-1] deleted)
stop when both i and j reach 0
```

Tracing our example backwards produces (then reversed into file order):

```
= A      (diagonal)
- B      (A had B, B did not)
= C      (diagonal)
- D      (A had D, B did not)
= E      (diagonal)
```

So: keep `A`, delete `B`, keep `C`, delete `D`, keep `E`. That ordered list is
the **edit script**, the universal intermediate form every output format is
rendered from.

> **Why prefer "up" on ties?** When `L[i-1][j] == L[i][j-1]` either path is
> optimal, but always listing deletions before insertions in a changed region
> matches GNU diff's output, so our hunks line up with the system tool.

### The unified format, field by field

The unified hunk header is the part that looks cryptic:

```
@@ -1,7 +1,7 @@
   │  │ │  │ │
   │  │ │  │ └─ length: 7 lines from FILE B appear in this hunk
   │  │ │  └─── start:  hunk begins at line 1 of FILE B
   │  │ └────── '+' marks the second (new) file's range
   │  └──────── length: 7 lines from FILE A appear in this hunk
   └─────────── '-' marks the first (old) file's range, starting at line 1
```

So `@@ -1,7 +1,7 @@` reads: "lines 1–7 of the old file correspond to lines 1–7
of the new file in this chunk." When a side has exactly **one** line the `,len`
is dropped (`@@ -2 +2 @@`); when a side has **zero** lines (a pure insertion or
deletion) the start is the line number *just before* the change, e.g. `-1,0`.

Below the header, each line carries a one-character prefix:

| Prefix | Meaning                          |
| ------ | -------------------------------- |
| ` `    | context — unchanged, in both     |
| `-`    | removed from the old file        |
| `+`    | added in the new file            |

**Context and hunks.** A unified diff shows a few unchanged lines (default
**3**) around each change so a human — and `patch` — can locate it even if line
numbers have drifted. Changes whose context windows touch or overlap are merged
into a single hunk; changes far apart get separate `@@` hunks. `-U n` tunes the
context width (`-U0` shows none).

## 🏗️ Architecture & Design

The code is a clean pipeline: **read → LCS → edit script → format**, split so
each idea lives in its own file.

```
main.go         CLI: flag parsing, file reading, exit codes, stdin ("-")
lcs.go          the LCS dynamic-programming table (the algorithm, from scratch)
editscript.go   backtracking the table into an ordered []edit
format.go       three renderers: normal, unified (-u), context (-c)
diff_test.go    table-driven tests + end-to-end CLI tests
```

The **edit script** (`[]edit`, defined in `editscript.go`) is the seam in the
middle: the algorithm half produces it, the formatting half consumes it. Each
`edit` records its kind (`opEqual` / `opDelete` / `opInsert`), the line text,
and its 0-based index in each file so the formatters can compute hunk ranges
without re-deriving anything.

> **Go idiom:** the operation kind is a named-int "enum" generated with `iota`
> (see `editscript.go`) — Go's idiomatic stand-in for Python's `enum.Enum`.

## 🔨 Step-by-Step Implementation

1. **Read both files into `[]string`** (`readLines` in `main.go`). A trailing
   newline does not create a spurious empty final line, and `-` means stdin.
2. **Build the LCS table** (`lcsTable` in `lcs.go`) — the three-rule recurrence
   above, nothing more.
3. **Backtrack into an edit script** (`buildEditScript` in `editscript.go`),
   reversing the collected edits into forward file order.
4. **Short-circuit identical files** (`hasChanges`) → exit 0, no output.
5. **Render** the script in the requested format (`format.go`):
   - `normalDiff` groups consecutive changes and labels them `a`/`d`/`c`.
   - `unifiedDiff` clusters changes into context-padded hunks and emits `@@`.
   - `contextDiff` emits the older `***`/`---` block format.

## 🧪 Testing Strategy

`diff_test.go` is table-driven and covers the cases that actually break naive
implementations:

- **Identical files** → no changes, exit 0.
- **Pure insertion** and **pure deletion** → exactly one `+` or one `-`.
- **Mixed change** → exact `2c2 / < / --- / >` normal output.
- **Empty file vs. non-empty** → `0a1,2` add-at-top behaviour.
- **Unified hunk correctness** → exact `@@ -1,7 +1,7 @@` headers, including the
  pure-insertion `-1,2 +1,3` case, the single-line `-2 +2` shorthand under
  `-U0`, and **two separate hunks** when changes are far apart.
- **CLI end to end** → exit codes 0/1/2, missing-file errors, and stdin via `-`.

```console
$ go test ./...
ok   diff

$ go vet ./...
```

It was also verified by eye against the system tool — `diff -u` output structure
(headers, hunk ranges, prefixes) matches GNU/BSD `diff` on real multi-line files.

### Exit codes

| Code | Meaning                          |
| ---- | -------------------------------- |
| `0`  | files are identical              |
| `1`  | files differ                     |
| `2`  | trouble (bad flag, missing file) |

These mirror real `diff`, so the tool composes correctly in shell scripts
(`if diff -u a b; then ...`).

## 💡 Key Takeaways

- **diff is an LCS problem.** Find the longest in-order shared backbone; the rest
  is forced inserts and deletes. This single idea powers `git`, code review, and
  `patch`.
- **Dynamic programming** turns an exponential search ("try every subsequence")
  into an `O(n·m)` table fill plus an `O(n+m)` backtrack.
- **An intermediate edit script** decouples the algorithm from presentation, so
  three output formats share one engine.
- **Unified-format hunks** exist to make diffs *applyable* and human-locatable:
  the `@@ -l,s +l,s @@` ranges plus context lines are what `patch` reads.
- **Go specifics learned:** 2-D slices (`[][]int`) for the DP table, `iota`
  enums for operation kinds, and `strings.Builder` for cheap output assembly.

## 📖 Further Reading

- Challenge spec — [Build Your Own diff](https://codingchallenges.fyi/challenges/challenge-diff/)
- Hunt–McIlroy, *An Algorithm for Differential File Comparison* (the original
  Unix `diff` paper)
- Eugene Myers, *An O(ND) Difference Algorithm and Its Variations* (what modern
  `git diff` actually uses — a faster refinement of the same idea)
- [Longest Common Subsequence (Wikipedia)](https://en.wikipedia.org/wiki/Longest_common_subsequence)
- GNU diffutils manual — [Unified Format](https://www.gnu.org/software/diffutils/manual/html_node/Unified-Format.html)
- Project primer — [Go Quickstart for a Python Developer](../../docs/go-quickstart.md)
