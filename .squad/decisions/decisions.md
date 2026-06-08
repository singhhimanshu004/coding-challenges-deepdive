# Decisions Log

## Decision: Learning Curriculum v2 (regenerated)

**Author:** Grant (Lead) · **Date:** 2026-06-08 · **Model:** claude-opus-4.8 · **Requested by:** Himanshu Singh

### Summary
`CURRICULUM.md` at the repo root was **regenerated from scratch on claude-opus-4.8** at the user's request. The prior version (a weaker-model draft) was read for reference only; this v2 is a fresh, improved plan and fully overwrites it.

### What changed vs v1
- **Honest challenge count:** v1 inflated the list to "65" by inventing a duplicate *Calculator (GUI)*. v2 covers the **64 challenges actually listed in the README** — none dropped, none fabricated.
- **Difficulty rubric:** added explicit 4-level rubric (🟢 Beginner / 🔵 Intermediate / 🟠 Advanced / 🔴 Expert) and colored ratings per challenge.
- **Effort estimates:** every challenge now carries an ⏱️ S/M/L/XL estimate.
- **Dependency rigor:** phase capstones, "skill unlocked" framing, and a pruned dependency graph showing only load-bearing edges.
- **More learning paths:** added Protocols & Networking and Language & Interpreters paths alongside Sprint/Systems/Full-Stack/Fun-First.
- **Trackability:** added a progress-at-a-glance table plus per-challenge checkboxes.

### Phase structure (retained 8 phases, counts corrected)
1. Foundations: Parsing, Encoding & Data Structures (4) — 🟢→🔵
2. Core Unix: Text Processing (11) — 🟢→🔵
3. Advanced CLI & Orchestration (7) — 🔵
4. Networking Fundamentals (7) — 🔵→🟠
5. Servers & Infrastructure (7) — 🟠→🔴
6. Applications & Full-Stack (9) — 🔵→🔴
7. Developer Tools & Internals (6) — 🔵→🔴
8. Games, Interpreters & Creative (13) — 🟢→🔴

**Total: 64.**

### Progression rationale
Each phase boundary is a capability jump: parse bytes → stream text → orchestrate processes → open sockets → run concurrent servers → build products → build tooling internals → apply creatively. Within phases, challenges introduce one new idea at a time. Key hard orderings preserved: **JSON Parser → jq/Lisp/Calculator**, **wc → … → Shell**, **DNS Resolver → Forwarder**, **Web Server → Load Balancer / URL Shortener**, **Redis Server → Redis CLI**, **curl → networking + Load Tester**, **Spotify Client → Playlist Backup**.

### Language strategy (unchanged intent, counts corrected)
Go 35 (CLI/networking/servers/Git), Python 18 (parsing/data/games/interpreters/API), TypeScript 11 (web/real-time/browser/canvas games), Java 0 (first-class alternate, esp. server tier).

### Mandates respected
- README-first policy enforced for all 64 challenges (8-section teaching structure).
- Source of truth: project README + codingchallenges.fyi.

---

## Decision: Repository Directory Layout Convention

**Author:** Malcolm (Content Dev) · **Date:** 2026-06-08 · **Status:** Proposed

### Context
The curriculum (CURRICULUM.md) defines 8 sequential phases containing 64 challenges. We need a consistent, predictable on-disk layout so every contributor places challenges the same way and the README-first mandate is enforced uniformly.

### Decision
Adopt the following directory layout convention for ALL challenges:

1. **Phase directories** live at the repo root and are named:
   `phase-NN-short-slug/`
   - `NN` is the zero-padded phase number (01–08).
   - `short-slug` is a kebab-case phase name.
   - Mapping: phase-01-foundations, phase-02-core-unix, phase-03-advanced-cli, phase-04-networking, phase-05-servers-infrastructure, phase-06-applications-fullstack, phase-07-developer-tools, phase-08-games-interpreters.

2. **Challenge directories** live inside their phase and are named with a **kebab-case slug** derived from the challenge name (e.g. `json-parser/`, `huffman-compression/`, `http-forward-proxy/`). Order matches CURRICULUM.md.

3. **Every challenge directory MUST contain a `README.md`** following the required README-first skeleton:
   - H1 = challenge title.
   - Metadata block: phase, difficulty, recommended language, effort estimate.
   - `**Status:**` line using 🔲 Not started → 🚧 In progress → ✅ Done.
   - The 7 required teaching sections: 🎯 What We're Building, 📚 Core Concepts, 🏗️ Architecture & Design, 🔨 Step-by-Step Implementation, 🧪 Testing Strategy, 💡 Key Takeaways, 📖 Further Reading.

4. **Every phase directory MUST contain a `README.md`** with a table of its challenges (linking to each subfolder) listing difficulty and language, mirroring CURRICULUM.md.

5. Implementation source code lives inside the challenge directory; language is the recommended language from CURRICULUM.md unless a deliberate alternate is chosen.

### Consequences
- The full skeleton (8 phases, 64 challenge stubs) has been seeded; future work only fills in stubs and adds code.
- All future challenges must follow this layout — no ad-hoc placement at the repo root.

---

## Decision: Conventions for Code-Bearing Challenges (Layout, Tests, README)

**Author:** Malcolm (Content Dev) · **Date:** 2026-06-08 · **Status:** Proposed

### Context
The repo-structure decision covered *where* challenges live and the README skeleton. With Phase 1 Challenge 1 (JSON Parser, Python) now implemented, we have a concrete, verified template for challenges that ship actual code + tests. Capturing it keeps every future challenge consistent so the whole team (and Himanshu's learning experience) stays uniform.

### Decision

#### 1. Source code layout
- Implementation lives **inside the challenge directory**, in a package/module folder named after the tool slug (e.g. `jsonparser/` inside `json-parser/`).
- Split code into **teaching-sized modules**, one per pipeline stage, rather than one monolith. For parser-style challenges the canonical split is:
  `tokens.py` (shared types) · `lexer.py` (stage 1) · `parser.py` (stage 2) · `errors.py` (one exception type) · `cli.py` + `__main__.py` (front end).
- Expose a **single top-level convenience function** mirroring the stdlib equivalent (e.g. `parse(text)` ~ `json.loads`) so the public API is obvious.
- Every module gets a docstring explaining *which stage it is and why*; comment the "why", not the obvious "what".

#### 2. CLI conventions
- Runnable via `python -m <package> [FILE]` with **stdin fallback** when no file is given.
- **Exit codes:** `0` success/valid · `1` domain failure (e.g. invalid input) · `2` usage/IO error (bad args, file not found). Errors go to **stderr**.
- Provide a `--quiet` validator mode where it makes sense.

#### 3. Test layout
- Tests in a `tests/` package; configure with a minimal `pytest.ini` (`testpaths = tests`).
- **Layer the tests:** unit tests per stage + escalating step fixtures (in the spirit of codingchallenges.fyi `step1…stepN` valid/invalid files) + CLI tests.
- Where a trusted reference implementation exists (e.g. stdlib `json`, GNU `wc`), **assert our output/verdict agrees with it** — it's the cheapest, most powerful correctness oracle.
- Parametrize valid and invalid corpora; always cover the format's classic edge cases explicitly.
- **Verify tests pass before marking a challenge Done.**

#### 4. Environment
- System Python may be PEP-668 externally-managed; use a per-challenge **`.venv/`** for test deps and **gitignore** it (plus `__pycache__/`, `*.pyc`, `.pytest_cache/`). Document the venv + `pip install pytest` steps in the README "How to run it" section.

#### 5. README-first (reaffirmed, with specifics)
- Write the README before/alongside the code; the 7 required sections in order.
- Strong READMEs include: an **ASCII data-flow / pipeline diagram**, a **trade-off table** for the central design choice (e.g. recursive descent vs table-driven), real-world context, progressive step-by-step build, explicit edge-case list, and concrete run/test commands.
- On completion: set `**Status:** ✅ Done` and tick the matching CURRICULUM.md checkbox (`[ ]` → `[x]`).

### Consequences
- Future challenges have a copy-paste-able skeleton for code, CLI, tests, and env setup — less bikeshedding, more learning.
- The "agree with the reference tool" testing tactic becomes the default correctness gate across the repo (Phase 2's `wc`/`cat`/etc. can diff against the GNU coreutils equivalents the same way).

---

## Review Verdict — Phase 1 / Challenge 1: JSON Parser

**Reviewer:** Ellie · **Date:** 2026-06-08 · **Path:** `phase-01-foundations/json-parser/`

### ✅ APPROVED

The JSON Parser meets the bar on all four review axes — correctness, completeness, code quality, and (the mandatory gate) README teaching depth.

### Verification performed

- Ran the suite: `./.venv/bin/python -m pytest -q` → **110 passed in 0.05s**.
- Manual CLI checks confirmed exit-code contract:
  - Valid stdin → `0`
  - Trailing comma `{"a": 1,}` → `Invalid JSON: Trailing comma in object (line 1, column 9)`, exit `1`
  - Unterminated `{"a":` → exit `1`
  - `--quiet` valid → no output, exit `0`
  - Missing file → exit `2` (usage)
  - `--no-duplicate-keys` on dup keys → exit `1`

### Key findings

**README (quality gate — PASS).** All seven mandated sections present and substantive: What We're Building → Core Concepts → Architecture & Design → Step-by-Step Implementation → Testing Strategy → Key Takeaways → Further Reading. Includes real-world context (APIs, configs, V8/serde), a clear lex→parse explanation, recursive-descent vs table-driven trade-off table, the stack-depth trade-off called out honestly, ASCII pipeline/data-flow diagrams, a number-grammar railroad diagram, and complete run/test instructions. A reader genuinely learns from it.

**Code (PASS).** Clean two-stage split (tokens / lexer / parser / errors / cli). Lexer correctly rejects leading zeros, bare `-`, `1.`, `.5`, `1e`; decodes simple escapes, `\uXXXX`, and UTF-16 surrogate pairs; rejects raw control chars and unpaired surrogates. Parser is faithful LL(1) recursive descent, rejects trailing commas/junk/missing separators, preserves int-vs-float, supports opt-in strict duplicate-key mode. Every failure raises a single `JSONParseError` carrying line/column. Well-documented without over-commenting.

**Tests (PASS).** Four layers (lexer, parser, steps, CLI). Strong technique: accept/reject verdicts cross-checked against stdlib `json` for both valid and invalid corpora. Edge cases all covered: escapes/unicode/surrogates, exponents, trailing commas (array+object), unterminated strings/objects, missing commas/colons, unquoted & single-quoted keys, leading zeros, 200-deep nesting, duplicate keys (last-wins + strict reject), huge integers, empty/whitespace input, trailing junk, and exact line/column assertions.

### Optional nice-to-haves (NON-BLOCKING)

1. **Graceful deep-recursion handling.** A pathologically deep document (e.g. 5000 levels) raises a raw `RecursionError` traceback on stderr (exit code is still non-zero/`1`). The README documents this trade-off, so it is not a blocker, but catching `RecursionError` in `cli.main` / `parse` and emitting a clean `Invalid JSON: maximum nesting depth exceeded` would make the failure mode match the project's "errors are a feature" ethos.
2. Cosmetic: the number railroad ASCII diagram in the README is slightly misaligned — purely visual.

No required changes. Challenge 1 is approved as Done.

---

## Decision: Conventions for Go Code-Bearing Challenges (Layout, Tests, .gitignore)

**Author:** Malcolm (Content Dev) · **Date:** 2026-06-08 · **Status:** Approved

### Context

The earlier "Conventions for Code-Bearing Challenges" decision was derived from
the Python JSON Parser. Phase 1 Challenge 2 (Huffman Compression) is the first
**Go** challenge, and Go has its own strong idioms that differ from Python.
Capturing them now keeps the ~35 Go challenges in the curriculum consistent.

### Decision

#### 1. Module + source layout
- Each Go challenge is its own module: a `go.mod` at the challenge root with a
  **short module path equal to the tool slug** (e.g. `module huffman`) and a
  `go <version>` line matching the toolchain (`go 1.22`).
- **CLI entry is `main.go` at the challenge root** (`package main`), kept thin:
  parse subcommands/flags, delegate to packages, map errors to exit codes.
- **Algorithm/library code lives in `internal/` packages**, split into
  teaching-sized units with one responsibility each (e.g. `internal/bitio/`,
  `internal/huffman/`). Use `internal/` so the packages are clearly
  implementation detail, and name files by role (`tree.go`, `heap.go`,
  `codec.go`).
- **Factor out genuinely reusable primitives into their own package** with no
  knowledge of the surrounding challenge (e.g. `bitio` knows nothing about
  Huffman) so later challenges can lift them.
- Prefer the **standard library** (`container/heap`, `encoding/binary`,
  `bufio`) over hand-rolling; comment the "why", not the obvious "what".

#### 2. Test layout (Go-specific — diverges from Python)
- Tests live **beside the code** as `*_test.go` in the same package — **NOT** a
  separate `tests/` folder (that was a Python/pytest convention). This is the Go
  idiom and lets tests touch unexported helpers.
- Run with `go test ./...`; static-check with `go vet ./...`. Both must be clean
  before marking a challenge Done.
- Keep the **"property/oracle" testing philosophy** from the Python decision: for
  codecs the oracle is the **lossless round-trip** `decode(encode(x)) == x` over
  an edge-case-rich corpus, plus a **seeded fuzz loop** for breadth. (The fuzzer
  caught a real non-determinism bug in Huffman that fixed inputs missed.)

#### 3. CLI + exit codes (unchanged contract)
- Subcommands (`compress`/`decompress`, with short aliases `c`/`d`) and a small
  flag parse (`-o output`). Errors to **stderr**.
- Exit codes stay consistent with the repo: `0` success · `1` domain failure
  (corrupt/invalid input) · `2` usage/IO error (bad args, missing file).

#### 4. `.gitignore` for Go challenges
Add a per-challenge `.gitignore` and **never commit build artifacts**:
```
/<tool>        # compiled binary (e.g. /huffman)
*.exe
*.test
*.out
*.<artifact>   # generated outputs, e.g. *.huf for the compressor
.DS_Store
```

#### 5. Determinism contract for self-describing binary formats
When a format stores a *seed* (frequency table, etc.) and both sides must
reconstruct identical structures: **never let reconstruction depend on Go's
randomized map iteration order.** Make any tie-break key on stable data (e.g. the
byte value), not insertion/iteration order. This generalizes to every future
binary-format challenge (tar, DNS, NTP).

### Consequences
- Go challenges now have a copy-pasteable skeleton (module, `main.go` + `internal/`,
  `*_test.go` beside code, `.gitignore`) distinct from the Python skeleton.
- The bit-I/O package (`internal/bitio`) established here is the reuse target for
  `tar` (19), `xxd` (15), and the binary network protocols in Phase 4.

---

## Review Verdict — Phase 1 / Challenge 2: Huffman Compression (Go)

**Reviewer:** Ellie
**Date:** 2026-06-08
**Path:** `phase-01-foundations/huffman-compression/`
**Verdict:** ✅ **APPROVED**

---

### Independent verification

| Check | Result |
|---|---|
| `go vet ./...` | clean |
| `go test ./...` | PASS (`internal/bitio`, `internal/huffman`) |
| CLI round-trip — README.md | 18177 B → 11762 B (ratio 0.647, saved 35.3%); sha256 input == output ✅ |
| CLI round-trip — empty file | 0 B → 14 B → 0 B, byte-identical ✅ |
| CLI round-trip — single byte `"A"` | 1 B → 17 B → 1 B, byte-identical ✅ |
| Bad magic / truncated body | rejected, exit 1 ✅ |

**Exit-code contract verified:** `0` success; `1` domain failure (bad magic,
truncated body); `2` usage/IO (no args, unknown command, missing input,
decompress without `-o`, `-o` without value); `--help` → 0. Compression ratio
output is sane (header overhead on tiny inputs reported honestly).

### Code quality
- Clean package separation: `bitio` (MSB-first BitWriter/BitReader) is fully
  Huffman-agnostic and unit-testable; `huffman` splits heap / tree / codec.
- Comments explain the *why*: zero-padding safety via `totalSymbols`, min-heap
  rationale, and the determinism contract.
- Standout correctness detail: deterministic tie-break keyed on byte value
  (leaves 0–255, internal nodes from 256) so encode and decode rebuild the
  identical tree despite Go's randomized map iteration order.

### Tests
Round-trip corpus covers empty, single-byte, single-symbol run, text, repeated
text, all-256-byte-values, random binary, and newline-heavy input; plus
prefix-free property, skewed-data shrink check, bad-magic rejection, and a
seeded 50-iteration fuzz loop. Bit-I/O units cover a 13-bit non-aligned pattern,
MSB-first `0xB2`, EOF, and empty flush. Coverage is meaningful and edge-complete.

### README (README-first gate)
**Passes decisively.** All 7 mandated sections present. Teaches entropy and the
Shannon bound, prefix codes as binary trees, Huffman greedy optimality, the
header-format trade-off (frequency table vs canonical Huffman vs tree
serialization, in a table), bit I/O with padding-safety reasoning, the
map-iteration determinism subtlety, ASCII diagrams, and full run/test
instructions. A reader genuinely learns the concept.

### Required changes
None — no blockers.

### Optional non-blocking nice-to-haves
1. `decompress` requires `-o` while `compress` defaults to `<input>.huf`; could
   default decompress output by stripping `.huf` for symmetry.
2. Tiny inputs print `saved -1600.0%`; honest but could add a "(too small to
   benefit)" hint. README already documents header cost.

---

## Decision: Bloom Filter Spell Checker — reusable conventions

**Author:** Malcolm (Content Dev) · **Date:** 2026-06-09 · **Challenge:** Phase 1, Challenge 3 — Bloom Filter Spell Checker (Go) — ✅ Done

### Context

Third code-bearing challenge, second in Go. Confirmed and extended the Go
template established by Huffman (Challenge 2). These conventions are now proven
across two Go challenges and should be the default for future Go work.

### Reusable conventions

#### Go module + layout (now confirmed twice — treat as the Go standard)
- `go.mod` with a short module path = tool slug (`module bloom`, `go 1.22`).
- Thin CLI `main.go` (package `main`) at the challenge root — subcommand parse +
  flags + I/O only; all logic delegated to `internal/`.
- One internal package per responsibility, teaching-sized:
  - `internal/<datastructure>/` for the core algorithm (here `internal/bloom`:
    bitset + hashing + filter).
  - `internal/codec/` for serialize/deserialize — a clean, reusable split. The
    codec depends on the algorithm package, never the reverse.
- Tests live **beside** the code as `*_test.go` (Go idiom), not in a separate
  `tests/` dir (that's the Python convention).
- `testdata/` for sample inputs (Go ignores dirs named `testdata` in builds).
- `.gitignore` covers the compiled binary (`/bloom`), `*.test`, `*.out`, and the
  tool's output artifacts (`*.bf`). Never commit build artifacts.

#### Exit-code convention (consistent across Go challenges)
- `0` success; `1` domain signal (corrupt input / misspelled word found);
  `2` usage or I/O error (bad args, missing file, empty input).
- Domain-meaningful exit `1` makes tools scriptable (e.g. `check` returns 1 if
  any word is flagged → usable in a commit hook).

#### Binary file format convention (extends Huffman's `HUF1` → `BLM1`)
- 4-byte ASCII magic that encodes name+version (`HUF1`, `BLM1`), a 1-byte
  version field, then fixed-width **big-endian** params, then payload.
- **Store every parameter the reader needs to reconstruct behaviour.** For
  Huffman that's the frequency table; for Bloom it's `m` and `k` (bit positions
  depend on them). Self-describing files load from nothing but themselves.
- Include a payload length and validate it (`nbytes == ceil(m/8)`) to detect
  truncation/corruption → return a sentinel `ErrBadFormat`.

#### Hashing trick worth reusing (caches, dedup, sketches)
- Kirsch–Mitzenmacher double hashing: synthesise k hashes from two base hashes
  via `g_i = h1 + i*h2 mod m`. Derive h1/h2 by splitting one 64-bit FNV-1a digest
  into halves. Guard `h2 != 0`. Use FNV-1a (or another fast non-crypto hash) when
  you need distribution, not adversary-resistance.

#### Testing convention for probabilistic / property-based structures
- Assert the hard invariant exactly (Bloom: zero false negatives over thousands
  of inserts).
- For statistical properties (observed FP rate), assert a *bound with slack*
  (≤ 3× target) and `t.Logf` the measured value — never assert an exact rate, or
  the test flakes.
- Always include the trivial edge cases: empty, single-element, invalid params.

---

## Review Verdict — Phase 1, Challenge 3: Bloom Filter Spell Checker (Go)

**Reviewer:** Ellie · **Date:** 2026-06-09T01:27:56+05:30 · **Path:** `phase-01-foundations/bloom-filter-spell-checker/` · **Verdict:** ✅ **APPROVED**

### Independent verification
- `go vet ./...` → clean (exit 0).
- `go test ./...` → PASS (`internal/bloom`, `internal/codec`).
- FP test (verbose): target p=0.0100, **observed 0.0089 (178/20000)**.
- End-to-end on `/usr/share/dict/words` (235,976 words): m=2,261,844 bits
  (276.1 KB), k=7, estimated FP 0.009736. `receive`→present, `recieve`→MISSPELLED.
- Exit codes: 0 all-present, 1 misspelling/corrupt-filter, 2 usage/IO — all confirmed.
  Stdin pipe path works.

### Why it passes
- **Correctness:** no-false-negatives test (5,000 words, zero tolerance) and a
  measured FP-rate test near p both pass; round-trip is byte-identical with same m/k.
- **Completeness:** build + check subcommands, optimal m/k sizing, FNV-1a double
  hashing with h2==0 guard, self-describing BLM1 codec with truncation check.
- **Quality:** clean bottom-up layering, `internal/bloom` decoupled from files/CLI,
  comments explain the *why*.
- **Docs:** README teaches the concept in depth — one-sided guarantee, full m/k
  derivation, double hashing, production uses, serialization format, diagrams,
  run/test instructions. All 7 mandated sections present.
- **Tests:** all four required (no false negatives, measured FP near p, Save→Load
  round-trip, edge cases incl. empty/single-word/clamped/corrupt input).

### Non-blocking nice-to-haves
- README sample dict counts differ slightly from current macOS dict (illustrative).
- `check -f` could default the filter path for symmetry with build's `-o` default.
- Tiny filters report `(0.0 KB)`; could show bytes for sub-KB sizes.

---

## Decision: Phase 2 Challenge 11: sort (Go) — External Merge Sort

**Author:** Malcolm (Content Dev) · **Date:** 2026-06-09 · **Status:** ✅ Done

### What got built
`phase-02-core-unix/sort/` — a complete Go sort implementation with external merge sort for large files.

Clean separation across files:
- `args.go` — hand-rolled flag parsing (bundled short flags `-rn`, attached/separated `-k`/`-t`, long flags).
- `compare.go` — the single shared comparator (key extraction, numeric/text/fold, reverse) + forgiving `leadingNumber` parser.
- `memsort.go` — in-memory path (`sort.SliceStable`, adjacent dedupe) + line I/O helpers.
- `external.go` — external merge sort: split → sorted runs on disk → k-way merge via `container/heap`.
- `main.go` — CLI wiring, path selection, exit codes (0/1/2).

Flags: `-r -n -u -f -k -t` plus teaching knobs `--external` and `--chunk-lines N`.

### Key learnings
1. **Module name can collide with stdlib import.** Naming the module `sort` made `import "sort"` ambiguous. Fix: module is `ccsort`, binary is `ccsort`. **Rule:** never name a Go module after a stdlib package you import.
2. **One comparator, both paths.** The strongest correctness guarantee was a test asserting the external path equals the in-memory path across flag combos.
3. **Stability must be engineered into the k-way merge.** Break heap ties by **run index** (runs are produced in input order, each internally stable).
4. **Compare against `LC_ALL=C sort`, not plain `sort`.** GNU sort uses locale-aware collation by default.
5. **External-sort scratch files** are created with `os.CreateTemp(".", "sort-run-*.tmp")` (local, not system temp) and removed via a `defer` that runs even on error.

### Verification
`go test ./...` and `go vet ./...` pass; output diffs clean against `LC_ALL=C sort` for lexicographic, `-r`, `-n`, `-nr`, `-u`, and a 5000-line numeric file (both in-memory and forced-external paths).

---

## Decision: Phase 2 Challenge 12: grep (Go) — RE2 Pattern Matching

**Author:** Malcolm (Content Dev) · **Date:** 2026-06-09 · **Status:** ✅ Done

### What shipped
`phase-02-core-unix/grep/` — a from-scratch Go grep with clean four-file split:
- `matcher.go` — compiles the pattern into a `Matcher` (regex + invert flag).
- `walker.go` — turns operands into named `Source`s (files / stdin / recursive walk).
- `output.go` — the reporting engine (counts, file lists, lines, context).
- `main.go` — argv parsing, wiring, exit codes.

Flags: `-i -v -n -c -w -r -l` plus context `-A/-B/-C`.

### Key learnings
- **Build flags into the pattern, not the loop.** `-i` → prepend `(?i)`; `-w` → wrap as `\b(?:PATTERN)\b`. The non-capturing group is *essential*.
- **RE2 is the headline teaching point.** Go's `regexp` is linear-time / no catastrophic backtracking, at the cost of backreferences + lookaround.
- **Context needs random access.** Read each source fully into `[]string` and merge `[i-B, i+A]` spans (merge on overlap OR touch, gap==0).
- **`go run` collapses non-zero exit codes to 1** — must build a binary to verify the 0/1/2 contract.
- **Filename prefix rule:** show `name:` when `recursive || len(fileOperands) > 1`, matching GNU. stdin's display name is `(standard input)`.

### Verification
`go test ./...` and `go vet ./...` pass; spot-checked against system `grep`. Exit codes verified live: **0** match, **1** no match, **2** bad pattern / dir without -r.

---

## Decision: Phase 2 Challenge 13: sed (Go) — Parser/Executor Interpreter

**Author:** Malcolm (Content Dev) · **Date:** 2026-06-09 · **Status:** ✅ Done

### What shipped
`phase-02-core-unix/sed/` — a from-scratch Go sed framed explicitly as a tiny interpreter, with clean parser/executor split:
- `internal/sed/command.go` — data model (`address`, `command`) + the two per-line behaviours: `applies` (addressing + range state machine) and `substitute` (first-vs-global, backref expansion).
- `internal/sed/parser.go` — `Parse`: hand-written recursive-descent over the script → `[]*command`. Includes `convertReplacement` (sed → Go dialect).
- `internal/sed/executor.go` — `Run`: the read → execute → auto-print cycle, the pattern space, `-n` suppression, `SplitLines`.
- `main.go` — `-n`/`-i` flag parsing, stdin/file/in-place wiring, exit codes.

Supports: `s/re/repl/[g][i][p]` with `\1` backrefs + `&`; `p`; `d`; addresses (line N, `$`, `/regex/`) and ranges `addr1,addr2`; `-n`; `-i` in-place.

### Key learnings
- **sed is the first "tool = language" challenge.** The whole story is parse-once → execute-per-line.
- **First-vs-global needs hand-rolled replacement.** `regexp.ReplaceAllString` always replaces *all* matches.
- **Two regex dialects collide — translate at the boundary.** Go is **RE2** (ERE-style), so groups are `(\w+)`, NOT BRE `\(\w+\)`. Replacement backrefs differ too: sed `\1`/`&` vs Go `${1}`/`$0`. `convertReplacement` is the single meeting point.
- **Ranges are a per-command state machine.** `addr1,addr2` carries an `active` bool *between lines* on the `command` struct.
- **Addresses match the pattern space, not the original line.** After an `s///` rewrites the pattern space, later commands' `/regex/` addresses see the modified text.
- **`d` semantics:** clears the line, skips remaining commands, AND suppresses auto-print. One `deleted` bool + `break` handles all three.
- **Any char can be the `s` delimiter** (`s|/usr|/opt|`). Read the byte after `s` and parse against it; `\<delim>` escapes a literal delimiter.

### Verification
`go test ./...` and `go vet ./...` pass; differential-checked against system `sed`.

---

## Decision: Phase 2 Challenge 14: diff (Go) — LCS Dynamic Programming

**Author:** Malcolm (Content Dev) · **Date:** 2026-06-09 · **Status:** ✅ Done

### What was built
The phase capstone for Phase 2: a from-scratch `diff` in Go at `phase-02-core-unix/diff/`. No diff library — the Longest Common Subsequence is computed with a dynamic-programming table and backtracked into an edit script. Supports normal (default), unified (`-u`, `-U n`), and context (`-c`) formats, plus stdin via `-`. Exit codes 0 (identical) / 1 (differs) / 2 (error).

### Design decisions worth reusing
- **Edit script as the seam.** The algorithm half (`lcs.go` → `editscript.go`) emits one `[]edit` intermediate; the formatting half (`format.go`) consumes it. Three output formats then share a single engine.
- **Each `edit` carries both 0-based file indices** (`aIndex`, `bIndex`) even when one is unused. This lets the unified/normal formatters compute hunk line ranges directly without re-walking the files.
- **Tie-break "up before left" in backtracking** (prefer deletion when LCS lengths are equal) reproduces GNU/BSD diff's deletions-before-insertions ordering.
- **Hunk merging rule** for unified format: cluster changes, pad each with `ctx` context lines, merge clusters whose windows touch/overlap. Default ctx = 3, `-U0` shows none.

### Verification
Compared `-u` and normal output against system `diff` on a 7-line edit — identical hunk header `@@ -1,7 +1,7 @@`, `+/-/space` prefixes, and `2c2 / 7c7` normal blocks. Tests cover identical, pure insertion, pure deletion, mixed change, empty-vs-nonempty, unified hunk-header correctness, single-line shorthand under `-U0`, and two separate hunks for distant changes.

---

## Decision: Phase 2 Challenge 15: xxd (Go) — Hex Dump + Reverse Parser

**Author:** Malcolm (Content Dev) · **Date:** 2026-06-09 · **Status:** ✅ Done

### What shipped
Built `xxd` in Go under `phase-02-core-unix/xxd/`: a hex dumper plus a reverse parser, byte-for-byte compatible with system `xxd` for the supported flags. Clean file split — `main.go` (CLI + flag parsing), `dump.go` (forward engine), `reverse.go` (reverse engine).

Flags: `-l` (length), `-c` (cols), `-s` (seek), `-g` (group), `-r` (reverse).

### Learnings worth keeping
- **xxd line layout, decoded precisely.** `OFFSET: ` then, for each of `cols` slots, a single space at every group boundary plus 2 hex chars (or 2 spaces when the slot is empty), then a **2-space separator**, then the ASCII gutter. The one-vs-two distinction is the key to a robust reverse parser.
- **`-g 0` means "one group of `cols` bytes"** (no internal spaces) — mirror BSD/GNU.
- **Reverse parsing strategy:** read the offset before `:`, read hex up to the first double-space run, ignore the ASCII gutter entirely (hex is the source of truth), pad zero bytes to honour offset gaps.
- **Binary-safe forward path:** read with `io.ReadFull` into a byte buffer, never decode runes. `-s` skip uses `io.CopyN(io.Discard, ...)` so it works on pipes/stdin where Seek is illegal.
- **Verification gotcha:** `diff <(xxd) <(./xxd)` with a single upstream pipe is a RACE — both process substitutions read the same stdin and one gets nothing. Compare via a temp file or capture each to a variable instead.

### Verification
`go test ./...` ✅ · `go vet ./...` ✅ · forward output == system `xxd` ✅ · `-r` round-trips all 256 byte values across multiple `-c`/`-g` configs ✅.

---

## Review Verdict — Phase 2 Wave 2 (sort, grep, sed, diff, xxd)

**Reviewer:** Ellie · **Date:** 2026-06-09 · **Verdict:** ✅ ALL FIVE APPROVED — Phase 2 complete.

Every tool: `go vet ./...` clean, `go test ./...` green, and behaviour spot-checked against the system binary. READMEs clear the teaching gate (all sections, Go idioms explained for a Python dev, primer linked).

### sort — ✅ APPROVED
- **External merge sort is real and works.** split → sort runs → k-way merge via `container/heap`, peak memory O(#runs).
- `-n/-r/-u/-f` and key fields (`-k`, `-t`) verified. Numeric `-n` matches system sort.
- Comparator isolated so both paths share identical ordering. Exit codes 0/1/2.

### grep — ✅ APPROVED
- Regex via RE2 (explained in README, incl. the no-backtracking tradeoff).
- `-i/-v/-n/-c/-w` all spot-checked correct; `-w` compiles `\b(?:…)\b`.
- Recursive walk via `filepath.WalkDir` (skips non-regular files); `-r` over a nested dir verified.
- `-A/-B/-C` context with span-merging + `--` block separators; matches GNU shape.
- Exit codes verified live: **0** match, **1** no match, **2** bad pattern / dir without -r.

### sed — ✅ APPROVED
- Clean parser → command-list → executor model (internal/sed package).
- `s///` with `g`/`i`/`p` flags, `\1..\9` backrefs and `&` whole-match — all verified.
- Addressing: number, `$`, `/regex/`, and ranges with a correct range state machine; `-n` suppress; `-i` in-place. All spot-checked against system sed.

### diff — ✅ APPROVED
- **LCS dynamic programming implemented from scratch** (lcs.go), backtracked into an edit script (editscript.go) with GNU's delete-before-insert bias.
- Unified `@@` hunks **byte-identical to `diff -u`** on change, pure-insert, and pure-delete cases; normal format matches too.
- Context window merging matches GNU. Exit codes verified: **0** identical, **1** differ, **2** trouble.

### xxd — ✅ APPROVED
- Forward dump **byte-identical to system xxd** (default and `-c 8`).
- `-r` reverse **round-trips binary** (500 random bytes) exactly.
- Binary-safe I/O throughout (`io.ReadFull`, `io.CopyN` for `-s`). Exit codes 0/1/2.
