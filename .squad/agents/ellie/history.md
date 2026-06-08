# Ellie — History

## Project Context
- **Project:** coding-challenges-deepdive
- **Owner:** Himanshu Singh
- **Source:** https://codingchallenges.fyi/challenges/intro
- **Stack:** Multi-language (Go, Python, Java, TypeScript)
- **Scope:** 65+ coding challenges — review and quality assurance

## Learnings

### 2026-06-08 — Review: Phase 1 / Challenge 1 — JSON Parser (`phase-01-foundations/json-parser/`)

**Verdict: ✅ APPROVED.**

- Independently ran `./.venv/bin/python -m pytest -q` → **110 passed in 0.05s**.
- Manual CLI checks confirm the exit-code contract: valid→0, invalid (trailing
  comma / unterminated)→1 with precise `line, column` messages, missing file→2,
  `--quiet`→silent+0, `--no-duplicate-keys` rejects dups→1.
- README clears the README-first gate: all 7 mandated sections, real-world
  context, lex→parse + recursive-descent explanation, trade-off table, diagrams,
  run/test instructions. Reader genuinely learns.
- Code: clean two-stage lexer/parser split; rejects leading zeros, bare `-`,
  `1.`, `.5`, `1e`, trailing commas, trailing junk, unpaired surrogates, raw
  control chars; decodes `\uXXXX` + surrogate pairs; preserves int/float;
  single `JSONParseError` with position; opt-in strict duplicate-key mode.
- Tests: 4 layers, cross-checked against stdlib `json` for valid+invalid corpora;
  all required edge cases covered.
- **Non-blocking nice-to-have:** very deep input (~5000 levels) emits a raw
  `RecursionError` traceback on stderr (exit still 1). README documents the
  stack-depth trade-off, so not a blocker — but catching `RecursionError` for a
  clean "maximum nesting depth exceeded" message would be a nice polish.
- Verdict written to `.squad/decisions/inbox/ellie-json-parser-review.md`.

### 2026-06-08 — Review: Phase 1 / Challenge 2 — Huffman Compression (`phase-01-foundations/huffman-compression/`)

**Verdict: ✅ APPROVED.**

- Independently ran `go vet ./...` → clean, and `go test ./...` → both packages
  (`internal/bitio`, `internal/huffman`) pass.
- Real end-to-end CLI round-trips, all byte-identical:
  - README.md (18177 B → 11762 B, ratio 0.647 / saved 35.3%) — sha256 of input
    and decompressed output match exactly.
  - empty file (0 B → 14 B header → 0 B) IDENTICAL.
  - single-byte `"A"` (1 B → 17 B → 1 B) IDENTICAL; ratio honestly reports header
    overhead.
  - truncated `.huf` and bad-magic input both rejected.
- Exit-code contract verified: 0 success; 1 domain failure (bad magic, truncated
  body); 2 usage/IO (no args, unknown cmd, missing input, decompress without -o,
  -o without value). `--help` → 0.
- Code quality: idiomatic, clean separation — `bitio` (MSB-first BitWriter/
  BitReader) is Huffman-agnostic and independently testable; `huffman` splits
  heap/tree/codec cleanly. Comments explain the *why* (determinism contract,
  zero-padding safety, min-heap rationale). Standout: deterministic tie-break
  keyed on byte value (leaves 0–255, internal nodes from 256) so encode/decode
  rebuild the identical tree despite Go's randomized map iteration — the exact
  subtle bug the fuzz test guards against.
- Tests: meaningful round-trip corpus (empty, single-byte, single-symbol run,
  text, repeated, all-256-bytes, binary, newlines), prefix-free property, skewed
  shrink check, bad-magic rejection, seeded 50-iter fuzz, bit-I/O unit tests
  (13-bit non-aligned pattern, MSB-first 0xB2, EOF, empty flush).
- README clears the README-first gate decisively: all 7 sections, entropy +
  Shannon bound, prefix codes ⇄ trees, greedy optimality, header-format trade-off
  table (freq table vs canonical vs tree serialization), bit-I/O + padding safety,
  determinism subtlety, diagrams, run/test instructions. Reader genuinely learns.

**Non-blocking nice-to-haves:**
- `decompress` requires `-o` while `compress` defaults to `<input>.huf`; could
  default decompress output by stripping `.huf` for symmetry.
- Single-byte/tiny inputs print `saved -1600.0%`; honest but could be softened
  with a "(file too small to benefit)" note. README already explains header cost.
- Verdict written to `.squad/decisions/inbox/ellie-huffman-review.md`.

### 2026-06-09 — Review: Phase 1 / Challenge 3 — Bloom Filter Spell Checker (`phase-01-foundations/bloom-filter-spell-checker/`)

**Verdict: ✅ APPROVED.**

- Independently ran `go vet ./...` → clean (exit 0), and `go test ./...` → both
  packages (`internal/bloom`, `internal/codec`) pass.
- Verbose FP test: target p=0.0100, **observed FP rate=0.0089 (178/20000)** — the
  m/k math is right, not just the plumbing. No-false-negatives test inserts 5,000
  words and confirms every one reports present (zero tolerance).
- Real end-to-end run on system dictionary `/usr/share/dict/words` (235,976 words):
  `m = 2,261,844 bits (276.1 KB), k = 7, estimated FP 0.009736` — sane and matches
  README's illustrative numbers closely. `receive`/`definitely` → probably present;
  `recieve`/`definately` → MISSPELLED. Small testdata dict (40 words) also correct.
- Exit-code contract verified: 0 = all words present; 1 = ≥1 word flagged OR corrupt
  filter (bad magic → `ErrBadFormat`); 2 = usage/IO (no args → usage, missing file).
  Stdin path (`tr ' ' '\n' | check`) works.
- Code quality: clean bottom-up layering (BitSet → hashing → Filter → codec → CLI),
  `internal/bloom` has zero file/CLI knowledge. Standouts: packed bitset with
  Kernighan popcount; Kirsch–Mitzenmacher double hashing from one FNV-1a split into
  hi/lo 32 bits, with the **h2==0 guard** (else all derived hashes collapse to h1);
  self-describing `BLM1` header storing m/k so lookups are reproducible, with
  `nbytes == ceil(m/8)` truncation check. Comments explain the *why* throughout.
- Tests cover all four mandated properties: no-false-negatives proof, measured FP
  near p, Save→Load round-trip (byte-identical bits + same m/k), and edge cases
  (single word, n=0 / p≤0 / p≥1 rejected, clamped params, bad magic/truncated/empty).
- README clears the README-first gate decisively: all 7 sections, what a Bloom
  filter is, one-sided guarantee (no false negatives / possible false positives),
  full m/k derivation from n and p (incl. P(bit=0)≈e^(-kn/m) intuition), double
  hashing, production uses (Cassandra/RocksDB/Chrome/Bitcoin SPV), BLM1 serialization
  format, ASCII diagrams, run/test instructions. Reader genuinely learns.

**Non-blocking nice-to-haves:**
- README sample output (line 27–28: 235886 words, 276.0 KB) differs slightly from
  the current macOS dict (235976 words, 276.1 KB) — illustrative, not a defect.
- `check -f` could default the filter path (e.g. `<wordlist>.bf`) for symmetry with
  build's default output, but the explicit flag is fine.
- Tiny filters print `m = ... (0.0 KB)`; honest but could show bytes for sub-KB.
- Verdict written to `.squad/decisions/inbox/ellie-bloom-filter-review.md`.
