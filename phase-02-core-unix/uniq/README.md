# uniq

> **Phase:** 2 — Core Unix: Text Processing
> **Difficulty:** 🟢
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** S

**Status:** ✅ Done

> 🐍➡️🐹 **New to Go?** Read the project's
> [**Go Quickstart for a Python Developer**](../../docs/go-quickstart.md) first.
> It maps every Go idiom used here (`defer`, `bufio`, structs, error returns)
> back to the Python you already know. This README assumes you've skimmed it.

---

## 🎯 What We're Building

`uniq` is the Unix tool that **collapses repeated lines**. Give it text and it
prints each line once — but with one crucial twist that trips up almost everyone
the first time:

> **uniq only removes duplicates that are _next to each other_ (adjacent).**

That's it. That single rule explains everything else about the tool. Here's the
classic surprise:

```
$ printf "apple\napple\nbanana\napple\n" | uniq
apple
banana
apple        ← the second "apple" survives!
```

The two `apple` lines separated by `banana` are **not adjacent**, so uniq keeps
both. If you wanted *true* deduplication, you'd `sort` first (more on that
below).

Our version supports the flags you reach for most often:

| Flag | Meaning |
| ---- | ------- |
| `-c` | **count** — prefix each line with how many times it occurred |
| `-d` | **duplicated only** — print one copy of lines that repeated (count ≥ 2) |
| `-u` | **unique only** — print only lines that appeared exactly once |

Plus optional file arguments: `uniq [input [output]]`. With no input file (or
`-`), it reads **stdin**; with no output file, it writes **stdout** — so it
behaves in a pipeline exactly like the real thing.

---

## 📚 Core Concepts

### 1. Adjacent-duplicate semantics (the whole game)

uniq compares **line N** with **line N−1** only. It has no memory of anything
older. So duplicates are merged only when they form an unbroken run:

```
a          a            a            a
a    →     (dropped)    a   ─run─►   ▼ one "a"
b          b            b            b
a          a            a            a   ← separate run, kept
```

### 2. Why you `sort` first

Because uniq only sees adjacent lines, the idiom for *real* deduplication is:

```
sort file | uniq          # bring equal lines together, THEN collapse
sort file | uniq -c       # ...and count how many of each (a histogram!)
```

`sort` guarantees all equal lines become neighbours, which is precisely the
shape uniq needs. This `sort | uniq -c` pairing is one of the most useful
one-liners in all of Unix — instant frequency counts of anything.

### 3. Carrying one line of state across a stream

The implementation never loads the file into memory. It streams line by line and
remembers just **two values**:

- `prev` — the line of the group we're currently inside
- `count` — how many times we've seen it in a row

When a *different* line arrives, the current group is finished: we emit it, then
start a new group. This is the **run-length** pattern, and its one-line-deep
memory is *exactly why* uniq is an adjacent-only tool. (If we instead used a
`set`/`map` of every line ever seen — like Python's `seen = set()` — we'd have
built `sort -u`, a different tool with unbounded memory.)

### 4. The three modes are just a filter on each finished group

Once a group `(line, count)` is complete, deciding whether to print it is tiny:

| Mode  | Rule                            |
| ----- | ------------------------------- |
| plain | always print                    |
| `-d`  | print only if `count >= 2`      |
| `-u`  | print only if `count == 1`      |
| `-c`  | prefix the line with `count`    |

---

## 🏗️ Architecture & Design

```
stdin / file ──► bufio.Scanner ──► group loop ──► emit() filter ──► bufio.Writer ──► stdout / file
                 (one line at a    (carry prev    (-d/-u/-c)         (batched
                  time, no \n)      + count)                          writes)
```

Two source files, deliberately split so the logic is testable in isolation:

| File          | Responsibility |
| ------------- | -------------- |
| `main.go`     | CLI plumbing: parse `-c/-d/-u`, resolve input/output files vs. stdin/stdout, exit codes. `main()` stays trivial; the real work is in `run(args) int` so tests can call it directly. |
| `uniq.go`     | `uniqStream(r, w, opt)` — the streaming algorithm — and `emit()`, the per-group filter/formatter. Pure I/O on `io.Reader`/`io.Writer`, so tests feed it strings. |

**Why `io.Reader`/`io.Writer` instead of files?** It's the Go equivalent of
"program to an interface." `uniqStream` doesn't care whether the bytes come from
a file, a network socket, or an in-memory `strings.Reader` in a test — anything
that can be read works. (Python dev parallel: duck typing, but checked at compile
time.)

---

## 🔨 Step-by-Step Implementation

1. **Parse flags by hand.** A short loop over `os.Args[1:]` sets `-c/-d/-u` and
   collects positional paths. We skip the stdlib `flag` package on purpose:
   uniq lets a lone `-` mean stdin and allows flags in any position, which is
   simpler to express directly.
2. **Resolve I/O.** Open the input file or fall back to `os.Stdin`; create the
   output file or fall back to `os.Stdout`. `defer f.Close()` guarantees
   cleanup (Go's `with open(...)`).
3. **Stream with `bufio.Scanner`.** It yields one line at a time with the
   trailing `\n` stripped — the streaming twin of Python's `for line in file:`.
4. **Carry the group.** Hold `prev` + `count`; on a matching line bump the
   count, on a different line `emit()` the old group and open a new one.
5. **Flush the last group** after the loop — the final run has no "different
   line" to trigger it.
6. **`emit()` applies the mode filter** (`-d`/`-u`) and the `-c` count prefix,
   writing through a `bufio.Writer` for batched, syscall-cheap output.

---

## 🧪 Testing Strategy

`go test ./...` covers the behaviours that matter:

- `-c`, `-d`, `-u` each in isolation, plus `-c -d` combined
- **adjacent** duplicates collapse vs. **non-adjacent** duplicates kept (the
  signature semantic)
- **stdin fallback** — drives the full `run()` path through a real pipe
- edge cases: **empty input**, a **single line**, and a final line with **no
  trailing newline**

Run it:

```bash
go test ./...     # behavioral tests
go vet ./...      # static checks
```

**Differential testing against the real tool** (the confidence-builder):

```bash
go build -o uniq .
printf "apple\napple\nbanana\napple\ncherry\ncherry\n" > s.txt
diff <(./uniq -c s.txt)        <(uniq -c s.txt)        && echo OK
diff <(sort s.txt | ./uniq -c) <(sort s.txt | uniq -c) && echo OK
```

> **Note on `-c` width:** BSD/macOS `uniq -c` right-justifies the count in a
> **4-wide** field; GNU/Linux uses **7**. This build matches the local
> (BSD/macOS) system so the `diff` above is clean. Flip the `%4d` in `uniq.go`
> to `%7d` on a GNU box.

---

## 💡 Key Takeaways

- **uniq is "adjacent-only" by design** — and that's a feature, not a
  limitation. It's the streaming half of the `sort | uniq` duo.
- **One line of state is the whole trick.** Remembering `prev` + `count` is what
  makes it O(1) memory and what makes it adjacent-only. Same coin, both sides.
- **`sort | uniq -c` is a frequency counter** for anything — log lines, words,
  IPs. Internalize this one-liner.
- **Stream, don't slurp.** `bufio.Scanner` + `bufio.Writer` process files larger
  than RAM and keep syscalls cheap.
- **Separate logic from I/O** (`uniqStream` on interfaces) and keep `main()`
  thin (`run()` returns an exit code) — both pay off directly in testability.

---

## 📖 Further Reading

- 🐍➡️🐹 [Go Quickstart for a Python Developer](../../docs/go-quickstart.md) — the
  project's Go primer
- [Coding Challenges — Build Your Own uniq](https://codingchallenges.fyi/challenges/challenge-uniq)
- [Go `bufio` package](https://pkg.go.dev/bufio) — `Scanner` and `Writer`
- `man uniq` — the reference behaviour we mirror
