# head

> **Phase:** 2 — Core Unix: Text Processing
> **Difficulty:** 🟢
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** S

**Status:** ✅ Done

> 🐍→🐹 **New to Go?** Read the repo's [**Go quick-start for Python developers**](../../docs/go-quickstart.md) first. This README assumes you know Python/Java and points out the Go-specific bits as we go.

---

## 🎯 What We're Building

`head` prints the **beginning** of a file — by default the first 10 lines. It's the
mirror image of `tail`, and one of the most-used commands in any terminal: peeking
at the top of a CSV, sampling a giant log file, or sanity-checking a download.

Our version matches the [codingchallenges.fyi "build your own head"](https://codingchallenges.fyi/challenges/challenge-head/) spec and the real GNU/BSD tool:

```bash
head file.txt              # first 10 lines (the default)
head -n 5 file.txt         # first 5 lines
head -c 20 file.txt        # first 20 bytes
cat file.txt | head -n 3   # read from a pipe (stdin)
head a.txt b.txt           # multiple files, each with a "==> name <==" header
```

The single most important idea here is **early termination**: `head -n 5` on a
10 GB log file must be *instant*. We never read the whole file — we read just
enough to produce the requested output, then stop.

## 📚 Core Concepts

### 1. Early-termination streaming (the whole point)

A naïve `head` would read the entire file into memory, split it into lines, and
print the first N. That works for a 2 KB file and falls over on a 10 GB one.

The Unix way — and what we do — is to **stream**: pull one line at a time and
**stop the instant we've emitted N**. The rest of the file is never touched. This
is the difference between O(N) work and O(filesize) work, and it's why `head` on a
huge file returns immediately.

```
read line ─┐
print it    │ repeat until we've printed N lines…
count++  ───┘   …then RETURN. The remaining bytes are never read.
```

### 2. Lines vs. bytes

`head` has two counting modes:

| Flag | Unit | Example | Notes |
|------|------|---------|-------|
| `-n N` | **lines** | `head -n 5` | default; N = 10 if unspecified |
| `-c N` | **bytes** | `head -c 20` | counts raw bytes, can cut mid-line / mid-character |

A "line" is everything up to and including the next `\n`. We deliberately keep the
newline attached so our output is **byte-for-byte identical** to system `head`
(crucial for the `diff` test below). A final line with no trailing newline still
counts and is printed verbatim.

Byte mode is simpler — we just copy exactly N bytes — but it's "dumber": it can
split a UTF-8 character in half, exactly like the real tool.

### 3. Buffered I/O with `bufio`

Reading one byte at a time from the OS is a syscall per byte — painfully slow.
`bufio.Reader` wraps any input with an in-memory buffer, so we make a few big
reads under the hood while *our* code gets to ask for convenient pieces
(`ReadBytes('\n')` for a line). This is the standard Go streaming primitive and
reappears in `wc`, `cat`, and every later Phase 2 tool.

### 4. The stdin fallback & the `-` convention

A well-behaved Unix filter reads **stdin when given no file arguments**, so it can
sit in a pipe (`cat x | head`). The literal argument `-` *also* means stdin, even
when listed alongside real files. We honour both.

### 5. Multi-file headers

When you pass more than one file, GNU `head` prints a banner before each:

```
==> a.txt <==
...first lines of a.txt...

==> b.txt <==
...first lines of b.txt...
```

Note the **blank line before every header except the first**. We reproduce this
exactly.

## 🏗️ Architecture & Design

```
main()                      → os.Exit(run(args))         tiny, untestable shell
 └─ run([]string) int       → orchestration + exit codes  ← unit-tested directly
     ├─ parseArgs()         → []args  → config{count, byteMode, files}
     ├─ printHeader()       → "==> name <==" banners (multi-file only)
     └─ headStream()        → picks line mode or byte mode
         ├─ headLines()     → ReadBytes('\n') loop, stops at N  ← early termination
         └─ headBytes()     → io.CopyN(w, r, N)
```

**Why `run() int` instead of calling `os.Exit` everywhere?** A Go testing idiom:
`os.Exit` would kill the test process. By returning an integer, our tests call
`run(...)` and assert on the code without exiting. (Same pattern as the Phase 1
Go challenges in this repo.)

**Why hand-roll the flag parser?** The stdlib `flag` package doesn't model
`-n5` (glued value) or "stop parsing flags after the first filename" — both of
which real `head` does. Writing ~30 lines by hand keeps the rules visible, which
is the point of a learning repo.

## 🔨 Step-by-Step Implementation

1. **`config` struct** — group the parsed options (`count`, `byteMode`, `files`).
   Go has no classes; a struct is how you bundle related data.
2. **`parseArgs`** — handle `-n N`, `-n5`, `-c N`, `-c100`, `--`, `-`, and bare
   filenames. Reject bad flags/numbers with a usage error (exit 2).
3. **`headStream`** — wrap the reader in `bufio.NewReader`, then dispatch to
   `headLines` or `headBytes`.
4. **`headLines`** — loop `ReadBytes('\n')`, write each line, `written++`, and
   **return as soon as `written == n`**. Treat `io.EOF` as a normal end, not an error.
5. **`headBytes`** — `io.CopyN(w, r, n)`; swallow `io.EOF` (a short file is fine).
6. **`run`** — stdin fallback when no files; otherwise loop files, print headers
   when there's more than one, and accumulate the exit code.

> 🐹 **Go gotcha we call out in the code:** `defer f.Close()` runs when the
> *function* returns, **not** at the end of a loop iteration. Deferring inside a
> file loop would hold every handle open until the very end. We close explicitly
> per file instead — different from Python's `with open(...)` block scoping.

## 🧪 Testing Strategy

Run the unit tests and the vetter:

```bash
go test ./...
go vet ./...
```

The `*_test.go` suite covers:

- `-n` (default 10, and explicit N)
- `-c` (byte mode)
- **stdin** fallback (no file args)
- **multiple files** with `==> name <==` headers + blank-line separation
- **N larger than the file** (print all, stop cleanly)
- **empty input** (line and byte mode)
- a final line with **no trailing newline**
- argument parsing table (`-n5`, `-c 20`, bad flags, missing values)

### Differential testing against the real tool

The gold standard is matching system `head` byte-for-byte:

```bash
go build -o head .
seq 1 50 > sample.txt
seq 1 5  > b.txt

diff <(./head -n 7 sample.txt)       <(head -n 7 sample.txt)       && echo OK
diff <(./head -c 12 sample.txt)      <(head -c 12 sample.txt)      && echo OK
diff <(cat sample.txt | ./head -n 3) <(cat sample.txt | head -n 3) && echo OK
diff <(./head -n 2 sample.txt b.txt) <(head -n 2 sample.txt b.txt) && echo OK
```

All four print `OK` — our output is identical to the system tool.

## 💡 Key Takeaways

- **Early termination is a design choice, not an optimisation.** Streaming and
  stopping at N is what makes `head` usable on enormous files — the rest of the
  file is never read.
- **`bufio.Reader` is the Go streaming workhorse.** `ReadBytes('\n')` gives you
  line-at-a-time reads without slurping; you'll reuse it across all of Phase 2.
- **Preserve bytes exactly** (keep the `\n`, print unterminated final lines) so
  you can `diff` against the real tool and *prove* correctness.
- **`io.EOF` is a normal signal, not a failure** — handling it correctly is the
  difference between a clean program and a noisy one.
- **`defer` is function-scoped, not block-scoped** — a real difference from
  Python's `with`, and a classic Go beginner trap.
- **`run() int` keeps `main` testable** — push logic out of `main` so tests can
  drive it without `os.Exit` killing the test process.

## 📖 Further Reading

- [codingchallenges.fyi — Build Your Own head](https://codingchallenges.fyi/challenges/challenge-head/)
- [GNU Coreutils — `head` manual](https://www.gnu.org/software/coreutils/manual/html_node/head-invocation.html)
- [`bufio` package docs](https://pkg.go.dev/bufio)
- [`io.CopyN` docs](https://pkg.go.dev/io#CopyN)
- Repo: [Go quick-start for Python developers](../../docs/go-quickstart.md)
