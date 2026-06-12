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

### 2026-06-09 — Review: Phase 1 / Challenge 4 — QR Code Generator (`phase-01-foundations/qr-code-generator/`)

**Verdict: ✅ APPROVED.**

- Independently ran `./.venv/bin/python -m pytest -q` → **34 passed** (twice, clean).
  Round-trip tests genuinely execute (not skipped): venv has both `pyzbar`+zbar
  and `opencv-python-headless`; all 5 decode cases pass.
- **Real CLI generation + independent decode** (all exact): `qrgen "HELLO WORLD"
  -o` → zbar `HELLO WORLD`; byte-mode URL `https://github.com`; numeric `8675309`
  decoded by **both** zbar and OpenCV; stdin pipe; half-block Unicode + `--ascii`
  renders correct.
- **Exercised the version ≥ 7 path** beyond the bundled v1–4 cases: `A`×220 →
  version 7 (45×45), triggering BCH(18,6) version-info stamping — zbar decodes it
  back exactly. Confirms `write_version_info` is correct.
- From-scratch confirmed: Pillow paints pixels from *our* grid only; no encoder
  library. GF(256) (doubled EXP table, slide-rule mul), RS LFSR long division
  (matches Wikiversity vector), encode pipeline (matches Thonky HELLO WORLD V1-Q
  codewords), zig-zag placement, 8 masks + 4 penalty rules, BCH format/version
  info — all hand-rolled and well-commented on the *why*.
- Tests: 4 ground-truth layers (GF axioms over all 255 nonzero, RS reference, encode
  reference, decode round-trip) + structural invariants + edge cases (empty,
  max-capacity, oversized-raises).
- README clears the teaching gate decisively: all 7 sections, symbol anatomy
  diagram, encoding modes, RS over GF(256) **with the math** + syndrome/decode
  explanation, interleaving rationale, masking + all 4 penalty rules, pipeline
  diagram, run/test instructions w/ macOS zbar caveat.

**Non-blocking nice-to-haves:**
- `choose_best_mask` scores penalties without temporarily writing format-info bits
  (inline comment implies it does). Negligible effect — every decoder still reads
  the symbol — but comment is slightly misleading.
- Rule-1 docstring says `3 + run−5`; code uses equivalent `run − 2` (behavior right).
- `--scale` lacks a `>0` guard (`--scale 0` → 0-dim image).
- Verdict written to `.squad/decisions/inbox/ellie-qr-code-review.md`.

### 2026-06-09 — Review: Phase 2 Wave 1 — wc, cat, head, cut, uniq, tr (`phase-02-core-unix/`)

**Verdict: ✅ ALL SIX APPROVED.**

- Independently ran `go vet ./...` + `go test ./...` per tool → all clean
  (tr's tests live in `internal/translate`; `?` no-test on the thin main pkg).
- REAL differential spot-checks vs the system binaries, all matching:
  - **wc**: counts `3 7 33`, `-l/-w/-c/-m` (runes, `é`=1), stdin pipe, multi-file
    `total`. (Cosmetic: narrower column padding than system; counts identical.)
  - **cat**: byte-identical concat, `-n` continuous, `-b` non-blank override,
    `-E`, `-`/stdin interleave, 2 KB binary round-trip. GNU continuous numbering
    deviation is documented (README "Compatibility note") — acceptable.
  - **head**: `-n`/`-c`, `-n5` glued, `==> file <==` headers, N>file, stdin —
    all match; early-termination real (`headLines` stop, `io.CopyN`).
  - **cut**: `-f` lists/ranges/open/`-3`, `-d`, default TAB, `-c` + Unicode,
    no-delim passthrough, `-s`, input-order `-f3,1` — all match.
  - **uniq**: plain, `-c` (BSD 4-wide, matches macOS), `-d`, `-u`, adjacency,
    `sort|uniq` — all match. (Missing-file exit 2 vs GNU 1 = repo convention.)
  - **tr**: translate, `-d`, `-s`, `-cd`, SET2 padding (`abcde`→`xyyyy`),
    `[:upper:]`→`[:lower:]`, multibyte `é`→`e`, `-s ' '` — all match.
- Code quality high across the board: tiny `main`→`run` split for testability,
  hand-rolled parsers (bundling, `--`, `-`/stdin), buffered streaming, rune-aware
  Unicode, `defer` flush. cut factors a reusable LIST grammar; tr compiles a
  Spec→Transformer with correct per-mode squeeze-set selection.
- READMEs clear the teaching gate: all 7 mandated sections, Go idioms explained
  for a Python dev (bufio, runes, interfaces, defer, zero value, iota), each
  links `docs/go-quickstart.md`.
- Non-blocking nice-to-haves: wc padding width; uniq exit-code 2 vs 1; add
  CLI-layer tests for tr's `main.go` flag parsing.
- Verdicts written to `.squad/decisions/inbox/ellie-phase2-wave1-review.md`.

### 2026-06-09 — Review: Phase 2 Wave 2 — sort, grep, sed, diff, xxd (`phase-02-core-unix/`)

**Verdict: ✅ ALL FIVE APPROVED — Phase 2 complete.**

- Per tool: `go vet ./...` clean + `go test ./...` green; differential
  spot-checks vs the system binary.
- **sort:** external merge sort genuinely exercised — `TestExternalSortForced`
  (`--chunk-lines 4` → ~13 runs through the k-way `container/heap` merge),
  `TestExternalMatchesInMemory` proves both paths agree across `{} -r -n -u -f
  -fu`, `TestExternalUnique` collapses cross-run dupes. Heap `Less` tie-breaks by
  run index → stable, matches in-memory `SliceStable`. `-n` matches system sort.
- **grep:** RE2 regex; `-i/-v/-n/-c/-w` correct (`-w`=`\b(?:…)\b`); `-r` via
  `filepath.WalkDir`; `-A/-B/-C` span-merge + `--`. Exit codes live-verified
  0/1/2 (bad pattern & dir-without-`-r` → 2).
- **sed:** parser→command-list→executor; `s///` g/i/p + `\1..\9` + `&`;
  addressing N/`$`/`/re/`/ranges with state machine; `-n`, `-i` (preserves mode).
  Documents RE2/ERE `(…)` vs BRE `\(…\)` — BRE scripts intentionally won't match.
- **diff:** LCS DP **from scratch** → edit script (GNU delete-before-insert
  bias). Unified `@@` hunks **byte-identical to `diff -u`** incl. pure-insert
  (`-l,0`) & pure-delete; normal format matches. Exit 0/1/2.
- **xxd:** forward dump **byte-identical to system xxd** (default & `-c 8`); `-r`
  **binary round-trip exact** (500 random bytes), cross-tool reverse + offset
  zero-pad verified. Binary-safe (`io.ReadFull`/`io.CopyN`).
- READMEs clear the teaching gate; primer linked twice each.
- Non-blocking: sed README "🐍→🐹" mojibake on some terminals; sed lacks `s///N`;
  diff drops mtime (deliberate); xxd no `-u`/`-p`. None block.
- Verdicts written to `.squad/decisions/inbox/ellie-phase2-wave2-review.md`.

### 2026-06-09 — Review: Phase 3 Wave 1 — jq, yq, xargs, tar, crontab (`phase-03-advanced-cli/`)

**Verdict: ✅ ALL FIVE APPROVED — Phase 3 Wave 1 complete.**

- Per challenge: Go `go vet ./...` + `go test ./...` green; yq via its `.venv`
  pytest (**54 passed**). Plus real behavioral spot-checks; differential/interop
  for jq and tar.
- **jq (Go):** 44 tests. DIFFERENTIAL vs system jq 1.7.1 — every case
  byte-identical: `.foo`, `.addr.city`, `.[]`, `keys`, `map(select(.age>25))`,
  `.[]|.age`, `.a+.b`, comma, `[.[]|.*2]`, `.[-1]`, `has`, `select`, `.foo?`,
  rune-aware `length`, `not`, nested pipe+select. Clean lex→parse→eval, correct
  precedence grammar, cartesian binary ops, jq total-ordering compare, `try`
  swallows to empty. Documented divergence: `values` = object values (not jq's
  null-filter). README gate cleared, go-quickstart ×2.
- **yq (Python):** loader(safe_load_all)/query(lexer→parser→generator
  value-stream)/convert/cli. Verified anchors&aliases (shared value), multidoc
  `---`, YAML↔JSON both ways, `.tags[]`, `keys`. Deliberate query subset
  (identity/field/index/iterate/pipe/length/keys) — matches data-model framing.
  README thorough, Python (no primer needed).
- **xargs (Go):** 18 tests incl. real bounded-parallelism. Matches system xargs
  on batching, `-n2`, `-I {}`, `-0`. **-P VERIFIED REAL:** 4×0.5s sleeps under
  `-P4` → 0.51s; `-P2` → exactly 2 concurrent starts. Exit propagation verified:
  `false`→123, missing cmd→127. Semaphore(buffered chan)+WaitGroup pool correct.
  Non-blocking: whitespace-only tokenizer (no shell quoting).
- **tar (Go):** 9 tests. **INTEROP CONFIRMED BOTH WAYS** with system tar:
  mytar→`tar -tf`/`-xf` byte-identical (`diff -r` clean); system tar→mytar
  `-t`/`-x` byte-identical. **Traversal guard verified with a real Python-built
  malicious tar** (`../escape.txt` refused, nothing leaked); absolute paths too.
  Correct USTAR octal/checksum-with-spaces/prefix split/2-zero terminator/CopyN.
  Non-blocking: no `-C`, files+dirs only.
- **crontab (Go):** 29 tests. Real next-run all correct incl. **dom/dow OR rule**
  (`0 0 13 * 5` → Fridays AND the 13th), year rollover (`59 23 31 12 *`),
  step-dow `0/2`, named `jan-mar mon`, macros, impossible `0 0 30 2 *`→exit 2.
  Bitset-per-field + big-jump search w/ `time.Date` rollover. README gate cleared.
- Non-blocking nits noted (none gate): jq `values` divergence (documented);
  yq multi-scalar doc-stream output; xargs tokenizer quoting; tar `-C`/symlinks;
  crontab `-explain` minute-vs-hour formatting.
- Verdicts written to `.squad/decisions/inbox/ellie-phase3-wave1-review.md`.

### 2026-06-09 — Review: Phase 3 Wave 2 — curl + Shell capstone (Go) — ✅✅ BOTH APPROVED

**Verdict: Phase 3 COMPLETE.**

- **curl** (`phase-03-advanced-cli/curl/`) — ✅ APPROVED. `go vet` clean; `CGO_ENABLED=0 go test ./...` → 30 pass (incl. 3 e2e). Raw TCP (`net.Dial`+`tls.Client`), NOT net/http for the protocol. Verified request framing (Host, CRLFs, Content-Length, Connection: close), response parser, and the chunked decoder (hex sizes, `;ext`, per-chunk CRLF, 0-chunk+trailer) + Content-Length (`io.ReadFull`) + EOF fallback. Flags `-X/-H/-d/-o/-I/-v/-L` + redirect loop (cap 10, 301/302/303→GET, 307/308 preserve). README (326 lines) teaches socket→HTTP and ALREADY documents the `CGO_ENABLED=0` LC_UUID toolchain workaround — no doc gap.
- **Shell/gosh** (`phase-03-advanced-cli/shell/`) — ✅ APPROVED. `go vet` clean; `go test ./...` → 33 pass. Hand-drove binary: 2-stage pipe, redirect+readback, cd→pwd, `$?` after failure, `&&`/`||`, quote semantics — all correct. Tokenizer (quotes/escapes), recursive-descent parser, executor (os.Pipe N-stage + the parent-fd-close-for-EOF rule), in-process builtins (cd/pwd/exit/echo/export/type), `$VAR/${VAR}/$?/$$` expansion, SIGINT swallow. README (297 lines) has the fd pipeline diagram + "why cd must be a builtin" section + go-quickstart link.
- **Non-blocking nits:** shell `2>>` treated as `2>` (documented); no post-expansion word-splitting/globbing (reasonable scope cut). Neither blocks approval.
- **Reusable learning:** go1.22.2 darwin/arm64 cgo external-linker LC_UUID abort affects any Go binary importing net/crypto/tls at test load — verify such challenges with `CGO_ENABLED=0 go test`; it is NOT a code failure.

### 2026-06-13 — Review: Phase 4 Wave 1 — DNS Resolver (23), NTP Client (25), Port Scanner (27), netcat (28)

**Verdict: ✅ ALL FOUR APPROVED.** Independently ran `go vet ./...` (clean) + `CGO_ENABLED=0 go test ./...` (PASS) in each dir. LC_UUID cgo abort not treated as a failure; all 4 READMEs document the `CGO_ENABLED=0` workaround. The two repaired challenges are genuinely complete (real code + teaching README, no stubs).

- **dns-resolver (23):** Raw `net.DialUDP`, NO `net.Resolver`/`LookupHost`. Hand-packed 12-byte header (6×uint16 BE), QNAME label encode, full RR parse. **Name compression (0xC0) decode correct**: 14-bit offset `&0x3FFF`, returns continuation offset past the 2-byte pointer (not the jump target), caps jumps vs loops; RDATA names decoded against full message. Crafted-byte tests cover compression (name + offset) and pointer-loop rejection. README honestly states recursive default + iterative `--trace`. All 7 sections + quickstart.
- **ntp-client (25):** 48-byte packet, first byte `(0<<6)|(version<<3)|modeClient` (v3→0x1B). Epoch offset 2208988800; `(frac*1e9)>>32` fixed-point; cleanest check present (NTP secs==offset → Unix 0). offset/delay formulas correct. Crafted-byte tests, no live server. 7 sections + quickstart.
- **port-scanner (27):** TCP connect via `net.DialTimeout`; textbook worker pool (buffered jobs/results channels + N goroutines + `sync.WaitGroup` + closer goroutine). Tests use only `127.0.0.1:0` listeners; OPEN reported, closed excluded; single-worker + sorted cases. 7 sections + quickstart.
- **netcat (28):** Two concurrent `io.Copy` relay; TCP `CloseWrite()` half-close; UDP `-w` deadline + `udpListenConn` peer-learning adapter. Connect+listen, TCP+UDP. In-process loopback tests assert bytes BOTH directions (ping↑/pong↓). 7 sections + quickstart.

**Non-blocking nice-to-haves:** dns `formatIPv6` skips `::` compression (documented); netcat ignores send-side `io.Copy` error (fine for a relay). Verdicts written to `.squad/decisions/inbox/ellie-phase4-wave1-review.md`.

### 2026-06-13 — Review: Phase 4 WAVE 2 (Challenges 24/26/29) — ✅ ALL APPROVED (Phase 4 COMPLETE)

Independently ran `go vet ./...` (clean) + `CGO_ENABLED=0 go test -count=1 -v ./...`
(all PASS) in each dir. No LC_UUID abort under CGO_ENABLED=0.

- **dns-forwarder (#24) — ✅ APPROVED.** UDP server, goroutine-per-request, RWMutex
  TTL cache keyed on (QNAME,QTYPE,QCLASS). Tests use a LOCAL fake upstream with an
  atomic hit counter + injectable fake clock: TestForwardAndRelay (1 hit),
  TestSecondQueryServedFromCache (still 1 hit, txn-ID patched per client),
  TestCacheExpiresAfterTTL (within TTL=1 hit, past TTL=2 hits) — proves hit/miss +
  expiry with no internet. Defaults to :1053; README documents :53/sudo/setcap.
  Caches min-answer-TTL, skips TTL=0, lazy expiry with double-check under write lock.
- **traceroute (#26) — ✅ APPROVED.** Unprivileged ICMP: icmp.ListenPacket("udp4")
  + ipv4.PacketConn.SetTTL per probe (no root). buildEchoRequest/parseICMPReply
  unit-tested with crafted bytes (TimeExceeded/EchoReply/DstUnreach/echo-req-ignored
  + garbage→error); runTrace hop iteration driven by a fake prober (stops at dest,
  star on timeout, respects maxHops); formatHop rendering tested. go.mod/go.sum
  include golang.org/x/net v0.31.0. Live integration test self-skips on socket error.
- **http-forward-proxy (#29, CAPSTONE) — ✅ APPROVED.** Plain HTTP: absolute→origin
  rewrite + hop-by-hop stripping (RFC 7230 set, canonicalised), httptest origin
  asserts origin never sees absolute-form. HTTPS CONNECT: 200 Connection Established
  + bidirectional io.Copy relay with CloseWrite half-close; tested via httptest TLS
  server through http.Transport CONNECT AND a raw hand-written CONNECT + real TLS
  handshake over the tunnel (TestConnectRawHandshake). README explains TLS opacity
  (proxy can't decrypt; MITM needs forged CA) and ties back to curl/netcat.

READMEs: all 7 mandated sections each, 🐍➡️🐹 Go-idiom explanations (iota enums,
implicit interfaces, goroutine-per-conn, RWMutex, deadlines, blank-assignment
interface check), docs/go-quickstart.md linked, CGO_ENABLED=0 workaround documented.

**Non-blocking nice-to-have:** traceroute's TestTraceIntegration is gated only by
`testing.Short()`, so a plain `go test ./...` makes a live call to 8.8.8.8 (it
self-skips on error and can't fail the suite). Gating behind an env var (or
default-skip) would make the default run fully hermetic. Not a blocker.

Verdicts written to `.squad/decisions/inbox/ellie-phase4-wave2-review.md`.
**Phase 4 (Networking) is COMPLETE — all challenges approved.**

### 2026-06-13 — Review: Phase 5 Wave 1 — four Go server challenges — ✅ ALL APPROVED

Independently ran `go vet ./...` + `CGO_ENABLED=0 go test ./...` in each dir — all clean/PASS. No external deps in any go.mod (confirmed: no nats.go, no test frameworks; all stdlib).

- **web-server (#30) ✅** — HTTP/1.1 served from RAW TCP (net.Listen accept loop, goroutine-per-conn), NOT net/http on the serving path. Hand-rolled request-line/header/body parsing (CRLF, Content-Length via io.ReadFull, maxHeaderBytes cap), correct response framing, exact method+path routing with 404/405/501, static files with extension→MIME map + index.html, HEAD drops body. Path traversal: two-layer defense (filepath.Clean + absRoot+sep containment check → 403). Keep-alive honors HTTP/1.0 vs 1.1 default flip + read deadline. Tests on 127.0.0.1:0 prove keep-alive (2 sequential reqs, same socket via http.ReadResponse), raw `/../secret.txt` rejected (no leak), 404, content-type.
- **memcached-server (#33) ✅** — TEXT protocol over TCP; set/get/gets/add/replace/append/prepend/cas/delete/incr/decr/flush_all with STORED/END/VALUE/DELETED framing + noreply. Store uses map + container/list for O(1) LRU; lazy expiry (liveLocked reaps on access); CAS via monotonic token; memcached exptime quirk (relative ≤30d vs absolute); incr wraps, decr floors at 0. Injectable clock. Tests prove set→get, miss→END, delete, incr/decr, expiry (clock advance), LRU evicts coldest at cap (with and without touch).
- **nats-message-broker (#34) ✅** — text protocol CONNECT/PING-PONG/PUB/SUB/UNSUB/MSG/INFO/+OK/-ERR, goroutine-per-client + writeLoop. matchSubject correct: `*` single token, `>` tail (must be last + ≥1 remaining token, so `foo.>` ≠ `foo`). Queue groups: plain subs all delivered, each group picks exactly one via round-robin counter; enqueue outside lock. Does NOT import real nats.go. Tests prove wildcard match/non-match, `>` tail, queue-group single delivery, unsub stops delivery.
- **rate-limiter (#35) ✅** — token bucket, sliding window, fixed window, leaky bucket behind one Limiter interface; per-key maps; injectable Clock (fakeClock.Advance, ZERO time.Sleep in tests). net/http middleware sets X-RateLimit-* + Retry-After, returns 429 via httptest. Token bucket lazy refill with float tokens. Tests deterministic: burst+refill, window boundaries, per-client isolation, 429 + recovery.

READMEs: all 4 clear the gate — 7 mandated sections, go-quickstart.md link, 🐍 Python-analogy teaching, CGO_ENABLED=0/LC_UUID workaround documented. Did NOT reject over the cgo abort (used CGO_ENABLED=0 per toolchain note).

Non-blocking nice-to-haves: web-server only supports Content-Length bodies (no chunked) — documented; nats max_payload advertised in INFO but not enforced on PUB. Neither is a blocker.

Verdicts written to .squad/decisions/inbox/ellie-phase5-wave1-review.md.
