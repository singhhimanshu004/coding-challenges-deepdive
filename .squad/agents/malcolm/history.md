# Malcolm — History

## Project Context
- **Project:** coding-challenges-deepdive
- **Owner:** Himanshu Singh
- **Source:** https://codingchallenges.fyi/challenges/intro
- **Stack:** Multi-language (Go, Python, Java, TypeScript)
- **Scope:** 65+ coding challenges — building real-world tools from scratch

## Learnings

### 2026-06-08 — Repository scaffolding complete (stubs only)
Seeded the full directory structure for all 8 phases and all 64 challenges from CURRICULUM.md. No solutions implemented — placeholders only.

**Directory naming convention**
- Phase dirs at repo root: `phase-NN-short-slug/` (zero-padded number).
- Challenge dirs inside each phase: kebab-case slug (e.g. `json-parser/`, `huffman-compression/`).

**Phase → slug mapping**
- phase-01-foundations — Foundations: Parsing, Encoding & Data Structures (4)
- phase-02-core-unix — Core Unix: Text Processing (11)
- phase-03-advanced-cli — Advanced CLI & Orchestration (7)
- phase-04-networking — Networking Fundamentals (7)
- phase-05-servers-infrastructure — Servers & Infrastructure (7)
- phase-06-applications-fullstack — Applications & Full-Stack (9)
- phase-07-developer-tools — Developer Tools & Internals (6)
- phase-08-games-interpreters — Games, Interpreters & Creative Projects (13)

**Stub README skeleton (each challenge)**
- H1 = challenge title; metadata block (phase, difficulty, recommended language, effort estimate).
- `**Status:** 🔲 Not started` line.
- 7 required section headings (🎯 What We're Building → 📚 Core Concepts → 🏗️ Architecture & Design → 🔨 Step-by-Step Implementation → 🧪 Testing Strategy → 💡 Key Takeaways → 📖 Further Reading), each with a `> _TODO: …_` placeholder.
- Each phase dir also has a `README.md` table linking its challenges with difficulty + language, mirroring CURRICULUM.md.

**Verification:** 8 phase dirs, 64 challenge folders, 64 stub READMEs — matches the curriculum's "64 challenges / 8 phases".

### 2026-06-08 — Phase 1, Challenge 1: JSON Parser (Python) — COMPLETE ✅
Implemented the JSON Parser end-to-end in `phase-01-foundations/json-parser/`. This is the curriculum capstone for the parsing mindset; the same lex→parse skeleton is reused later by jq/yq, the Calculator, and the Lisp interpreter.

**Module layout (reusable for code-heavy challenges)**
- Source lives in a package folder named after the tool: `jsonparser/`.
  - `tokens.py` — shared vocabulary (`TokenType` enum + `Token` dataclass with line/column).
  - `lexer.py` — stage 1: text → tokens (local validation: number shapes, string escapes, unicode/surrogate pairs).
  - `parser.py` — stage 2: tokens → Python value via recursive descent (one method per grammar rule); also exposes a top-level `parse(text)` front door mirroring `json.loads`.
  - `errors.py` — single `JSONParseError(message, line, column)` exception type.
  - `cli.py` + `__main__.py` — `python -m jsonparser [FILE]`, stdin fallback, exit codes 0 valid / 1 invalid / 2 usage.
- `tests/` package, `pytest.ini` with `testpaths = tests`.
- A `.venv/` (gitignored) holds pytest since the system Python is PEP-668 externally-managed.

**The lex→parse pattern (the transferable skill)**
- Two stages, strictly separated: the lexer never thinks about grammar; the parser never thinks about characters.
- Recursive descent = one function per grammar rule, call stack tracks nesting. JSON is LL(1) → single-token lookahead, no backtracking.
- Tokens carry source positions so every error pinpoints line/column.
- Documented trade-off: recursive descent is bounded by host-language stack depth (CPython ~1000 frames) — chosen for clarity over an explicit-stack rewrite.

**Test approach**
- Layered: unit-test lexer, unit-test parser, step fixtures (codingchallenges.fyi step1–4 spirit), CLI tests.
- Key technique: for every sample, assert our accept/reject verdict **agrees with stdlib `json`** (the ground truth). Parametrized valid + invalid corpora.
- Edge cases: empty input, trailing commas (reject), unterminated strings, deep nesting, duplicate keys (last-wins default + opt-in strict reject), leading zeros, huge ints, trailing junk, line/column accuracy.
- Result: **110 tests pass**.

**README**: full README-first teaching doc written (7 required sections), status set to ✅ Done, with ASCII pipeline/grammar diagrams, recursive-descent vs table-driven trade-off table, and run/test instructions. CURRICULUM.md checkbox for Challenge 1 ticked.

### 2026-06-08 — Phase 1, Challenge 2: Huffman Compression (Go) — COMPLETE ✅
Implemented a lossless Huffman file compressor in `phase-01-foundations/huffman-compression/`. This is the curriculum's first **Go** code-bearing challenge and the source of the bit-I/O primitive reused later in `tar` (19), `xxd` (15), and the binary DNS/NTP protocols (23–25).

**Go module + layout (reusable template for Go challenges)**
- `go.mod` with a short module path = tool slug (`module huffman`, `go 1.22`).
- CLI entry `main.go` at the challenge root (package `main`); thin — parses subcommands and delegates.
- Algorithm in **internal packages** (teaching-sized, one responsibility each):
  - `internal/bitio/` — general-purpose **BitWriter/BitReader** (MSB-first), zero Huffman knowledge → reusable for tar/xxd/DNS. `bitio.go` + `bitio_test.go`.
  - `internal/huffman/` — `tree.go` (node, `CountFrequencies`, greedy build, code table), `heap.go` (min-heap via `container/heap`), `codec.go` (`Compress`/`Decompress`/header/`Ratio`), `codec_test.go`.
- Tests live **beside** the code as `*_test.go` (Go idiom) — NOT a separate `tests/` folder like the Python JSON parser. This is the key Go-vs-Python divergence.
- `.gitignore` covers the compiled binary (`/huffman`), `*.test`, `*.out`, and `*.huf` artifacts. Do NOT commit build artifacts.

**Header-format decision (store frequency table)**
- File format `HUF1`: magic(4) + totalSymbols(uint64 BE) + numDistinct(uint16 BE) + table[symbol(1B)+freq(uvarint)] + bit-packed body. All ints big-endian; freqs use `binary.PutUvarint` so small counts cost 1 byte.
- Chose **storing the (symbol,frequency) table** over canonical code-lengths or serialized tree: decoder reruns the identical build → simplest to teach/verify; overhead negligible. Canonical Huffman (store code lengths, the DEFLATE choice) is the production alternative — documented as the trade-off.
- `totalSymbols` in the header is what makes the **final partial byte safe**: decoder stops after exactly N symbols, never reads zero-padding.

**Bit I/O approach**
- Writer accumulates bits MSB-first in a one-byte buffer, emits on 8; `Flush` left-justifies + zero-pads the last byte. Reader mirrors it. Built on `bufio`.

**THE bug worth remembering (determinism contract):** storing freqs only round-trips if both sides build the *identical* tree. Go **randomizes map iteration order**, so the heap tie-break (equal frequencies) must NOT depend on it. Fix: key each leaf's tie-break `order` on its byte value (0–255), internal nodes get ids from 256 up in creation order. Caught by the seeded fuzz round-trip test (sizes matched, bytes didn't).

**Edge cases handled:** empty file (no tree), single distinct symbol (depth-0 tree → assign 1-bit code `"0"`, decode replays symbol N times), all-256-byte alphabet, incompressible binary.

**Testing:** round-trip property `Decompress(Compress(x))==x` over empty/single/repeated/text/binary/all-256/newline inputs + seeded 50-iteration fuzzer; bitio unit tests (13-bit non-aligned pattern, `0xB2` MSB ordering, EOF, empty flush); prefix-free assertion; corrupt-magic → `ErrBadFormat`. `go test ./...` + `go vet ./...` clean; verified a real 34 KB file round-trips identically (42.8% saved). CURRICULUM.md Challenge 2 checkbox ticked.

### 2026-06-09 — Phase 1, Challenge 3: Bloom Filter Spell Checker (Go) — COMPLETE ✅
Implemented a Bloom-filter-backed spell checker in `phase-01-foundations/bloom-filter-spell-checker/`. Reused the Go template from Huffman (Challenge 2): `module bloom`, algorithm in `internal/`, tests beside the code, `.gitignore` for the binary + `*.bf` artifacts.

**Layout**
- CLI `main.go` (package main) at challenge root — thin: `build` / `check` subcommands, flag parse, file/stdin I/O, exit codes 0/1/2.
- `internal/bloom/` — `bitset.go` (packed bit array: bit i in byte i/8 at offset i%8; Set/Test mod n; Count via Kernighan popcount), `hash.go` (FNV-1a → double hashing), `bloom.go` (Filter: optimal m/k, Add, Contains, FromParts, EstimatedFalsePositiveRate).
- `internal/codec/` — `codec.go` (Save/Load the `BLM1` format).
- `testdata/words.txt` — 40-word sample dictionary for tests.

**Hashing approach (the transferable trick)**
- Kirsch–Mitzenmacher double hashing: `g_i(x) = h1 + i*h2 mod m` synthesises k hashes from ONE 64-bit FNV-1a digest split into high/low 32-bit halves. No measurable FP-rate penalty vs k real hashes; far cheaper. FNV-1a chosen for being tiny/fast on short strings; non-crypto is fine (need distribution, not adversary-resistance).
- **Bug guarded against:** if h2==0 every derived hash collapses to h1 (i*0==0) → filter uses a single bit. Force h2 non-zero.

**The Bloom math (memorise)**
- `m = -(n·ln p)/(ln 2)²` bits; `k = (m/n)·ln 2` hashes. Optimal k keeps the array ~half full (min error). ~9.6 bits/item at p=0.01; +4.8 bits/item per 10× smaller p. Spot check: n=1e6, p=0.01 → m≈9.6M, k=7.
- One-sided guarantee: NO false negatives ever (Add only sets bits, no delete); false positives at tunable rate p. "Definitely not present" is the trustworthy answer that flags a misspelling.

**Serialization header decision (`BLM1`)**
- magic(4 "BLM1") + version(1) + m(uint64 BE) + k(uint64 BE) + nbytes(uint64 BE) + packed bits. All ints big-endian. Store m AND k because bit positions depend on them — a reader guessing different values computes wrong indices. nbytes must equal ceil(m/8) → detects truncation/corruption. Load validates magic/version/length → `ErrBadFormat`.

**Testing**
- Core guarantee test: insert 5000 words, assert EVERY one reports present (zero tolerance for false negatives).
- Measured FP-rate test: insert 10k, probe 20k absent words, assert observed FP ≤ 3×target (statistical slack). Proves the m/k math, not just plumbing.
- bitset units (byte-boundary bits 7/8, dup Count, mod wrap, byte sizing); optimal-param spot check; edge cases (single word, empty/invalid n & p rejected, params clamped to ≥1); codec round-trip (identical m/k + byte-identical bits + all words present) + bad-magic/truncated/empty → ErrBadFormat.
- `go test ./...` + `go vet ./...` clean. Real round-trip verified: known words (apple, receive) → present, gibberish (xyzzyqwert) → MISSPELLED, exit 1. CURRICULUM.md Challenge 3 checkbox ticked.

### 2026-06-09 — Phase 1, Challenge 4: QR Code Generator (Python) — COMPLETE ✅
Implemented a from-scratch QR encoder in `phase-01-foundations/qr-code-generator/` (no encoder library — Pillow used for pixel output only). Versions 1–10, EC levels L/M/Q/H, byte + numeric + alphanumeric modes, smallest-fitting-version auto-selection.

**The pipeline (the transferable skill: layered encoding with an error-correcting code)**
- text → mode analysis → smallest fitting version → bit packing (4-bit mode indicator + char-count + payload + terminator + 0xEC/0x11 padding) → split into blocks → Reed–Solomon ECC per block → interleave codewords → matrix layout → masking → BCH format/version info → render.
- Module split mirrors the JSON-parser convention: package named after the tool (`qrgen/`), one file per stage: `galois.py`, `reedsolomon.py`, `tables.py`, `encode.py`, `matrix.py`, `mask.py`, `generator.py`, `render.py`, `cli.py`, `__main__.py`. `tests/` package + `pytest.ini` (`testpaths = tests`). `.venv/` gitignored; PNGs gitignored.

**GF(256) + Reed–Solomon (the math worth remembering)**
- GF(256): addition/subtraction = XOR; multiplication = carry-less poly-multiply mod the primitive polynomial 0x11D. Precompute EXP/LOG (antilog/log) tables (doubled EXP to 512 so EXP[i+j] never overflows) → a·b = EXP[LOG[a]+LOG[b]] (slide-rule trick). div/inverse via subtracting exponents mod 255.
- RS EC codewords = remainder of M(x)·x^n ÷ generator g(x), where g(x)=∏(x−2^i). Implemented as an in-place shift-register long division, O(data×ecc). Generator built by folding (x−2^i) factors.
- Block interleaving spreads burst damage across blocks (column-major data interleave, then EC interleave).

**Masking + format info**
- 8 mask predicates XOR'd over data-only modules (function patterns reserved & skipped). 4 penalty rules (runs of 5+, 2×2 blocks, 1:1:3:1:1 finder-lookalikes ×40, dark-ratio deviation); pick lowest score.
- Format info = BCH(15,5), data = (EC 2-bit field <<3 | mask), remainder via XOR-shift, final XOR 0x5412. NOTE the EC-level field order is non-obvious: L=01, M=00, Q=11, H=10 (NOT L<M<Q<H). Version info (v≥7) = BCH(18,6).

**Validation method (decoder-independent + decoder round-trip)**
- Reference vectors are the decisive proof: RS matches the Wikiversity "Reed–Solomon codes for coders" canonical QR vector byte-for-byte; encode matches the Thonky "HELLO WORLD" V1-Q data codewords exactly; format-info bit strings match the published table.
- End-to-end round-trip: render PNG → decode with a 3rd-party decoder → assert original text. Decoder preference: **zbar (pyzbar) is reliable; OpenCV's detector is flaky on tiny v1 symbols** (failed to *locate* valid v1 codes that zbar and phones read fine — a detector limitation, not an encoder bug). On macOS pyzbar needs `brew install zbar`; a `tests/conftest.py` adds /opt/homebrew/lib to the loader path before import; tests skip cleanly if no decoder.
- 34 pytest tests pass. Verified real PNGs (incl. Unicode payloads) decode back to the original.

**Reuse note:** the `BitBuffer` (MSB-first bit packing) is the sibling of the Huffman bit-writer from Challenge 2 — same bit-packing skill. Phase 1 Challenge 4 is complete; CURRICULUM.md checkbox ticked, README status ✅ Done.

### 2026-06-09 — Go Quickstart guide for Python devs — COMPLETE ✅
Wrote `docs/go-quickstart.md` (new `docs/` dir): a project-specific Go primer that teaches Go by mapping it to Python, using THIS repo's real code (Huffman + Bloom filter) as running examples — not toy snippets. Purpose: meet Himanshu (Python/Java strong, Go-building) where he is.

**Contents (skimmable, tables + short code blocks):**
- "Why Go for some challenges" framing tied to the best-fit policy (bit/byte work, perf, bufio).
- Side-by-side Python↔Go cheat-sheet (vars/zero values, funcs/multi-return, `if err != nil` vs try/except, slices vs lists, maps vs dict incl. comma-ok + the map-determinism bug, structs/methods/interfaces, defer, packages/exported-caps/go.mod, goroutines/channels preview), each pointing to real file:line in our code.
- Line-by-region walkthrough of `internal/bitio/bitio.go`.
- "Things that surprise a Pythonista" gotchas (no truthiness, explicit conversions, nil≠None, value vs pointer receivers, caps=visibility, no comprehensions, `:=` vs `var`).
- Fast-track plan: Tour of Go → re-read our 2 Go programs → 3 exercises against existing code (verbose flag on Bloom CLI, print Huffman code table w/ sorted keys, Bloom `count` subcommand).
- Run/build/test section (`go run`/`build`/`test ./...`/`vet`/`fmt`) with concrete Huffman + Bloom sessions.
- Cross-linked from README.md (one-line note under Tech Stack).

**Reuse note:** Future Go challenges should link learners to `docs/go-quickstart.md` in their READMEs instead of re-explaining Go-vs-Python basics. The map-iteration-order determinism lesson is captured there for reuse.

### 2026-06-09 — Phase 2, Wave 1: Six Core Unix Tools (Go) — ✅ APPROVED (Ellie review)
Built in parallel (5 tools + Go quickstart): **wc** (Challenge 5), **cat** (6), **head** (7), **cut** (8), **uniq** (9), **tr** (10). All in `phase-02-core-unix/{tool}/`. Every tool:
- `go test ./...` and `go vet ./...` passing
- Differential spot-checks vs system tools matched byte-for-byte
- README-first teaching (7 sections, Python analogies, linked to docs/go-quickstart.md)
- Follows established patterns: `main` → `run()` (injected streams), hand-rolled flags, exit codes 0/1/2
- All approved by Ellie 2026-06-09; cleared to proceed with Phase 2 Wave 2

**Per-tool status:**
- **wc**: Streaming byte/line/word/rune counter; proved pure+injectable pattern; `bufio.ReadRune` for UTF-8; exit codes match real `wc` (1 for per-file errors).
- **cat**: Binary-safe `io.Copy` fast path + line-by-line flag mode; GNU-style continuous numbering across files (intentional BSD divergence noted); `ReadBytes` EOF gotcha documented.
- **head**: Early termination is the story — instant on huge files; hand-rolled flags beat stdlib; `defer` per-file close (not in loop).
- **cut**: LIST parser as reusable `Selector` type; membership test (not expansion) gives input order + dedup for free; bytes-vs-runes gotcha for `-c`.
- **uniq**: Adjacent-only is the headline; "carry one line of state" model; BSD 4-wide count format matched (GNU uses 7-wide).
- **tr**: Pure filter (no file args) — cleanest pipe-and-filter demo in the phase; rune-based (Unicode first-class); SET expansion (ranges, POSIX classes, escapes); state needed per mode (translate/delete stateless, squeeze carries one rune).

**Verified:** All 6 tools + Go quickstart staged + committed clean (no binaries). CURRICULUM.md checkboxes for challenges 5–10 all ticked.

### 2026-06-09 — Phase 2, Wave 2: Five Core Unix Tools (Go) — ✅ APPROVED (Ellie review)
Built in parallel (5 tools): **sort** (Challenge 11), **grep** (12), **sed** (13), **diff** (14), **xxd** (15). All in `phase-02-core-unix/{tool}/`. Every tool:
- `go test ./...` and `go vet ./...` passing
- Differential spot-checks vs system tools matched byte-for-byte
- README-first teaching (7 sections, Python analogies, linked to docs/go-quickstart.md)
- Follows established patterns from Wave 1

**Per-tool status:**
- **sort**: External merge sort is real — split → sort runs on disk → k-way merge via `container/heap`. Peak memory O(#runs). `-n/-r/-u/-f` and key fields verified. **One comparator, both paths** — strongest correctness guarantee. Stability engineered into k-way merge via run-index tie-break. Exit codes 0/1/2. Module name gotcha: named `ccsort` (not `sort`, which collides with stdlib import).
- **grep**: RE2 pattern matching with clean architecture (matcher / walker / output / main). Flags: `-i/-v/-n/-c/-w/-r/-l` plus context `-A/-B/-C`. Build flags into the pattern (e.g., `-i` → prepend `(?i)`, `-w` → wrap in word-boundary assertions). Context needs random access — read sources fully into `[]string`, merge `[i-B, i+A]` spans. Exit codes 0/1/2 (2 for bad pattern or missing dir without -r).
- **sed**: First "tool = language" challenge — parse-once → execute-per-line interpreter. Parser/executor split (internal/sed package). Supports `s/re/repl/[g][i][p]` with `\1` backrefs + `&`; `p`; `d`; addresses (line N, `$`, `/regex/`) and ranges; `-n` suppress; `-i` in-place. Critical learning: sed + Go regex dialects collide; translate at the boundary (`convertReplacement` maps sed `\1`/`&` to Go `${1}`/`$0`). Ranges are per-command state machine with an `active` bool carried *between lines*. Addresses match the *pattern space*, not the original line.
- **diff**: Phase capstone — LCS dynamic programming from scratch. No diff library used. Longest Common Subsequence computed with DP table, backtracked into edit script. Supports normal/unified/context formats, stdin via `-`. Three output formats share a single engine via the edit-script intermediate. Tie-break "up before left" in backtracking reproduces GNU's delete-before-insert bias. Hunk merging: cluster changes, pad with context, merge on overlap/touch.
- **xxd**: Hex dumper + reverse parser. Binary-safe I/O (`io.ReadFull`, `io.CopyN` for `-s`). Forward/reverse round-trips; supports `-l`, `-c`, `-s`, `-g`, `-r`. Key learning: xxd line layout has a one-space inter-group gap vs two-space pre-gutter separator — critical for robust reverse parsing. `-g 0` means one group of `cols` bytes. Verified: output == system xxd, `-r` round-trips all 256 byte values.

**Phase 2 complete:** 11/11 challenges done (wc, cat, head, cut, uniq, tr, sort, grep, sed, diff, xxd). All approved by Ellie 2026-06-09. CURRICULUM.md checkboxes for challenges 11–15 all ticked.
