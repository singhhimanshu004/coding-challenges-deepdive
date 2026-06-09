# Decisions

- Using claude-opus-4.6-1m for all agents per user directive.
- Source material: codingchallenges.fyi — 65+ challenges covering CLI tools, networking, servers, data processing, applications, games, and developer tools.
- Multi-language approach: choose the best language per challenge (Go for CLI/networking, Python for data, TypeScript for web, etc.)
- **README-first learning mandate (user directive):** Every challenge MUST include a comprehensive README.md that explains the concept, how it works in the real world, and a step-by-step walkthrough of the implementation. The goal is actual learning — not just code. Structure: What We're Building → Core Concepts → Architecture → Step-by-Step Implementation → Testing → Key Takeaways → Further Reading.

## Phase 1, Challenge 4: QR Code Generator (Python) — ✅ APPROVED

### Python layout conventions (reaffirmed & reusable)
- Package named after the tool (`qrgen/`), one module per pipeline stage
- `__main__.py` for `python -m <tool>`
- `tests/` package with `pytest.ini` configuration
- `.venv/` and generated artifacts (`*.png`) in `.gitignore`
- `requirements.txt` for dependencies

### Encoder validation methodology
- **Validate with published reference vectors**, not just round-trips
- Reed–Solomon: Wikiversity "Reed–Solomon codes for coders" vector
- QR data codewords: Thonky "HELLO WORLD" V1-Q
- BCH format info: published format-string table
- Reference vectors are decoder-independent and pinpoint failure stages

### Decoder preference for QR
- **Preferred:** `pyzbar` (zbar) — reliably reads small/dense symbols
- **Fallback only:** OpenCV's `QRCodeDetector` — flaky on tiny version-1 symbols
- Make round-trip tests skip cleanly when decoder is unavailable

### macOS native library configuration
- `pip install pyzbar` is insufficient on macOS; also require `brew install zbar`
- Pattern: `tests/conftest.py` prepends common lib dirs to `DYLD_LIBRARY_PATH`/`DYLD_FALLBACK_LIBRARY_PATH`/`LD_LIBRARY_PATH` before decoder import
- Linux: `apt install libzbar0`

### Rendering vs. encoding boundary
- Pillow used for pixel output only — encoding is hand-rolled from scratch
- Keep this distinction clear in challenges with a "build it yourself" mandate
- State explicitly in the README's design section

### Reusable finite-field building blocks
- GF(256) module (log/antilog tables, mul/div/inverse) reusable for future Reed–Solomon/CRC/BCH work
- Shift-register polynomial division applicable to error-correction schemes
- BitBuffer (MSB-first packing) same primitive as Huffman bit-writer (Challenge 2)

## Phase 2, Wave 1: Core Unix Tools (Go) — ✅ APPROVED (Malcolm + Ellie review)

All six tools built in parallel, reviewed 2026-06-09 by Ellie, all **approved**.

### wc — ✅ APPROVED (Challenge 5)
- **What:** from-scratch Unix `wc` in Go at `phase-02-core-unix/wc/`
- **Flags:** `-c/-l/-w/-m` (+ long forms `--bytes/--lines/--words/--chars`), default = lines+words+bytes
- **Features:** stdin fallback when no file args (or `-`), multiple files with a `total` row, correct UTF-8 rune counting for `-m`, aligned columns, exit codes (0 ok / 1 unreadable file / 2 usage error)
- **Verified:** differential-tested against system `wc` — counts match for files, stdin pipes, multiple files, multibyte text, and empty input
- **Layout conventions (reusable for Phase 2):**
  - Flat well-named files — NO `internal/` package for small single-purpose tools. Files: `main.go` (CLI/orchestration), `count.go` (pure streaming counter), `count_test.go` + `run_test.go`, `go.mod`, `.gitignore`
  - `go.mod`: `module wc` / `go 1.22`
  - `.gitignore`: ignores compiled `/wc` binary, `*.test`, `*.out`, `.DS_Store`
- **Reusable patterns established:**
  1. **Pure-logic + injectable-streams split.** `count(io.Reader) (counts, error)` stays pure and streaming; `run(args, stdin, stdout, stderr) int` takes the three streams as `io.Reader`/`io.Writer` so tests assert on a `bytes.Buffer` with no subprocess. `main()` only calls `run` + `os.Exit`. Reuse this shape for every Phase 2 filter (cat, head, cut, uniq, tr…).
  2. **`bufio.Reader.ReadRune()` for byte-and-rune counting in one pass** — gives both the rune and its byte width, so `-c` and `-m` stay consistent on UTF-8.
  3. **Hand-rolled flag parser** (not stdlib `flag`) to get short-flag bundling (`-lw`), long flags, `--` terminator, and `-` = stdin. This is the canonical Unix-filter ergonomics bundle — reusable verbatim across the phase.
  4. **Exit-code convention reaffirmed:** 0 success / 1 domain (unreadable file, matches real `wc`) / 2 usage. Note: real `wc` uses 1 for file errors, which we follow (file-not-found is treated as a per-file domain failure that doesn't abort the remaining files).
  5. **README-first for a Python dev:** every README links the project Go primer `docs/go-quickstart.md` and includes 🐍 Python-analogy callouts. Keep this header block + analogy style as the Phase 2 README template.
- **Status:** ✅ Done

### cat — ✅ APPROVED (Challenge 6)
- **What:** concatenation/stream-forwarding tool; first Phase 2 tool to heavily comment code for Python learner
- **Flags:** `-n` (number all), `-b` (number non-blank, overrides `-n`), `-E` (show line ends as `$`)
- **Features:** stdin/`-` convention, multiple files, bundled short flags (`-nE`), long forms, `--` terminator
- **Implementation:**
  - Two paths in `catStream`: no-flag fast path uses `io.Copy` (binary-safe, flat memory); flag mode reads line-by-line with `bufio.Reader.ReadBytes('\n')`
  - Exit codes: 0 ok, 1 per-file read error (others still processed), 2 usage error
  - Same `main`→`run` + injected-streams pattern as `wc`, for testability without temp files
- **Platform note:** GNU `cat -n`/`-b` numbers continuously across files; BSD/macOS resets per file. We chose GNU (the challenge reference). Manual `diff` parity checks against macOS `cat` will differ on multi-file numbering and on `-E` (BSD uses `-e`) — expected, documented in README.
- **Verified:** `go test ./...` and `go vet ./...` pass; output diffed against system `cat` for raw concat, single-file `-n`/`-b`, and stdin via `-`
- **Status:** ✅ Done

### head — ✅ APPROVED (Challenge 7)
- **What:** reads first N lines or bytes; from-scratch Go clone, third Phase 2 tool to lock in reusable pattern
- **Flags:** `-n N` (first N lines, default 10), `-c N` (first N bytes)
- **Features:** file arguments or stdin, prints `==> name <==` headers for multiple files (blank line before each header except the first), sensible exit codes (0 ok, 1 file error, 2 usage error)
- **Key insight (the headline lesson):** Early termination is the real story — the line loop returns the instant it has emitted N lines, so it's instant on huge files. Made this the README headline.
- **Output accuracy:** Keep trailing `\n` on each line and print unterminated final line verbatim. This is what makes `diff <(./head ...) <(head ...)` differential test pass cleanly — strongest correctness signal.
- **Flag parsing:** Hand-rolled (not stdlib `flag`) for authentic Unix ergonomics: accepts glued values (`-n5`), stops treating args as flags after the first filename.
- **Convention:** `run([]string) int` + tiny `main` — now settled across all Go challenges (matches Phase 1 bloom/huffman). Tests drive `run()` directly and capture `os.Stdout`/`os.Stdin` via `os.Pipe`.
- **Verified:** `go test ./...` and `go vet ./...` pass; byte-for-byte `diff` against system `head` for `-n`, `-c`, stdin, multi-file cases all identical
- **Go idiom notes:**
  - `defer f.Close()` is **function-scoped, not block-scoped** — deferring inside a file loop leaks descriptors until the function returns. We close explicitly per file. This is the biggest mental-model gap coming from Python's `with`.
  - `io.EOF` is a normal "stream finished" value, not an error to surface.
  - `bufio.Reader.ReadBytes('\n')` is the line-streaming workhorse.
- **Status:** ✅ Done

### cut — ✅ APPROVED (Challenge 8)
- **What:** from-scratch Go clone of Unix `cut` in `phase-02-core-unix/cut/`
- **Flags:** `-f` fields, `-c` characters, `-d` delimiter (default TAB), `-s` suppress
- **Features:** reads file arguments or stdin (`-` also means stdin), streams line by line, 1-based positions, rejects 0/negatives/decreasing ranges (`3-1`)
- **Key design decision:** Factor the LIST/range parser into its own type. A `Selector` (slice of `{lo, hi}` ranges, `hi == 0` = open-ended) parsed once and queried with a `contains(position)` method. `-f` and `-c` then share the exact same selection semantics — only what they slice (fields vs. runes) differs.
- **Semantics preserved from real `cut`:**
  - Membership test, not index expansion. Real `cut` emits columns in *input* order and collapses duplicates (`cut -f3,1` → field 1 then 3; `-f1,1` → once). Walking the line's columns and asking `contains()` gives both behaviours for free, no sorting/dedup needed. Easy to get wrong if you expand the spec into an ordered index list.
  - Bytes vs. runes matters for `-c`. Convert each line to `[]rune` so `-c` counts characters, not bytes (`cut -c1-2` on `héllo` → `hé`). Go strings index by byte by default; this is a recurring Go gotcha for a Python dev where `str` already indexes by code point.
  - A delimiter-less line is printed unchanged by default; `-s` drops it.
  - `-d`/`-s` are only valid with `-f`; `-d` must be exactly one character.
- **Hand-rolled flag parser:** Go's stdlib `flag` can't do attached short flags (`-f1,3`, `-d,`), which real users type constantly. A small manual loop that supports both attached and separated forms is worth it for a faithful clone.
- **Layout:** Module named after the tool (`module cut`), `go 1.22`, three small single-responsibility files (`ranges.go` parse, `cut.go` engine, `main.go` CLI), exit codes: 0 success, 1 domain failure (file open/read), 2 usage error (bad flags/LIST). Engine takes `io.Reader`/`io.Writer`, so tests feed strings and assert on buffers — no temp files.
- **Verified:** `go test ./...` and `go vet ./...` pass; output diffed byte-for-byte against system `cut` on real TSV/CSV input
- **Status:** ✅ Done

### uniq — ✅ APPROVED (Challenge 9)
- **What:** streaming `uniq` in Go under `phase-02-core-unix/uniq/`: collapses adjacent duplicate lines
- **Flags:** `-c` (count), `-d` (duplicated only), `-u` (unique only)
- **Features:** optional input/output file arguments, stdin/stdout fallback, clean "one line of state" run-length streamer
- **Key insight (the headline lesson):** **"uniq only compares adjacent lines."** Everything else — why you `sort` first, why memory stays tiny — falls out of that one fact. The algorithm: remember `prev` + `count`, emit a group when a different line arrives. One-line-deep memory is *the reason* it's adjacent-only — contrast with a `seen set` (that would be `sort -u`, unbounded memory).
- **Teaching angle:** Framed for beginner in Go, Python analogies: `defer` ≈ `with open`, `bufio.Scanner` ≈ `for line in file`, struct-of-bools ≈ `@dataclass`, `io.Reader/Writer` ≈ duck typing checked at compile time. Linked `docs/go-quickstart.md` top.
- **Platform note:** BSD/macOS uses a **4-wide** right-justified count (`%4d`); GNU/Linux uses **7-wide** (`%7d`). We matched local macOS system so `diff <(./uniq -c) <(uniq -c)` is clean. Documented the one-character flip for GNU boxes in README's testing section. Future Unix-tool challenges that mimic system output should expect BSD-vs-GNU formatting divergences and pick/document one.
- **Same layout as Phase-1 Go challenges:** `module uniq`, `go 1.22`, thin `main()` delegating to `run(args) int` for testability, exit codes: 0 success / 2 usage+IO error.
- **Verified:** `go test ./...` and `go vet ./...` pass; differential-tested against system tool: plain, `-c`, `-d`, `-u`, stdin piping, and `sort file | uniq -c` pipeline all match byte-for-byte
- **Minor:** missing input file exits 2 (repo "usage/IO" convention) where GNU `uniq` uses 1. Documented; harmless.
- **Status:** ✅ Done

### tr — ✅ APPROVED (Challenge 10)
- **What:** streaming `tr` in Go under `phase-02-core-unix/tr/`: translates SET1→SET2, deletes (`-d`), squeezes repeats (`-s`), complements (`-c`), full combination support (`-ds`, `-cd`, `-cs`, …)
- **SET features:** ranges (`a-z`), POSIX classes (`[:alpha:] [:digit:] [:space:] [:upper:] [:lower:]`, plus `alnum`, `blank`), backslash escapes
- **Nature:** Pure stdin→stdout filter — no file args. Files: `README.md`, `main.go` (CLI), `internal/translate/set.go` (SET expander), `internal/translate/translate.go` (engine), `translate_test.go`, `go.mod`, `.gitignore`.
- **Key insights (the teaching angle):**
  1. **`tr` is a pure filter** — takes *no file arguments*, only `stdin → transform → stdout`. This is the cleanest illustration of the Unix pipe-and-filter philosophy in the whole phase. LED WITH THIS.
  2. **Operate on runes, not bytes.** Unicode correctness is a first-class topic (`é`/`λ` are multi-byte). Made Unicode correctness a first-class topic and tied it to Go's `[]rune` and `bufio.ReadRune`. Backed with multibyte translate *and* delete tests.
  3. **Framed the four modes by state needed:** translate/delete are stateless (a rune's fate depends only on itself); **squeeze is the odd one**, needing exactly one remembered rune (`lastEmitted`). That single insight explains why squeeze runs on the *output* alphabet (SET2) after translation.
- **Implementation details:**
  - Spec→Transformer compile step; rune-based SET expansion; correct squeeze-set selection per mode; `-c` complement for both delete and translate.
  - **SET2 padding:** when SET2 is shorter than SET1, the last rune repeats (BSD/macOS behaviour). Did *not* implement GNU's explicit `[c*]` /`[c*n]` repeat syntax — noted in README. Covers common cases.
  - **Complement is computed at runtime, not precomputed.** `-c` makes the matched set "every rune NOT in SET1," which is unbounded, so you can't materialise a map. Membership is flipped on the fly in `inDeleteSet`/`translateRune`/`inSqueezeSet`. Future complement-based tools should do the same.
  - **Squeeze set depends on mode:** delete+squeeze and translate+squeeze squeeze SET2; squeeze-only squeezes SET1 (with `-c` applied). Easy to get wrong.
- **Layout:** Same as Phase-1 Go challenges: `module tr`, `go 1.22`, thin `main()` delegating to `run(args, stdin, stdout) int` for testability. Three-way exit-code convention: 0 success, 1 domain/IO failure mid-stream, 2 usage error. Deliberately split `translate.New` (validate/compile → exit 2 before touching stream) from `Run` (execute → exit 1 on stream error) to make that mapping clean.
- **Verified:** `go vet ./...` clean; `go test ./...` passes (20+ cases incl. empty input); differential-tested against system `/usr/bin/tr` for 12 cases (lower→upper, positional translate, delete digits, squeeze, targeted squeeze, `-cd`, `-c` translate, `[:upper:]`→`[:lower:]`, translate+squeeze, short-SET2 padding, range mapping, space-class delete) — all matched byte-for-byte
- **Nice-to-have:** add CLI-layer tests for `main.go` flag parsing (translate engine thoroughly covered; this is round-out). Non-blocking.
- **Status:** ✅ Done

## Phase 3, Wave 2: curl + Shell capstone (Go) — ✅ APPROVED (Phase 3 complete)

### Challenge 21 — curl — ✅ APPROVED

**What:** Raw-socket HTTP/1.1 client in Go at `phase-03-advanced-cli/curl/` — request framed by hand, response parsed by hand (including chunked decoding). NOT net/http for the protocol.

**Implementation:**
- **Raw TCP + TLS:** `net.Dial` opens the byte-pipe; `crypto/tls.Client` for https (stdlib handshake, everything above TLS is hand-rolled).
- **Flags:** `-X METHOD`, `-H 'Name: val'` (repeatable, overrides defaults), `-d DATA` (→ POST + Content-Length), `-o FILE`, `-I` (HEAD/headers-only), `-v` (verbose `>`/`<` to stderr), `-L` (follow 3xx redirects, capped 10).
- **Body framing — two schemes:** `Content-Length` (exact read via `io.ReadFull`) AND `Transfer-Encoding: chunked` (hand-written decoder: hex sizes, `;ext` stripped, per-chunk CRLF consumed, 0-chunk + trailer drained), with read-to-EOF fallback (valid because we send `Connection: close`).
- **File split:** `url.go` (parse + redirect resolution), `conn.go` (dial/TLS), `request.go` (framer), `response.go` (parser + chunked decoder), `main.go` (CLI/redirect loop).

**Reusable conventions:**
- Same Go layout: `module curl` / `go 1.22`, thin `main()` → testable `run(args, stdout, stderr) int`; flat well-named files, no `internal/`; `.gitignore` ignores `/curl`, `*.test`, `*.out`, `.DS_Store`.
- Hand-rolled flag parser (not stdlib `flag`) for authentic curl ergonomics (short flags, repeatable `-H`).
- Three-way exit codes: 0 success / 1 runtime / 2 usage.
- Dependency-injected I/O for tests: parser takes `*bufio.Reader`; `run` takes output streams. Tests use `strings.Reader` fixtures + local `net.Listener` — zero internet dependency.
- README-first: links `docs/go-quickstart.md`, byte-by-byte annotated request/response, ASCII chunked-format diagram, TLS in one paragraph.

**Teaching angles:**
1. **"A socket is just a byte pipe; HTTP is just text on it."** Demystifies networking.
2. **`\r\n` everywhere + blank line = end of headers.** The two beginner mistakes.
3. **Two body-framing schemes, not one.** Chunked decoder is the star: sizes are HEX, `0`-chunk terminates, each chunk's data has trailing CRLF (off-by-two bug magnet).
4. **TLS as a clean wrapper** — HTTP code is byte-identical for http/https.

**⚠️ Toolchain note (repo-wide for Phase 4+):**
- On macOS, **`go test ./...` aborts with `dyld: missing LC_UUID` error** for packages importing `net`/`crypto/tls` (cgo system resolver → external linker mismatch with Xcode CLT). NOT a code bug.
- **Fix: `CGO_ENABLED=0 go test ./...`** — pure-Go linker + native resolver. `go vet` and `go build` are fine either way.
- Future networking challenges (Phase 4+) will hit this — **default to `CGO_ENABLED=0` for test runs.** Documented in curl README; applies to web server, proxy, etc.

**Verification:** `go vet` clean; `CGO_ENABLED=0 go test ./...` — 30 pass (unit + e2e over local `net.Listener`). Live network: `-I http://example.com` → 200; `-v https://example.com` → TLS + chunked decoded; `-L http://github.com` → http→https redirect followed; `-o file` saved body.

**Non-blocking nice-to-haves:** `--data @file` / form encoding, connection reuse, progress meter. Out of scope.

**Status:** ✅ Done

### Challenge 22 — Shell (gosh) — ✅ APPROVED (Phase 3 capstone)

**What:** Working interactive Unix shell (`gosh`) in Go at `phase-03-advanced-cli/shell/` — tokenizer → recursive-descent parser → pipeline AST → fork/exec executor wiring real pipes/redirects. The orchestrator that runs every other Phase 2 tool.

**Implementation:**
- **Three-stage pipeline:** `lexer.go` (tokenize, quote/escape aware) → `parser.go` (recursive-descent → AST) → `executor.go` (fork/exec + fd wiring). One file per stage.
- **Features:** quotes (single/double) + backslash escapes; pipelines `a | b | c`; redirections `>` `>>` `<` `2>`; sequencing `;`; logical `&&`/`||` short-circuit; env expansion `$VAR`/`${VAR}`/`$?`/`$$`; builtins `cd`/`pwd`/`exit`/`echo`/`export`/`type`; Ctrl-C interrupts child (not shell); interactive REPL + `-c "string"` + script-file modes.
- **AST shape:** `List(;) → AndOr(&&/||) → Pipeline(|) → Command(args+redirs)`. Grammar nesting encodes precedence (`;` loosest, redirs tightest).

**Hard-won reusable lessons (process-spawning challenges):**
1. **#1 pipeline bug: parent must CLOSE its pipe-fd copies after starting children.** Each `exec` dups the fd into child; if parent keeps write-end open reader never sees EOF and pipeline hangs forever. Explicit "ownership rule": every pipe-end used by exactly one stage; external stages → parent closes after `Start()`, builtin stages (goroutine) → goroutine closes its own. See `execMulti` + `parentCloses`.
2. **`cd` MUST be a builtin** — working directory is per-process state; child `cd` changes its own dir then exits, leaving parent unmoved. Same for `exit`/`export`/assignment. README has prominent section with the "why".
3. **fork/exec framed as two-step with a gap:** `cmd.Start()` ≈ fork, `cmd.Wait()` ≈ wait; gap is where you rewire fds via `cmd.Stdin/Stdout/Stderr =` assignment.
4. **Lexer gotcha:** unquoted chars must be COALESCED into one expandable word-part, else `$MYVAR` tokenizes separate `$`+`M`+`Y`+… and `$` never sees name. Store words as `[]wordPart{text, expand}` so quoting context (single='literal', double/unquoted='expandable') survives to expansion. Caught via failing `export`/`$?` test.
5. **Glued operators** (`2>file`, `a"b"c`): detect pending word of exactly `"2"` immediately before `>` to emit stderr-redirect token.
6. **Signals:** shell + foreground child share process group, both get SIGINT; shell installs handler that swallows it (reprint prompt) while child dies. Simple, matches bash feel.

**Reusable conventions:**
- Same Go layout: `module gosh` / `go 1.22`, thin `main.go` (mode select only), all logic in `internal/shell/` for testability without TTY. `Shell` holds `In io.Reader`, `Out`/`Err io.Writer` → tests wire `bytes.Buffer`, production wires os.Std*. Injected-streams pattern scaled perfectly to large program.
- **Init-cycle gotcha:** `var builtins = map{...}` literal whose funcs call `isBuiltin` (reads map) is compile-time cycle in Go. Fix: populate map in `init()` instead. Remember for any dispatch-table-with-self-reference pattern.
- README-first: 🐍→🐹 analogies (Popen≈Start, .wait≈Wait), ASCII fd diagram of `cmd1 | cmd2 > file`, links `docs/go-quickstart.md`. The "what does the pipe actually do under the hood" diagram is centerpiece.

**Teaching angles:**
1. **The orchestrator that runs every other tool.** Phase 3 capstone tying everything together.
2. **Pipe EOF hang trap and the parent-close ownership rule.** Critical for any multi-process code.
3. **Why `cd` must be a builtin.** Process state mutation insight.

**Verification:** `go vet` clean; `go test ./...` → 33 pass (tokenizer/parser/expand/executor/builtins + REAL execution). Manually: `cd`+`pwd`, `echo a b | cat | wc -w` → 2, redirect+readback, `false ; echo $?` → 1, `true ; echo $?` → 0, `type cd/ls`, export+expansion — all correct.

**Scope boundaries (documented in README):**
- No job control (`&`, `fg`/`bg`), no globbing, no command substitution `$(...)`, no here-docs.
- `2>>` treated as `2>` (overwrite) — deliberate, documented.
- Expansion does not re-split on spaces (one arg stays one arg).

**Non-blocking nice-to-haves:** `2>>` append mode, post-expansion word-splitting/globbing. Not blockers.

**Status:** ✅ Done
