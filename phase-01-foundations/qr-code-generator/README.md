# QR Code Generator

> **Phase:** 1 — Foundations: Parsing, Encoding & Data Structures
> **Difficulty:** 🔵
> **Recommended Language:** 🟨 Python
> **Effort Estimate:** L

**Status:** ✅ Done

---

## 🎯 What We're Building

A **QR code generator built entirely from scratch** — no `qrcode` library doing
the encoding for us. You give it text; it returns the black-and-white module
grid of a real, scannable QR symbol and renders it to a PNG and/or the terminal.

```
$ python -m qrgen "https://codingchallenges.fyi" -o out.png
█▀▀▀▀▀█ ▀▄▀ █▀▀▀▀▀█
█ ███ █ ▀█▀ █ ███ █     version 2 · EC M · mask 5 · 25×25 modules
█ ▀▀▀ █ █▀█ █ ▀▀▀ █     wrote out.png
▀▀▀▀▀▀▀ ▀ ▀ ▀▀▀▀▀▀▀
... (scan it with your phone — it really works)
```

**Why this challenge matters.** A QR code looks like random noise, but it is one
of the most elegant pieces of consumer-facing engineering ever standardised. To
build one you have to touch three deep ideas at once:

- **Bit packing** — squeezing characters into a tight bitstream with
  variable-width fields, exactly like the Huffman coder from Challenge 2.
- **Reed–Solomon error correction** — the same finite-field math that protects
  CDs, DVDs, DSL, deep-space probes, and RAID-6 arrays. A QR code can lose up to
  **30%** of its area and still decode. That magic is Reed–Solomon.
- **2-D layout & masking** — turning a 1-D codeword stream into a 2-D image that
  a cheap camera can find and read from any angle, under bad lighting.

The "encoder is correct" bar here is unusually crisp: **a real phone/decoder
either reads back your exact text, or it doesn't.** No partial credit.

What we support: **byte mode** (UTF-8/Latin-1), plus **numeric** and
**alphanumeric** modes as a bonus; EC levels **L / M / Q / H**; versions
**1–10** (21×21 up to 57×57 modules), auto-selecting the smallest that fits.

---

## 📚 Core Concepts

### 1. The anatomy of a QR symbol

A QR code is a square grid of **modules** (the little squares). Some modules are
fixed **function patterns** that orient the scanner; the rest carry data.

```
  ┌─────────────────────────────────────────┐
  │ ███████   ·· timing ··   ███████         │   ← finder patterns (3 corners)
  │ █     █                  █     █         │      let the scanner lock on and
  │ █ ███ █                  █ ███ █         │      compute orientation/scale
  │ █ ███ █                  █ ███ █         │
  │ █ ███ █                  █ ███ █         │   timing patterns = dotted lines
  │ █     █                  █     █         │      that let it count modules
  │ ███████                  ███████         │
  │       ·                                  │   format info wraps the top-left
  │ t                        ·   ┌───┐       │      finder (EC level + mask)
  │ i        DATA REGION         │ █ │ ← alignment pattern
  │ m        (data + ECC         └───┘       │      keeps big symbols from
  │ i         codewords,                     │      skewing (v2+ only)
  │ n         zig-zag placed)                │
  │ g                                        │   dark module = one always-on
  │ ███████        ·                         │      module near bottom-left
  │ █     █        ·                         │
  │ █ ███ █        ·                         │
  │ █ ███ █   ·· format info ··              │
  │ █ ███ █                                  │
  │ █     █  ■ ← dark module                 │
  │ ███████                                  │
  └─────────────────────────────────────────┘
```

| Pattern | Purpose |
|---|---|
| **Finder patterns** (7×7, three corners) | The unmistakable "eyes." Their 1:1:3:1:1 dark/light ratio is detectable from any rotation, so the scanner finds the symbol and its orientation. |
| **Separators** | 1-module light border around each finder so it stands out. |
| **Timing patterns** | Alternating dark/light row & column (at index 6) — a ruler the decoder uses to count module coordinates. |
| **Alignment patterns** (5×5) | Appear from version 2 up; let the decoder correct perspective distortion on larger symbols. |
| **Dark module** | A single always-dark module at `(4·V+9, 8)`. A quirk of the spec. |
| **Format information** | 15 bits (BCH-protected) storing the **EC level** and **mask pattern** — the decoder reads this *first*. |
| **Version information** | 18 bits (BCH-protected), only for versions ≥ 7, naming the version explicitly. |
| **Data region** | Everything left over: the interleaved data + error-correction codewords. |

### 2. Versions and capacity

A **version** is just a size: side = `17 + 4·V` modules. V1 = 21×21, V10 = 57×57.
Each (version, EC level) pair has a fixed budget of **data codewords** (bytes).
We pick the smallest version whose budget holds the input.

### 3. Encoding modes — packing characters into bits

QR can encode the same text more or less compactly depending on its alphabet:

| Mode | Alphabet | Bits per char |
|---|---|---|
| **Numeric** | `0-9` | ~3.33 (10 bits / 3 digits) |
| **Alphanumeric** | `0-9 A-Z space $ % * + - . / :` | 5.5 (11 bits / 2 chars) |
| **Byte** | any byte (UTF-8) | 8 |

Our analyzer picks the tightest mode the input qualifies for. The encoded
bitstream is: **`[4-bit mode] [char-count] [payload] [terminator] [padding]`**.

### 4. Reed–Solomon error correction over GF(256)

This is the heart of the challenge. Reed–Solomon adds redundant **EC codewords**
so a damaged symbol can still be recovered.

**Finite field GF(256).** All RS arithmetic happens over the 256 byte values,
which form a *finite field*: a number system where +, −, ×, ÷ all stay within
`{0..255}` and every non-zero value has an inverse.

- **Addition = subtraction = XOR.** No carries. `3 + 1 = 2`.
- **Multiplication** is carry-less polynomial multiply, reduced modulo the
  *primitive polynomial* `x⁸+x⁴+x³+x²+1` (`0x11D`).
- Every non-zero element is a power of the generator `g = 2`, so we precompute
  **log/antilog tables** and turn multiplication into addition of exponents — a
  slide rule for bytes:  `a·b = EXP[LOG[a] + LOG[b]]`.

**The encoding itself.** Treat the data codewords as a polynomial `M(x)`. The EC
codewords are the *remainder* of `M(x)·xⁿ` divided by a **generator polynomial**

```
g(x) = (x − 2⁰)(x − 2¹)(x − 2²) … (x − 2ⁿ⁻¹)
```

Because the transmitted codeword is a multiple of `g(x)`, it evaluates to zero at
each of `g`'s roots. A decoder checks those evaluations ("syndromes"); any
non-zero result reveals *and locates* the errors. We implement the encoder side;
the division is a tidy O(data × ecc) shift-register loop.

```
  data codewords ─┐
                  ├─►  M(x)·xⁿ  mod  g(x)   ─►  n EC codewords
  generator g(x) ─┘        (long division in GF(256))
```

### 5. Block structuring & interleaving

Bigger symbols split data into **multiple blocks**, each with its own EC
codewords, then **interleave** them (`block0[0], block1[0], block0[1], …`). Why?
A coffee-cup ring damages a *contiguous* region; interleaving spreads that damage
thinly across every block, so each block only has to fix a few errors.

### 6. Data masking — making it scannable

Raw data can produce big blank areas or accidental finder-like patterns that
confuse scanners. So QR **XORs the data region** (never the function patterns)
with one of **8 mask patterns**, scores each result with **4 penalty rules**, and
keeps the lowest score:

1. **Rule 1** — runs of 5+ same-color modules in a line (`3 + run−5` points).
2. **Rule 2** — every solid 2×2 block (`3` points).
3. **Rule 3** — finder-lookalike `1:1:3:1:1` sequences (`40` points each).
4. **Rule 4** — deviation of the overall dark/light ratio from 50%.

The chosen mask index is recorded in the format information so the decoder can
XOR it back out.

---

## 🏗️ Architecture & Design

Clean module split — each file is one stage of the pipeline, easy to test in
isolation:

```
qrgen/
├── galois.py       GF(256) arithmetic: log/antilog tables, mul/div/inverse
├── reedsolomon.py  generator polynomial + EC codeword computation
├── tables.py       ISO-18004 spec data (capacities, ECC blocks, alignment)
├── encode.py       text → mode → version → bitstream → blocks → interleave
├── matrix.py       function patterns, zig-zag data placement, format/version info
├── mask.py         8 mask patterns + 4 penalty rules + best-mask selection
├── generator.py    orchestrates the whole pipeline → QRCode object
├── render.py       PNG (Pillow) + Unicode/ASCII terminal rendering
└── cli.py          `python -m qrgen` command-line front door
```

**The full pipeline:**

```
 text
  │  encode.py
  ▼
 analyze mode ─► choose smallest version ─► pack bits ─► terminator + pad
  │
  ▼  encode.py + reedsolomon.py + galois.py
 split into blocks ─► Reed–Solomon EC per block ─► interleave codewords
  │
  ▼  matrix.py
 draw finders/timing/alignment ─► zig-zag place data bits
  │
  ▼  mask.py
 try 8 masks ─► score with 4 penalty rules ─► pick best
  │
  ▼  matrix.py
 stamp BCH format info (EC + mask) [+ version info if v≥7]
  │
  ▼  render.py
 PNG  /  Unicode-block terminal art
```

**Design choices & trade-offs:**

- **Log/antilog tables over bit-by-bit multiply** — turns the inner RS loop into
  table lookups. Standard and fast; costs a one-time 512-entry table.
- **Store nothing we can recompute** — versions, capacities and ECC block layouts
  are static spec data, kept in one `tables.py` so the algorithms stay readable.
- **Pillow is rendering only.** It paints pixels from *our* grid; it never
  encodes. The challenge rule (no encoder libraries) is fully honored.
- **Versions 1–10** — covers numeric/alphanumeric/byte payloads of meaningful
  length (a v10-L symbol holds ~270 data bytes) while keeping the alignment-
  pattern and interleaving logic teachable. Extending to v40 is just more rows
  in `tables.py`.

---

## 🔨 Step-by-Step Implementation

The build was incremental, each stage verified before the next.

1. **GF(256) (`galois.py`).** Build the log/antilog tables by walking powers of
   2 with polynomial reduction. Verified `mul`, `div`, `inverse` obey field laws
   (`a · a⁻¹ = 1`, commutativity, associativity).

2. **Reed–Solomon (`reedsolomon.py`).** Build `generator_poly(n)` by folding
   `(x − 2ⁱ)` factors, then `encode()` via shift-register long division.
   Validated against the canonical *Wikiversity* RS vector (exact byte match).

3. **Spec tables (`tables.py`).** Transcribe ECC block structures, alignment
   positions, and capacity per (version, EC). Cross-checked total codewords per
   version against the standard.

4. **Data encoding (`encode.py`).** Mode analysis → version selection →
   `BitBuffer` packing (mode indicator, char count, payload) → terminator and
   `0xEC/0x11` padding → block split → RS per block → interleave. Validated
   against the *Thonky* "HELLO WORLD" V1-Q reference (exact data codewords).

5. **Matrix (`matrix.py`).** Place finders + separators, timing, alignment
   (skipping finder overlaps), the dark module; reserve format/version strips;
   walk the 2-wide zig-zag (skipping the timing column) to drop data bits.

6. **Masking (`mask.py`).** Implement the 8 mask predicates and 4 penalty rules;
   try all, score, pick the minimum.

7. **Format & version info (`matrix.py`).** BCH(15,5) format bits (EC level +
   mask, XOR `0x5412`) written in both redundant locations; BCH(18,6) version
   bits for v ≥ 7. Verified format strings against the published table.

8. **Render & CLI (`render.py`, `cli.py`).** Quiet-zone-padded PNG via Pillow and
   half-block Unicode terminal art; argparse CLI with stdin support.

---

## 🧪 Testing Strategy

Testing a QR encoder needs **external ground truth** at every layer, because a
symbol that looks plausible can still be subtly wrong.

- **GF(256) unit tests** — field axioms: tables are a complete permutation,
  `add` is XOR and self-inverse, `mul` commutes/associates, `a·a⁻¹ = 1` for all
  255 non-zero elements, division inverts multiplication, `÷0` raises.
- **Reed–Solomon reference vector** — match the **Wikiversity** canonical QR
  example byte-for-byte (10 EC codewords). The single most decisive RS test.
- **Encode reference vector** — match the **Thonky** "HELLO WORLD" V1-Q data
  codewords exactly; check version growth, EC-vs-version monotonicity, padding.
- **Matrix structural invariants** — all three finder patterns are pixel-exact,
  timing patterns alternate, the dark module is dark, module count = `17+4V`,
  every cell is binary, masks never touch function modules.
- **End-to-end round-trip decode** — the headline test: generate a PNG, decode
  it with a **third-party decoder**, assert it returns the original text. Covers
  byte/alphanumeric/numeric across versions 1–4 and EC levels L/M/Q/H, including
  Unicode (`café ☕`). If the decoder can read it, every stage is correct.
- **Edge cases** — empty string, max-capacity payloads, oversized input (raises).

**A note on decoders.** The round-trip tests prefer **zbar** (`pyzbar`), which
reads small/dense symbols reliably, and fall back to **OpenCV** (`opencv-python`,
zero-config `pip install`). During development OpenCV's detector occasionally
failed to *locate* tiny version-1 symbols that zbar (and real phones) read fine —
a detector limitation, not an encoder bug, which is exactly why the
reference-vector tests (RS + codewords + format bits) exist as decoder-
independent proof. If neither decoder is installed the round-trip tests **skip**
cleanly rather than fail.

**Result:** 34 tests pass; generated PNGs decode back to the original text.

---

## 🚀 Running It

```bash
cd phase-01-foundations/qr-code-generator
python3 -m venv .venv && source .venv/bin/activate
pip install -r requirements.txt

# Generate a PNG and print to the terminal:
python -m qrgen "https://codingchallenges.fyi" -o qr.png

# Choose an error-correction level and ASCII rendering:
python -m qrgen "HELLO WORLD" --ec Q --ascii

# Pipe from stdin:
echo "scan me" | python -m qrgen -o qr.png
```

**CLI flags:** `-o/--output` (PNG path), `--ec {L,M,Q,H}`, `--scale` (PNG pixels
per module), `--ascii`, `--quiet-terminal`, `--encoding`.

**Run the tests:**

```bash
pip install -r requirements.txt
python -m pytest -q
```

> **macOS + zbar:** `pyzbar` needs the native zbar library (`brew install zbar`).
> The test `conftest.py` automatically adds Homebrew's lib directory to the
> loader path. On Linux, `apt install libzbar0`. Without zbar, tests fall back to
> OpenCV or skip — they never hard-fail on a missing decoder.

---

## 💡 Key Takeaways

- **Finite fields are the workhorse of error correction.** GF(256) makes "bytes"
  into a number system where division always works — the precondition for
  Reed–Solomon, which guards CDs, QR codes, RAID-6, and spacecraft telemetry.
- **The log/antilog trick** turns field multiplication into integer addition —
  the same "do the hard operation once, then look it up" idea behind logarithm
  tables and many DSP kernels.
- **Redundancy + interleaving = physical robustness.** EC codewords plus block
  interleaving are *why* a smudged or partly-missing QR code still scans.
- **Masking is a tiny optimization problem.** Eight candidates, four heuristic
  penalties, pick the best — a clean example of search-by-scoring.
- **Correctness needs an oracle.** Reference vectors (Wikiversity RS, Thonky
  codewords) and a real decoder round-trip are what separate "looks like a QR
  code" from "is a QR code."
- **Reused skills:** bit-stream packing carries straight over from Huffman
  (Challenge 2); the BitBuffer here is its sibling.

---

## 📖 Further Reading

- **Coding Challenges — Build Your Own QR Code Generator:**
  https://codingchallenges.fyi/challenges/challenge-qrcode
- **ISO/IEC 18004** — the official QR Code specification.
- **Thonky QR Code Tutorial** — the clearest step-by-step reference (used for our
  encode reference vector): https://www.thonky.com/qr-code-tutorial/
- **Wikiversity — Reed–Solomon codes for coders** (our RS reference vector):
  https://en.wikiversity.org/wiki/Reed%E2%80%93Solomon_codes_for_coders
- **Reed–Solomon error correction** — https://en.wikipedia.org/wiki/Reed%E2%80%93Solomon_error_correction
- **Finite field (GF(256)) arithmetic** — https://en.wikipedia.org/wiki/Finite_field_arithmetic
