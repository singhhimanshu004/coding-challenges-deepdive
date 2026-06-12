# Malcolm — History

## Project Context
- **Project:** coding-challenges-deepdive
- **Owner:** Himanshu Singh
- **Source:** https://codingchallenges.fyi/challenges/intro
- **Stack:** Multi-language (Go, Python, Java, TypeScript)
- **Scope:** 65+ coding challenges — building real-world tools from scratch


## Summary by Phase

**Phase 1: Foundations (JSON Parser)** — Lex→parse pipeline, recursive descent, testable module layout.

**Phase 2: Core Unix (11 challenges)** — Text processing patterns: stream + regex (wc, uniq, sort, grep, sed), structured parsing (CSV, XML, YAML), archive handling (tar), and version comparison (cut/paste).

**Phase 3: Advanced CLI & Orchestration (7 challenges)** — Process orchestration (make, cron, git hooks), shell parsing & execution (recursive descent AST, fork/exec/pipe, builtin dispatch), pipe EOF hang (critical pattern: parent must close pipe copies after fork), working directory mutations.

**Phase 4: Networking (4/7 done)** — UDP fundamentals (DNS wire format, name-compression pointers, NTP epoch math), TCP (connect scan worker pool, netcat bidirectional relay), half-close patterns (interface checks not bools), deadline-driven termination for connectionless protocols, CGO_ENABLED=0 workaround for darwin/arm64.

Key reusable patterns:
1. Module split: lexer→parser→executor (or equivalent domain stages).
2. Testable via interface injection (io.Reader, io.Writer, error handlers).
3. Hand-crafted unit tests on byte literals (no network, file I/O, randomness).
4. UDP needs deadline; TCP needs half-close; processes need fd ownership.
5. Goroutines + channels for parallelism; WaitGroup + closer goroutine for clean shutdown.

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

### 2026-06-09 — Phase 3, Wave 1: Five Advanced CLI & Orchestration Tools (Go/Python) — ✅ APPROVED (Ellie review)

Built in parallel (5 tools): **jq** (Challenge 16, Go), **yq** (17, Python), **xargs** (18, Go), **tar** (19, Go), **crontab** (20, Go). All in `phase-03-advanced-cli/{tool}/`. Every challenge passed `go vet`/`go test -race` (Go) or pytest (Python), plus real behavioral spot-checks and differential/interop verification.

**Per-tool status:**
- **jq** (Go): From-scratch filter/expression language evaluator + tree-walking interpreter. Teaching headline: "two parsers, one skeleton" (JSON data + filter program both lex→parse→eval). Key insight: every filter is `input → []output` (a stream); pipe composes, comma concatenates, select produces 0 outputs, .[] produces many — all from ONE rule. Insertion-ordered Object (not map) preserves key order for byte-for-byte jq compatibility. Hand-rolled JSON parser learned key-order; documented trade-off vs stdlib. `go test ./... -race` 40+ cases pass. Differential check: output == system jq on `.name`, `.address.city`, `.[] | select(.age > 30)`, `map(.name)`, `keys`, compact/pretty, all identical.
- **yq** (Python): Jq-like query interpreter for YAML/JSON. Delegated YAML tokenisation to PyYAML (safe_load_all only — RCE-safe); query interpreter built from scratch (lex→parse→generate value streams). Same stream-based composition model as jq. 54 pytest cases all pass. Real queries on anchors/aliases, multi-document streams, JSON↔YAML conversion — all correct. Value-stream model reinforces Phase 1 lex→parse→eval throughline.
- **xargs** (Go): Process orchestration tool — batches stdin items into argv, spawns children with bounded `-P` parallelism. Key reusable patterns: (1) injectable `runner` function type (testability seam for process spawning), (2) buffered channel as counting semaphore + WaitGroup (canonical Go worker-pool), (3) deterministic parallelism test via arrival barrier (not sleeps), (4) os.Pipe for race-safe output buffering. Exit codes: 127 (not found) > 126 (not exec) > 123 (any child 1–125) > 0. Verified: `-P4` timing shows genuine 2 waves (4 concurrent tasks × 0.3s each ≈ 0.6s total, not 2.4s serial).
- **tar** (Go): POSIX USTAR archiver with -c/-t/-x modes. Hand-rolled 512-byte header format (no archive/tar stdlib). Teaching payload: binary formats as fixed-offset constants; USTAR numbers are octal ASCII; checksum counts spaces (0x20); two-zero-block terminator; splitPath for >100-byte names. Security highlight: safeJoin rejects absolute paths and .. traversal (pattern for any extract tool). Interop verified both ways: our archive → system tar reads/extracts, system archive → our tool lists/extracts. Byte-for-byte identical on real files.
- **crontab** (Go): Cron-expression parser + next-run scheduler. Fields as uint64 bitsets (bit v set ⇒ value v allowed). Steps always ride on a range (no special-casing). dom/dow OR rule (the gotcha): when both day-of-month AND day-of-week restricted, day matches if EITHER matches; requires `domStar`/`dowStar` booleans to distinguish "user wrote *" from "all values listed". Time arithmetic: jump month/day/hour/minute (not crawl); time.Date handles rollovers/leap years; 5-year safety bound for impossible exprs. Strictly-after semantics: +1m before search so 09:15 queries return 09:30, not 09:15. Verified: `0 0 13 * 5` fires on every Fri AND 13th; year rollover, impossible dates all correct.

**Phase 3 Wave 1 complete:** 5/7 challenges done (jq, yq, xargs, tar, crontab). curl & shell (wave 2) not yet started. All approved by Ellie 2026-06-09. CURRICULUM.md checkboxes for challenges 16–20 all ticked.

### 2026-06-09 — Phase 3, Wave 2: curl + Shell capstone (Go) — ✅ APPROVED (Ellie review)

Built the final two challenges of Phase 3: **curl** (Challenge 21, Go), **Shell/gosh** (22, Go capstone). Both in `phase-03-advanced-cli/{tool}/`.

**curl (Challenge 21):**
- **Raw-socket HTTP/1.1 client:** `net.Dial` opens byte-pipe; `crypto/tls.Client` for https. No net/http for protocol — request framed by hand, response parsed by hand.
- **Flags:** `-X METHOD`, `-H` (repeatable, override defaults), `-d DATA` (→ POST), `-o FILE`, `-I` (HEAD), `-v` (verbose), `-L` (follow 3xx, capped 10).
- **Body framing (the teaching payload):** Two schemes — `Content-Length` (exact read) AND `Transfer-Encoding: chunked` (hand-written decoder). Chunked decoder unit-tested hard: hex sizes, `;ext` stripped, per-chunk CRLF consumed, 0-chunk + trailer drained. Read-to-EOF fallback (valid because we send `Connection: close`).
- **File split:** `url.go` (parse + redirect resolution), `conn.go` (dial/TLS), `request.go` (framer), `response.go` (parser + chunked decoder), `main.go` (CLI/redirect loop).
- **Layout:** Same Go pattern (thin main → testable run), injected streams, hand-rolled flags, exit codes 0/1/2. `.gitignore` ignores `/curl` binary.
- **README-first:** 326 lines, teaches socket→bytes→HTTP from first principles, byte-by-byte request/response diagrams, ASCII chunked format diagram, TLS in one paragraph, links go-quickstart.md. Already documents the CGO_ENABLED=0 workaround (no doc gap).
- **Verification:** `go vet` clean; `CGO_ENABLED=0 go test ./...` → 30 pass (unit + 3 e2e over local `net.Listener`). Live network: `-I example.com` → 200; `-v example.com https` → TLS + chunked; `-L github.com` → redirect chain; `-o file` saved body. Approved.

**⚠️ Toolchain note for repo-wide future use:**
- On macOS, `go test ./...` aborts with `dyld: missing LC_UUID load command / signal: abort trap` for packages importing `net`/`crypto/tls` (cgo system resolver → external linker ↔ Xcode CLT mismatch). NOT a code bug.
- **Fix: `CGO_ENABLED=0 go test ./...`** — pure-Go linker + native resolver. `go vet` and `go build` unaffected.
- **Phase 4+ networking challenges (web server, proxy, etc.) will hit this — default to `CGO_ENABLED=0` for test runs.** Curl README documents this; future challenges should reference that documentation.

**Shell/gosh (Challenge 22, Phase 3 capstone):**
- **Working interactive Unix shell:** Tokenizer → recursive-descent parser → pipeline AST → fork/exec executor wiring real pipes/redirects.
- **Features:** quotes (single/double) + backslash escapes; pipelines `|`; redirections `>`, `>>`, `<`, `2>`; sequencing `;`; logical `&&`/`||`; env expansion `$VAR`/`${VAR}`/`$?`/`$$`; builtins `cd`/`pwd`/`exit`/`echo`/`export`/`type`; Ctrl-C swallows at shell (child dies), interactive REPL + `-c "cmd"` + script-file modes.
- **AST shape:** `List(;) → AndOr(&&/||) → Pipeline(|) → Command(args+redirs)` — grammar nesting encodes precedence (`;` loosest, redirs tightest).
- **File split:** `lexer.go` (tokenize + quote/escape/operator recognition), `parser.go` (recursive-descent → AST), `executor.go` (fork/exec + fd wiring), `expand.go` (`$VAR` expansion), `builtins.go` (in-process commands), `repl.go` (interactive loop + SIGINT handler), `main.go` (mode select). All under `internal/shell/` for testability without TTY.

**Hard-won reusable lessons (process-spawning patterns):**
1. **#1 pipe hang bug: parent must CLOSE its pipe-fd copies after starting children.** Each exec dups fd into child; if parent keeps write-end open reader never sees EOF and hangs forever. Solution: explicit "ownership rule" — every pipe-end used by exactly one stage. External stages → parent closes after `Start()`, builtin stages (goroutine) → goroutine closes own. Comments in `execMulti` + `parentCloses` explain this critical pattern.
2. **`cd` MUST be a builtin** — working directory is per-process state; child `cd` changes its own dir then exits, parent unmoved. Same for `exit`/`export`/assignment (all mutate shell state). README has prominent section with the "why".
3. **fork/exec as two-step with gap:** `cmd.Start()` ≈ fork (returns immediately), `cmd.Wait()` ≈ wait; the gap is where you rewire fds via `cmd.Stdin/Stdout/Stderr =` assignment.
4. **Lexer word-part coalescing gotcha:** unquoted chars must coalesce into one expandable word-part, else `$MYVAR` tokenizes as separate `$` + `M` + `Y` + `A` + `R` tokens and the `$` never sees the name. Store words as `[]wordPart{text, expand}` so quoting context (single-quote=literal, double/unquoted=expandable) survives to expansion stage. Caught via failing `export`/`$?` test.
5. **Glued operators** (`2>file`, `a"b"c`): detect word exactly matching `"2"` immediately before `>` to emit stderr-redirect token.
6. **SIGINT model:** shell + foreground child share process group → both get SIGINT; shell installs handler that swallows it (reprints prompt) while child dies by default. Simple, matches bash feel.

**Init-cycle gotcha (Go dispatch-table pattern):**
- `var builtins = map{...}` literal whose funcs call `isBuiltin` (reads the map) is a compile-time initialization cycle. Fix: populate the map in `init()` instead. Pattern reusable for any dispatch-table-with-self-reference.

**Layout:** Same Go pattern (internal/shell injected-streams package, thin main.go mode-selector, testable via `bytes.Buffer`). README-first: 297 lines, fd-level `cmd1 | cmd2 > file` pipeline diagram, dedicated "Why `cd` must be a builtin" section, EOF/parent-close hang trap, Python analogies (Popen≈Start), links go-quickstart.md.

**Verification:** `go vet` clean; `go test ./...` → 33 pass (tokenizer/parser/expand/executor/builtins + REAL execution tests). Manually: `cd`+`pwd` works; `echo a b | cat | wc -w` → 2; redirect+readback; `false ; echo $?` → 1; `true ; echo $?` → 0; `type cd/ls` resolves; export + `$HOME` expansion correct. All correct.

**Scope boundaries (documented in README):**
- No job control (`&` background, `fg`/`bg`), no globbing, no command substitution `$(...)`, no here-docs. `2>>` treated as `2>` (overwrite) — deliberate, documented.
- Expansion does not re-split on spaces — one arg stays one arg.

**Teaching angles:**
1. The orchestrator that runs every other Phase 2 tool — capstone tying everything together.
2. The pipe EOF hang and parent fd-close ownership rule — critical for any multi-process code.
3. Why `cd` must be a builtin — process-state-mutation insight.
4. fork/exec as two-step with a fd-rewiring gap.

**Status:** ✅ Approved by Ellie 2026-06-09. **Phase 3 (Advanced CLI) COMPLETE — 22/22 challenges done.** CURRICULUM.md checkboxes for challenges 21–22 all ticked.


---


## Phase 4 Wave 1 Repair & Completion — Challenges 23, 25, 27, 28

**Date:** 2026-06-13 · **Context:** Parallel overnight build half-failed (two challenges got only go.mod + stub README). This session repaired and completed all four.

### Challenge 23: DNS Resolver — Built from scratch

**What I built:**
- `message.go` — wire format: 12-byte Header, QNAME labels, ResourceRecord parsing, message parsing.
- `resolver.go` — UDP exchange, recursive and iterative modes with NS referrals.
- `main.go` — CLI parsing, dig-like output, testable run function.
- Tests: unit tests with crafted bytes, integration test gated DNS_NETWORK_TEST=1.

**Key design decisions:**
1. Both resolution modes clearly labelled (default recursive, --trace iterative).
2. decodeName always takes full message + offset to handle name compression pointers.
3. Single running offset threads through parseMessage for compression pointer handling.
4. UDP deadline prevents hangs (no connection drop signal in UDP).
5. Test wire format on crafted bytes, not just round-trip our encoder.

**Verification:** go vet clean; CGO_ENABLED=0 go test all 11 pass; live queries verified.

**Ellie note:** Name compression decode is the highlight. Thoroughly tested. APPROVED.

### Challenge 25: NTP Client — README restored

**What I built:** README only; code was already complete. Documented 48-byte packet, epoch offset (2208988800), four timestamps, offset/delay formulas.

**Key insight:** Epoch conversion math: NTP seconds == offset → Unix epoch 1970-01-01T00:00:00Z.

**Verification:** go vet clean; CGO_ENABLED=0 go test all pass (unit + live against pool.ntp.org).

**Ellie note:** Epoch math correct, 48-byte format verified, README complete. APPROVED.

### Challenge 27: Port Scanner — README restored

**What I built:** README only; code was already complete. Documented TCP connect scan, worker-pool pattern with channels/goroutines, why timeouts are essential.

**Key pattern:** Worker pool with buffered channels, N goroutines, WaitGroup, separate closer goroutine for clean shutdown.

**Verification:** go vet clean; CGO_ENABLED=0 go test all pass.

**Ellie note:** Timeout non-negotiable, worker-pool textbook, README complete. APPROVED.

### Challenge 28: Netcat — Built from scratch

**What I built:**
- main.go — CLI parsing, connect/listen mode dispatch.
- relay.go — bidirectional relay for TCP and UDP, TCP half-close via interface check, UDP deadline-driven termination.
- Tests: 6 self-contained tests on 127.0.0.1:0.

**Key design decisions:**
1. One relay core for both TCP and UDP via io.ReadWriter interface.
2. Half-close via interface check (halfCloser), not protocol flag.
3. UDP termination by read deadline (normal end of connectionless relay).
4. udpListenConn adapter wraps UDPConn to satisfy io.ReadWriter.
5. Test seams take already-bound listener/socket.

**Verification:** go vet clean; CGO_ENABLED=0 go test all 6 pass.

**Ellie note:** Textbook half-close via interface check. Reusable adapter. README complete. APPROVED.

### Environment gotcha (recurring Phase 4)

go1.22.2 / darwin-arm64: importing net pulls cgo; external linker mismatch causes dyld: missing LC_UUID abort. Fix: CGO_ENABLED=0 go test and go build. All four READMEs document this.

### Overall: Phase 4 Wave 1 Complete

Four Go networking challenges, all Ellie-approved: DNS wire format + compression decode, NTP 48-byte + epoch math, concurrent worker-pool scanner, bidirectional TCP/UDP relay.

**Status:** All four approved by Ellie 2026-06-13. Phase 4 networking: 4/7. CURRICULUM.md checkboxes 23/25/27/28 ticked.


## Phase 4 Wave 2 — Challenges 24, 26, 29 (Networking Complete)

**Date:** 2026-06-13 · **Status:** ✅ All three approved by Ellie.

### Challenge 24: DNS Forwarder (UDP listen + caching)

**What I built:**
- `cache.go` — concurrent-safe `sync.RWMutex` cache keyed on (QNAME, QTYPE, QCLASS) triple, injectable clock for testability.
- `forwarder.go` — UDP listen loop, query forward to upstream, answer relay + patched transaction ID.
- `main.go` — CLI parsing (--listen, --upstream, --verbose).
- Tests: fake local upstream with hit counter proves caching; table-driven TTL boundaries.

**Key decisions:**
1. Cache key must be the FULL triple (QNAME, QTYPE, QCLASS) — IPv4 answer to AAAA query is wrong. Wrote this as the headline gotcha.
2. Minimum TTL across answer set (answer is only as fresh as its shortest-lived record).
3. Copy UDP read buffer before goroutine (reused on next read — classic UDP gotcha).
4. Patch transaction ID when serving from cache or client rejects as unsolicited.
5. Injectable clock lets TTL tests advance time instantly (no `time.Sleep` flakiness).

**Testability win (reusable for forward/relay components):**
Real local UDP peer + atomic hit counter on 127.0.0.1:0 is the cleanest offline proof of caching. No internet, fully hermetic.

**Verification:** `CGO_ENABLED=0 go vet ./...` + `CGO_ENABLED=0 go test ./...` all pass.

**Scope boundaries (in README):** No on-the-wire TTL decrement (serve original TTL), no negative caching, no EDNS0, no TCP fallback, no size-based eviction. Natural follow-ups.

**Status:** ✅ Complete and Ellie-approved.

### Challenge 26: Traceroute (unprivileged ICMP)

**What I built:**
- `icmp.go` — build echo-request bytes, parse/classify replies (Time Exceeded / Echo Reply / Dest Unreachable).
- `trace.go` — TTL iteration loop via `prober` interface, hop rendering.
- `main.go` — CLI parsing (--max-hops, --probes, --timeout, --resolve).
- Tests: scripted fake prober drives loop offline; live test self-skips on socket error.

**Key decisions:**
1. **Unprivileged ICMP:** Used `icmp.ListenPacket("udp4", ...)` + `ipv4.PacketConn.SetTTL` per probe (same as macOS `ping`, no root needed).
2. **Testability seam:** Three pure pieces (build bytes, parse/classify, loop) + `prober` interface. Fake prober drives deterministic tests; live test self-skips on offline (never fails).
3. **Toolchain:** `golang.org/x/net` pinning — latest v0.56 requires Go ≥ 1.25. Pinned v0.31.0 + `GOTOOLCHAIN=local` to stay on repo's go1.22.

**Reusable lessons I learned:**
1. Unprivileged datagram ICMP peer carries junk `:0` port (discovered via live run vs unit tests).
2. Read deadlines (not per-call timeouts) — Go uses `SetReadDeadline(absolute-time)`, Python analogue `settimeout`.
3. macOS LC_UUID linker bug reappeared — plain `go test` → abort. Fixed: `CGO_ENABLED=0 go test ./...`.

**Verification:** `CGO_ENABLED=0 go vet ./...` + `CGO_ENABLED=0 go test ./...` pass; live reached 8.8.8.8 at hop 8.

**Scope (README):** ICMP-probe only (like Windows `tracert`); UDP variant explained but not built. IPv4 only. Reverse-DNS opt-in, best-effort.

**Status:** ✅ Complete and Ellie-approved.

### Challenge 29: HTTP Forward Proxy (Phase 4 capstone)

**What I built:**
- `proxy.go` — HTTP request rewriting (absolute→origin), hop-by-hop header stripping, `Connection: close` framing.
- `tunnel.go` — CONNECT tunnel: dial origin, reply 200, `io.Copy` both directions (TCP half-close via interface check).
- `main.go` — CLI, listen/accept loop.
- Tests: httptest origin + proxy (plain HTTP); httptest.NewTLSServer + proxy (CONNECT); raw socket CONNECT test.

**Why this is the capstone:**
Literally reuses prior Phase 4 lessons. CONNECT tunnel = netcat bidirectional relay. Plain-HTTP rewriting = curl's "HTTP is just text on a socket". README explicitly calls both connections out.

**Teaching angle (headline — made #1 Key Takeaway):**
**TLS opacity.** After `200 Connection Established` proxy never sees session keys. Cannot read/alter HTTPS traffic. Only way to "see inside" is TLS interception with forged cert (mitmproxy / corporate MITM) — exactly why HTTPS makes that refusable.

**Key decisions:**
1. **Hand-rolled parsing logic:** `http.ReadRequest` (parsing already taught), but request rewriting (absolute→origin), hop-by-hop stripping, CONNECT tunnel all hand-rolled.
2. **Request rewriting:** `req.URL.RequestURI()` converts URL→path (the thing that makes a proxy a proxy).
3. **`Connection: close` simplification:** Forces it on origin → response ends at EOF → proxy `io.Copy` relay (no response parsing). Good teaching move; production would loop for keep-alive.
4. **Read client tunnel side through `bufio.Reader`** (not raw conn) so pipelined bytes after CONNECT aren't dropped.
5. **TCP half-close via interface check** (not protocol flag) — checks if conn satisfies `halfCloser` interface.

**Testing (fully self-contained):**
- Plain HTTP: `httptest.NewServer` origin asserts it never sees absolute-form (proves rewrite).
- CONNECT: TLS handshake completing *through tunnel* is itself proof relay is byte-accurate.
- Raw-socket CONNECT: hand-written line, manual TLS handshake, wire-level verification.

**Toolchain:** Same macOS LC_UUID issue; `CGO_ENABLED=0 go test ./...` required.

**Verification:** `go vet ./...` clean; `CGO_ENABLED=0 go test ./...` all pass; `CGO_ENABLED=0 go build .` succeeds.

**Status:** ✅ Complete and Ellie-approved. **Phase 4 (Networking) COMPLETE — 7/7 challenges approved.**

### Overall Phase 4 completion

Phase 4 is now end-to-end as a learning arc: curl → DNS wire format + NTP epoch + scanner → netcat relay → this proxy. Each challenge layered on prior lessons.

**Total Phase 4:** 7/7 challenges approved. First four (23/25/27/28) approved 2026-06-13 morning; final three (24/26/29) approved 2026-06-13 end of wave.

**Curriculum status:** 25/64 challenges complete (Phases 1–4 complete; Phases 5–8 pending).

---

## Phase 5: Servers & Infrastructure (4/7 Wave 1 — Go)

### Challenges 30, 33, 34, 35 — Four Go Server Challenges (2026-06-13)

**Status:** ✅ Completed and Ellie-approved.

All four servers built from scratch over raw TCP sockets, no external dependencies (deliberate choice). Key thread: **framing styles and concurrency patterns are reusable across server builds**.

#### Reusable Patterns from Phase 5 Wave 1

1. **`serve(ln net.Listener)` split from `listenAndServe()`** — testability game-changer. Unit tests bind `127.0.0.1:0`, read `ln.Addr()`, drive real requests. Zero fixed ports, zero network flakiness. Reusable for all Phase 5 servers (Redis #32, load-balancer #31) and beyond.

2. **Two framing styles in one protocol** — the universal lesson:
   - Control frames: delimiter-framed (CRLF newline).
   - Payloads: length-prefixed (read exactly N bytes, then CRLF).
   - Enables binary-safe data in both memcached and NATS.

3. **Injectable clock (`Clock` interface, default `time.Now`)** — deterministic testing without `time.Sleep`. Tests advance clock by hand. Applied in: rate-limiter (all four algorithms), memcached (TTL expiry), DNS forwarder (#24, Phase 4). Becoming a phase-5 standard.

4. **Per-connection concurrency pattern:**
   - Goroutine-per-connection (cheap threads).
   - Dedicated write goroutine per client (channel-based, serialized socket writes).
   - Single `sync.Mutex` for critical sections (subscription registry, cache access).
   - Applied: web-server, memcached-server, nats-broker.

5. **LRU via map + `container/list`** — hand-built version of Python's `functools.lru_cache`. Front=hot, back=cold. Touch-on-access moves front. Past-cap evicts back. O(1) ops. Memcached-specific but reusable pattern for any bounded cache.

6. **Lazy refill, no background tasks** — rate-limiter: compute token accrual on access (`elapsed × rate`). Zero background goroutines/timers. Exact, zero idle cost. Philosophy: computation on demand beats background cleanup.

7. **`CGO_ENABLED=0` as Phase 5 standard** — all four challenges import `net` or `net/http`, all hit macOS LC_UUID linker bug (cgo resolver mismatch with Xcode CLT). Fix is universal. Updated README toolchain note in each; worth standardizing across all Phase 5 Go challenges.

8. **Go idioms for Python/Java learners** — called out consistently:
   - Goroutines (cheap) vs threads/asyncio.
   - `bufio.Reader` + `ReadString('\n')` = readline.
   - `sync.Mutex` vs GIL (Go has real parallelism).
   - Structural interfaces (no `implements` keyword).
   - `http.Handler` middleware = decorator pattern.
   - `io.ReadFull` for fixed-length bodies (single `Read` can short-read).

#### Challenge 30: Web Server

- **Raw TCP, hand-rolled HTTP/1.1:** `net.Listen`, goroutine per connection, parse request line/headers/body by hand, frame responses with CRLF, route method+path to handlers, serve static files (Content-Type by extension), 404/405/501 semantics, path-traversal defense (two layers: lexical + resolved-abs containment), keep-alive with read deadlines (slow-loris defense).
- **Key reusable:** `serve(listener)` split enables unit testing with `127.0.0.1:0`.
- **HTTP/1.0 vs 1.1 keep-alive flip:** 1.0 closes by default, 1.1 keeps by default. Made this the single most-called-out protocol detail (README table + test cases).
- **10 tests:** parsing, static, 404, traversal rejection, keep-alive reuse, Connection:close, dynamic route, 405.
- **Verification:** `go vet` clean, `CGO_ENABLED=0 go test` PASS.

#### Challenge 33: Memcached Server

- **TEXT protocol over TCP:** set/add/replace/append/prepend/cas/get/gets/delete/incr/decr/flush_all.
- **Two framing styles:** command lines (CRLF), values (length-prefixed).
- **Store:** map + container/list LRU (O(1)), per-item expiry (lazy eviction), 32-bit FLAGS, CAS token, goroutine per connection.
- **Injectable clock:** deterministic expiry tests (no sleeps).
- **LRU:** hand-built map+list = Python `functools.lru_cache` equivalent.
- **24 tests:** unit + end-to-end TCP.
- **Verification:** `go vet` clean, `CGO_ENABLED=0 go test` PASS.

#### Challenge 34: NATS Message Broker

- **NATS text protocol:** CONNECT/PING/PONG/PUB/SUB/UNSUB/INFO/MSG/±OK.
- **Subject routing:** `*` (one token), `>` (tail, must be last, ≥1 token). Isolated in `subject.go` with table-driven test.
- **Concurrency:** goroutine per client, dedicated write loop, channel-based dispatch, single mutex over registry (held only while building recipient list).
- **Fan-out vs queue groups:** plain subs all receive; queue groups: exactly one per group (round-robin).
- **At-most-once:** slow consumer drops = feature, not error (select on quit channel).
- **Verification:** `go vet` clean, `CGO_ENABLED=0 go test` PASS.

#### Challenge 35: Rate Limiter

- **Four algorithms:** token bucket, leaky bucket, fixed window, sliding-window log — all behind one `Limiter` interface.
- **Exposed as `net/http` middleware** → 429 + `Retry-After` + `X-RateLimit-*` headers.
- **Injectable clock:** fake clock advances in tests (zero sleeps).
- **Lazy refill:** compute token accrual on access (no background tasks).
- **Fixed window flaw:** boundary-burst is real and demonstrated in test (contrast sliding-window).
- **Verification:** `go vet` clean, `CGO_ENABLED=0 go test` PASS.

#### Phase 5 Progress

- **Cumulative:** 4/7 Phase 5 challenges complete (web-server, memcached, NATS, rate-limiter).
- **Overall:** 29/64 total challenges.
- **Next wave:** load-balancer (#31), redis-server (#32), docker (#38, optional).

#### Recurring Technical Debt & Observations

1. **macOS `CGO_ENABLED=0` is now universal** — every Go challenge importing `net`/`net/http`/`crypto/tls` hits the LC_UUID linker bug. Worth adding to a global "Go Quickstart" FAQ or pre-check script.

2. **Two-framing-styles lesson generalized** — appears in curl (HTTP Content-Length), memcached, NATS, DNS (variable-length name compression pointers). This is a canonical lesson across the curriculum.

3. **Injected time/clock = de facto standard** — DNS forwarder, rate-limiter, memcached all use it. Consider adding to `docs/go-quickstart.md` as idiomatic test pattern.

4. **`serve(listener)` split reusable** — already applies to web-server; same pattern fits redis-server, load-balancer, future proxy servers. Recommend codifying in docs.


#### Challenge 31: Load Balancer

- **HTTP reverse-proxy:** listens on one addr, picks healthy backend per request, forwards + streams response. `httputil.ReverseProxy` for mechanics; hand-wrote scheduling algorithms + health checks (the lesson).
- **Scheduler interface** = Strategy pattern: RoundRobin / LeastConn / Random / WeightedRoundRobin all satisfy `Next([]*Backend) *Backend`. Balancer never branches on algorithm name.
- **Pool filters health; schedulers health-blind:** `HealthyBackends()` returns only live backends, so round-robin "skipping" is automatic.
- **Active + passive health:** Active = `time.Ticker` probing `/health` (brings backends back). Passive = ReverseProxy `ErrorHandler` marking DOWN on real failure (fails OUT in ms).
- **Counter lifetime via `defer`:** `acquire()` then `defer release()` ensures active count stays high for request duration. That's what makes least-connections meaningful.
- **Atomics vs mutex split:** Per-backend `alive` (atomic.Bool) + `active` (atomic.Int64) on hot path; `sync.RWMutex` only around backend *list*.
- **11 tests:** round-robin, least-conn, weighted, health check, 503 when all down, passive mark-down, CLI parsing.
- **Verification:** `CGO_ENABLED=0 go test ./...` all 11 pass.

#### Challenge 32: Redis Server (XL)

- **RESP2 protocol from scratch:** hand-rolled encoder/decoder, TCP goroutine-per-connection, in-memory store, expiry (lazy + active), RDB snapshot persistence.
- **Two framing styles headline:** delimiter framing (simple strings/errors/integers, read until CRLF) + length-prefix framing (bulk strings/arrays, header says bytes/elements). Binary-safe bulk strings. Same lesson as HTTP Content-Length + Memcached (cross-linked).
- **One tagged `Value` struct** vs interface hierarchy: keeps `Marshal` as single switch, makes byte-exact tests trivial.
- **`io.ReadFull` for TCP short-read fix:** read exactly len+2 bytes for bulk string body.
- **Dispatch via `map[string]commandHandler`:** idiomatic Go, adding command is one map entry.
- **`sync.RWMutex` + no GIL:** RLock for KEYS/TTL (no mutation); Lock for writers + lazy-delete. Go has real parallelism so lock is mandatory correctness, not politeness (concurrent map write = crash).
- **Expiry = lazy + active both:** Lazy delete inside Get/Set; background sweeper on `time.Ticker` bounds memory for never-read keys.
- **Injectable clock:** `Store.now func() time.Time` for deterministic expiry tests (zero sleeps).
- **RDB snapshot = custom text format:** atomic temp-file + `os.Rename` for crash safety; load-on-start + save-on-shutdown + explicit SAVE/BGSAVE.
- **Graceful shutdown:** `quit` channel + `sync.WaitGroup`, `Close` guarded by `sync.Once` to avoid double-close panic.
- **42 tests:** codec, store, TCP round-trip, active sweep, save/reload with TTL, race detection. `CGO_ENABLED=0 go test -race ./...` PASS.
- **Note:** Phase 7 Challenge 47 (Redis CLI) will be RESP *client* + REPL talking to THIS server. Wire contract fixed here.

#### Challenge 36: Docker (CAPSTONE, Linux-only)

- **Container runtime = Linux process + namespaces + cgroups:** UTS + PID + mount + network + IPC namespaces, `pivot_root` into rootfs, fresh `/proc`, hostname, memory/pids limits (v1 + v2), exec user command as PID 1.
- **Linux-only solved via build tags:** `run_linux.go` + `cgroup_linux.go` have `//go:build linux`; `run_other.go` is `//go:build !linux` (stub returns clear error naming GOOS/GOARCH). Pure logic in `main.go`/`config.go`/`layers.go` (no tag) compiled everywhere. Tests split same way.
- **Re-exec trick = heart of it:** Go can't safely `fork()` from multi-threaded runtime, so parent sets `SysProcAttr.Cloneflags` + re-executes `/proc/self/exe child …`; child finishes setup *inside* new namespaces. ASCII diagram + full prose.
- **Config crosses re-exec via argv:** `childArgs(cfg)` → `child --membytes N --pids N --hostname H <rootfs> <cmd> [args]`; `parseChildArgs` parses back. Round-trip unit test is safety net.
- **Resolve units in parent:** `--mem 100m` → bytes in parent; child sees only `--membytes` int. Keeps modes in sync, child code unit-free.
- **`flag` stops at first positional:** container command like `/bin/ls -la` keeps own `-la`. Dedicated test.
- **`pivot_root` over `chroot`:** canonical 4-step (make private, bind-mount self, pivot_root, detach old root). chroot is escapable; real runtimes use pivot_root.
- **cgroup v1 AND v2:** detect v2 by presence of `/sys/fs/cgroup/cgroup.controllers`; write `memory.max`/`pids.max` for v2 or `memory.limit_in_bytes` tree for v1.
- **OverlayFS path math testable cross-platform:** `layers.go` reverses base-first order, renders `lowerdir=…,upperdir=…,workdir=…` option string — unit-tested on macOS even though mount is Linux-only.
- **User namespace documented-but-off:** `CLONE_NEWUSER` + uid/gid mappings as commented code (fights with `/proc` mount + `CLONE_NEWNET` on some kernels).
- **10 platform-neutral tests:** parseSize, parseRunArgs (flags-after-cmd, defaults, missing-arg), childArgs round-trip, dispatch, overlay layout. All pass on macOS.
- **Verification:** `GOOS=linux go build ./...` succeeds (cross-compile). Linux recipe in README: export alpine rootfs, `sudo ./gocker run … /bin/sh`, prove isolation with `hostname`/`ps`/`ls /`.

#### Phase 5 Milestone

- **All 7/7 Phase 5 challenges complete** (web-server, memcached, NATS, rate-limiter, load-balancer, redis-server, docker).
- **32/64 overall challenges (50% curriculum complete).**
- **All three Wave 2 challenges APPROVED by Ellie.**
