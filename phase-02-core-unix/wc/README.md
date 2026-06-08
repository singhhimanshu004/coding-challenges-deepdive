# wc (word count)

> **Phase:** 2 — Core Unix: Text Processing
> **Difficulty:** 🟢
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** S

**Status:** ✅ Done

> 🐍 **New to Go?** This README is written for a Python developer learning Go.
> Read the project primer first: [`docs/go-quickstart.md`](../../docs/go-quickstart.md).
> It maps every Go concept used here (slices, structs, `bufio`, interfaces,
> `defer`, multiple return values) back to the Python you already know.

---

## 🎯 What We're Building

A from-scratch rebuild of the classic Unix `wc` ("word count"), following the
[codingchallenges.fyi "build your own wc"](https://codingchallenges.fyi/challenges/challenge-wc/)
spec. Given some text it reports how many **lines**, **words**, **characters**
and **bytes** it contains:

```console
$ ./wc test.txt
   7  58 339 test.txt

$ echo "hello world" | ./wc -w
2
```

It supports the four canonical flags, reads files **or** standard input, handles
multiple files with a `total` line, counts multibyte (UTF-8) characters
correctly, and lines its columns up like the real tool.

| Flag | Long form | Meaning |
| --- | --- | --- |
| `-c` | `--bytes` | byte count |
| `-l` | `--lines` | line count (number of newlines) |
| `-w` | `--words` | word count (runs of non-whitespace) |
| `-m` | `--chars` | character count (Unicode runes) |

With **no flags**, `wc` prints the historical default: **lines, words, bytes**.
With **no file arguments** (or `-`), it reads **standard input**.

---

## 📚 Core Concepts

### The Unix philosophy

`wc` is the textbook example of a Unix *filter*: a small program that does one
thing, reads from **stdin**, writes to **stdout**, and composes with other tools
through **pipes**. You never need a "wc library" — you pipe data into it:

```console
$ grep ERROR app.log | ./wc -l      # how many error lines?
$ find . -name '*.go' | ./wc -l     # how many Go files?
```

Because it speaks the universal interface (a stream of bytes), it cooperates
with every other tool on the system. That composability is the whole point of
this phase.

### Streaming & buffered I/O — why `bufio`

A naïve reader asks the operating system for data one byte at a time. Each
request is a **syscall** — a relatively expensive jump into the kernel. Reading
a 1 GB file byte-by-byte would mean a billion syscalls.

`bufio.Reader` fixes this: it asks the OS for a big chunk (4 KiB by default),
stashes it in an in-memory buffer, and serves your reads from there. Thousands
of tiny reads collapse into a handful of large ones.

> 🐍 **Python analogy:** opening a file in Python already gives you a buffered
> reader (`io.BufferedReader`). In Go you opt in explicitly by wrapping a raw
> reader: `bufio.NewReader(f)`. The primer covers this in the buffered-I/O
> section.

Crucially, we **stream**: we never load the whole file into memory. We pull one
rune at a time and update running totals. The same code handles a 10-byte string
and a 10-GB log with constant memory.

### Bytes vs. characters — Unicode & runes

In ASCII, one byte = one character, so `-c` and `-m` agree. With UTF-8 they
diverge: `é` is **2 bytes** but **1 character**; `😀` is **4 bytes** but **1
character**.

- **`-c` (bytes)** counts raw bytes on disk.
- **`-m` (chars)** counts *runes* — Go's name for a Unicode code point.

We use `bufio.Reader.ReadRune()`, which decodes one UTF-8 rune and tells us both
the rune *and* how many bytes it spanned, so we can keep both tallies in one
pass.

> 🐍 **Python analogy:** a Go `rune` is an `int32` code point — like iterating a
> Python `str` (which yields code points) versus iterating `bytes` (which yields
> integers 0–255).

### Counting words

A "word" is a maximal run of non-whitespace characters. We track a single
boolean, `inWord`. Every time we cross from whitespace into non-whitespace we
have started a new word, so we increment. This one-pass state-machine approach
is the same pattern you'll reuse in `uniq`, `tr`, and friends.

### Flag parsing & CLI ergonomics

Real `wc` lets you **bundle** short flags (`-lw` == `-l -w`) and stop flag
parsing with `--`. Go's standard `flag` package does neither cleanly, so we
hand-roll a tiny parser. It also recognises long forms (`--lines`) and treats a
lone `-` as "read stdin".

### Exit codes

Faithful CLI tools signal success/failure through their exit status so scripts
can branch on `$?`:

| Code | Meaning |
| --- | --- |
| `0` | success |
| `1` | a file could not be read (matches real `wc`) |
| `2` | usage error (unknown flag) |

---

## 🏗️ Architecture & Design

Flat, well-named files (the layout convention from our earlier Go challenges —
small tools don't need an `internal/` package):

```
wc/
├── main.go        # CLI: flag parsing, file iteration, output formatting
├── count.go       # the streaming counter (counts struct + count())
├── count_test.go  # unit tests: counting + flag parsing + formatting
├── run_test.go    # integration tests: stdin, files, totals, exit codes
├── go.mod
└── .gitignore     # ignores the compiled /wc binary
```

The design separates **pure logic** from **I/O orchestration**:

- `count(io.Reader) (counts, error)` is pure and streaming — it knows nothing
  about flags, files, or formatting. That makes it trivial to unit-test with an
  in-memory `strings.NewReader`.
- `run(args, stdin, stdout, stderr) int` orchestrates everything and returns the
  exit code. Passing the three streams **in** (instead of touching
  `os.Stdin`/`os.Stdout` directly) is a Go testing idiom: tests feed a
  `bytes.Buffer` and assert on the captured output without spawning a process.
- `main()` is a three-liner whose only job is to call `run` and `os.Exit`.

> 🐍 **Python analogy:** this is the same instinct as keeping logic in importable
> functions and putting only the entry point under `if __name__ == "__main__":`.

---

## 🔨 Step-by-Step Implementation

1. **The `counts` struct** (`count.go`). Four `int` fields: `lines`, `words`,
   `chars`, `bytes`. Its zero value is already a valid empty count — no
   constructor needed. An `add` method (pointer receiver) accumulates one
   `counts` into another, used to build the `total` row.

2. **The streaming counter** `count(r io.Reader)`. Wrap `r` in a
   `bufio.NewReader`, then loop on `ReadRune()`:
   - add `size` to `bytes`, increment `chars`;
   - if the rune is `'\n'`, increment `lines`;
   - run the `inWord` state machine for `words`.
   `io.EOF` ends the loop cleanly; any other error is returned.

3. **Flag parsing** `parseArgs`. Walk the args: `--` stops flag parsing, a lone
   `-` is a filename (stdin), `--xxx` is a long flag, `-abc` is a bundle of
   short flags, anything else is a filename. Unknown flags return an error → the
   caller exits `2`.

4. **Defaults & stdin fallback** (`run`). If no flags were given, turn on
   lines/words/bytes. If no files were given, use a single empty name that means
   "read stdin".

5. **Per-file counting** `countNamed`. For `""`/`-` count stdin; otherwise
   `os.Open` the file and `defer f.Close()`. Read errors print to stderr and set
   the exit code to `1` but **don't stop** the remaining files — exactly like
   real `wc`.

6. **Column formatting**. Compute one shared field width from the largest number
   we'll print (including the total), then right-align every column with
   `fmt.Sprintf("%*d", width, n)`. Columns always appear in the fixed order
   **lines, words, chars, bytes**, followed by the filename.

---

## 🧪 Testing Strategy

Run the suite:

```console
go test ./...
go vet ./...
```

Coverage:

- **`count_test.go`** — table-driven unit tests for every counted quantity:
  empty input, single word, trailing newline, leading/trailing whitespace, tabs,
  **multibyte runes** (`héllo wörld`, where bytes > chars), and an **emoji**
  (a single 4-byte rune). Plus flag-parsing tests (bundling, long flags, `--`,
  unknown flags) and column-formatting tests.
- **`run_test.go`** — integration tests driving the whole `run` function:
  **stdin fallback**, a single real file, **multiple files with a `total`
  line**, a **missing file** (exit `1`), a **bad flag** (exit `2`), and all four
  flags together verifying column order.

**Differential testing against the system tool.** The ultimate check is matching
real `wc`:

```console
$ go build -o wc .
$ ./wc -l -w -c -m sample.txt
 3 12 58 63 sample.txt
$ wc  -l -w -c -m sample.txt
       3      12      58      63 sample.txt   # same numbers
```

The counts match exactly for files, stdin pipes, multiple files, multibyte text
and empty input. (The only visible difference is padding: we use GNU-style
minimal column width; BSD `wc` on macOS pads to a fixed width. The *numbers* are
identical.)

---

## 💡 Key Takeaways

- **Filters are the heart of Unix.** Read stdin, write stdout, exit with a
  meaningful code, and your tool composes with everything via pipes.
- **Stream, don't slurp.** `bufio` + a one-rune-at-a-time loop gives constant
  memory and few syscalls on inputs of any size.
- **Bytes ≠ characters.** UTF-8 makes the distinction real; `ReadRune` hands you
  both the rune and its byte width in a single call.
- **Separate pure logic from I/O.** A pure `count(io.Reader)` and an injectable
  `run(..., stdin, stdout, stderr)` make the whole tool testable in-memory.
- **Faithful ergonomics matter.** Flag bundling, stdin fallback, a `total` row,
  aligned columns and correct exit codes are what make a clone feel real.

---

## 📖 Further Reading

- [codingchallenges.fyi — Build Your Own `wc`](https://codingchallenges.fyi/challenges/challenge-wc/)
- [`docs/go-quickstart.md`](../../docs/go-quickstart.md) — the project Go primer for Python devs
- [Go `bufio` package docs](https://pkg.go.dev/bufio)
- [Go blog — Strings, bytes, runes and characters](https://go.dev/blog/strings)
- [POSIX `wc` specification](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/wc.html)
