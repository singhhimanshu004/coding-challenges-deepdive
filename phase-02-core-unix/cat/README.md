# cat

> **Phase:** 2 — Core Unix: Text Processing
> **Difficulty:** 🟢
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** S

**Status:** ✅ Done

> 🐍➡️🐹 **New to Go?** Read the project primer first:
> [`docs/go-quickstart.md`](../../docs/go-quickstart.md). It teaches Go by
> mapping every concept back to Python. This README assumes you've skimmed it
> and re-explains the Go-specific bits inline as we go.

---

## 🎯 What We're Building

`cat` ("con**cat**enate") is the most fundamental Unix filter. It reads one or
more inputs and writes their bytes, in order, to standard output. That's the
whole job — and yet it's the perfect first lesson in **stream forwarding**: data
flows *through* the program from input to output without being held in memory all
at once.

We mirror the [codingchallenges.fyi "build your own cat"](https://codingchallenges.fyi/challenges/challenge-cat/)
exercise. Our `cat` can:

| Invocation | What it does |
|---|---|
| `cat` | Copy standard input to standard output (type text, end with Ctrl-D). |
| `cat a.txt b.txt` | Concatenate files in the order given. |
| `cat a.txt - b.txt` | A `-` argument means "read stdin **here**", in sequence. |
| `... \| cat` | Acts as a pass-through in a pipe. |
| `cat -n file` | Number **every** output line. |
| `cat -b file` | Number only **non-blank** lines (overrides `-n`). |
| `cat -E file` | Show the end of each line as `$`. |

**Python analogy:** the no-flag case is essentially:

```python
import sys, shutil
for path in files:
    with open(path, "rb") as f:
        shutil.copyfileobj(f, sys.stdout.buffer)
```

…but we'll see why doing it the streaming, buffered Go way matters.

---

## 📚 Core Concepts

### 1. Streaming, not slurping
A naive `cat` could read the whole file into memory (`data = f.read()`) and then
print it. That breaks the moment someone runs `cat huge.iso`. The Unix way is to
**stream**: move a small buffer's worth of bytes at a time from input to output.
In Go this is `io.Copy(dst, src)`, which loops over a fixed 32 KiB buffer
internally. Memory use stays flat no matter how big the file is.

### 2. The `io.Reader` / `io.Writer` interfaces
Go's I/O is built on two tiny interfaces:

```go
type Reader interface { Read(p []byte) (n int, err error) }
type Writer interface { Write(p []byte) (n int, err error) }
```

> **Go note for a Python dev:** an *interface* is a contract — "anything with a
> `Read` method counts as a Reader." It's Go's version of Python duck typing,
> but checked at compile time. A file, a network socket, an in-memory
> `bytes.Buffer`, and `os.Stdin` are *all* Readers. That's why our `run()` takes
> `io.Reader`/`io.Writer` parameters: tests inject a `bytes.Buffer`, production
> injects the real `os.Stdin`/`os.Stdout`. Same code, no special-casing.

### 3. Buffered output
Writing one byte at a time means one syscall per byte — painfully slow. We wrap
stdout in a `bufio.Writer`, which collects bytes and flushes them in big chunks.
The catch: **you must `Flush()`** or the last buffered bytes never reach the
screen. We guarantee that with `defer out.Flush()`.

> **Go note:** `defer` schedules a call to run when the *surrounding function*
> returns — like a `try/finally`. It's the idiomatic way to make sure cleanup
> always happens.

### 4. Binary-safe forwarding
With no flags, `cat` must be a *faithful* copy — every byte, including NUL
(`0x00`) and non-text bytes, passes through untouched. Because `io.Copy` never
interprets the data, our no-flag path is fully binary-safe. We only switch to
line-by-line reading when a flag (`-n`, `-b`, `-E`) actually requires us to
understand line structure.

### 5. Resilient error handling
If you run `cat missing.txt good.txt`, real `cat` reports the missing file to
**stderr**, sets a non-zero exit code, but **still prints `good.txt`**. One bad
input must not abort the others. We follow the repo's exit-code convention:

| Code | Meaning |
|---|---|
| `0` | Success |
| `1` | A file couldn't be read (others still processed) |
| `2` | Usage error (unknown flag) |

---

## 🏗️ Architecture & Design

Everything lives in one file, `main.go`, because the tool is small. The shape
mirrors the other Go challenges in this repo (huffman, bloom): a tiny `main()`
that immediately delegates to a testable `run()`.

```
main()                       process entry point; only calls os.Exit(run(...))
 └─ run(args, stdin, out, err)   orchestration + exit code
     ├─ parseArgs(...)           args  → (options, files, code)
     └─ catStream(src, out, ...) copy ONE source to output
          ├─ fast path: io.Copy            (no flags → binary-safe)
          └─ line mode: bufio.ReadBytes    (-n / -b / -E)
```

**Why split `main` from `run`?** `os.Exit()` terminates the process immediately
and skips `defer`s. If `main` did the work, tests couldn't call it (it would
kill the test runner) and `defer out.Flush()` would never run. By keeping
`main` to a single line and putting logic in `run()`, tests call `run()`
directly with fake streams and check the returned int. This is the single most
important testability pattern in the repo's Go code.

**Continuous line numbering.** The line counter is declared once in `run()` and
passed by pointer into `catStream` for every file, so numbering flows
*continuously* across files — exactly like GNU `cat`. (See the compatibility
note at the end: BSD/macOS `cat` resets per file; we deliberately follow the
GNU reference behavior, which is what the challenge spec assumes.)

---

## 🔨 Step-by-Step Implementation

### Step 1 — `main` → `run`
```go
func main() {
    os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
```
`os.Args[0]` is the program name, so we slice it off with `[1:]`. The three
streams are injected so tests can swap them out.

### Step 2 — Parse flags (`parseArgs`)
We support short flags (`-n`, `-b`, `-E`), **bundled** short flags (`-nE` ==
`-n -E`), long flags (`--number`, `--number-nonblank`, `--show-ends`), `--` to
stop flag parsing, and a lone `-` which is a *file name* meaning stdin. Unknown
flags print a usage line and return exit code `2`.

> **Go note:** `for _, c := range arg[1:]` iterates the runes after the dash —
> that's how we unbundle `-nE`. A `switch` with no condition reads like a clean
> `if/elif` ladder.

### Step 3 — Decide the input list
```go
if len(files) == 0 {
    files = []string{"-"}   // no files → read stdin
}
```
This one line is what makes `echo hi | cat` and a bare interactive `cat` work.

### Step 4 — Buffer stdout once
```go
out := bufio.NewWriter(stdout)
defer out.Flush()
```
All files write into the same buffered writer, flushed exactly once at the end.

### Step 5 — Loop over inputs
For each name: `-` uses the injected stdin; anything else is `os.Open`ed. If the
open fails we print to stderr, set the exit code to `1`, and `continue`. We
close real files explicitly **inside the loop** (not with `defer`) so we don't
hold every handle open until the program ends.

### Step 6 — Copy one source (`catStream`)
- **No flags:** `io.Copy(out, src)` — fast, binary-safe.
- **Line mode:** read with `bufio.Reader.ReadBytes('\n')`. Each call returns one
  line *including* its trailing `\n` (if any). At EOF it returns the final
  partial line **plus** `io.EOF`, so we process the bytes *before* checking the
  error — this is how the last line without a trailing newline is preserved.

  For each line we optionally prepend `"%6d\t"` (six-wide number + tab, matching
  real `cat`), optionally append `$` for `-E`, and re-emit the newline only if
  the input had one. `-b` skips numbering blank lines *and* doesn't advance the
  counter; `-b` wins over `-n` when both are given.

---

## 🧪 Testing Strategy

Run everything with:

```bash
go test ./...      # unit tests
go vet ./...       # static checks
```

`main_test.go` drives `run()` with in-memory streams (`strings.NewReader` for
stdin, `bytes.Buffer` for stdout/stderr) and uses `t.TempDir()` for real files.
Covered cases:

- **Single & multiple files** concatenate in order.
- **Stdin fallback** when no files are given.
- **`-` ordering** — file, stdin, file interleave correctly.
- **`-n`, `-b`, `-E`** formatting, including `-b` not numbering blank lines and
  `-b` overriding `-n`.
- **Continuous numbering** across multiple files.
- **Missing file** → exit `1`, error on stderr, *other files still print*.
- **Empty input** → no output.
- **Last line without newline** is preserved (raw and with `-n`).
- **Binary-safe** raw copy of NUL / high bytes.
- **Unknown flag** → exit `2`; **`--`** stops flag parsing.

### Manual parity check against system `cat`
```bash
go build -o cat .
printf 'a\nb\n\nc\n' > f1.txt ; printf 'd\ne\n' > f2.txt

diff <(./cat f1.txt f2.txt)             <(cat f1.txt f2.txt)        # raw concat
diff <(./cat -n f1.txt)                 <(cat -n f1.txt)            # numbering (single file)
diff <(./cat -b f1.txt)                 <(cat -b f1.txt)            # non-blank numbering
diff <(printf 'X\n' | ./cat f1.txt - )  <(printf 'X\n' | cat f1.txt - )  # stdin via '-'
```
All produce no diff on a single file. (See the compatibility note below for the
one intentional multi-file difference on macOS.)

---

## 💡 Key Takeaways

- **`io.Copy` is the streaming workhorse** — flat memory, binary-safe, and the
  right default for any "move bytes from A to B" task.
- **Inject your streams.** Taking `io.Reader`/`io.Writer` as parameters (instead
  of reaching for `os.Stdin` directly) makes the whole program trivially
  testable. This pattern reappears in every Phase 2 tool.
- **Buffer + `defer Flush()`** is the standard idiom for fast, correct output.
- **`ReadBytes` returns data *and* `io.EOF` together** at the end — always
  handle the bytes before the error, or you'll drop the last unterminated line.
- **Be a good Unix citizen:** errors to stderr, sensible exit codes, and keep
  going after a per-file failure.

---

## ⚠️ Compatibility note: continuous vs. per-file numbering

GNU `cat` (Linux) numbers lines **continuously** across multiple files; BSD
`cat` (macOS) **resets** the counter at each file. We follow **GNU** behavior
because that's the reference the challenge assumes. So on macOS:

```bash
./cat -n a.txt b.txt   # our output: continuous 1,2,3,4…
cat   -n a.txt b.txt   # macOS BSD: resets to 1 at b.txt
```

Single-file output is identical on both platforms. `-E` is a GNU flag (BSD uses
`-e`), so comparing `-E` against macOS `cat` will show an "illegal option"
message from the system tool — that's expected.

---

## 📖 Further Reading

- [codingchallenges.fyi — Build Your Own cat](https://codingchallenges.fyi/challenges/challenge-cat/)
- [Go primer for this repo](../../docs/go-quickstart.md) — read this if any Go
  syntax above was unfamiliar.
- `go doc io.Copy`, `go doc bufio.Reader.ReadBytes`, `go doc bufio.Writer`
- GNU coreutils `cat` manual: `info coreutils 'cat invocation'`
- Sibling challenge [`../wc`](../wc) — same buffered-streaming mindset for
  counting; [`../head`](../head) — early-termination reads.
