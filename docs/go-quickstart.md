# Go Quickstart for a Python Developer

> A project-specific Go primer that teaches Go **by mapping it to Python**, using
> the real code already in this repo as the running examples. If you can read
> Python, you can read this. By the end you'll be comfortable opening any `.go`
> file in our two completed Go challenges and knowing what every line does.

**Who this is for:** Himanshu — comfortable in Python/Java, still building
muscle memory in Go.

**The two repos we cite throughout:**

- `phase-01-foundations/huffman-compression/` — a lossless file compressor
  (`main.go`, `internal/bitio/bitio.go`, `internal/huffman/heap.go`, `tree.go`, `codec.go`)
- `phase-01-foundations/bloom-filter-spell-checker/` — a Bloom-filter spell
  checker (`main.go`, `internal/bloom/bitset.go`, `bloom.go`, `hash.go`, `internal/codec/codec.go`)

---

## 1. Why Go for *some* of these challenges?

Our team policy is **best-fit, multi-language** (see `.squad/decisions.md`): we
pick the right tool per challenge, not one language for everything. Python wins
for data/encoding pipelines (JSON parser, QR generator). **Go wins when a
challenge is about bytes, bits, performance, and streaming I/O.** That's exactly
why Huffman and the Bloom filter are written in Go:

| What the challenge needs | Why Go fits |
| --- | --- |
| Flip and pack **individual bits** into bytes | Go's fixed-width integers (`byte`, `uint64`) and bit operators (`<<`, `>>`, `&`, `\|`) map straight to the hardware. Python's arbitrary-precision `int` hides the byte boundary you actually care about. See `internal/bitio/bitio.go`. |
| Read big files **fast, streaming** | `bufio.Reader` / `bufio.Writer` give buffered, allocation-light I/O. We lean on them in `bitio.NewWriter` and the Bloom CLI's `bufio.NewScanner`. |
| Predictable **performance & memory** | A Bloom filter's whole selling point is memory frugality — a packed `[]byte` bit array (see `internal/bloom/bitset.go`) does that with no GC churn. |
| Easy single-binary CLIs | `go build` produces one static binary. No virtualenv, no interpreter on the target box. |

**Reassurance:** the *concepts* transfer directly. A Go slice is "a Python list
with a fixed element type." A Go map is "a Python dict." A struct with methods is
"a class without inheritance." You are mostly learning new *syntax* and a few new
*rules*, not a new way of thinking.

---

## 2. Python ↔ Go cheat-sheet (the everyday essentials)

### Variables & types, zero values

```python
# Python — dynamic, no declaration
count = 0
name = "huffman"
ratio = 0.0
```
```go
// Go — static types, two ways to declare
var count int            // explicit type, gets the ZERO VALUE: 0
name := "huffman"        // := infers the type (string) from the value
var ratio float64        // zero value 0.0
```

**Zero values** are Go's killer convenience: every variable is usable
immediately, no `None`/`undefined`. `int`→`0`, `float64`→`0.0`, `bool`→`false`,
`string`→`""`, pointers/slices/maps→`nil`. This is why our structs work
unconfigured:

> `internal/bitio/bitio.go:28` — `NewWriter` returns `&BitWriter{w: bufio.NewWriter(w)}`.
> We set only `w`; the `cur byte` and `nbits uint` fields default to their zero
> values (`0`, `0`) — exactly the "empty bit buffer" we want.

### Functions & multiple return values

```python
def base_hashes(data):
    ...
    return h1, h2          # Python: tuple
h1, h2 = base_hashes(data)
```
```go
func baseHashes(data []byte) (h1, h2 uint64) {  // named return values
    ...
    return h1, h2
}
h1, h2 := baseHashes(data)
```

Go has first-class multiple returns (not a packed tuple — genuinely separate
values). Named results double as documentation.

> Real example: `internal/bloom/hash.go:22` — `func baseHashes(data []byte) (h1, h2 uint64)`.

### Error handling — `if err != nil` vs try/except

This is the single biggest day-one adjustment. Go has **no exceptions for
ordinary errors**. Functions that can fail return an `error` as their *last*
return value, and you check it right there.

```python
try:
    out = os.open(out_path)
except OSError as e:
    print(f"error: {e}", file=sys.stderr)
    return 2
```
```go
out, err := os.Create(outPath)
if err != nil {
    fmt.Fprintf(os.Stderr, "error: %v\n", err)
    return 2
}
```

> This exact pattern is everywhere in `bloom-filter-spell-checker/main.go` — e.g.
> lines 117–121 (`os.Create`) and 161–165 (`os.Open`). The repeated
> `if err != nil { ... }` *is* idiomatic Go, not boilerplate to feel bad about.

Errors are just values. `WriteBit` returns one so callers can stop on a bad
write:

> `internal/bitio/bitio.go:32-49` — `func (bw *BitWriter) WriteBit(bit uint) error`
> returns the underlying `WriteByte` error, propagated up through `WriteBits`.

(`panic`/`recover` exist, but are reserved for truly unrecoverable bugs — the Go
equivalent of "this should never happen," not normal control flow.)

### Slices vs Python lists; arrays

A **slice** (`[]T`) is the everyday growable sequence — your `list`. An **array**
(`[N]T`) has a fixed compile-time length and is rarely used directly.

```python
words = []
words.append("the")     # grow
n = len(words)
first3 = words[0:3]     # slice
```
```go
var words []string                  // nil slice, ready to append
words = append(words, "the")        // append RETURNS a new slice header — reassign!
n := len(words)                     // length
c := cap(words)                     // capacity (allocated room before regrow)
first3 := words[0:3]                // half-open slice [0,3)
```

Two gotchas vs Python:
- `append` returns a (possibly relocated) slice — you must assign the result
  back: `words = append(words, x)`. Forgetting this is a classic bug.
- `len` is a built-in **function**, not a method: `len(words)`, never `words.len()`.

> Real examples:
> - `bloom-filter-spell-checker/main.go:214-221` — `var words []string` then
>   `words = append(words, w)` while scanning the dictionary.
> - `internal/huffman/tree.go:58-60` — building the priority queue:
>   `pq.items = append(pq.items, &node{...})`.
> - `internal/bloom/hash.go:41-47` — `out := make([]uint64, k)` preallocates a
>   slice of known length, then fills by index.

### Maps vs Python dict (comma-ok, iteration order)

```python
freqs = {}
freqs[b] = freqs.get(b, 0) + 1
val = freqs.get(b)            # None if missing
```
```go
freqs := make(map[byte]uint64)
freqs[b]++                    // missing key reads as zero value, then increments
val, ok := freqs[b]           // COMMA-OK: ok is false if key absent
```

The **comma-ok idiom** replaces Python's `in`/`.get()`: the second boolean tells
you whether the key existed (distinguishing "absent" from "present but zero").

> `internal/huffman/tree.go:32-37` — `CountFrequencies` is a one-liner thanks to
> zero-value-on-missing: `freqs[b]++` works even on the first sighting of a byte.

**⚠️ Iteration order is RANDOMIZED.** Unlike Python 3.7+ dicts (insertion
order), ranging over a Go map visits keys in a *deliberately random* order each
run. This bit us hard:

> **The Huffman map-determinism bug.** We store only the frequency *table* in the
> compressed file and rebuild the tree on decode. That only round-trips if encode
> and decode build the *identical* tree. Because map iteration is random, the
> heap's tie-break for equal frequencies could differ between runs → different
> (still valid) trees → corrupt output. **Fix:** never let ordering depend on map
> iteration. We key each leaf's tie-break `order` on its byte value, and give
> internal nodes ids from 256 up in creation order. See the long comment at
> `internal/huffman/tree.go:54-60` and the `order int` field at `tree.go:27`.

Lesson for every future Go challenge: **if correctness depends on order, never
derive it from map iteration.**

### Structs + methods + interfaces vs Python classes

Go has no classes and **no inheritance**. You compose: a `struct` holds data,
and *methods* are functions with a receiver attached.

```python
class BitWriter:
    def __init__(self, w):
        self.w = BufferedWriter(w)
        self.cur = 0
        self.nbits = 0
    def write_bit(self, bit):
        ...
```
```go
type BitWriter struct {        // data
    w     *bufio.Writer
    cur   byte
    nbits uint
}

func NewWriter(w io.Writer) *BitWriter {        // "constructor" by convention
    return &BitWriter{w: bufio.NewWriter(w)}
}

func (bw *BitWriter) WriteBit(bit uint) error { // method: receiver (bw *BitWriter)
    ...
}
```

> Straight from `internal/bitio/bitio.go:20-49`. `bw` is the explicit `self`. The
> `*BitWriter` (pointer) receiver lets the method mutate the struct's fields.

**Interfaces** are satisfied *implicitly* — no `implements` keyword. If a type
has the right methods, it fits the interface. Our writers accept any
`io.Writer`:

> `internal/bitio/bitio.go:27` — `func NewWriter(w io.Writer) *BitWriter`. An
> `*os.File`, a `bytes.Buffer`, or a network socket all satisfy `io.Writer`
> (they each have a `Write([]byte) (int, error)` method), so the same code writes
> to files in production and to in-memory buffers in tests.

Another real interface: our min-heap implements the standard library's
`heap.Interface` by providing `Len/Less/Swap/Push/Pop` — then `container/heap`
drives it.

> `internal/huffman/heap.go:12-43` — `priorityQueue` "becomes" a heap purely by
> having those five methods. No declaration that it implements anything.

### `defer` vs context managers / `finally`

`defer` schedules a call to run when the surrounding function returns — Go's
answer to `with`/`try…finally`. Perfect for "close what I opened."

```python
with open(path) as file:
    ...                 # auto-closed on block exit
```
```go
file, err := os.Open(path)
if err != nil {
    return nil, err
}
defer file.Close()      // runs no matter how the function returns
...
```

> `bloom-filter-spell-checker/main.go:208-212` (`defer file.Close()`),
> `:122` (`defer out.Close()`), `:166` (`defer in.Close()`). Deferred calls run
> in LIFO order if you stack several.

### Packages, imports, exported vs unexported, go.mod

- A **package** is a directory of `.go` files sharing the first-line
  `package X` declaration.
- **Visibility is capitalization**, not keywords. `CapitalizedName` is
  *exported* (public, usable from other packages). `lowercaseName` is package-
  private. There is no `public`/`private`.

```go
func CountFrequencies(...)   // exported — callable as huffman.CountFrequencies
func buildTree(...)          // unexported — internal to package huffman
```

> Compare `internal/huffman/tree.go`: `CountFrequencies` (line 32, public API) vs
> `buildTree` / `buildCodes` (lines 48, 87 — implementation details hidden from
> the CLI).

- `go.mod` declares the module path (the import prefix). Ours are deliberately
  short:

> `huffman-compression/go.mod` → `module huffman`, so internal code imports as
> `"huffman/internal/huffman"` (see `main.go:20`). The Bloom module is
> `module bloom`, imported as `"bloom/internal/bloom"` (`main.go:27`).

- The special `internal/` directory is a Go rule: packages under `internal/` can
  only be imported by code rooted at the same parent. It enforces "this is
  private to my project."

### Goroutines & channels (a 30-second preview)

You won't need these in Huffman/Bloom, but you'll meet them constantly in the
**networking phase** (DNS, port scanner, proxies):

```go
ch := make(chan int)
go worker(ch)          // `go` launches a concurrent goroutine (cheap, ~KB stack)
result := <-ch         // receive from a channel (blocks until a value arrives)
```

Mental model: a goroutine is "a function running concurrently," and a channel is
"a typed, thread-safe queue you pass values through." Think Python's
`threading`/`queue.Queue`, but goroutines are far lighter and channels are the
idiomatic way to communicate. We'll teach these properly when a challenge needs
them.

---

## 3. Code walkthrough: `internal/bitio/bitio.go` line-by-region

This file is small, self-contained, and packed with Go-isms a Pythonista trips
on. Open it alongside this section.

**(a) Package doc comment — lines 1–9.** A comment block immediately above
`package bitio` *is* the package documentation (surfaced by `go doc`). Go
documents by convention: comments that start with the name of the thing they
describe.

**(b) Imports — lines 11–14.** `import ( "bufio"; "io" )`. Only standard-library
packages, no third-party deps. Unused imports are a **compile error** in Go (not
a warning) — a deliberate tidiness rule that surprises Python devs.

**(c) The struct — lines 20–24.**
```go
type BitWriter struct {
    w     *bufio.Writer
    cur   byte // bit buffer; bits fill from MSB toward LSB
    nbits uint // number of valid bits currently in cur (0..7)
}
```
`byte` is an alias for `uint8` — a single 8-bit byte, *not* Python's `bytes`
object. `*bufio.Writer` is a **pointer** (the `*`). Pointers exist in Go but
without pointer arithmetic; think "a reference I can mutate through."

**(d) Constructor — lines 27–29.** `NewWriter` returns `&BitWriter{...}`. The
`&` takes the address, returning a `*BitWriter`. We only set `w`; `cur` and
`nbits` get **zero values** automatically (Section 1's payoff).

**(e) `WriteBit` — lines 32–49.** The heart of the file:
```go
func (bw *BitWriter) WriteBit(bit uint) error {
    bw.cur <<= 1            // shift left to open the next bit slot
    if bit != 0 {          // NO truthiness — must compare explicitly
        bw.cur |= 1        // bitwise OR to set the low bit
    }
    bw.nbits++
    if bw.nbits == 8 {     // a full byte accumulated
        if err := bw.w.WriteByte(bw.cur); err != nil {  // declare err IN the if
            return err
        }
        bw.cur = 0
        bw.nbits = 0
    }
    return nil             // nil = "no error"
}
```
Pythonista trip-points, all in nine lines:
- `(bw *BitWriter)` is the receiver — Go's explicit `self`. Pointer receiver →
  mutations to `bw.cur`/`bw.nbits` persist.
- `if bit != 0` — Go has **no truthiness**. You cannot write `if bit:`. A
  condition must be a real `bool`.
- `<<=`, `|=`, `&` — bit operators on a fixed-width `byte`. This is the precise
  control Go gives you over Python's unbounded `int`.
- `if err := bw.w.WriteByte(bw.cur); err != nil` — the **init-statement form**:
  declare `err` *scoped to the if* and test it in one line. Hugely common idiom.
- `return nil` — the zero value of `error` means success.

**(f) `Flush` — lines 71–81.** Emits the final partial byte, left-justified and
zero-padded (`bw.cur <<= (8 - bw.nbits)`), then flushes the buffered writer.
This is why a caller **must** `defer bw.Flush()` — otherwise the last few bits
never hit disk. (Compare context-manager `__exit__`.)

**(g) `BitReader.ReadBit` — lines 98–111.** The mirror image:
```go
func (br *BitReader) ReadBit() (uint, error) {
    if br.nbits == 0 {
        b, err := br.r.ReadByte()
        if err != nil {
            return 0, err        // propagate io.EOF to the caller
        }
        br.cur = b
        br.nbits = 8
    }
    br.nbits--
    bit := uint(br.cur>>br.nbits) & 1   // explicit uint(...) conversion
    return bit, nil
}
```
Note `uint(br.cur>>br.nbits)` — Go requires **explicit numeric conversions**;
there's no implicit `byte`→`uint` promotion. And returning `(0, err)` on failure
is the multiple-return error pattern again.

---

## 4. Things that will surprise a Pythonista

- **No truthiness.** `if x:` is illegal unless `x` is a `bool`. Write
  `if bit != 0`, `if len(words) == 0`, `if err != nil`. (See `bitio.go:37`,
  `main.go:103`.)
- **Explicit conversions.** No implicit numeric coercion. You'll write
  `float64(n)`, `uint64(len(words))`, `int(sym)`, `uint(br.cur>>n)`. Examples:
  `bloom.go:66` (`-float64(n)*math.Log(p)`), `main.go:108`
  (`uint64(len(words))`), `tree.go:59` (`order: int(sym)`).
- **`nil` is not `None`.** `nil` is the zero value for pointers, slices, maps,
  interfaces, channels, and funcs — but it's typed. A `nil` slice is still safe
  to `len()` and `append()` to; a `nil` map is safe to *read* but panics on
  write. (Our `var words []string` then `append` works precisely because nil
  slices append fine — `main.go:214`.)
- **Value vs pointer receivers.** `func (bw *BitWriter)` (pointer) can mutate the
  struct; `func (pq *priorityQueue) Len() int` uses a pointer too. A *value*
  receiver gets a copy and can't mutate the original. Rule of thumb: use pointer
  receivers when the method changes state or the struct is large. (All of
  `bitio.go`'s methods are pointer receivers — they mutate the bit buffer.)
- **Capitalization = visibility.** `Add` is public, `loadBits` is private — same
  struct, different reach (`bloom.go:93` vs `:129`). Renaming a field's first
  letter changes its API surface.
- **No list comprehensions / no `map`/`filter` sugar.** You write explicit
  `for` loops. `for i := uint64(0); i < k; i++ { out[i] = ... }`
  (`hash.go:43-47`) is the Go way to build a list — verbose but obvious.
- **`:=` vs `var`.** `:=` declares *and* infers type, only usable inside
  functions, and the variable must be new. `var` works at package level, lets
  you state the type, and gives the zero value when you omit the initializer.
  Use `:=` for "assign me this value" (`name := "huffman"`), `var` for "give me
  a zero-valued X" (`var count int`). Mixing them up — e.g. `:=` on an existing
  variable — is a compile error.
- **`for` is the only loop.** No `while`. A bare `for cond {}` is your while
  loop; `for {}` is an infinite loop. (`bitset.go:61` — `for x != 0` is a
  while-style loop counting bits.)
- **Unused variables/imports are compile errors,** not warnings. Go forces you
  to clean up as you go.

---

## 5. A practical fast-track plan

You learn Go fastest by reading code you already understand the *problem* behind
— and you have two such programs sitting in this repo.

**Step 1 — A Tour of Go (½ day).** Run through <https://go.dev/tour/>. Don't try
to memorize; just get syntax exposure for variables, slices, maps, structs,
methods, interfaces, goroutines. Skim the concurrency section — you'll revisit it
in Phase 4.

**Step 2 — Re-read our two Go programs (½ day), in this order:**
1. `internal/bitio/bitio.go` — smallest, walked through in Section 3.
2. `internal/bloom/bitset.go` — packed bits, the Kernighan popcount loop.
3. `internal/bloom/hash.go` then `bloom.go` — double hashing + the sizing math.
4. `internal/huffman/heap.go` (interface satisfaction) then `tree.go`
   (the map-determinism story) then `codec.go` (the file format).
5. The two `main.go` files — hand-rolled flag parsing, exit codes, `defer`.

**Step 3 — Modify them as exercises (the real learning).** Pick 2–3:

1. **Add a `-v/--verbose` flag to the Bloom CLI.** In
   `bloom-filter-spell-checker/main.go`'s `doCheck`, parse a `-v` flag (mirror
   the existing `-f` case in the arg loop at lines 142–154) and, when set, also
   print each word's k hash indices using `bloom.hashes(...)`. *Skills: slices,
   flag parsing, calling into a package.*

2. **Print the Huffman code table.** Add a `-table` flag to `huffman compress`
   that, after building codes, prints each `symbol → bitstring`. You'll iterate
   the `map[byte]string` from `buildCodes` (`tree.go:87`) — and since map order
   is random, **sort the keys first** to get stable output (lesson from the
   determinism bug!). *Skills: maps, sorting (`sort` pkg), the iteration-order
   gotcha.*

3. **Add a `count` subcommand to the Bloom CLI** that loads a filter and prints
   `f.Bits().Count()` (the popcount) and `f.EstimatedFalsePositiveRate()`. Wire
   it into the `switch` in `run` (`main.go:41-53`). *Skills: methods, returning
   exit codes, struct accessors.*

After each change, run `go fmt`, `go vet`, and `go test ./...` (next section) to
confirm you didn't break anything.

---

## 6. How to run, build, test Go in this repo

Run all of these **from inside a challenge directory** (the one with `go.mod`),
e.g. `cd phase-01-foundations/huffman-compression`.

| Command | What it does | Example in this repo |
| --- | --- | --- |
| `go run .` | Compile + run the current package without leaving a binary | `go run . compress -o out.huf README.md` |
| `go build` | Compile to a binary (named after the module) | `go build` → `./huffman compress README.md` |
| `go test ./...` | Run every `*_test.go` in this module, recursively | `go test ./...` runs `bitio_test.go`, `codec_test.go`, … |
| `go vet ./...` | Static analysis for likely bugs (suspicious printf, etc.) | `go vet ./...` |
| `go fmt ./...` | Auto-format to canonical Go style (non-negotiable in Go) | `go fmt ./...` |

**Concrete sessions:**

```bash
# Huffman: build, round-trip a file, verify
cd phase-01-foundations/huffman-compression
go test ./...                       # all tests green?
go run . compress   -o README.huf README.md
go run . decompress -o README.out README.huf
diff README.md README.out           # identical → lossless

# Bloom: build a filter, check words
cd ../bloom-filter-spell-checker
go test ./...
go run . build -p 0.01 -o words.bf testdata/words.txt
go run . check -f words.bf apple recieve receive
echo $?                             # exit 1 if any word flagged MISSPELLED
```

**Notes:**
- Tests live **beside** the code as `*_test.go` (e.g.
  `internal/bitio/bitio_test.go`) — *not* in a separate `tests/` folder like our
  Python challenges. That's the Go idiom.
- `go test ./...` and `go vet ./...` are the two gates every Go challenge here
  must pass before it's marked done.
- Don't commit build artifacts — each Go challenge's `.gitignore` already covers
  the binary and `*.huf` / `*.bf` outputs.

---

## 7. Where to go next

- **Official:** [A Tour of Go](https://go.dev/tour/) ·
  [Effective Go](https://go.dev/doc/effective_go) ·
  [Go by Example](https://gobyexample.com/) (Python-dev friendly, snippet-based).
- **In this repo:** the per-challenge READMEs in
  `phase-01-foundations/huffman-compression/` and `…/bloom-filter-spell-checker/`
  teach the *algorithms*; this guide teaches the *language* underneath them.
- **Coming up:** the networking phase (Phase 4) is where goroutines, channels,
  and `net` go from "preview" to "daily driver."

> Keep this open in a second pane while you read Go code. The fastest way to
> learn Go is to recognize each construct as "oh, that's just the Python thing,
> spelled differently."
