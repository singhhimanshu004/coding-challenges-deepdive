# Bloom Filter Spell Checker

> **Phase:** 1 — Foundations: Parsing, Encoding & Data Structures  
> **Difficulty:** 🔵  
> **Recommended Language:** 🟦 Go  
> **Effort Estimate:** S

**Status:** ✅ Done

---

## 🎯 What We're Building

A **spell checker** that can tell you, instantly, whether a word is in a
dictionary of hundreds of thousands of words — while using a tiny fraction of
the memory a normal hash set would need.

The trick is a **Bloom filter**: a probabilistic data structure that answers
"have I seen this before?" using nothing but a bit array and a few hash
functions. It never stores the words themselves.

We build a two-phase command-line tool:

```text
# Phase 1 — build the filter once from a word list
$ bloom build -p 0.01 -o words.bf /usr/share/dict/words
built filter from /usr/share/dict/words: 235886 words
  m = 2260992 bits (276.0 KB), k = 7 hashes
  target false-positive rate: 0.01
  saved to words.bf

# Phase 2 — check words against the saved filter
$ bloom check -f words.bf recieve receive teh the
recieve              MISSPELLED (definitely not in dictionary)
receive              probably present
teh                  MISSPELLED (definitely not in dictionary)
the                  probably present
```

The whole 235k-word English dictionary compresses to ~276 KB of bits with a 1%
error rate. A `map[string]struct{}` holding the same words would cost several
megabytes.

---

## 📚 Core Concepts

### What a Bloom filter actually is

A Bloom filter is a bit array of `m` bits (all 0 at first) plus `k` hash
functions. Two operations:

- **Add(x):** hash `x` with all `k` functions to get `k` positions, set those
  bits to 1.
- **Contains(x):** hash `x` the same way; if **all** `k` bits are 1, answer
  "probably present"; if **any** bit is 0, answer "definitely not present".

```text
m = 16 bits, k = 3

Add("cat"):  hashes -> {2, 7, 11}
index:  0  1  2  3  4  5  6  7  8  9 10 11 12 13 14 15
bit:    0  0 [1] 0  0  0  0 [1] 0  0  0 [1] 0  0  0  0

Add("dog"):  hashes -> {2, 9, 14}      (note: bit 2 already set — shared!)
index:  0  1  2  3  4  5  6  7  8  9 10 11 12 13 14 15
bit:    0  0 [1] 0  0  0  0 [1] 0 [1] 0 [1] 0  0 [1] 0

Contains("cat") -> bits 2,7,11 all 1  -> "probably present"  ✓
Contains("cow") -> hashes {2, 5, 11}
                   bit 5 is 0          -> "definitely not present"  ✓
```

### Why false positives but NEVER false negatives

This asymmetry is the entire personality of a Bloom filter.

- **No false negatives.** When you `Add(x)`, you *set* its `k` bits. They can
  never go back to 0 (a plain Bloom filter has no delete). So when you later
  check `x`, those bits are guaranteed still 1 → it always reports present. **If
  you inserted it, you will always find it.**

- **False positives are possible.** Bits are *shared* between words. A word you
  never inserted might happen to hash to `k` positions that were all set by
  *other* words. Then it falsely reports "probably present." In the diagram
  above, imagine a word `"emu"` hashing to `{2, 7, 9}` — all already set by cat
  and dog — it would look present even though we never added it.

So the answer is one-sided:

| Filter says | Reality |
|---|---|
| "definitely not present" | **Certain.** The word is truly absent. |
| "probably present" | **Probably** true, but could be a false positive. |

For a spell checker this is exactly the right trade: a "definitely not present"
verdict reliably flags a misspelling. A false positive just means we
occasionally let a genuinely-misspelled word slip through unflagged — annoying,
but never wrong about a *correct* word.

### The math: choosing `m` and `k`

Given `n` items to store and a target false-positive rate `p`, two formulas give
the optimal sizing:

```text
        n · ln p
m  =  - ----------        (number of bits)
         (ln 2)²

        m
k  =  ( - ) · ln 2        (number of hash functions)
        n
```

**Where these come from (intuition, not full proof):**

After inserting `n` items into `m` bits using `k` hashes, the probability that
any one specific bit is *still 0* is:

```text
P(bit = 0)  =  (1 - 1/m)^(k·n)  ≈  e^(-k·n/m)
```

A false positive happens when all `k` bits for an absent word are 1, so:

```text
p  ≈  (1 - e^(-k·n/m))^k
```

Minimising this over `k` gives `k = (m/n)·ln 2`, which is the value that keeps
the array **almost exactly half full** (half the bits set to 1). That is the
sweet spot: fewer hashes underuses the array, more hashes saturates it — both
raise the error rate. Substituting the optimal `k` back and solving for `m`
gives the first formula.

**What the numbers feel like:** at the optimal `k`, you need about **9.6 bits
per item** for a 1% error rate, and roughly **+4.8 bits per item** for every
10× reduction in `p`. Accuracy costs space, linearly in the exponent.

| Items `n` | Target `p` | Bits `m` | Hashes `k` | Memory |
|---|---|---|---|---|
| 1,000,000 | 0.01 (1%)   | ~9.6 M  | 7  | ~1.2 MB |
| 1,000,000 | 0.001 (0.1%)| ~14.4 M | 10 | ~1.8 MB |
| 1,000,000 | 0.0001      | ~19.2 M | 13 | ~2.4 MB |

### Generating `k` hashes from `2` — double hashing

We need `k` *independent* hash functions, but running `k` separate hash
algorithms is slow and tedious. The **Kirsch–Mitzenmacher** result says you can
fake them all from just two base hashes:

```text
g_i(x) = h1(x) + i · h2(x)   mod m,   for i = 0 .. k-1
```

This family behaves, statistically, like `k` independent hashes with **no
measurable increase** in the false-positive rate. We get `h1` and `h2` for free
by computing one 64-bit **FNV-1a** hash and splitting it into its high and low
32-bit halves.

> **Why FNV-1a?** It is tiny, dependency-free, fast on short strings (dictionary
> words are short), and disperses bits well. It is **not** cryptographic — and
> it doesn't need to be. A Bloom filter only needs *good distribution*, not
> resistance to an adversary crafting collisions.

### Where Bloom filters live in production

They show up anywhere a cheap "is it even worth looking?" pre-check saves an
expensive operation:

- **Databases (Cassandra, HBase, RocksDB, LevelDB):** before hitting disk to
  look for a key in an SSTable, check a Bloom filter in RAM. "Definitely not
  present" → skip the disk read entirely. This is their killer use case.
- **CDNs & caches:** "is this object cached anywhere?" before a costly lookup.
  A common pattern avoids caching one-hit-wonders until they've been seen twice.
- **Google Chrome (historically):** checked URLs against a local Bloom filter of
  known-malicious sites; only on a "maybe" did it call the Safe Browsing server.
- **Bitcoin SPV clients:** request only transactions matching a Bloom filter,
  without revealing exactly which addresses they care about.
- **Medium / web platforms:** "has this user already seen this article?"

The pattern is always the same: **a fast, memory-cheap, no-false-negative filter
in front of a slow, expensive source of truth.**

---

## 🏗️ Architecture & Design

The code is split so each piece has exactly one responsibility, and the reusable
data-structure logic is isolated from the CLI.

```text
bloom-filter-spell-checker/
├── main.go                      # thin CLI: build / check subcommands, flags, I/O
├── go.mod                       # module "bloom"
├── internal/
│   ├── bloom/
│   │   ├── bitset.go            # packed bit array (Set/Test/Count)
│   │   ├── hash.go              # FNV-1a + double hashing → k indices
│   │   └── bloom.go             # Filter: optimal m/k, Add, Contains
│   └── codec/
│       └── codec.go             # serialize (Save) / deserialize (Load)
└── testdata/
    └── words.txt                # small sample dictionary for tests
```

**Layering, bottom to top:**

```text
   BitSet        ← knows only "flip bit i", "test bit i", "count ones"
     ↑
   hashing       ← turns a key into k bit indices (FNV-1a + double hashing)
     ↑
   Filter        ← computes optimal m/k, ties BitSet + hashing into Add/Contains
     ↑
   codec         ← writes/reads a Filter as a self-describing binary file
     ↑
   main (CLI)    ← reads dictionaries, parses flags, prints verdicts
```

`internal/bloom` has zero knowledge of files or the CLI; `internal/codec`
depends only on `bloom`; `main` orchestrates. This mirrors the Go layout used in
the Huffman challenge (algorithm in `internal/`, tests beside the code).

### The serialization header decision

Building a filter over a big dictionary is the slow part — we want to do it
**once** and reload instantly. So we serialize to a small **self-describing**
binary format, `BLM1`:

```text
offset  size  field
------  ----  -------------------------------------------
0       4     magic   "BLM1"   — identifies format + version
4       1     version (1)      — bump if the layout changes
5       8     m       uint64   — number of bits
13      8     k       uint64   — number of hash functions
21      8     nbytes  uint64   — length of the bit payload
29      N     bits             — the packed bit array (ceil(m/8) bytes)
```

All integers are **big-endian** (network byte order — the portable default).

**Why store `m` and `k` in the header?** The bit positions a key maps to depend
entirely on `m` and `k`. A reader that guessed different values would compute
*different* indices and every lookup would be wrong. Saving them makes the file
self-contained: `Load` needs nothing but the file itself. The **magic** lets us
reject garbage input fast, and **version** lets the format evolve later. Storing
`nbytes` lets the loader detect truncation/corruption (it must equal
`ceil(m/8)`).

---

## 🔨 Step-by-Step Implementation

Built bottom-up, each layer tested before the next.

### Step 1 — The bit array (`bitset.go`)

The smallest unit Go can address is a byte, but we need individual bits. So we
pack them: **bit `i` lives in byte `i/8`, at offset `i%8`.** Using a full byte
per bit would waste 8× the memory — and memory frugality is the whole point.

```go
func (b *BitSet) Set(i uint64)  { i %= b.n; b.bits[i/8] |= 1 << (i % 8) }
func (b *BitSet) Test(i uint64) bool {
    i %= b.n
    return b.bits[i/8]&(1<<(i%8)) != 0
}
```

Indices are reduced `mod n` so we can pass raw hash outputs without
bounds-checking first. `Count()` (population count, via Kernighan's
`x &= x-1` trick) lets us estimate the real fill ratio.

### Step 2 — Hashing to k positions (`hash.go`)

One FNV-1a 64-bit hash, split into two 32-bit halves, then double-hashed into
`k` indices:

```go
func hashes(data []byte, k, m uint64) []uint64 {
    h1, h2 := baseHashes(data)        // high/low 32 bits of FNV-1a
    out := make([]uint64, k)
    for i := uint64(0); i < k; i++ {
        out[i] = (h1 + i*h2) % m       // g_i(x) = h1 + i·h2
    }
    return out
}
```

> **Subtle bug guarded against:** if `h2` were 0, every derived hash would
> collapse to `h1` (because `i·0 == 0`), so the filter would effectively use a
> single bit. We force `h2` to be non-zero.

### Step 3 — The filter & the math (`bloom.go`)

`New(n, p)` computes optimal `m` and `k` from the formulas, then `Add` and
`Contains` are four lines each:

```go
func (f *Filter) Add(key []byte) {
    for _, idx := range hashes(key, f.k, f.m) { f.bits.Set(idx) }
}

func (f *Filter) Contains(key []byte) bool {
    for _, idx := range hashes(key, f.k, f.m) {
        if !f.bits.Test(idx) { return false }   // one 0 bit ⇒ definitely absent
    }
    return true
}
```

The early return in `Contains` is both faster and the literal expression of "any
unset bit means definitely-not-present."

### Step 4 — Serialize & load (`codec.go`)

`Save` writes the `BLM1` header then the raw bit bytes. `Load` validates the
magic and version, reads `m`/`k`/`nbytes`, checks `nbytes == ceil(m/8)` to catch
corruption, then rebuilds the filter via `bloom.FromParts(m, k, payload)`.

### Step 5 — The CLI (`main.go`)

Two subcommands, repo-standard exit codes:

- `build [-p rate] [-o out] <wordlist>` — read words (lower-cased, trimmed),
  size the filter, insert all, save. Prints `m`, `k`, target and estimated FP
  rate, and filter size in KB.
- `check -f <filter> [words...]` — load the filter; for each word (CLI args, or
  stdin if none) print "probably present" or "MISSPELLED". Exits `1` if any word
  was flagged — so it is scriptable (e.g. fail a commit hook on unknown words).

| Exit code | Meaning |
|---|---|
| 0 | success; for `check`, all words probably present |
| 1 | domain signal: corrupt filter, or ≥1 word flagged absent |
| 2 | usage / I/O error: bad args, file not found, empty dictionary |

---

## 🧪 Testing Strategy

Run everything with `go test ./...`. The suite is layered to match the
architecture and to nail the two properties that *define* a Bloom filter.

**Bit-set unit tests (`bitset_test.go`)**
- Set/Test across byte boundaries (bits 7, 8) and the final bit.
- `Count` ignores duplicate sets (set bit 3 thrice → count 1).
- Index wrap-around (`Set(13)` on a 10-bit set hits bit 3).
- Byte sizing: 10 bits → 2 bytes, 16 bits → exactly 2 bytes (no off-by-one).

**Filter tests (`bloom_test.go`)** — the important ones:
- **No false negatives** *(the core guarantee)*: insert 5,000 words, assert
  **every single one** reports present. Zero tolerance — one miss is a bug.
- **Measured false-positive rate**: insert 10,000 words, probe 20,000 *absent*
  words, assert the observed FP rate is near the configured `p = 0.01`. This
  proves the `m`/`k` math is right, not just the plumbing.
- **Optimal-params spot check**: `n = 1,000,000`, `p = 0.01` → `m ≈ 9.6 M`,
  `k = 7`.
- **Edge cases**: single-word filter (the word is present, an unrelated word is
  not); empty/invalid inputs rejected (`n = 0`, `p ≤ 0`, `p ≥ 1`); degenerate
  params clamped so `m, k ≥ 1`.

**Codec tests (`codec_test.go`)**
- **Round-trip**: build → `Save` → `Load`; restored filter has identical `m`,
  `k`, byte-for-byte identical bits, and still finds every inserted word.
- Corruption handling: bad magic, truncated payload, and empty input all return
  `ErrBadFormat`.
- Single-word round-trip (smallest filter still serializes).

**End-to-end verification** (run manually before shipping):

```bash
go vet ./...
go test ./...
go build -o bloom .
./bloom build -p 0.01 -o testdata/words.bf testdata/words.txt
./bloom check -f testdata/words.bf apple receive xyzzyqwert
#   apple        probably present
#   receive      probably present
#   xyzzyqwert   MISSPELLED (definitely not in dictionary)   → exit 1
```

A known word reports present; gibberish reports absent. ✅

---

## 💡 Key Takeaways

- **A Bloom filter trades certainty for space.** It answers set membership with a
  bit array + `k` hashes, never storing the keys — orders of magnitude smaller
  than a hash set.
- **The guarantee is one-sided.** No false negatives ever; false positives at a
  tunable rate `p`. "Definitely not present" is the answer you can *trust*, and
  that's exactly what flags a misspelling.
- **The math is two formulas.** `m = -n·ln p / (ln 2)²` bits and
  `k = (m/n)·ln 2` hashes. The optimal `k` keeps the array half full — the point
  of minimum error. Smaller `p` costs more bits, linearly in the exponent
  (~9.6 bits/item at 1%, +4.8 bits/item per extra 10× of accuracy).
- **Double hashing (Kirsch–Mitzenmacher)** synthesises `k` hashes from `2`
  (`g_i = h1 + i·h2`) with no quality loss — a clean, cheap implementation trick.
- **Self-describing file formats** (magic + version + params + length) make
  serialized data robust and forward-compatible. Store the parameters the reader
  needs; without `m` and `k`, the saved bits are meaningless.
- **This is a production pattern, not a toy.** Cheap no-false-negative filter in
  front of an expensive source of truth — databases, CDNs, and browsers all do
  this.

**Trade-offs we chose deliberately:**

| Decision | We chose | Alternative | Why |
|---|---|---|---|
| Hash functions | Double hashing from one FNV-1a | `k` distinct real hashes | Faster, simpler, no measurable accuracy loss |
| Hash family | FNV-1a (non-crypto) | SHA-256 / MurmurHash | We need distribution, not adversary-resistance; FNV is tiny & fast |
| Delete support | None | Counting Bloom filter | Plain filter can't delete; counting variant costs ~4× memory |
| `p` default | 1% | smaller | 1% is the classic accuracy/size sweet spot for spell checking |

---

## 📖 Further Reading

- **The challenge:** [codingchallenges.fyi — Build Your Own Spell Checker](https://codingchallenges.fyi/challenges/challenge-bloom/)
- **Original paper:** Burton H. Bloom, *"Space/Time Trade-offs in Hash Coding
  with Allowable Errors"* (CACM, 1970).
- **Double hashing:** Kirsch & Mitzenmacher, *"Less Hashing, Same Performance:
  Building a Better Bloom Filter"* (2006).
- **Interactive intuition:** [Bloom Filters by Example](https://llimllib.github.io/bloomfilter-tutorial/)
- **In the wild:** Cassandra, RocksDB/LevelDB SSTable Bloom filters; Google Safe
  Browsing; Bitcoin SPV (BIP 37).
- **FNV hash:** [Fowler–Noll–Vo hash function](https://en.wikipedia.org/wiki/Fowler%E2%80%93Noll%E2%80%93Vo_hash_function)
- **Variants to explore next:** Counting Bloom filters (support deletion),
  Cuckoo filters (deletion + better locality), Scalable Bloom filters (grow with
  the data).

---

### Run it yourself

```bash
cd phase-01-foundations/bloom-filter-spell-checker

# Test & vet
go test ./...
go vet ./...

# Build the binary
go build -o bloom .

# Build a filter from a dictionary (try the system one!)
./bloom build -p 0.01 -o words.bf /usr/share/dict/words

# Check some words
./bloom check -f words.bf recieve receive definately definitely

# Or pipe text through it, one word per line
echo "teh quick brown fox" | tr ' ' '\n' | ./bloom check -f words.bf
```
