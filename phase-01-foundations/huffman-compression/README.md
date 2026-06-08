# Huffman Compression

> **Phase:** 1 — Foundations: Parsing, Encoding & Data Structures
> **Difficulty:** 🔵
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** M

**Status:** ✅ Done

---

## 🎯 What We're Building

A real, lossless file compressor — `huffman compress` shrinks a file, and
`huffman decompress` restores it **byte-for-byte identical** to the original.
This is the classic [codingchallenges.fyi "build your own compression
tool"](https://codingchallenges.fyi/challenges/challenge-huffman/) challenge.

```
$ huffman compress -o book.huf book.txt
compressed book.txt (34000 bytes) -> book.huf (19451 bytes)
compression ratio: 0.572 (57.2% of original, saved 42.8%)

$ huffman decompress -o book.out book.huf
decompressed book.huf (19451 bytes) -> book.out (34000 bytes)

$ cmp book.txt book.out      # silence == identical
```

Why this matters as a *learning* challenge: it forces you to confront two skills
that show up everywhere in systems programming:

1. **Greedy tree-building over a priority queue (min-heap).** The same
   "repeatedly take the two smallest" pattern appears in Dijkstra, A\*, and
   scheduling.
2. **Bit-level I/O.** Files and sockets speak in *bytes*, but compression speaks
   in *bits*. You must build a **bit-writer** and **bit-reader** that pack and
   unpack sub-byte values. That exact skill is reused later in this curriculum
   for `tar`, `xxd`, and binary network protocols like **DNS** and **NTP**.

Huffman coding (David Huffman, 1952) is not a museum piece — it is a building
block inside DEFLATE (`gzip`, `zip`, PNG), JPEG, and MP3. Understanding it
demystifies how `gzip` can take a 100 KB log file down to 10 KB.

---

## 📚 Core Concepts

### The problem with fixed-width encoding

ASCII/UTF-8 spends a **fixed 8 bits on every byte**, whether it is a super-common
space character or a rare `~`. In English text the letter `e` might appear 80×
more often than `q`, yet both cost 8 bits. That is wasteful.

> **Key insight:** if some symbols are more frequent than others, we can spend
> *fewer* bits on the frequent ones and *more* on the rare ones, and come out
> ahead on average.

### Information theory: entropy is the speed limit

Claude Shannon (1948) gave us the exact lower bound. For a source where symbol
`s` occurs with probability `p(s)`, the **entropy** is:

```
H  =  − Σ  p(s) · log₂ p(s)      bits per symbol
```

Entropy is the *minimum average bits per symbol* any symbol-by-symbol coder can
achieve. Some intuition:

- A file of one repeated byte (`p = 1`) has entropy **0** — it carries no
  surprise, so it should compress to almost nothing. (Our header still costs a
  few bytes, which is honest.)
- A file where all 256 byte values are equally likely has entropy **8 bits** —
  it is incompressible by this technique; there is no skew to exploit.
- Real English text sits around **4–5 bits/char**, so we expect ~40–50% savings.

Huffman coding gets to **within one bit of `H`** on average, and is **provably
optimal** among all coders that map each symbol to a whole number of bits. (To
beat it you need *arithmetic coding* or *range coding*, which let a symbol cost a
fractional bit — out of scope here.)

### Prefix codes: how variable-length codes stay unambiguous

If `e` = `0` and another symbol = `01`, then the stream `01` is ambiguous: is it
`e` then something, or the single 2-bit symbol? We forbid this. A **prefix code**
(a.k.a. prefix-free code) is one where **no code is a prefix of another**. Then a
left-to-right scan is unambiguous — the moment you recognize a code you know it
is complete.

Prefix codes correspond exactly to **binary trees**: every symbol is a *leaf*,
and the path from the root spells its code (`0` = go left, `1` = go right).
Because symbols only live at leaves, no code can be a prefix of another. 🎉

```
                (root)
               0/     \1
              /        \
           (•)          'a'        a = 1
          0/  \1
         'b'  'c'                  b = 00 , c = 01
```

Decoding is just "walk from the root, one bit per edge; on reaching a leaf, emit
its symbol and jump back to the root."

### Huffman's greedy algorithm

How do we choose the tree that minimizes total bits? Greedily:

1. Make a leaf node for every symbol, weighted by its frequency.
2. Put them all in a **min-heap** keyed by frequency.
3. Pop the **two smallest** nodes; make a new parent whose frequency is their
   sum; push the parent back.
4. Repeat until one node remains — that is the root.

The two least-frequent symbols end up *deepest* in the tree (longest codes), and
the most-frequent symbols end up *shallow* (shortest codes). That is exactly the
trade we wanted. The greedy choice is optimal — proven by an exchange argument
(any optimal tree can be reshaped into the Huffman tree without increasing cost).

**Why a min-heap?** The algorithm's hot loop is "give me the two smallest." A
min-heap does extract-min and insert in `O(log n)`, so the whole build is
`O(n log n)` over `n` distinct symbols — versus `O(n²)` if you rescanned a list
each merge. Go's standard library ships `container/heap`, which we use.

---

## 🏗️ Architecture & Design

### Data flow

```
COMPRESS
  input bytes ──► count frequencies ──► build Huffman tree ──► derive code table
                                                                       │
        ┌──────────────────────────────────────────────────────────────┘
        ▼
  write HEADER (so decode can rebuild the tree)  ──►  bit-pack body ──► .huf file


DECOMPRESS
  .huf file ──► read HEADER ──► rebuild identical tree ──► bit-read body,
                                                           walk tree ──► output bytes
```

### Package layout

Following the repo's convention (teaching-sized modules, one responsibility
each), source lives in small internal packages:

```
huffman-compression/
├── go.mod                       module "huffman"
├── main.go                      CLI: subcommands, flags, exit codes
├── .gitignore                   ignore compiled binary + *.huf artifacts
├── README.md                    ← you are here
└── internal/
    ├── bitio/                   the reusable bit-level I/O primitive
    │   ├── bitio.go             BitWriter / BitReader (MSB-first)
    │   └── bitio_test.go
    └── huffman/                 the algorithm + container format
        ├── tree.go              node, frequency count, tree build, code table
        ├── heap.go              min-heap priority queue (container/heap)
        ├── codec.go             Compress / Decompress / header / Ratio
        └── codec_test.go
```

`bitio` is deliberately split out from `huffman`: bit packing is a
general-purpose primitive with **no knowledge of Huffman**. Keeping it separate
makes it trivially reusable for `tar`/`xxd`/DNS later, and trivially unit-testable
on its own.

### The header-format decision (the crux of the design)

The compressed file is useless unless the decoder can rebuild the **exact same
tree** the encoder used. So we must persist enough information in a header. We
considered three options:

| Option | What you store | Overhead | Notes |
|---|---|---|---|
| **A. Frequency table** ✅ chosen | each distinct `symbol` + its `frequency` | small (≤ ~256 × a few bytes) | Decoder reruns the *same* build algorithm → identical tree. Simplest to reason about and verify. |
| B. Canonical Huffman code lengths | one **code length** per symbol + a canonicalization rule | smaller (1 byte/symbol) | The format `gzip`/DEFLATE actually uses. Codes are regenerated from lengths by a fixed rule, so the tree itself is never stored. More moving parts to get right. |
| C. Serialize the tree shape | a bit per node (leaf vs internal) + leaf symbols | smallest for many inputs | Compact, but you must hand-serialize/deserialize the tree structure carefully. |

We chose **(A) store the frequency table**. For a teaching implementation it is
the clearest: *"the decoder literally rebuilds the tree the same way the encoder
did."* The overhead is at most a few hundred bytes and is negligible for any
non-trivial file. The README-worthy trade-off is that canonical Huffman (B) is
the production choice precisely because it shaves that header overhead and avoids
shipping frequencies at all.

> **The one subtlety that makes (A) work:** the tree must be built
> **deterministically**. When two nodes have equal frequency, the heap must break
> the tie the *same way* on encode and decode. Go randomizes map iteration order,
> so we must **not** let tie-breaks depend on it. We key each leaf's tie-break on
> its byte value (0–255) and give internal nodes ids starting at 256 in creation
> order. Same frequencies in → identical tree out, every time. (This bug is easy
> to hit and was caught by the round-trip tests — see Testing.)

### File format (HUF1)

All multi-byte integers are big-endian (network byte order):

```
+---------+--------------+--------------+-----------------------------+---------------+
| "HUF1"  | totalSymbols | numDistinct  | table: numDistinct entries  | packed bits   |
| 4 bytes | uint64       | uint16       | [ symbol:1B | freq:uvarint ] | body          |
+---------+--------------+--------------+-----------------------------+---------------+
```

- **magic `"HUF1"`** — identifies the format/version; a wrong magic is rejected
  cleanly instead of misread.
- **totalSymbols** — the count of bytes in the *original* file. This is what
  makes the final partial byte safe (see bit I/O below): the decoder stops after
  exactly this many symbols and never mistakes zero-padding for real data.
- **numDistinct + table** — the frequency of each byte value that appears.
  Frequencies use a **uvarint** so small counts cost 1 byte instead of 8.

### CLI & exit codes (repo convention)

```
huffman compress   [-o out] <input>     # alias: c   (default out: <input>.huf)
huffman decompress  -o out  <input>     # alias: d
```

| Code | Meaning |
|---|---|
| `0` | success |
| `1` | domain failure (corrupt/invalid `.huf`, decode error) |
| `2` | usage / I/O error (bad args, file not found) |

---

## 🔨 Step-by-Step Implementation

The build mirrors the challenge's own incremental steps. Each step is small,
testable, and lands a real capability.

### Step 1 — Frequency analysis (`tree.go`)

Read the whole input and tally a `map[byte]uint64`. Only symbols that actually
occur appear in the map.

```go
func CountFrequencies(data []byte) map[byte]uint64 {
    freqs := make(map[byte]uint64)
    for _, b := range data {
        freqs[b]++
    }
    return freqs
}
```

### Step 2 — Build the tree with a min-heap (`heap.go` + `tree.go`)

Implement `heap.Interface` over a slice of `*node`, ordering by frequency with a
**deterministic** tie-break:

```go
func (pq *priorityQueue) Less(i, j int) bool {
    a, b := pq.items[i], pq.items[j]
    if a.freq != b.freq {
        return a.freq < b.freq
    }
    return a.order < b.order   // stable tie-break → identical tree on both sides
}
```

Then run the greedy merge loop until one root remains. Two special cases matter:

- **0 symbols** (empty file) → no tree (`nil`); there is nothing to encode.
- **1 distinct symbol** (e.g. `"aaaa"`) → a lone leaf. A tree of depth 0 would
  give an *empty* code, so by convention that symbol gets the 1-bit code `"0"`.

### Step 3 — Derive the code table

Walk the tree, appending `'0'` going left and `'1'` going right; record the
bit-string at each leaf. Result: `map[byte]string`, e.g. `{'a': "1", 'b': "00"}`.

### Step 4 — The bit-writer (`bitio.go`)

This is the heart of the challenge. We accumulate bits **most-significant-first**
into a one-byte buffer; when 8 bits are queued we emit the byte. `Flush`
left-justifies and writes any final partial byte (zero-padded on the right):

```go
func (bw *BitWriter) WriteBit(bit uint) error {
    bw.cur <<= 1
    if bit != 0 { bw.cur |= 1 }
    bw.nbits++
    if bw.nbits == 8 {
        err := bw.w.WriteByte(bw.cur)
        bw.cur, bw.nbits = 0, 0
        return err
    }
    return nil
}
```

> **Why is the zero-padding safe?** Because `totalSymbols` in the header tells the
> decoder exactly how many symbols to emit. It stops at the last real symbol and
> never reads into the padding bits. This is the standard way to handle "the last
> byte isn't full."

### Step 5 — Encode (compress) (`codec.go`)

Write the header, then stream every input byte's code through the bit-writer and
`Flush`. Done.

### Step 6 — The bit-reader + decode (`bitio.go` + `codec.go`)

The bit-reader is the mirror image: refill a one-byte buffer from the stream and
hand out bits MSB-first. Decoding then walks the tree:

```go
n := root
for uint64(len(out)) < total {           // stop after exactly `total` symbols
    bit, err := br.ReadBit()
    if err != nil { return nil, fmt.Errorf("truncated body: %w", err) }
    if bit == 0 { n = n.left } else { n = n.right }
    if n.leaf { out = append(out, n.symbol); n = root }
}
```

### Step 7 — CLI + compression ratio (`main.go`)

Parse `compress`/`decompress` (+ `c`/`d` aliases) and an optional `-o`, wire up
exit codes, and report the ratio: `compressed / original` (smaller is better),
plus the human-friendly "saved X%".

---

## 🧪 Testing Strategy

Run everything from this directory:

```bash
go test ./...      # all unit + round-trip tests
go vet ./...       # static checks
go build -o huffman . && ./huffman compress -o demo.huf README.md \
  && ./huffman decompress -o demo.out demo.huf && cmp README.md demo.out && echo OK
```

The correctness gate, per repo convention, is a **property the answer must
satisfy** rather than hand-checked outputs. Here that property is the
**lossless round-trip**: `Decompress(Compress(x)) == x` for *every* input `x`.

**`internal/bitio` — bit primitive unit tests**
- Write a deliberately **non-byte-aligned** 13-bit pattern, flush, read it back
  bit-for-bit (proves the partial-byte padding logic).
- `WriteBits("10110010")` produces exactly `0xB2` (proves MSB-first ordering).
- Reading an empty stream returns `io.EOF`; flushing an empty writer writes 0
  bytes.

**`internal/huffman` — algorithm + codec tests**
- **Round-trip over varied inputs**, hitting every edge case the challenge calls
  out:
  - empty file (0 bytes),
  - single byte (`"A"`),
  - single-symbol run (`"aaaa…"`, the depth-0-tree special case),
  - normal text, highly repetitive text,
  - **all 256 byte values**,
  - random **binary** data (incompressible — must still round-trip),
  - newline-heavy text.
- **Compression actually shrinks** a skewed distribution (9000×`a` + 1000×`b`).
- **Prefix-free property**: no symbol's code is a prefix of another's.
- **Corrupt input rejected**: wrong magic / empty input → `ErrBadFormat`.
- A **randomized fuzz loop** (50 iterations, seeded) builds many tree shapes via
  varying small alphabets and asserts the round-trip every time. *This is the
  test that caught the map-iteration determinism bug:* sizes matched but bytes
  didn't, which only happens when encode and decode build different trees.

All tests pass, `go vet` is clean, and a real file round-trips identically.

---

## 💡 Key Takeaways

- **Variable-length + prefix-free = free lunch on skewed data.** Spend short
  codes on frequent symbols; a prefix code keeps the stream decodable without
  separators. Prefix codes *are* binary trees.
- **Entropy is the speed limit.** Huffman gets within 1 bit of Shannon's entropy
  and is optimal among whole-bit symbol coders. No skew (entropy ≈ 8 bits/byte) →
  no compression; that's physics, not a bug.
- **Greedy + min-heap** builds the optimal tree in `O(n log n)`. Reach for
  `container/heap` rather than hand-rolling sift operations.
- **Bit I/O is its own skill.** A bit-writer accumulates bits into a byte buffer
  and flushes; the reader mirrors it. Handle the final partial byte by recording
  the symbol count in the header so padding is never misread. **This primitive
  is reused in `tar`, `xxd`, DNS, and NTP** — that is why we isolated it in its
  own package.
- **Self-describing formats need a determinism contract.** Storing the frequency
  table is only correct if both sides rebuild the *identical* tree. Beware
  language-level nondeterminism (Go's randomized map order) leaking into
  tie-breaks. Make tie-breaks depend on stable data (the symbol value), not
  iteration order.
- **The round-trip is the oracle.** `decompress(compress(x)) == x` over a wide,
  edge-case-rich corpus (plus a seeded fuzzer) is a cheap, devastating
  correctness test — it caught a subtle non-determinism bug that size checks
  alone would have missed.

---

## 📖 Further Reading

- **Challenge spec:** [Build Your Own Compression Tool — codingchallenges.fyi](https://codingchallenges.fyi/challenges/challenge-huffman/)
- **Original paper:** D. A. Huffman, *"A Method for the Construction of
  Minimum-Redundancy Codes,"* Proc. IRE, 1952.
- **Information theory:** C. E. Shannon, *"A Mathematical Theory of
  Communication,"* 1948 (entropy, the source coding theorem).
- **Go stdlib:** [`container/heap`](https://pkg.go.dev/container/heap),
  [`encoding/binary`](https://pkg.go.dev/encoding/binary) (uvarint, big-endian),
  [`bufio`](https://pkg.go.dev/bufio).
- **Where it's used in the wild:** DEFLATE / RFC 1951 (gzip, zip, PNG) pairs
  Huffman with LZ77; JPEG and MP3 use Huffman tables on their quantized
  coefficients.
- **What beats Huffman:** *arithmetic coding* / *range coding* and *ANS*
  (asymmetric numeral systems, used in Zstandard and modern codecs) encode
  symbols in *fractional* bits and so can dip below Huffman's whole-bit floor.
- **Next in this curriculum:** the bit-reader/bit-writer here is the prerequisite
  for `tar` (Challenge 19) and the binary `DNS`/`NTP` protocols (Phase 4).
