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
