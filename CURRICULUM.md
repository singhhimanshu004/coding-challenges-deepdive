# 📚 Coding Challenges Deep Dive — Learning Curriculum

> **The master learning plan for every challenge in this repo** — sourced from [codingchallenges.fyi](https://codingchallenges.fyi/) by John Crickett.
> Designed as a *progressive, dependency-ordered* path: you start by parsing bytes and finish by building servers, version control, and language interpreters.

*Curriculum regenerated 2026-06-08 on a stronger model. This v2 sharpens difficulty ratings, makes conceptual dependencies explicit, adds effort estimates, and removes redundancy from the prior draft.*

---

## 🧭 How to Use This Curriculum

The curriculum sorts **64 challenges** into **8 sequential phases**. Each phase deliberately reuses skills built in earlier ones — by the time you build a Redis server you've already written a parser, streamed bytes, and opened sockets. Work the phases in order for the strongest foundation, or use the **dependency graph** and **learning paths** at the bottom to chart your own route.

### The golden rule: README-first

For **every** challenge, you write the teaching README *before* the code. This is the core learning mandate of the project (see the [README-First Policy](#-readme-first-policy) below). The code proves you understood it; the README proves you can *teach* it.

### For each challenge you will:

1. **Read the spec** on [codingchallenges.fyi](https://codingchallenges.fyi/) and note the requirements.
2. **Write `README.md` first** — What We're Building → Core Concepts → How It Works in the Real World → Architecture → Step-by-Step Implementation → Testing → Key Takeaways → Further Reading.
3. **Implement** in the recommended language (see rationale per challenge).
4. **Test** against the real tool's behaviour where possible (e.g. diff your `wc` output against GNU `wc`).
5. **Reflect** — fill in Key Takeaways and tick the checkbox.

### Suggested cadence

| Track | Pace | Time to complete |
|-------|------|------------------|
| 🔥 Full-time | 1 challenge / 2–3 days | ~5–6 months |
| 🌙 Evenings & weekends | 1 challenge / week | ~14–15 months |
| 🎯 Pick-and-choose | Follow one phase or learning path | Varies — check prerequisites first |

**Legend:** Languages 🟦 Go · 🟨 Python · 🟩 TypeScript · 🟧 Java (alternate). Effort ⏱️ is a rough solo estimate (S ≤ ½ day · M ≈ 1–2 days · L ≈ 3–5 days · XL ≈ 1 week+).

### Difficulty rubric

| Rating | What it means |
|--------|---------------|
| 🟢 **Beginner** | Single concept, linear data flow, standard library covers most of it. |
| 🔵 **Intermediate** | Multiple moving parts, a non-trivial algorithm, or a real protocol/format spec. |
| 🟠 **Advanced** | Concurrency, stateful protocols, persistence, or system-design trade-offs. |
| 🔴 **Expert** | Deep systems internals or large surface area (OS primitives, full VCS, language runtime). |

---

## 🗺️ Curriculum Overview

| Phase | Name | Core Skill Unlocked | Difficulty | # |
|-------|------|---------------------|------------|---|
| 1 | Foundations: Parsing, Encoding & Data Structures | Turning bytes into meaning | 🟢→🔵 | 4 |
| 2 | Core Unix: Text Processing | Streaming I/O & the Unix philosophy | 🟢→🔵 | 11 |
| 3 | Advanced CLI & Orchestration | Structured data, processes, HTTP clients | 🔵 | 7 |
| 4 | Networking Fundamentals | Sockets & binary protocols | 🔵→🟠 | 7 |
| 5 | Servers & Infrastructure | Concurrent servers & system design | 🟠→🔴 | 7 |
| 6 | Applications & Full-Stack | Databases, APIs, auth, real-time | 🔵→🔴 | 9 |
| 7 | Developer Tools & Internals | Tooling internals (Git, load testing) | 🔵→🔴 | 6 |
| 8 | Games, Interpreters & Creative | Game loops, language design, API integration | 🟢→🔴 | 13 |

**Total: 64 challenges.**

> 💡 **Ordering philosophy:** every phase boundary represents a *capability jump*. You don't move to networking until you can stream bytes; you don't build servers until you can open a socket; you don't build apps until you can serve HTTP. Within a phase, challenges are ordered so each one introduces *exactly one* major new idea.

---

## Phase 1 — Foundations: Parsing, Encoding & Data Structures

> **Learning Objective:** Learn to convert raw bytes and text into structured meaning, and back again. Recursive-descent parsing, binary I/O, bit manipulation, and hash-based data structures are the bedrock skills that nearly every later challenge reuses.

**Key Concepts:** lexing & tokenizing · recursive-descent parsing · tree data structures · bit-level I/O · prefix codes · hash functions · probabilistic data structures
**Prerequisites:** comfort with one language; arrays, maps, recursion, and trees.

| # | Challenge | Diff | Lang | ⏱️ | Why this language / what it teaches | ✅ |
|---|-----------|------|------|----|-------------------------------------|----|
| 1 | **JSON Parser** | 🟢 | 🟨 Python | M | Cleanest way to learn a recursive-descent parser end-to-end; expressive string handling lets you focus on grammar, not boilerplate. | [x] |
| 2 | **Huffman Compression** | 🔵 | 🟦 Go | M | Greedy tree-building + reading/writing individual *bits*. Go's `bufio`/`io` make a bit-writer natural. | [x] |
| 3 | **Bloom Filter Spell Checker** | 🔵 | 🟦 Go | S | Hash functions, bit arrays, and the false-positive/space trade-off. Go's speed makes large dictionaries practical. | [x] |
| 4 | **QR Code Generator** | 🔵 | 🟨 Python | L | Reed–Solomon error correction, bit packing, and matrix layout; `numpy`/`Pillow` make the visual output painless. | [x] |

**🎓 Why Phase 1 is first:** The JSON parser is the single highest-leverage challenge in the whole curriculum — the *exact same* lex→parse→evaluate skeleton reappears in `jq`, `yq`, the Calculator, and the Lisp interpreter. Huffman introduces the bit-level I/O you'll reuse in `tar`, `xxd`, and every binary network protocol. Bloom filters introduce the hashing intuition behind caches and databases.

**🏁 Phase capstone:** the JSON Parser — if you can parse JSON cleanly, you've internalised the parsing mindset.

---

## Phase 2 — Core Unix: Text Processing

> **Learning Objective:** Build the classic Unix text toolkit and absorb the Unix philosophy: small composable programs that read stdin, write stdout, and chain through pipes. Master buffered streaming, line/field/byte processing, and faithful CLI ergonomics (flags, exit codes, stdin fallback).

**Key Concepts:** stream processing · buffered I/O · file descriptors & stdin/stdout · Unicode/rune handling · flag parsing · exit codes · regular expressions (intro) · the pipe-and-filter model
**Prerequisites:** Phase 1 (parsing mindset); basic terminal fluency.

| # | Challenge | Diff | Lang | ⏱️ | Why this language / what it teaches | ✅ |
|---|-----------|------|------|----|-------------------------------------|----|
| 5 | **wc** (word count) | 🟢 | 🟦 Go | S | The canonical starter: bytes/words/lines, flags, stdin fallback. `bufio.Scanner` is built for it. | [ ] |
| 6 | **cat** | 🟢 | 🟦 Go | S | Multi-file concatenation & stream forwarding; line numbering flags. | [x] |
| 7 | **head** | 🟢 | 🟦 Go | S | Early-termination reads (lines *and* bytes); builds on `wc` patterns. | [x] |
| 8 | **cut** | 🟢 | 🟦 Go | S | Field/delimiter parsing *within* a line — column-oriented thinking. | [x] |
| 9 | **uniq** | 🟢 | 🟦 Go | S | Adjacent-duplicate collapse; carrying state across a stream (`-c`, `-d`). | [ ] |
| 10 | **tr** | 🔵 | 🟦 Go | S | Character translation/deletion/squeeze; rune handling & character classes. | [x] |
| 11 | **sort** | 🔵 | 🟦 Go | M | Comparators, numeric/reverse/unique sorting, and external sort for big files. | [x] |
| 12 | **grep** | 🔵 | 🟦 Go | M | Regex matching, recursive directory walks, context flags (`-A/-B/-C`). | [x] |
| 13 | **sed** | 🔵 | 🟦 Go | M | Stream editing: substitution, addressing, in-place edits — a mini command language. | [x] |
| 14 | **diff** | 🔵 | 🟦 Go | M | Longest-common-subsequence / edit distance and the unified-diff format. | [x] |
| 15 | **xxd** | 🔵 | 🟦 Go | S | Hex dumps & reverse mode; byte-level formatting bridges you toward binary protocols. | [x] |

**🎓 Why Go for the CLI toolkit:** Go compiles to a single dependency-free binary, ships first-class I/O primitives (`bufio`, `io`, `os`), and reflects how modern Unix tools are actually written. You get systems-level discipline without C's footguns. Build these in roughly the listed order — each adds one new idea (multi-file → fields → state → regex → diff algorithms → binary).

**🏁 Phase capstone:** `grep` + `diff` — pattern matching and the LCS algorithm are the conceptual peak of this phase.

---

## Phase 3 — Advanced CLI & Orchestration

> **Learning Objective:** Step up from single-stream filters to tools that handle structured data formats, spawn and coordinate processes, speak HTTP, and ultimately *orchestrate* other programs. This phase ends by building a shell — the program that runs everything else.

**Key Concepts:** JSON/YAML processing · expression evaluation over trees · process spawning & `fork/exec` · pipes & redirection · archive formats & file metadata · cron expressions · HTTP/1.1 client · TLS basics
**Prerequisites:** Phase 1 (**JSON Parser** is a hard dependency for `jq`), Phase 2 (CLI ergonomics, byte I/O for `tar`).

| # | Challenge | Diff | Lang | ⏱️ | Why this language / what it teaches | ✅ |
|---|-----------|------|------|----|-------------------------------------|----|
| 16 | **jq** (JSON processor) | 🔵 | 🟦 Go | L | Reuses your Phase 1 parser, then adds a filter/expression language + tree-walking evaluator. | [x] |
| 17 | **yq** (YAML processor) | 🔵 | 🟨 Python | M | YAML is a JSON superset; `ruamel`/`pyyaml` let you focus on the data model and a deliberate contrast to Go's `jq`. | [ ] |
| 18 | **xargs** | 🔵 | 🟦 Go | M | Batching stdin into argv, spawning processes, and bounded parallelism. | [x] |
| 19 | **tar** | 🔵 | 🟦 Go | M | Binary header parsing & file metadata — directly extends Phase 1's bit/byte skills. | [ ] |
| 20 | **crontab** | 🔵 | 🟦 Go | M | Cron-expression parsing and time-based scheduling logic. | [ ] |
| 21 | **curl** | 🔵→🟠 | 🟦 Go | L | An HTTP/1.1 client from raw TCP: request framing, headers, chunked encoding, TLS. The bridge into networking. | [x] |
| 22 | **Shell** | 🟠 | 🟦 Go | L | The orchestrator: tokenizing input, `fork/exec`, pipes, redirects, built-ins, `PATH` resolution, job basics. | [ ] |

**🎓 Why the Shell comes last here:** A shell is the *conductor* of every Unix tool you just built. Implementing it after the individual tools makes pipes, redirects, and process lifecycles click — you finally see what `cmd1 | cmd2 > file` is doing under the hood. `curl` is placed immediately before networking on purpose: it's your first real protocol client and the natural on-ramp to Phase 4.

**🏁 Phase capstone:** the **Shell** — the most integrative CLI project in the curriculum.

---

## Phase 4 — Networking Fundamentals

> **Learning Objective:** Understand how the internet actually works at the wire level. Build tools that speak DNS, NTP, ICMP, and raw TCP/UDP. Learn socket programming, binary protocol encoding/decoding, network byte order, and concurrency for connections.

**Key Concepts:** TCP vs UDP sockets · DNS wire format · ICMP & TTL · NTP timestamp format · network byte order & struct packing · timeouts & retries · goroutine-per-connection concurrency · HTTP CONNECT tunnelling
**Prerequisites:** Phase 3 (`curl` for HTTP basics), Phase 1–2 (binary I/O from `xxd`/`tar`/Huffman — DNS/NTP are binary protocols).

| # | Challenge | Diff | Lang | ⏱️ | Why this language / what it teaches | ✅ |
|---|-----------|------|------|----|-------------------------------------|----|
| 23 | **DNS Resolver** | 🔵 | 🟦 Go | M | UDP sockets + hand-encoding/decoding the DNS wire format; recursive resolution from the root. | [x] |
| 24 | **DNS Forwarder** | 🔵 | 🟦 Go | M | Becomes a *server*: listen on UDP, forward queries, cache responses with TTL. Depends on #23. | [x] |
| 25 | **NTP Client** | 🔵 | 🟦 Go | S | A tight 48-byte UDP packet; struct packing and clock-offset math. | [x] |
| 26 | **Traceroute** | 🔵 | 🟦 Go | M | Raw sockets, ICMP, and incrementing TTL to discover the path hop by hop. | [x] |
| 27 | **Port Scanner** | 🔵 | 🟦 Go | M | TCP connect scanning at scale; goroutines + worker pools + timeouts. | [x] |
| 28 | **Netcat** | 🔵→🟠 | 🟦 Go | M | Bidirectional TCP/UDP relay between sockets and stdin/stdout — the networking "Swiss-army knife". | [x] |
| 29 | **HTTP Forward Proxy** | 🟠 | 🟦 Go | L | HTTP `CONNECT`, request rewriting, TLS tunnelling, connection management. Ties HTTP to raw sockets. | [x] |

**🎓 Why Go owns this phase:** Go was built for networked systems — goroutines make concurrent connections trivial, and `net` gives clean socket abstractions with no runtime to deploy. **Order matters:** Resolver → Forwarder (the forwarder *is* a resolver plus a server), and the connection-oriented tools (Netcat, Proxy) come after the connectionless UDP tools (DNS, NTP).

**🏁 Phase capstone:** the **HTTP Forward Proxy** — combines sockets, HTTP, TLS, and concurrency.

---

## Phase 5 — Servers & Infrastructure

> **Learning Objective:** Build production-style server software. Master concurrent connection handling, stateful protocol implementation, caching/eviction strategies, pub/sub, rate limiting, and finally OS-level isolation. This is the systems-programming summit of the curriculum.

**Key Concepts:** accept loops & connection multiplexing · protocol state machines · RESP & memcached protocols · in-memory stores with expiry · LRU eviction · persistence (RDB/AOF) · pub/sub & subject routing · token-bucket / sliding-window algorithms · Linux namespaces, cgroups & union filesystems
**Prerequisites:** Phase 4 (sockets, concurrency), Phase 1 (parsing for RESP/memcached protocols).

| # | Challenge | Diff | Lang | ⏱️ | Why this language / what it teaches | ✅ |
|---|-----------|------|------|----|-------------------------------------|----|
| 30 | **Web Server** | 🟠 | 🟦 Go | L | HTTP/1.1 from scratch: request parsing, routing, static files, keep-alive. The server counterpart to `curl`. | [x] |
| 31 | **Load Balancer** | 🟠 | 🟦 Go | L | Reverse proxy with health checks and round-robin / least-connections scheduling. Depends on #30. | [ ] |
| 32 | **Redis Server** | 🟠 | 🟦 Go | XL | The RESP protocol, an in-memory key-value store, key expiry, and persistence. Reuses Phase 1 parsing. | [ ] |
| 33 | **Memcached Server** | 🟠 | 🟦 Go | L | Text/binary protocol + LRU eviction & slab-allocation concepts; instructive contrast to Redis. | [x] |
| 34 | **NATS Message Broker** | 🟠 | 🟦 Go | L | Pub/sub, subject routing, client lifecycle, and delivery semantics. | [x] |
| 35 | **Rate Limiter** | 🔵→🟠 | 🟦 Go | M | Token-bucket & sliding-window algorithms as reusable middleware; distributed considerations. | [x] |
| 36 | **Docker** | 🔴 | 🟦 Go | XL | Linux namespaces, cgroups, and layered filesystems — what a container *actually* is. | [ ] |

**🎓 The server learning arc:** every server challenge follows the same four steps — (1) read the protocol spec, (2) build a minimal single-connection version, (3) add concurrency, (4) add persistence/advanced features. This mirrors how real infrastructure evolves. Build the **Web Server before the Load Balancer** (you need a backend to balance) and the **Redis Server before the Redis CLI** in Phase 7 (the client needs a server to talk to).

**🏁 Phase capstone:** **Docker** — the deepest dive into OS primitives in the entire curriculum.

---

## Phase 6 — Applications & Full-Stack

> **Learning Objective:** Build complete, user-facing products with persistent storage, REST/real-time APIs, authentication, encryption, and file handling. Shift from "how does the protocol work" to "how do I design a correct, secure system".

**Key Concepts:** REST API design · relational/NoSQL schema design · base62/short-ID generation · WebSockets & broadcasting · IRC text protocol · CRUD & state management · encryption-at-rest (AES, KDFs) · tokenization & audit logging · chunked upload & delta sync · calendar/recurrence/timezone logic
**Prerequisites:** Phase 5 (**Web Server** + HTTP), and familiarity with at least one database.

| # | Challenge | Diff | Lang | ⏱️ | Why this language / what it teaches | ✅ |
|---|-----------|------|------|----|-------------------------------------|----|
| 37 | **URL Shortener** | 🔵 | 🟩 TypeScript | M | The quintessential first web app: REST + DB + base62 encoding. Node/TS is ideal for quick APIs. | [ ] |
| 38 | **Pastebin** | 🔵 | 🟩 TypeScript | M | URL-shortener++: text storage, expiry, syntax highlighting. Reinforces CRUD + storage. | [ ] |
| 39 | **Realtime Chat** | 🟠 | 🟩 TypeScript | L | WebSockets, rooms, and message broadcasting — Node shines at real-time fan-out. | [ ] |
| 40 | **IRC Client** | 🟠 | 🟨 Python | L | The IRC protocol (RFC 1459): an event-driven TCP text-protocol client. | [ ] |
| 41 | **Google Keep** | 🔵→🟠 | 🟩 TypeScript | L | Full-stack CRUD with labels, reminders, and rich notes; front-to-back app structure. | [ ] |
| 42 | **Scheduling App** | 🟠 | 🟩 TypeScript | L | Recurring-event rules, timezone math, and availability calculation — surprisingly deep domain logic. | [ ] |
| 43 | **Password Manager** | 🟠 | 🟨 Python | L | AES-256, key derivation (PBKDF2/Argon2), and secure secret storage. Security-first design. | [ ] |
| 44 | **Data Privacy Vault** | 🟠 | 🟦 Go | L | Tokenization, encryption-at-rest, access policies, and audit logging — privacy engineering. | [ ] |
| 45 | **Dropbox** | 🔴 | 🟦 Go | XL | File sync: chunking, delta sync, conflict resolution, and filesystem watching. The hardest app. | [ ] |

**🎓 Ordering rationale:** start with the two simplest storage apps (**URL Shortener → Pastebin**, near-identical shape), graduate to real-time (**Chat → IRC Client**), then full CRUD products (**Keep → Scheduling**), then the security-sensitive trio (**Password Manager → Data Privacy Vault**), and finish with **Dropbox**, which integrates everything (storage, sync protocol, concurrency, conflict resolution).

**🏁 Phase capstone:** **Dropbox** — distributed-systems thinking applied to a real product.

---

## Phase 7 — Developer Tools & Internals

> **Learning Objective:** Build the tools developers use every day and understand their internals — content-addressable storage in Git, load-testing methodology, network simulation, and browser-extension architecture.

**Key Concepts:** content-addressable storage · Git object model (blobs/trees/commits) & packfiles · REPL clients over a protocol · concurrent load generation & latency percentiles · graph algorithms & topology simulation · log parsing & visualization · browser APIs (Manifest V3)
**Prerequisites:** Phase 5 (Redis Server for the Redis CLI; servers for the Load Tester), Phase 2 (file I/O for Git objects).

| # | Challenge | Diff | Lang | ⏱️ | Why this language / what it teaches | ✅ |
|---|-----------|------|------|----|-------------------------------------|----|
| 46 | **Git** | 🔴 | 🟦 Go | XL | The object model (blobs/trees/commits), refs, the index, packfiles, and merge basics. A landmark project. | [ ] |
| 47 | **Redis CLI** | 🔵 | 🟦 Go | M | A RESP client + REPL — pairs directly with the Phase 5 Redis Server (build server first). | [ ] |
| 48 | **Load Tester** | 🟠 | 🟦 Go | M | Concurrent HTTP load, throughput, and latency percentiles. Reuses `curl` + goroutine skills. | [ ] |
| 49 | **Network Modelling Tool** | 🟠 | 🟨 Python | L | Topology graphs, routing, and simulation — `networkx` makes the graph theory tractable. | [ ] |
| 50 | **Git Contributions Visualizer** | 🔵 | 🟨 Python | S | Parse `git log`, aggregate by date, render a heatmap (terminal/SVG). Lightweight + rewarding. | [ ] |
| 51 | **Chrome Extension** | 🔵 | 🟩 TypeScript | M | Manifest V3, content scripts, background workers, and message passing. | [ ] |

**🎓 Ordering rationale:** **Git** anchors the phase as the deepest internals project; the **Redis CLI** intentionally follows the Phase 5 Redis Server; the **Load Tester** builds on your `curl` and concurrency skills. The lighter visualizer/extension projects are good "palate cleansers" between heavy builds.

**🏁 Phase capstone:** **Git** — arguably the most respected "build it yourself" project in software.

---

## Phase 8 — Games, Interpreters & Creative Projects

> **Learning Objective:** Apply everything to interactive, intellectually rich, and fun projects. Build game loops and rendering, design small languages (lexer → parser → evaluator), and integrate third-party APIs with real auth flows.

**Key Concepts:** the game loop (input → update → render) · collision detection & simple physics · grid/state machines · minimax + alpha-beta search · operator-precedence parsing · tree-walking interpreters & environments · REST/WebSocket API integration · OAuth 2.0 (PKCE) · pagination · PDF/image generation
**Prerequisites:** varies per sub-track (noted below); the interpreters lean on the Phase 1 parsing mindset.

### 8A — Games (interactive & visual)

| # | Challenge | Diff | Lang | ⏱️ | Why this language / what it teaches | ✅ |
|---|-----------|------|------|----|-------------------------------------|----|
| 52 | **Snake** | 🟢→🔵 | 🟨 Python | S | The minimal game loop: grid state + keyboard input via `curses`/`pygame`. | [ ] |
| 53 | **Minesweeper** | 🔵 | 🟩 TypeScript | M | Grid generation, flood-fill reveal, and mine placement. | [ ] |
| 54 | **Pong** | 🔵 | 🟨 Python | M | Velocity, collision, and a real-time render loop with two-player input. | [ ] |
| 55 | **Tetris** | 🔵→🟠 | 🟩 TypeScript | L | Rotation matrices, line clearing, and gravity/speed ramping on a canvas. | [ ] |
| 56 | **Space Invaders** | 🟠 | 🟩 TypeScript | L | Sprite & projectile management, wave spawning, and collision systems. | [ ] |
| 57 | **Chess** | 🔴 | 🟨 Python | XL | Legal move generation, check/checkmate, and a minimax + alpha-beta AI. | [ ] |

### 8B — Interpreters & language tools

| # | Challenge | Diff | Lang | ⏱️ | Why this language / what it teaches | ✅ |
|---|-----------|------|------|----|-------------------------------------|----|
| 58 | **Calculator** | 🟢→🔵 | 🟨 Python | S | Operator precedence via Pratt / shunting-yard parsing, then evaluation. | [ ] |
| 59 | **Lisp Interpreter** | 🟠 | 🟨 Python | L | A full pipeline — tokenizer → parser → evaluator → environment + closures. The ultimate parsing payoff. | [ ] |

### 8C — API-driven & creative tools

| # | Challenge | Diff | Lang | ⏱️ | Why this language / what it teaches | ✅ |
|---|-----------|------|------|----|-------------------------------------|----|
| 60 | **Discord Bot** | 🔵 | 🟨 Python | M | The Discord gateway (WebSocket) + command handling via `discord.py`. | [ ] |
| 61 | **Spotify Client** | 🔵→🟠 | 🟩 TypeScript | L | OAuth 2.0 PKCE, REST consumption, and playback concepts. | [ ] |
| 62 | **Spotify Playlist Backup** | 🔵 | 🟨 Python | S | API pagination, serialization, and scheduled backups. Reuses Spotify auth from #61. | [ ] |
| 63 | **Social Media Tool** | 🔵 | 🟨 Python | M | Multi-platform API integration, scheduling, and analytics. | [ ] |
| 64 | **LinkedIn Carousel Generator** | 🔵 | 🟨 Python | M | Template-driven PDF/image generation and layout. | [ ] |

**🎓 Ordering rationale:** games ramp **Snake → Pong → Tetris → Space Invaders → Chess** (each adds physics, then complex state, then AI). The interpreters (**Calculator → Lisp**) are the climactic callback to the Phase 1 JSON Parser — same skeleton, full language. Do **Spotify Client before Playlist Backup** so the OAuth flow is already in place.

**🏁 Phase capstone:** the **Lisp Interpreter** — and a satisfying bookend, since the curriculum opened with a parser.

---

## 🌐 Language Distribution

| Lang | Count | Where it's used & why |
|------|-------|-----------------------|
| 🟦 **Go** | 35 | CLI tools, networking, servers, infrastructure, Git, load testing — single static binaries, superb I/O, goroutines. |
| 🟨 **Python** | 18 | Parsing, data, games, interpreters, API/creative tools — expressive, batteries-included, great for algorithms. |
| 🟩 **TypeScript** | 11 | Web apps, real-time, browser tools, canvas games — the natural home of the modern web stack. |
| 🟧 **Java** | 0\* | Reserved as an alternate — strong for the server tier (Redis/Memcached/NATS) where its threading + type system shine. |

\* *Java is a first-class option for any challenge; it's simply not the default pick. Re-implementing a server or two in Java is an excellent way to compare concurrency models.*

---

## 🔗 Dependency Graph (the orderings that matter)

Later challenges assume skills from earlier ones. These edges are the *load-bearing* dependencies:

```
JSON Parser (1) ─┬─→ jq (16) ──→ yq (17)
                 ├─→ Redis Server (32) ──→ Redis CLI (47)
                 ├─→ Memcached (33)
                 └─→ Calculator (58) ──→ Lisp Interpreter (59)

Huffman (2) ──→ tar (19) ;  xxd (15) ──→ DNS/NTP binary protocols (23–25)

wc (5) ──→ cat/head/cut (6–8) ──→ sort/grep/sed (11–13) ──→ Shell (22)

curl (21) ─┬─→ HTTP Forward Proxy (29)
           ├─→ Web Server (30) ──→ Load Balancer (31)
           └─→ Load Tester (48)

DNS Resolver (23) ──→ DNS Forwarder (24)
Web Server (30) ──→ URL Shortener (37) ──→ Pastebin (38)
Snake (52) ──→ Pong (54) ──→ Tetris (55) ──→ Space Invaders (56)
Spotify Client (61) ──→ Spotify Playlist Backup (62)
```

**Read it as:** "to build the node on the right, you'll want the skills from the node on the left." Cross a `──→` and you should already have ticked the source box.

---

## 📖 README-First Policy

**Every single challenge** begins with a teaching README — written *before* any implementation code. This is the project's core learning contract.

> **README structure (required for all 64 challenges):**
> 1. **What We're Building** — the tool and the problem it solves.
> 2. **Core Concepts** — the CS/systems ideas that make it work.
> 3. **How It Works in the Real World** — how the production tool/spec actually behaves.
> 4. **Architecture** — the high-level design of *your* implementation.
> 5. **Step-by-Step Implementation** — an incremental build plan.
> 6. **Testing** — how you verify correctness (ideally against the real tool).
> 7. **Key Takeaways** — the lessons, distilled.
> 8. **Further Reading** — specs, RFCs, papers, and deeper dives.

The README is the most valuable artifact of each challenge. **Code without a teaching README is incomplete.**

---

## 🧭 Suggested Learning Paths

Not everyone needs all 64. Pick a path that matches your goal:

### 🏃 Sprint Path — 10 challenges, maximum breadth
For the broadest coverage in the least time:
`JSON Parser (1) → wc (5) → grep (12) → Shell (22) → DNS Resolver (23) → Web Server (30) → Redis Server (32) → URL Shortener (37) → Git (46) → Lisp Interpreter (59)`

### 🛠️ Systems Engineer Path
Phases **1 → 5**, then **Git (46)** and **Docker (36)**. Everything sockets, servers, and OS internals.

### 🌐 Full-Stack / Web Path
Phase **1–2** (foundations), then jump to Phase **5** (Web Server) → Phase **6** (all apps) → **Chrome Extension (51)** + **Spotify Client (61)**.

### 🔌 Protocols & Networking Path
`curl (21) → Phase 4 (23–29) → Web Server (30) → Load Balancer (31) → Redis Server (32) → Load Tester (48)`.

### 🎮 Fun-First Path
Start with Phase **8A games (52–57)** for momentum, then loop back to Phase **1–3** when you want the underlying rigor.

### 🧠 Language & Interpreters Path
`JSON Parser (1) → jq (16) → Calculator (58) → Lisp Interpreter (59)` — the parsing throughline, start to finish.

---

## ✅ Progress at a Glance

| Phase | Challenges | Done |
|-------|-----------|------|
| 1 — Foundations | 1–4 | ☐ 0 / 4 |
| 2 — Core Unix | 5–15 | ☐ 0 / 11 |
| 3 — Advanced CLI | 16–22 | ☐ 0 / 7 |
| 4 — Networking | 23–29 | ☐ 0 / 7 |
| 5 — Servers | 30–36 | ☐ 0 / 7 |
| 6 — Applications | 37–45 | ☐ 0 / 9 |
| 7 — Dev Tools | 46–51 | ☐ 0 / 6 |
| 8 — Games & Creative | 52–64 | ☐ 0 / 13 |
| **Total** | **1–64** | **☐ 0 / 64** |

---

*Curriculum v2 — regenerated 2026-06-08. Covers all 64 challenges from the project README, sourced from [codingchallenges.fyi](https://codingchallenges.fyi/). Update the checkboxes and the progress table as you go.*
