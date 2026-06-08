# tr

> **Phase:** 2 — Core Unix: Text Processing  
> **Difficulty:** 🔵  
> **Recommended Language:** 🟦 Go  
> **Effort Estimate:** S

**Status:** ✅ Done

> 🐍→🐹 **New to Go?** Read [`docs/go-quickstart.md`](../../docs/go-quickstart.md)
> first. It teaches Go *by mapping it to Python*, using this repo's code as the
> running example. Every Go-specific idiom below is also commented inline in the
> source files, so keep them open side by side.

---

## 🎯 What We're Building

`tr` ("translate") is one of the smallest, sharpest tools in the Unix box. It
reads text on **standard input**, rewrites it one character at a time, and
writes the result to **standard output**. It does exactly four things:

| Mode | Flag | What it does | Example |
|------|------|--------------|---------|
| **Translate** | *(none)* | Replace each char in SET1 with the char at the same position in SET2 | `tr a-z A-Z` → uppercase |
| **Delete** | `-d` | Drop every char that appears in SET1 | `tr -d [:digit:]` → strip numbers |
| **Squeeze** | `-s` | Collapse runs of repeated chars into one | `tr -s ' '` → one space |
| **Complement** | `-c` | Operate on every char *not* in SET1 | `tr -cd [:digit:]` → keep only digits |

The flags combine: `tr -cs '[:alpha:]' '\n'` ("squeeze every non-letter into a
single newline") is the classic one-word-per-line trick.

The single most important design fact: **`tr` is a pure filter.** It takes *no
file arguments*. You never write `tr file.txt`; you write `cat file.txt | tr …`.
Its only job is `stdin → transform → stdout`, which is why it composes so well
in pipelines.

```bash
echo "Hello, World!" | tr a-z A-Z        # HELLO, WORLD!
echo "a1b2c3"        | tr -d '[:digit:]'  # abc
echo "aaabbbccc"     | tr -s a-z          # abc
echo "a1b2c3"        | tr -cd '[:digit:]' # 123
```

## 📚 Core Concepts

### 1. The pipe-and-filter model
A *filter* is a program that reads a stream, transforms it, and writes a stream,
holding as little state as possible. Filters chained with `|` are the heart of
the Unix philosophy: small programs that each do one thing, composed into
pipelines. `tr` is the textbook example — it can't even open a file, forcing you
to compose it.

### 2. Characters, not bytes — *runes* and Unicode
A byte is not a character. The letter `é` is **two** UTF-8 bytes; the Greek `λ`
is two; an emoji can be four. If `tr` worked on raw bytes it would mangle any
non-ASCII text. Go's word for "one Unicode code point" is a **rune** (an alias
for `int32`). We decode the input stream into runes and operate on those, so a
multibyte character is always treated as a single unit. See `TestMultibyteRunes`
for the proof.

### 3. SET expansion: ranges and character classes
A SET operand is shorthand that expands into an explicit list of runes:

- **Ranges:** `a-z` → `abc…z`, `0-9` → the ten digits.
- **POSIX classes:** `[:alpha:]`, `[:digit:]`, `[:space:]`, `[:upper:]`,
  `[:lower:]` (plus `[:alnum:]`, `[:blank:]`). Each names a predefined set.
- **Backslash escapes:** `\n`, `\t`, `\r`, `\\`, etc. for non-printing chars.

Expansion order matters: for translation, `tr` maps SET1\[i] → SET2\[i]
*positionally*, so `tr abc xyz` sends `a→x, b→y, c→z`.

### 4. The SET2-padding rule
When SET2 is shorter than SET1, its **last rune repeats** to cover the rest.
`tr abcd x` maps all four letters to `x`. (GNU `tr` also has an explicit `[c*]`
repeat syntax; we implement the implicit last-char padding, which covers the
common cases and matches BSD/macOS `tr`.)

### 5. Squeeze is stateful
Translate and delete are *stateless* — each rune's fate depends only on itself.
**Squeeze is different:** to know whether to drop a character you must remember
the previous one you emitted. That single rune of carried state (`lastEmitted`)
is the whole trick, and it's why squeeze runs *after* translation in the
pipeline (you squeeze the *output* alphabet, SET2).

## 🏗️ Architecture & Design

Two layers, mirroring the other Go challenges in this repo (a thin `main.go`
command wrapping a testable `internal/` package):

```
tr/
├── main.go                       # CLI: flag parsing, stdin→stdout wiring, exit codes
├── internal/translate/
│   ├── set.go                    # ExpandSet: ranges, [:classes:], \escapes  → []rune
│   ├── translate.go              # Spec → Transformer; the translate/delete/squeeze engine
│   └── translate_test.go         # unit tests (incl. system-tr-parity cases)
├── go.mod
└── .gitignore
```

The data flow is a clean pipeline:

```
args ──▶ Spec ──New()──▶ Transformer ──Run(stdin, stdout)──▶ bytes out
              (validate)     (compiled maps + squeeze state)
```

- **`Spec`** is the parsed request: the two SET strings plus the `-c/-d/-s`
  booleans. A plain data struct (like a Python dataclass).
- **`ExpandSet`** is factored out as the single place that understands SET
  syntax. Both SET1 and SET2 go through it, so ranges and classes work
  identically everywhere. It returns `[]rune`, never bytes.
- **`Transformer`** is the compiled form: it expands the sets once, builds
  lookup maps for O(1) membership tests, and holds the squeeze state. `Run`
  streams runes through `bufio` so we never do one syscall per character.

Why split `New` (validate/compile) from `Run` (execute)? So usage errors
(translating with no SET2) surface as **exit code 2** *before* we touch the
stream, while mid-stream I/O errors surface as **exit code 1** — matching the
repo's exit-code convention (`0` success · `1` domain failure · `2` usage/IO).

## 🔨 Step-by-Step Implementation

1. **Expand a SET into runes** (`set.go`). Walk the operand rune-by-rune.
   At each position, check in order: is it a `[:class:]`? a `\escape`? the start
   of an `x-y` range? otherwise a literal. Append the expansion to a `[]rune`,
   preserving order.
2. **Compile a `Spec` into a `Transformer`** (`New`). Expand both sets, validate
   the mode (translation requires a non-empty SET2), and build three maps:
   `index1` (rune → its position in SET1, for translation), `in1` and `in2`
   (fast membership tests).
3. **Process one rune** (`writeRune`). If deleting, drop runes in the delete set
   and emit the rest. Otherwise translate through SET1→SET2, then emit.
4. **Emit with squeeze** (`emit`). If `-s` is on and this rune is in the squeeze
   set *and* equals the previously emitted rune, skip it; otherwise write it and
   remember it.
5. **Stream it** (`Run`). Wrap stdin/stdout in `bufio`, loop `ReadRune` until
   `io.EOF`, and `defer` a flush so the buffer always drains.
6. **Wire the CLI** (`main.go`). Parse clustered flags (`-ds`), collect the one
   or two SET operands, build the `Spec`, and map errors to exit codes.

### Go idioms you'll meet here (with the Python translation)

- `[]rune(s)` decodes a UTF-8 string into code points — the Unicode-safe way to
  index "characters." (`set.go`)
- **Multiple return values:** `lo, adv := decodeEscape(...)` returns two values
  with no tuple object — like Python unpacking, but native. (`set.go`)
- **Interfaces:** `Run(r io.Reader, w io.Writer)` accepts *anything* with the
  right method, so production passes `os.Stdin` and tests pass a
  `strings.Reader`. (`translate.go`)
- **`defer`:** `defer bw.Flush()` schedules cleanup to run on return — Go's
  `try/finally`. (`translate.go`)
- **Zero values:** struct fields like `haveLast bool` start as `false` and
  `lastEmitted rune` as `0` with no constructor boilerplate. (`translate.go`)
- **Exported vs unexported:** `ExpandSet` (capital E) is public; `decodeEscape`
  (lowercase) is package-private — visibility is enforced by the compiler, not
  convention. (both files)

## 🧪 Testing Strategy

Run everything from this directory:

```bash
go vet ./...     # static checks — must be clean
go test ./...    # unit tests   — must pass
```

The unit tests in `translate_test.go` cover every mode and the tricky edges:

- translate (`abc→xyz`), range translate (`a-z`/`A-Z`), short-SET2 padding
- delete (`-d`), squeeze-only (`-s`), targeted squeeze, translate-then-squeeze
- complement delete (`-cd`) and complement translate (`-c`)
- a character class (`[:upper:]`→`[:lower:]`)
- **multibyte** translate *and* delete (`λ`, `é`) to prove rune-correctness
- **empty input** → empty output
- `ExpandSet` ranges, escapes, classes, and an unknown-class error
- a usage-error case (translating with no SET2)

### Differential testing against the real `tr`
Beyond unit tests, the implementation was checked **against the system `tr`**
byte-for-byte across a dozen cases (uppercase, delete, squeeze, complement,
classes, padding) by piping the same input to both and diffing the output — all
matched. You can reproduce the idea quickly:

```bash
go build -o tr .
for args in "a-z A-Z" "-d [:digit:]" "-s a-z" "-cd [:digit:]"; do
  in="Hello 123 world"
  a=$(printf '%s' "$in" | ./tr $args)
  b=$(printf '%s' "$in" | /usr/bin/tr $args)
  [ "$a" = "$b" ] && echo "OK   $args" || echo "DIFF $args ($a vs $b)"
done
rm -f tr
```

## 💡 Key Takeaways

- **A filter is a contract:** `stdin → transform → stdout`, no files, minimal
  state. Honour it and your tool drops into any pipeline.
- **Operate on runes, never bytes,** the moment text might be non-ASCII. In Go
  that means `[]rune` and `bufio.ReadRune`; the cost is one conversion and the
  payoff is correctness for the whole of Unicode.
- **Separate the "what" from the "how."** Expanding SETs (`ExpandSet`) and
  validating the request (`New`) are isolated from the hot streaming loop
  (`Run`), which keeps each piece small and independently testable.
- **State is the dividing line:** translate/delete are stateless; squeeze needs
  exactly one remembered rune. Recognising how much state an operation truly
  needs is a core stream-processing instinct.
- **Differential testing is gold** for clones: when a reference implementation
  exists, diffing against it catches edge cases no hand-written assertion would.

## 📖 Further Reading

- Coding Challenges — [Build Your Own `tr`](https://codingchallenges.fyi/challenges/challenge-tr/)
- POSIX spec — [`tr`](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/tr.html)
- GNU Coreutils manual — [`tr` invocation](https://www.gnu.org/software/coreutils/manual/html_node/tr-invocation.html)
- Go blog — [Strings, bytes, runes and characters in Go](https://go.dev/blog/strings)
- Repo primer — [`docs/go-quickstart.md`](../../docs/go-quickstart.md)
