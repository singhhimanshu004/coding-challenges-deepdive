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

## Phase 4 Wave 2: DNS Forwarder, Traceroute, HTTP Forward Proxy (Capstone) — ✅ ALL APPROVED

Completed 2026-06-13. Three Go networking challenges built by Malcolm, reviewed by Ellie — all approved. Phase 4 (Networking) **COMPLETE** — 7/7 challenges approved.

### Challenge 24: DNS Forwarder — ✅ APPROVED

**What:** Caching, forwarding DNS server at `phase-04-networking/dns-forwarder/`. Listens on UDP (default `:1053`), forwards client queries to upstream resolver (default `8.8.8.8:53`), relays answers, and caches each reply for its TTL. CLI: `--listen`, `--upstream`, `--verbose`.

**Design highlights:**
- Cache key is the full (QNAME, QTYPE, QCLASS) triple — not just the name (classic mistake; IPv4 answer to AAAA query is wrong).
- Minimum TTL across answer set used for expiry (answer is only as fresh as shortest-lived record).
- Concurrency-safe `sync.RWMutex` cache, one goroutine per request, copies datagram before handing to goroutine (UDP buffer reused on next read — gotcha).
- Patches transaction ID when serving from cache, or client rejects as unsolicited.

**Testing strategy (reusable for forward/relay components):**
- Fake upstream: local `net.ListenUDP` on 127.0.0.1 with atomic hit counter — proves caching, no internet needed.
- Injectable clock `cache.now func() time.Time` → TTL expiry test advances time instantly (no flaky sleeps).
- Table-driven TTL boundaries (fresh/just-before/exactly-at/after expiry/zero-TTL-never-cached).
- Tests: `TestForwardAndRelay` (first query → 1 hit), `TestSecondQueryServedFromCache` (still 1 hit, ID patched), `TestCacheExpiresAfterTTL` (past TTL → 2 hits).

**Defaults & docs:**
- Defaults to `:1053` (no root). README documents `:53` + `sudo` + Linux `setcap cap_net_bind_service`.

**Verification:** `go vet` clean; `CGO_ENABLED=0 go test ./...` all pass; no internet needed.

**Status:** ✅ Approved by Ellie 2026-06-13.

### Challenge 26: Traceroute — ✅ APPROVED

**What:** Unprivileged ICMP traceroute at `phase-04-networking/traceroute/` discovering network path hop by hop. No root, no raw sockets. CLI: `[--max-hops 30] [--probes 3] [--timeout 1s] [--resolve] <host>`.

**Teaching idea (headline):**
Repurpose IP TTL field as "reveal yourself" probe. Each router decrements TTL; when TTL hits 0 router drops packet and mails back ICMP "Time Exceeded" whose source address is that router. Sweeping TTL 1..30 forces every hop to announce itself in order.

**Technical decisions:**
- **Unprivileged ICMP via `icmp.ListenPacket("udp4", ...)`** from `golang.org/x/net/icmp` + per-packet TTL via `ipv4.PacketConn.SetTTL` (same as macOS `ping`).
- **Testability via seam:** Three pure, network-free pieces: build echo-request bytes, parse/classify reply (Time Exceeded vs Echo Reply vs Dest Unreachable), TTL iteration loop. Loop depends on `prober` interface → scripted fake drives tests offline, no root, fully deterministic. Live test self-skips on socket error (never fails suite).

**Reusable lessons for future Phase 4+ challenges:**
1. **`golang.org/x/net` versioning:** Latest x/net (v0.56) requires Go ≥ 1.25 (auto-downloads newer toolchain). Pinned `v0.31.0` + `GOTOOLCHAIN=local` to stay on repo's go1.22 baseline.
2. **macOS LC_UUID linker bug (recurring):** Plain `go test` aborts "missing LC_UUID" (importing `net` pulls cgo, linker mismatch with Xcode CLT). Fix: `CGO_ENABLED=0 go test ./...`.
3. **Unprivileged datagram ICMP carries junk `:0` port:** `*net.UDPAddr.String()` on datagram ICMP peer is "1.2.3.4:0" — strip port for display.
4. **Read deadlines, not per-call timeouts:** Go binds socket read via `SetReadDeadline(absolute-time)`, not timeout argument (Python analogue: `settimeout`). Reusable for every socket-based challenge.

**Verification:** `CGO_ENABLED=0 go vet ./...` + `CGO_ENABLED=0 go test ./...` pass; live sanity reached 8.8.8.8 at hop 8 with sensible per-hop RTTs.

**Scope (documented in README):** ICMP-probe traceroute only (like Windows `tracert`); UDP variant explained but not implemented. IPv4 only. Reverse-DNS opt-in (`--resolve`), best-effort.

**Status:** ✅ Approved by Ellie 2026-06-13.

### Challenge 29: HTTP Forward Proxy (Phase 4 capstone) — ✅ APPROVED

**What:** Forward proxy in Go at `phase-04-networking/http-forward-proxy/` listening on TCP (`:8080` default), one goroutine per client. Handles:
- **Plain HTTP** — parses absolute-form request, rewrites to origin-form, strips hop-by-hop headers, forwards, relays response.
- **HTTPS via CONNECT** — dials origin, replies `200 Connection Established`, relays raw bytes bidirectionally so client's TLS handshake passes through opaquely.

**Why it's a capstone:**
Literally reuses prior Phase 4 lessons. CONNECT tunnel = netcat bidirectional byte relay with two sockets. Plain-HTTP rewriting builds on curl's "HTTP is just text on a socket". README explicitly calls both out so learner sees the arc.

**Teaching angle (headline):**
**TLS opacity** — after `200 Connection Established` client and origin negotiate session keys the proxy never sees. Proxy *cannot* read/alter HTTPS traffic. Only way to "see inside" is TLS interception with forged cert (mitmproxy / corporate MITM), which is exactly why HTTPS makes that refusable. Made this the #1 Key Takeaway.

**Implementation details:**
- **Parsing:** `http.ReadRequest` (parsing already taught by curl, not the lesson here). Request rewriting (absolute→origin), hop-by-hop stripping, CONNECT tunnel all hand-rolled.
- **Request rewriting:** `req.URL.RequestURI()` converts URL→path (the thing that makes a proxy a proxy).
- **`Connection: close` simplification:** Forces it on origin request → response ends at EOF → proxy never parses response, just `io.Copy` relay. Good teaching simplification; production would loop for keep-alive.
- **Read client tunnel side through `bufio.Reader`** (not raw conn) so bytes pipelined right after CONNECT aren't dropped.

**Testing (fully self-contained, no internet):**
- **Plain HTTP:** `httptest.NewServer` origin + `http.Client` with `Transport.Proxy` set to our proxy. Origin asserts it never sees absolute-form (proves rewrite). Table-driven over root/nested/query paths.
- **CONNECT:** `httptest.NewTLSServer` + transport trusting test cert AND using proxy. TLS handshake completing through tunnel is itself proof relay is byte-accurate.
- **Raw-socket CONNECT test:** Hand-writes CONNECT line, runs TLS handshake manually, shows wire steps with nothing hidden.
- **Helper unit tests:** Two pure helpers tested in isolation.

**Toolchain (same as curl & all Phase 4 Go challenges):**
macOS: plain `go test` aborts `missing LC_UUID` (cgo linker quirk). Fix: `CGO_ENABLED=0 go test ./...`.

**Verification:** `go vet ./...` clean; `CGO_ENABLED=0 go test ./...` all pass; `CGO_ENABLED=0 go build .` succeeds.

**Status:** ✅ Approved by Ellie 2026-06-13. **Phase 4 Networking COMPLETE — 7/7 challenges approved.**

---

## Overall: Phase 4 Wave 2 Review Summary

**Date:** 2026-06-13
**Reviewer:** Ellie
**Scope:** dns-forwarder (#24), traceroute (#26), http-forward-proxy (#29, capstone)
**Method:** `go vet ./...` + `CGO_ENABLED=0 go test -count=1 -v ./...` + source review + README quality gate

**All three ✅ APPROVED.**

### README Quality Gate (all three pass — 7 mandatory sections + Go idioms)

All three READMEs include: What We're Building → Core Concepts → Architecture → Step-by-Step → Testing → Key Takeaways → Further Reading. Explain Go idioms for Python dev (🐍→🐹: iota enums, implicit interface satisfaction, goroutine-per-connection, RWMutex, read deadlines, blank-assignment interface check). Link `docs/go-quickstart.md`. Document `CGO_ENABLED=0` toolchain workaround.

### Non-blocking nice-to-haves

**traceroute:** `TestTraceIntegration` gated only by `testing.Short()` so plain `go test ./...` makes live call to 8.8.8.8. Self-skips on socket error, cannot fail suite, but gating behind env var (or default-skip) would make default run fully hermetic. Not a blocker.

**Overall:** Phase 4 (Networking) is **COMPLETE** — every challenge in the phase is approved. 25/64 overall challenges done (Phase 1–3 complete, Phase 4 complete, Phase 5–8 pending).

---

## Phase 5, Wave 1 Review: Four Go Server Challenges

**Date:** 2026-06-13
**Scope:** web-server (#30), memcached-server (#33), nats-message-broker (#34), rate-limiter (#35)
**Builder:** Malcolm (Content Dev)
**Reviewer:** Ellie
**Verification:** `go vet ./...` + `CGO_ENABLED=0 go test ./...` + source review + README quality gate

**All four ✅ APPROVED**

### Challenge 30: Web Server — ✅ APPROVED

**What:** HTTP/1.1 web server over raw TCP at `phase-05-servers-infrastructure/web-server/`. Deliberate choice to NOT use `net/http` on the serving path — instead `net.Listen`/`net.Conn` with hand-rolled request parsing and response framing. Routes `method+path` to handlers, serves static files (Content-Type by extension), defends against path traversal with two-layer defense (lexical `filepath.Clean` + resolved-absolute containment check), and supports HTTP/1.1 keep-alive with per-request read deadlines. CLI: `web-server [--addr :8080] [--root ./public] [--verbose]`.

**Key decisions (reusable pattern):**
- **`serve(ln net.Listener)` split from `listenAndServe()`** — enables testability: unit tests bind `127.0.0.1:0`, read `ln.Addr()`, drive real requests through the server. No fixed ports, no network flakiness. Reusable for Redis, load balancer.
- **Keep-alive in per-connection loop** — each `handleConn` arms read deadline, parses, routes, writes response + `Connection` header, loops or returns. Clean EOF/timeout on kept-alive connection = normal, not logged as error (important for noise-free verbose logs).
- **HTTP/1.0-vs-1.1 keep-alive default flip** — 1.0 closes by default, 1.1 keeps alive by default. Encoded in `request.wantsKeepAlive()`. Made this the single most-called-out protocol detail with a README table + dedicated test cases.
- **Path traversal = two layers** — lexical `filepath.Clean` AND resolved-absolute containment check against root (with trailing separator to prevent `/srv/www-evil` prefix-bypass). Layer-1 alone is defeatable by symlinks; test sends raw `/../secret.txt` over socket (well-behaved client normalizes `..` away before wire, so this subtlety is a teaching point).
- **Content-Length frames the body** — announcing body length is precisely what lets the client find the end without connection close, which enables persistent connections. This is the throughline of HTTP/1.1 framing.
- **Read timeout (`SetReadDeadline`) before every request** — stops idle/slow clients pinning goroutines (slow-loris defense). Also caps total header bytes.

**Testing (fully self-contained):**
- 10 tests: table-driven parsing, static serve, 404, path-traversal rejection, keep-alive reuse (2 sequential requests over the same socket via `http.ReadResponse`), Connection: close, dynamic route, 405 (method exists for another verb).
- Tests run on `127.0.0.1:0` (OS picks free port). Raw `/../secret.txt` over socket proves symlink-aware defense.

**Toolchain:** macOS LC_UUID linker bug (importing `net` pulls cgo). Fix: `CGO_ENABLED=0 go test ./...`. Documented in README exactly as curl does.

**Verification:** `go vet ./...` clean; `CGO_ENABLED=0 go test ./...` all pass; manual live run confirmed `/` (200), `/css/style.css` (200, HEAD drops body), `/hello` (dynamic), 404, rejected traversal, 2 requests over single keep-alive.

**Scope limitation:** Content-Length request bodies only (no chunked). Most clients use Content-Length; documented in README as deliberate scope cut. Load Balancer (#31) can sit in front.

**Status:** ✅ Approved.

### Challenge 33: Memcached Server — ✅ APPROVED

**What:** Memcached TEXT protocol server over TCP at `phase-05-servers-infrastructure/memcached-server/`. Commands: set, add, replace, append, prepend, cas, get, gets, delete, incr, decr, flush_all, version, quit. In-memory store with per-item expiry (lazy eviction on access), 32-bit FLAGS field, CAS monotonic version token, goroutine-per-connection concurrency, and O(1) LRU eviction via map + `container/list` under configurable `--max-items` cap. CLI: `memcached [--addr :11211] [--max-items 1000] [--verbose]`.

**Key decisions (reusable):**
- **Two framing styles in one protocol** — headline teaching point: command lines are delimiter-framed (CRLF), values are length-prefixed (`<bytes>` then exactly that many raw bytes via `io.ReadFull`, then framing CRLF NOT counted). This lets the cache hold opaque binary blobs. Mirrors HTTP Content-Length lesson from curl; cross-linked in README.
- **Injectable clock** — `Store.now func() time.Time` (default `time.Now`) so expiry tests are deterministic, instant, no `time.Sleep`, no flakiness. Same dependency-injection spirit as curl's injected I/O.
- **LRU = map + `container/list`** — front=hot, back=cold. Touch-on-access moves to front; insert-past-cap evicts back. Hand-built version of Python's `functools.lru_cache`/`OrderedDict.move_to_end` — called out for cross-language learners.
- **Single mutex** for clarity. README notes production would shard the map to reduce contention.
- **Slab allocation** (real memcached): explained conceptually (1 MB pages → fixed-size chunks, ~1.25× growth, per-class LRU, kills fragmentation) and contrasted with simpler Go-heap approach, as the task requested.
- **Redis contrast table** — blob cache + slabs (memcached) vs. rich-type store + persistence (Redis), since Redis is a separate Phase 5 challenge.
- **CLI:** stdlib `flag` for long flags (`--addr`, `--max-items`, `--verbose`). Short-flag hand-rolling reserved for Unix-filter challenges with exotic bundling.

**Module structure (reusable Phase 5 pattern):**
- Named after tool (`module memcached`), `go 1.22`, flat well-named files, thin `main()` → testable `run()`.
- `.gitignore`: binary, `*.test`, `*.out`, `.DS_Store`.
- README: 7 sections, links `docs/go-quickstart.md`, 🐍 Python-dev callouts (net.Listener, bufio, container/list, sync.Mutex, goroutines, type assertions).

**Testing:**
- 24 tests: store unit tests + end-to-end over real local TCP socket.
- `io.ReadFull` behavior (single `Read` can short-read) called out as key idiom.
- Injectable clock verifies instant expiry (no sleep).

**Toolchain:** Same macOS LC_UUID issue; `CGO_ENABLED=0 go test ./...` required. Documented in README.

**Verification:** `go vet ./...` clean; `CGO_ENABLED=0 go test ./...` PASS; live smoke test via `nc`: set/get, incr on non-numeric → CLIENT_ERROR, LRU eviction at cap all work.

**Status:** ✅ Approved.

### Challenge 34: NATS Message Broker — ✅ APPROVED

**What:** NATS-style pub/sub message broker over TCP at `phase-05-servers-infrastructure/nats-message-broker/`. Core NATS text protocol: CONNECT/PING/PONG/PUB/SUB/UNSUB/INFO/MSG/+OK/-ERR. Subject routing with `*` (single-token) and `>` (tail, must be last, ≥1 token consumed) wildcards. Queue-group load balancing (plain subs all receive, queue-group members each receive exactly once via round-robin). Per-client goroutine + dedicated write loop for safe serialization. No external dependencies (real `nats.go` deliberately not imported). CLI: `nats-broker [--addr :4222] [--verbose]`.

**Key decisions (reusable):**
- **Two framing styles in one protocol** — headline: control lines newline-delimited; payloads length-prefixed (binary data can contain newlines). Single idea beginners most need to internalize. Same lesson as memcached + curl.
- **Subject matching = core** — isolated in `subject.go` with table-driven test. Wildcard rules crisp and verifiable (`*` = one token, `>` = tail + must-be-last + ≥1 token; so `foo.>` does NOT match `foo`).
- **Fan-out vs queue groups** — plain subs all get copy; queue-group members each get exactly one (fan-in). Both can coexist for same subject. Made both clear in tests + README.
- **At-most-once, no persistence** — dropping frame to slow/gone consumer = feature, not error. Implemented via `select` on quit channel in `enqueue` — at-most-once semantics.
- **Registry = `map[*client]map[sid]*subscription`** — O(1) disconnect, O(n) routing scan. README notes production NATS uses subject trie. Flat scan is right call for learning.
- **Queue-group selection round-robin** — via server counter. Map iteration order non-deterministic, but "exactly one" always holds.
- **`--verbose` flag** gates `+OK`. Honors `verbose` in CONNECT JSON via substring check (not full parse — adequate for core). Simplification noted in README.

**Go idioms for Python/Java dev:**
- Goroutine per connection (cheap threads); channel + dedicated writer goroutine per client for serialized socket writes.
- `bufio.Reader.ReadString('\n')` for line framing, `io.ReadFull` + `Discard(2)` for length-prefixed payload.
- README links `docs/go-quickstart.md`, sprinkles 🐍 callouts.

**Testing:**
- Table-driven `subject.go` tests: wildcard match/non-match, `>` tail semantics.
- Integration tests: queue-group single delivery, unsub stops delivery, broker bound on `127.0.0.1:0`.

**Toolchain:** Same macOS LC_UUID issue; `CGO_ENABLED=0 go test ./...` required. Documented in README.

**Verification:** `go vet ./...` clean; `CGO_ENABLED=0 go test ./...` PASS; `CGO_ENABLED=0 go build` succeeds.

**Scope note:** Single NATS protocol server; original stub `nats-message-broker/` (untouched) plus completed `nats-broker/` now both exist in Phase 5. Canonical build is `nats-message-broker/` (new stub replaced with actual code).

**Status:** ✅ Approved.

### Challenge 35: Rate Limiter — ✅ APPROVED

**What:** HTTP rate limiter in Go with four algorithms behind one `Limiter` interface at `phase-05-servers-infrastructure/rate-limiter/`. Algorithms: token bucket, leaky bucket, fixed window, sliding-window log. Exposed as `net/http` middleware returning 429 Too Many Requests + `Retry-After` + `X-RateLimit-*` headers. CLI demo server: `rate-limiter [--algo token-bucket|leaky-bucket|fixed-window|sliding-window] [--rate 10] [--burst 5] [--window 1m] [--addr :8080]`. Table-driven deterministic test suite.

**Key decisions (reusable pattern):**
- **One small interface, many strategies** — `Limiter.Allow(key) Decision` is the contract. Middleware, CLI, tests never reference concrete algorithm. `--algo` flag swaps implementations zero downstream changes. Central teaching point; mirrors how future Redis-backed limiter would slot in.
- **Injectable clock** — every algorithm reads "now" via `Clock` interface. Tests use hand-advanced fake clock (`clk.Advance(...)`) instead of real sleeps. Fast, flake-free. Highest-value pattern in the challenge; leaned on heavily in README.
- **Lazy refill** (no background goroutines/timers) — buckets compute accrued tokens on access (`elapsed × rate`). Exact, zero cost while idle, no cleanup needed.
- **Fixed window aligned to boundaries** — truncate `now` to window. Classic **boundary-burst** flaw is real and directly demonstrated in test (contrast against exact sliding-window log).
- **Distributed considerations** in README — shared Redis state, atomic check-increment via Lua, clock skew. Emphasized: scaling changes *where state lives*, not the algorithm itself. Natural tie-in to Phase 5 Redis Server.

**Go idioms for Python/Java dev:**
- Structural interfaces (no `implements` keyword), `http.Handler` middleware = decorator pattern, `sync.Mutex` vs GIL (Go has real parallelism), `defer` unlock.

**Testing:**
- Table-driven: burst+refill, window boundaries, per-key isolation, 429 middleware path + post-refill recovery.
- All tests use fake clock — zero `time.Sleep`, fully deterministic.

**Verification:** `go vet ./...` pass; `CGO_ENABLED=0 go test ./...` pass; `gofmt -l .` clean; manual: demo server `--rate 2 --burst 3` → six rapid curls return `200 200 200 429 429 429`; 429 carries `Retry-After: 1`, `X-RateLimit-Limit: 3`, `X-RateLimit-Remaining: 0`.

**Toolchain:** macOS LC_UUID issue (importing `net/http`). Fix: `CGO_ENABLED=0` for both test and build. Documented in README Testing Strategy section, matching curl. Worth standardizing this note across all Go challenges importing `net`/`net/http`/`crypto/tls`.

**Status:** ✅ Approved.

### README Quality Gate (all four pass)

All four READMEs include: What We're Building → Core Concepts → Architecture → Step-by-Step → Testing → Key Takeaways → Further Reading. Explain Go idioms for Python dev (🐍→🐹 callouts: net.Listener, bufio, container/list, sync.Mutex, goroutines, type assertions, interfaces, channels). Link `docs/go-quickstart.md`. Document `CGO_ENABLED=0` toolchain workaround consistently.

### Non-blocking nice-to-haves (not required)

- **web-server:** Only Content-Length request bodies (no chunked). Documented; deliberate scope cut.
- **nats-message-broker:** `max_payload` advertised in INFO but not enforced on PUB.

Neither blocks the verdict.

### Phase 5 Wave 1 Summary

**All four challenges ✅ APPROVED** — 29/64 overall challenges complete (Phases 1–4 complete, 4/7 Phase 5 complete).

**Curriculum status:** CURRICULUM.md checkboxes #30/#33/#34/#35 ticked by Coordinator.

**Next wave:** Phase 5 Wave 2 (load-balancer #31, redis-server #32, and docker #38 optional).


---

## Phase 5 Wave 2 — Load Balancer, Redis Server, Docker (Capstone)

### Challenge 31: Load Balancer — ✅ APPROVED

**What:** HTTP reverse-proxy load balancer in Go at `phase-05-servers-infrastructure/load-balancer/`. CLI: `load-balancer [--addr :8080] [--backends url1,url2,...] [--algo round-robin|least-conn|random|weighted] [--health-interval 5s] [--health-path /health] [--health-timeout 2s] [--verbose]`.

**Key decisions (reusable):**
- **Borrow the proxying, hand-write the lesson:** Used `httputil.NewSingleHostReverseProxy` for mechanics (already hand-rolled HTTP rewriting in #29) and hand-wrote the scheduling algorithms + health-check loop — where the lesson lives. Boundary called out explicitly in README.
- **Scheduler interface, not a switch:** One method `Next([]*Backend) *Backend`. RoundRobin / LeastConn / Random / WeightedRoundRobin satisfy it. Balancer never branches on algorithm name; only `newScheduler()` does. Strategy pattern reused from earlier Go challenges.
- **Pool filters health; schedulers stay health-blind:** `HealthyBackends()` returns only live backends in pool order, so round-robin "skipping" is automatic and each scheduler stays tiny.
- **Active AND passive health checking — headline concept:** Active = `time.Ticker` goroutine probing `GET /health` (detects recovery, brings backends back). Passive = ReverseProxy `ErrorHandler` marking backend DOWN on real forward failure. Production runs both: passive fails OUT in ms, active fails BACK IN.
- **Counter lifetime = request lifetime via `defer`:** `acquire()` then `defer release()` keeps active count high for exactly as long as the backend is busy (even on panic/client disconnect). That's what makes least-connections meaningful.
- **Atomics vs mutex split:** Per-backend `alive` (atomic.Bool) and `active` (atomic.Int64) on hot path; `sync.RWMutex` only around backend *list*.

**Testing:**
- Deterministic health without sleeps: each httptest backend exposes toggleable `/health` (200 ↔ 503); health loop exposes `CheckAll(ctx)` for exactly one probe sweep. No ticker timing, no flakiness.
- Least-connections tested by setting counters directly (deterministic, fast).
- End-to-end: 2-3 real backends, assert on `X-Served-By` labels, status, headers, body.
- **11 tests:** round-robin order, end-to-end spread, least-conn, weighted 3:1, proxying preserves body/header/status, unhealthy skipped + recovered, 503 when all down, passive mark-down, table-driven CLI parsing.

**Verification:** `CGO_ENABLED=0 go vet ./...` clean; `CGO_ENABLED=0 go test ./...` all 11 pass; `gofmt -l .` clean; `CGO_ENABLED=0 go build -o load-balancer .` succeeds.

**Status:** ✅ Approved.

### Challenge 32: Redis Server (XL) — ✅ APPROVED

**What:** From-scratch Redis-compatible server speaking RESP2 protocol over raw TCP at `phase-05-servers-infrastructure/redis-server/`. Hand-rolled protocol encoder/decoder, concurrency-safe in-memory store, key expiry (lazy + active), RDB-style snapshot persistence. Commands: PING, ECHO, SET (EX/PX/NX/XX), GET, GETSET, APPEND, MGET, MSET, DEL, EXISTS, KEYS, EXPIRE, TTL, INCR, DECR, SAVE, BGSAVE, COMMAND.

**Key decisions (reusable):**
- **RESP's two framing styles = headline lesson:** Mixes delimiter framing (simple strings/errors/integers — read until CRLF) with length-prefix framing (bulk strings/arrays — header says bytes/elements). Length-prefixing makes bulk strings binary-safe (body may contain CRLF/NUL). Same idea as HTTP Content-Length and Memcached; cross-linked across challenges.
- **One tagged `Value` struct** (typ byte + str/num/array/null fields) instead of interface hierarchy — keeps `Marshal` a single switch, makes byte-exact tests trivial. Constructors (`BulkString`, `NullBulk`, `Array`, …) keep handlers readable.
- **`io.ReadFull` for bulk-string bodies** — read exactly len+2 bytes; the standard fix for "single TCP read may short-read."
- **Dispatch via `map[string]commandHandler`** — idiomatic Go; adding a command is one map entry.
- **`sync.RWMutex` over keyspace map:** reads take RLock (KEYS/TTL don't mutate); writers + lazy-delete take Lock. README flags that production would shard the map. Go has no GIL so lock is mandatory correctness, not politeness (concurrent map write = hard crash).
- **Expiry = lazy + active, deliberately both:** Lazy delete inside Get/Set for correctness-on-access; background goroutine on `time.Ticker` calling `SweepExpired` to bound memory for never-read keys. Neither alone is enough.
- **Injectable clock** — `Store.now func() time.Time` (default `time.Now`) so expiry unit tests advance time instantly (zero `time.Sleep`, zero flakiness).
- **RDB snapshot = OUR OWN simple length-prefixed text format** — explicitly NOT binary-compatible with real Redis .rdb (no opcodes/LZF/CRC64). Atomic temp-file + `os.Rename` for crash safety. Load-on-start + save-on-shutdown (signal-driven) plus explicit SAVE/BGSAVE.
- **Graceful shutdown:** `quit` channel + `sync.WaitGroup`, `Close` guarded by `sync.Once` so explicit-Close-then-Cleanup pattern doesn't double-close the channel.

**Testing:**
- **42 test cases:** codec (all 5 RESP types + null forms), store (set/get/expiry/lazy-delete), TCP server round-trip, active sweep, save/reload with TTL survival, race detection.
- `CGO_ENABLED=0 go test -race ./...` PASS (locking is sound).
- Live smoke test via `nc`: PING→PONG, SET/GET round-trip, SAVE produced readable snapshot.

**Verification:** `go vet ./...` clean; `CGO_ENABLED=0 go test ./...` PASS (42 tests); `CGO_ENABLED=0 go test -race ./...` PASS.

**Note for Phase 7 (Challenge 47 — Redis CLI):** That challenge is a RESP *client* + REPL talking to THIS server. Wire contract fixed here: client sends array of bulk strings, reads exactly one RESP value, `$-1`/`*-1` as null. Can reuse this server's `resp.go` framing as reference (or port it).

**Status:** ✅ Approved.

### Challenge 36: Docker (CAPSTONE, Linux-only) — ✅ APPROVED

**What:** Minimal container runtime in Go at `phase-05-servers-infrastructure/docker/` (`gocker run [--mem 100m] [--pids 50] [--hostname name] <rootfs> <cmd> [args...]`). Shows a container is just a normal Linux process given a different view of the world (namespaces) and a spending limit (cgroups). Creates UTS + PID + mount + network + IPC namespaces, `pivot_root`s into image rootfs, mounts fresh `/proc`, sets hostname, applies memory/pids cgroup limits (v1 and v2), then exec's user command as PID 1. README-first with 7 sections and 🐧 Linux-only badge.

**Key decisions (reusable for cross-platform challenges):**
- **Build-tag split solves Linux-only on macOS dev box:** Namespaces/cgroups/pivot_root/OverlayFS are Linux kernel features that don't exist on Darwin. Solved with Go build tags:
  - `run_linux.go`, `cgroup_linux.go` → `//go:build linux` (real syscalls).
  - `run_other.go` → `//go:build !linux` (stub returning clear "this only runs on Linux — you are on darwin/arm64…" error).
  - `main.go`, `config.go`, `layers.go` → **no tag**, pure logic, compiled everywhere.
  - Tests split same way: `config_test.go` + `layers_test.go` run on macOS; `run_linux_test.go` is `//go:build linux` with `t.Skip()` unless root.
- **The re-exec trick is the heart:** Go can't safely `fork()` from multi-threaded runtime, so parent sets `SysProcAttr.Cloneflags` and re-executes `/proc/self/exe child …`; child finishes setup *inside* new namespaces. ASCII diagram + full prose explanation provided.
- **Config crosses re-exec boundary via argv:** `childArgs(cfg)` serializes to `child --membytes N --pids N --hostname H <rootfs> <cmd> [args]`; `parseChildArgs` parses it back. Round-trip unit test (`parseChildArgs(childArgs(cfg)) == cfg`) is safety net — if that breaks, containers launch with wrong limits.
- **Resolve units once, in parent:** `--mem 100m` → bytes in parent; child only sees integer `--membytes`. Keeps modes in sync, child code unit-free.
- **`flag` stops at first positional** — exactly what we want: container command like `/bin/ls -la` keeps own `-la` instead of gocker stealing it. Dedicated test + README callout.
- **`pivot_root` over `chroot`:** Implemented canonical 4-step dance (make private → bind-mount self → pivot_root → detach old root) and explained why chroot is escapable. Real runtimes use pivot_root.
- **cgroup v1 AND v2:** Detect v2 by presence of `/sys/fs/cgroup/cgroup.controllers`; write `memory.max`/`pids.max` for v2 or the `memory.limit_in_bytes` / per-controller tree for v1.
- **OverlayFS path math is testable slice of image story:** `layers.go` reverses base-first order into overlay's highest-priority-first lowerdir, renders `lowerdir=…,upperdir=…,workdir=…` option string — unit-tested on macOS even though mount itself is Linux-only.
- **User namespace left documented-but-off:** `CLONE_NEWUSER` + uid/gid mappings included as commented code with explanation (fights with mounting `/proc` and `CLONE_NEWNET` on some kernels).

**Testing:**
- **10 platform-neutral tests** (all pass on macOS): parseSize, parseRunArgs (flags-after-cmd, defaults, missing-arg), childArgs round-trip, dispatch unknown/help, overlay layout + mount options + path cleaning.
- Linux tests are tag-excluded on macOS.

**Verification (macOS):**
- `gofmt -l .` clean.
- `go vet ./...` passes.
- `CGO_ENABLED=0 go test ./...` all 10 neutral tests pass.
- `GOOS=linux go build ./...` succeeds (real runtime cross-compiles).
- README documents: Linux-only at top with how-to-run-on-Linux recipe (export rootfs, build, run privileged, integration test isolation with `hostname`/`ps`/`ls /`); teaches namespaces (incl. re-exec), cgroups, chroot vs pivot_root, OverlayFS.

**Linux verification (not done here; documented in README):**
- Export alpine rootfs, build for Linux, `sudo ./gocker run … /bin/sh`, then prove isolation with `hostname` / `ps` / `ls /` / `cat /etc/os-release`.

**Status:** ✅ Approved. Phase 5 (Servers & Infrastructure) COMPLETE (7/7).

**Overall:** All three APPROVED. 32/64 overall challenges complete; Phase 5 done.
