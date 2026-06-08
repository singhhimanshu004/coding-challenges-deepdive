# Orchestration Log — Malcolm — Phase 2 Wave 1

**Date:** 2026-06-08T21:13:01Z  
**Agent:** Malcolm (Content Developer)  
**Task:** Build Phase 2 Wave 1: wc, cat, head, cut, uniq, tr (Go)

## Summary

Parallel build of six Core Unix text-processing tools in Go. All tools follow the established Phase 2 pattern: `main` → `run()` with injected streams, hand-rolled flags for authentic Unix ergonomics, exit codes 0/1/2, README-first teaching with Python analogies.

## Tools Built

1. **wc** (Challenge 5) — Byte/line/word/rune counter
2. **cat** (Challenge 6) — Concatenate/stream-forward with optional numbering/visibility flags
3. **head** (Challenge 7) — First N lines/bytes with early termination
4. **cut** (Challenge 8) — Column/character selection with range grammar
5. **uniq** (Challenge 9) — Adjacent-duplicate collapse with run-length counting
6. **tr** (Challenge 10) — Transliterate/delete/squeeze with SET expansion

## Key Learnings Locked In

- Pure logic + injectable streams: `count/cut/uniq/tr` logic takes `io.Reader`/`io.Writer`, CLI delegates via `run()`, tests feed buffers.
- Hand-rolled flag parsing beats stdlib `flag` for Unix authenticity (bundled shorts, glued values, `--` terminator, `-` = stdin).
- **Bytes vs. runes matters** — `cut -c`, `tr` on multi-byte characters requires `[]rune` conversion; Go's byte-default indexing is the gotcha.
- **Platform differences** — GNU vs. BSD formatting (e.g., uniq `-c` width, cat `-n` numbering scope) require intentional choices; ours are documented.
- **Stream shape preserved** — early termination (`head`), one-line-of-state (`uniq`), stateless filter (`tr` translate/delete) are all teachable via the core algorithm.

## QA Passed

- **Ellie review (2026-06-09):** All six tools ✅ APPROVED. `go vet`/`go test` clean. Differential-tested byte-for-byte against system tools.
- **Staged:** Each tool directory under `phase-02-core-unix/{name}/` with `.gitignore` (no binaries), `go.mod`, `*.go` tests, README.
- **CURRICULUM.md updated:** Challenges 5–10 checkboxes flipped to `[x]`.

## Files Created/Modified

- `phase-02-core-unix/wc/` — ✅
- `phase-02-core-unix/cat/` — ✅
- `phase-02-core-unix/head/` — ✅
- `phase-02-core-unix/cut/` — ✅
- `phase-02-core-unix/uniq/` — ✅
- `phase-02-core-unix/tr/` — ✅
- `.squad/decisions.md` — Phase 2 Wave 1 section merged
- `.squad/agents/malcolm/history.md` — Learnings appended

## Next Steps

Phase 2 Wave 2: sort, grep, sed, diff, xxd. Already scaffolded (empty stubs); ready for Wave 2 build.
