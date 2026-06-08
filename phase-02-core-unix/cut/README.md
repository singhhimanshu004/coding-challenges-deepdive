# cut

> **Phase:** 2 — Core Unix: Text Processing
> **Difficulty:** 🟢
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** S

**Status:** ✅ Done

> 🆕 **New to Go?** Read the project's [Go Quickstart for a Python Developer](../../docs/go-quickstart.md)
> first — it explains slices, structs, `error` returns, interfaces, and `defer`
> by mapping each one to the Python you already know. Every Go idiom used below
> is cross-referenced there.

---

## 🎯 What We're Building

`cut` is one of the oldest Unix text tools. It pulls **columns** out of
line-oriented text. Two ways to say what a "column" is:

- **Fields** (`-f`): split each line on a delimiter (default a TAB) and keep
  some of the resulting pieces. This is how you slice a CSV/TSV.
- **Characters** (`-c`): keep characters at certain positions, ignoring
  delimiters entirely. This is how you slice fixed-width text.

```console
$ printf 'name\tage\tcity\nhimanshu\t30\tdelhi\n' | cut -f1,3
name    city
himanshu        delhi

$ printf 'a,b,c,d\n' | cut -d, -f2-4
b,c,d

$ printf 'abcdef\n' | cut -c2-4
bcd
```

If you come from Python, think of `cut -f` as `line.split(delim)` followed by
picking indices, run over every line of a stream — but with 1-based positions
and Unix's exact edge-case rules.

## 📚 Core Concepts

### 1. Column-oriented thinking

Most Unix text tools are **line-oriented**: read a line, transform it, print it,
repeat. `cut` adds a second axis — **within** a line it thinks in *columns*.
The whole tool is therefore a tiny 2-D loop: outer loop over lines, inner loop
over the columns of one line, emitting the ones you asked for.

### 2. The LIST grammar (fields *and* ranges)

The argument to `-f` or `-c` is a **LIST**: a comma-separated set of items where
each item is a single position or a range. All positions are **1-based** (field
1 is the first field — there is no field 0).

| Item   | Meaning                                  | Example output of `-f<item>` on `a b c d` |
| ------ | ---------------------------------------- | ----------------------------------------- |
| `N`    | a single position                        | `-f2` → `b`                               |
| `N-M`  | closed range `N..M` inclusive            | `-f2-3` → `b c`                           |
| `-M`   | from the start through `M`               | `-f-2` → `a b`                            |
| `N-`   | from `N` through the end of the line     | `-f3-` → `c d`                            |
| mix    | combine with commas                      | `-f1,3-` → `a c d`                        |

Two subtle rules that match GNU/BSD `cut` and are easy to get wrong:

- **Output is in *input* order, never spec order.** `cut -f3,1` prints field 1
  then field 3. So we don't expand the LIST into an ordered list of indices; we
  walk the line's columns left-to-right and ask "is this position selected?".
- **Duplicates collapse.** `cut -f1,1` prints field 1 once. Membership-testing
  gives us this for free too.

### 3. Delimiter semantics

- The default field delimiter is a **single TAB** (`\t`), which is why TSV files
  "just work" with bare `cut -f...`.
- `-d` sets the delimiter and must be **exactly one character**.
- A line that **doesn't contain the delimiter** is printed *unchanged* by
  default (cut assumes it's a single field). Pass **`-s`** ("suppress") to drop
  such lines instead — handy for skipping comment/header lines that don't fit
  the column format.
- `-c` (character mode) ignores delimiters completely, so `-d`/`-s` are only
  valid with `-f`.

### 4. Bytes vs. characters

In `-c` mode positions count **characters, not bytes**. Go strings index by
byte, so we convert each line to `[]rune` (Unicode code points) first — that's
what makes `cut -c1-2` on `héllo` return `hé` rather than a mangled half-byte.
(Python's `str` already indexes by code point; the `[]rune` conversion is how we
reach the same behaviour in Go.)

## 🏗️ Architecture & Design

Three small files, each with one job — the same "factor cleanly" discipline the
team used for Huffman and the Bloom filter:

| File         | Responsibility                                                              |
| ------------ | -------------------------------------------------------------------------- |
| `ranges.go`  | Parse a LIST string into a `Selector` and answer `contains(position)`.     |
| `cut.go`     | The engine: stream lines via `bufio.Scanner`, slice each, write results.   |
| `main.go`    | CLI: hand-rolled flag parsing (`-f -c -d -s`), file/stdin plumbing, exits. |

The key design decision is the **`Selector`** abstraction. Both `-f` and `-c`
need the identical "is position *p* in this LIST?" logic — only *what they slice*
differs (whole fields vs. runes). By factoring the range-list parser into its
own type, `-f` and `-c` share one well-tested code path.

```
        argv ──▶ parseArgs ──▶ config{ mode, Selector, delim, suppress }
                                   │
   file/stdin ──▶ run (bufio.Scanner) ──▶ processLine ──▶ cutFields | cutChars
                                   │
                              bufio.Writer ──▶ stdout
```

### Exit codes (repo convention)

| Code | Meaning                                            |
| ---- | -------------------------------------------------- |
| `0`  | success                                            |
| `1`  | domain failure (a file couldn't be opened/read)    |
| `2`  | usage error (bad flags, bad LIST, multi-char `-d`) |

## 🔨 Step-by-Step Implementation

1. **The range parser (`ranges.go`).** Model one interval as `rng{lo, hi}` where
   `hi == 0` is the sentinel for an open-ended `N-`. Parse each comma item: a
   bare number is `N-N`; otherwise split on `-` and treat an empty side as
   "start" (`1`) or "end" (`0`). Reject `0`, negatives, decreasing ranges
   (`3-1`), and empty items — matching real `cut`'s error behaviour.
2. **Membership, not expansion.** `Selector.contains(p)` loops its ranges and
   returns true if any contains `p`. This single method delivers input-order
   output and duplicate collapsing automatically (see Core Concepts §2).
3. **The engine (`cut.go`).** `run` wraps the reader in a `bufio.Scanner` (with
   an enlarged buffer for very wide rows) and the writer in a `bufio.Writer`
   (flushed with `defer`). For each line, `processLine` dispatches to
   `cutFields` or `cutChars`.
4. **Field cutting.** If the line lacks the delimiter: print as-is, or skip when
   `-s`. Otherwise `strings.Split`, keep the selected indices, re-`Join` with the
   same delimiter.
5. **Character cutting.** Convert to `[]rune`, keep selected positions, rebuild
   with a `strings.Builder`.
6. **The CLI (`main.go`).** Hand-roll the parser so both attached (`-f1,3`,
   `-d,`) and separated (`-f 1,3`, `-d ,`) forms work — Go's stdlib `flag`
   package can't do attached short flags. Validate flag combinations, then loop
   over file operands (`-` means stdin), accumulating a non-zero exit on read
   errors but continuing with the remaining files.

## 🧪 Testing Strategy

`cut_test.go` drives the engine through in-memory readers/writers (no temp files
needed — `run` takes `io.Reader`/`io.Writer` interfaces). Coverage:

- **LIST parsing:** singles, `N-M`, `-M`, `N-`, mixed lists, and a battery of
  invalid specs (`0`, `-`, `3-1`, `1,,3`, `2-b`, …).
- **Field selection:** comma lists, ranges, open-ended ranges, input-order
  output, custom delimiter, fields beyond what the line has.
- **Character mode:** ASCII ranges and a Unicode (`héllo`) case.
- **Missing delimiter:** printed by default, suppressed with `-s`.
- **Streaming:** multi-line stdin and empty input.
- **Arg parsing:** attached vs. separated flags, and rejected flag combinations.

Run it:

```console
$ go test ./...   # unit tests
$ go vet ./...    # static checks
```

### Verified against the real tool

The implementation was diffed against the system `cut` on real TSV/CSV input for
`-f` lists, ranges, open-ended ranges, custom delimiters, `-s`, `-c`, and stdin —
all byte-for-byte identical.

```console
$ go build -o cut .
$ diff <(./cut -d, -f2-4 sample.csv) <(cut -d, -f2-4 sample.csv)   # no output = match
```

## 💡 Key Takeaways

- **One abstraction can serve two features.** The `Selector` / range-list parser
  powers both `-f` and `-c`; isolating it kept the engine tiny and the tests
  sharp.
- **Membership beats expansion.** Testing "is position *p* selected?" while
  walking columns in order is what gives `cut` its input-order, de-duplicated
  output — no sorting required.
- **Edge cases *are* the spec.** 1-based indexing, the no-delimiter rule, `-s`,
  and bytes-vs-runes are exactly where a naïve clone diverges from real `cut`.
- **Interfaces make code testable.** Accepting `io.Reader`/`io.Writer` instead of
  `*os.File` let every test feed a string and assert on a buffer.

## 📖 Further Reading

- Coding Challenges — [Build Your Own cut](https://codingchallenges.fyi/challenges/challenge-cut)
- GNU Coreutils manual — [`cut` invocation](https://www.gnu.org/software/coreutils/manual/html_node/cut-invocation.html)
- POSIX specification — [`cut`](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/cut.html)
- Project primer — [Go Quickstart for a Python Developer](../../docs/go-quickstart.md)
- Related challenges in this phase: `cat`, `head`, `wc` (line-oriented streaming siblings)
