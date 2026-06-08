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
