# 📚 Coding Challenges Deep Dive — Learning Curriculum

> **Master learning plan for 65+ coding challenges from [codingchallenges.fyi](https://codingchallenges.fyi/).**
> Designed as a progressive, structured path from foundational skills to expert-level systems programming.

---

## How to Use This Curriculum

This curriculum organizes every challenge into **8 sequential learning phases**. Each phase builds on skills from the previous one, so working through them in order gives you the strongest foundation. However, phases are also designed to be somewhat self-contained — if you already have experience with a topic, you can jump ahead.

**Suggested cadence:**
- **Full-time learner:** ~1 challenge per 2–3 days → complete in ~6 months
- **Part-time / evenings:** ~1 challenge per week → complete in ~15 months
- **Pick-and-choose:** Focus on one phase that interests you; just check the prerequisites

**For each challenge, you will:**
1. Read the challenge requirements on [codingchallenges.fyi](https://codingchallenges.fyi/)
2. Write a comprehensive **README.md first** (What We're Building → Core Concepts → Architecture → Step-by-Step Implementation → Testing → Key Takeaways → Further Reading)
3. Implement the solution in the recommended language
4. Test thoroughly, then review and iterate

**Progress tracking:** Use the checkboxes `[ ]` below to mark completed challenges.

**Language legend:** 🟦 Go · 🟨 Python · 🟧 Java · 🟩 TypeScript

---

## Curriculum Overview

| Phase | Name | Focus | Difficulty | # Challenges |
|-------|------|-------|------------|--------------|
| 1 | Foundations: Parsing & Data | JSON, compression, data structures | Beginner | 4 |
| 2 | Core Unix: Text Processing | Classic CLI tools, stdin/stdout, pipes | Beginner–Intermediate | 11 |
| 3 | Advanced CLI & Scripting | Complex CLI tools, regex, scheduling | Intermediate | 7 |
| 4 | Networking Fundamentals | Protocols, DNS, sockets, scanning | Intermediate | 7 |
| 5 | Servers & Infrastructure | HTTP servers, caches, brokers, containers | Advanced | 7 |
| 6 | Applications & Full-Stack | Real-world apps with storage, auth, APIs | Advanced | 9 |
| 7 | Developer Tools & Internals | Git, testing tools, dev workflow | Advanced–Expert | 6 |
| 8 | Games, Interpreters & Creative | Game loops, rendering, language design | Intermediate–Expert | 14 |

**Total: 65 challenges**

---

## Phase 1 — Foundations: Parsing & Data

> **Learning Objective:** Master fundamental data formats (JSON, binary encoding), compression algorithms, and probabilistic data structures. These skills underpin nearly every challenge that follows.

**Key Concepts:** Lexing & parsing, tree structures, binary encoding, hash functions, bit manipulation
**Prerequisites:** Comfortable reading/writing code in at least one language; basic data structures (arrays, maps, trees)

| # | Challenge | Difficulty | Language | Rationale | Status |
|---|-----------|------------|----------|-----------|--------|
| 1 | JSON Parser | Beginner | 🟨 Python | Excellent for rapid prototyping a recursive-descent parser; clear string handling | [ ] |
| 2 | Huffman Compression | Intermediate | 🟦 Go | Bit-level I/O and binary tree traversal; Go's `io` package is ideal | [ ] |
| 3 | Bloom Filter Spell Checker | Intermediate | 🟦 Go | Hash functions and bit arrays; Go's performance makes large datasets practical | [ ] |
| 4 | QR Code Generator | Intermediate | 🟨 Python | Matrix manipulation and encoding math; Python's PIL/numpy make image output easy | [ ] |

**Why this phase is first:** JSON parsing teaches lexing/tokenizing/recursive descent — the same skills needed for `jq`, `yq`, config parsers, and the Lisp interpreter later. Huffman compression introduces binary I/O used in `tar`, `xxd`, and networking. Bloom filters introduce hash-based data structures used in caches and databases.

---

## Phase 2 — Core Unix: Text Processing

> **Learning Objective:** Build the fundamental Unix text-processing toolkit. Master stdin/stdout streaming, line-by-line processing, buffered I/O, and the Unix philosophy of composable tools.

**Key Concepts:** Stream processing, buffered I/O, file descriptors, Unicode handling, exit codes, command-line argument parsing
**Prerequisites:** Phase 1 (parsing fundamentals); comfort with terminal/shell basics

| # | Challenge | Difficulty | Language | Rationale | Status |
|---|-----------|------------|----------|-----------|--------|
| 5 | wc (Word Count) | Beginner | 🟦 Go | Perfect starter: simple I/O, flags, stdin; Go's `bufio.Scanner` is purpose-built | [ ] |
| 6 | cat | Beginner | 🟦 Go | File concatenation and stream forwarding; introduces multi-file handling | [ ] |
| 7 | head | Beginner | 🟦 Go | Line/byte counting with early termination; builds on `wc` patterns | [ ] |
| 8 | cut | Beginner | 🟦 Go | Field/delimiter parsing within lines; introduces column-oriented thinking | [ ] |
| 9 | uniq | Beginner | 🟦 Go | Adjacent-line deduplication; state tracking across a stream | [ ] |
| 10 | tr | Beginner–Intermediate | 🟦 Go | Character-level translation/deletion; rune handling and character classes | [ ] |
| 11 | sort | Intermediate | 🟦 Go | Sorting algorithms, comparators, memory management for large files | [ ] |
| 12 | grep | Intermediate | 🟦 Go | Pattern matching, regex basics, recursive file walking | [ ] |
| 13 | sed | Intermediate | 🟦 Go | Stream editing, regex substitution, in-place file modification | [ ] |
| 14 | diff | Intermediate | 🟦 Go | Longest common subsequence (LCS), edit distance, unified diff format | [ ] |
| 15 | xxd | Intermediate | 🟦 Go | Hex dump / binary inspection; byte-level I/O and formatting | [ ] |

**Why Go for CLI tools:** Go compiles to a single static binary, has excellent I/O primitives (`bufio`, `io`, `os`), and mirrors how real Unix tools are increasingly being rewritten (e.g., `ripgrep` in Rust follows similar patterns). It teaches systems-level thinking without C's complexity.

---

## Phase 3 — Advanced CLI & Scripting

> **Learning Objective:** Build more complex command-line tools that involve structured data processing, job scheduling, archiving, and HTTP interaction. Bridges CLI fundamentals with networking.

**Key Concepts:** Structured data formats (JSON/YAML), cron expressions, archive formats, HTTP client, argument parsing, process management
**Prerequisites:** Phase 1 (JSON Parser), Phase 2 (basic CLI patterns)

| # | Challenge | Difficulty | Language | Rationale | Status |
|---|-----------|------------|----------|-----------|--------|
| 16 | jq (JSON processor) | Intermediate | 🟦 Go | Builds directly on Phase 1 JSON parser; expression evaluation, tree walking | [ ] |
| 17 | yq (YAML processor) | Intermediate | 🟨 Python | YAML superset of JSON; Python's `pyyaml` ecosystem strong; good contrast to Go jq | [ ] |
| 18 | xargs | Intermediate | 🟦 Go | Process spawning, argument batching, parallel execution | [ ] |
| 19 | tar | Intermediate | 🟦 Go | Archive format parsing, file metadata, binary headers; uses Phase 1 binary skills | [ ] |
| 20 | crontab | Intermediate | 🟦 Go | Cron expression parsing, time scheduling, daemon-style execution | [ ] |
| 21 | curl | Intermediate–Advanced | 🟦 Go | HTTP client from scratch: TCP sockets, HTTP/1.1 protocol, TLS, headers | [ ] |
| 22 | Shell | Advanced | 🟦 Go | Tying it all together: process management, pipes, redirects, built-ins, PATH resolution | [ ] |

**Why Shell comes last in CLI:** The shell is the _orchestrator_ of Unix tools. Building it after you've built the individual tools gives deep appreciation for how pipes, redirects, and process management work under the hood.

---

## Phase 4 — Networking Fundamentals

> **Learning Objective:** Understand how the internet works at the protocol level. Build tools that speak DNS, NTP, ICMP, and raw TCP/UDP. Learn socket programming, binary protocol parsing, and network debugging.

**Key Concepts:** TCP/UDP sockets, DNS protocol, ICMP, binary protocol encoding/decoding, network byte order, timeouts, connection management
**Prerequisites:** Phase 2–3 (comfortable with binary I/O from `xxd`, `tar`, and HTTP basics from `curl`)

| # | Challenge | Difficulty | Language | Rationale | Status |
|---|-----------|------------|----------|-----------|--------|
| 23 | DNS Resolver | Intermediate | 🟦 Go | UDP sockets, DNS wire format, recursive resolution; Go's `net` package excels | [ ] |
| 24 | DNS Forwarder | Intermediate | 🟦 Go | Builds on resolver; adds server-side UDP listening, request forwarding, caching | [ ] |
| 25 | NTP Client | Intermediate | 🟦 Go | UDP protocol, binary struct packing, time synchronization concepts | [ ] |
| 26 | Traceroute | Intermediate | 🟦 Go | Raw sockets, ICMP, TTL manipulation, network path discovery | [ ] |
| 27 | Port Scanner | Intermediate | 🟦 Go | TCP connect/SYN scanning, concurrency (goroutines), timeouts | [ ] |
| 28 | Netcat | Intermediate–Advanced | 🟦 Go | TCP/UDP client-server, bidirectional I/O, the "Swiss army knife" of networking | [ ] |
| 29 | HTTP Forward Proxy | Advanced | 🟦 Go | HTTP CONNECT, request rewriting, TLS tunneling, connection pooling | [ ] |

**Why Go dominates networking:** Go was designed for networked systems. Goroutines make concurrent connections trivial, the `net` package provides clean socket abstractions, and the compiled binary means no runtime dependencies on target machines.

---

## Phase 5 — Servers & Infrastructure

> **Learning Objective:** Build production-style server software. Master concurrent connection handling, protocol implementation, caching strategies, message queuing, rate limiting, and container isolation.

**Key Concepts:** Event loops, connection multiplexing, protocol state machines, in-memory data stores, pub/sub patterns, token bucket / sliding window algorithms, Linux namespaces & cgroups
**Prerequisites:** Phase 4 (socket programming, TCP/UDP); Phase 1 (parsing for RESP/memcached protocols)

| # | Challenge | Difficulty | Language | Rationale | Status |
|---|-----------|------------|----------|-----------|--------|
| 30 | Web Server | Advanced | 🟦 Go | HTTP/1.1 from scratch: request parsing, routing, static files, keep-alive | [ ] |
| 31 | Load Balancer | Advanced | 🟦 Go | Reverse proxy, health checks, round-robin/least-connections, connection forwarding | [ ] |
| 32 | Redis Server | Advanced | 🟦 Go | RESP protocol, in-memory key-value store, expiry, persistence (RDB/AOF) | [ ] |
| 33 | Memcached Server | Advanced | 🟦 Go | Text/binary protocol, LRU cache eviction, slab allocation concepts | [ ] |
| 34 | NATS Message Broker | Advanced | 🟦 Go | Pub/sub, subject routing, client connection management, message delivery guarantees | [ ] |
| 35 | Rate Limiter | Intermediate–Advanced | 🟦 Go | Token bucket, sliding window algorithms; middleware pattern; distributed considerations | [ ] |
| 36 | Docker | Expert | 🟦 Go | Linux namespaces, cgroups, layered filesystems, container runtime fundamentals | [ ] |

**Architecture note:** Each server challenge follows the same learning arc: (1) understand the protocol spec, (2) build a minimal single-threaded version, (3) add concurrency, (4) add persistence/advanced features. This mirrors how real infrastructure software evolves.

---

## Phase 6 — Applications & Full-Stack

> **Learning Objective:** Build complete, user-facing applications with databases, APIs, authentication, real-time communication, and file storage. Learn system design trade-offs for real products.

**Key Concepts:** REST API design, WebSockets, database schema design, authentication/authorization, file storage, CRUD operations, real-time updates, encryption
**Prerequisites:** Phase 5 (web server, basic HTTP); comfortable with at least one database

| # | Challenge | Difficulty | Language | Rationale | Status |
|---|-----------|------------|----------|-----------|--------|
| 37 | URL Shortener | Intermediate | 🟩 TypeScript | REST API, database, base62 encoding; TS/Node ideal for quick web APIs | [ ] |
| 38 | Pastebin | Intermediate | 🟩 TypeScript | Similar to URL shortener with text storage, expiry, syntax highlighting | [ ] |
| 39 | Realtime Chat | Advanced | 🟩 TypeScript | WebSockets, rooms, message broadcasting; Node.js excels at real-time I/O | [ ] |
| 40 | IRC Client | Advanced | 🟨 Python | IRC protocol (RFC 1459), TCP text protocol, event-driven client architecture | [ ] |
| 41 | Google Keep | Intermediate–Advanced | 🟩 TypeScript | CRUD app with labels, reminders, rich notes; full-stack patterns | [ ] |
| 42 | Password Manager | Advanced | 🟨 Python | Encryption (AES-256), key derivation (PBKDF2/Argon2), secure storage, clipboard | [ ] |
| 43 | Data Privacy Vault | Advanced | 🟦 Go | Tokenization, encryption-at-rest, access policies, audit logging | [ ] |
| 44 | Dropbox | Expert | 🟦 Go | File sync, chunked upload, delta sync, conflict resolution, filesystem watching | [ ] |
| 45 | Scheduling App | Advanced | 🟩 TypeScript | Calendar logic, recurring events, timezone handling, availability calculation | [ ] |

---

## Phase 7 — Developer Tools & Internals

> **Learning Objective:** Build the tools that developers use every day. Understand version control internals, load testing methodology, network simulation, and browser extension architecture.

**Key Concepts:** Content-addressable storage, object graphs (trees, commits, blobs), HTTP/TCP benchmarking, statistical analysis, network topology modeling, browser APIs
**Prerequisites:** Phase 4–5 (networking, server architecture); Phase 2 (file I/O for Git objects)

| # | Challenge | Difficulty | Language | Rationale | Status |
|---|-----------|------------|----------|-----------|--------|
| 46 | Git | Expert | 🟦 Go | Object model (blobs, trees, commits), packfiles, refs, merge algorithms | [ ] |
| 47 | Redis CLI | Intermediate | 🟦 Go | RESP protocol client, REPL interface; pairs perfectly with Phase 5 Redis Server | [ ] |
| 48 | Load Tester | Advanced | 🟦 Go | Concurrent HTTP requests, latency percentiles, throughput measurement; goroutines shine | [ ] |
| 49 | Network Modelling Tool | Advanced | 🟨 Python | Graph algorithms, topology simulation, routing; Python's networkx ideal | [ ] |
| 50 | Git Contributions Visualizer | Intermediate | 🟨 Python | Git log parsing, date aggregation, terminal/SVG rendering | [ ] |
| 51 | Chrome Extension | Intermediate | 🟩 TypeScript | Browser APIs, manifest v3, content scripts, message passing | [ ] |

---

## Phase 8 — Games, Interpreters & Creative Projects

> **Learning Objective:** Apply everything you've learned to creative, interactive, and intellectually rich projects. Build game engines, language interpreters, bots, and API-driven tools.

**Key Concepts:** Game loops, rendering (terminal/canvas), input handling, state machines, language design (lexer → parser → evaluator), API integration, OAuth flows
**Prerequisites:** Varies per challenge (noted below); generally assumes comfort with all prior phases

### 8A — Games (Interactive & Visual)

| # | Challenge | Difficulty | Language | Rationale | Status |
|---|-----------|------------|----------|-----------|--------|
| 52 | Snake | Beginner–Intermediate | 🟨 Python | Simple game loop, grid state, keyboard input; `curses` or `pygame` | [ ] |
| 53 | Minesweeper | Intermediate | 🟩 TypeScript | Grid logic, flood-fill reveal, mine placement; web canvas or terminal | [ ] |
| 54 | Pong | Intermediate | 🟨 Python | Physics (velocity, collision), rendering loop, two-player input | [ ] |
| 55 | Tetris | Intermediate–Advanced | 🟩 TypeScript | Piece rotation matrices, line clearing, increasing speed; browser canvas ideal | [ ] |
| 56 | Space Invaders | Advanced | 🟩 TypeScript | Sprite management, projectile systems, wave spawning, collision detection | [ ] |
| 57 | Chess | Expert | 🟨 Python | Move generation, check/checkmate detection, basic AI (minimax + alpha-beta) | [ ] |

### 8B — Interpreters & Language Tools

| # | Challenge | Difficulty | Language | Rationale | Status |
|---|-----------|------------|----------|-----------|--------|
| 58 | Calculator | Beginner–Intermediate | 🟨 Python | Expression parsing (Pratt/shunting-yard), operator precedence, evaluation | [ ] |
| 59 | Lisp Interpreter | Advanced | 🟨 Python | Full language: tokenizer → parser → evaluator → environment; Python's dynamic nature fits | [ ] |

### 8C — API-Driven & Creative Tools

| # | Challenge | Difficulty | Language | Rationale | Status |
|---|-----------|------------|----------|-----------|--------|
| 60 | Discord Bot | Intermediate | 🟨 Python | Discord API/WebSocket gateway, command handling; `discord.py` ecosystem | [ ] |
| 61 | Spotify Client | Intermediate–Advanced | 🟩 TypeScript | OAuth 2.0 PKCE, REST API consumption, audio playback concepts | [ ] |
| 62 | Spotify Playlist Backup | Intermediate | 🟨 Python | API pagination, data serialization, scheduled backups | [ ] |
| 63 | Social Media Tool | Intermediate | 🟨 Python | Multi-platform API integration, content scheduling, analytics | [ ] |
| 64 | LinkedIn Carousel Generator | Intermediate | 🟨 Python | PDF generation, template rendering, image manipulation | [ ] |
| 65 | Calculator (GUI) | Intermediate | 🟩 TypeScript | DOM manipulation, event handling, display formatting (if web-based variant) | [ ] |

---

## Language Distribution Summary

| Language | Count | Primary Use Cases |
|----------|-------|-------------------|
| 🟦 Go | 36 | CLI tools, networking, servers, infrastructure, dev tools |
| 🟨 Python | 18 | Parsing, data processing, games, interpreters, API tools, scripting |
| 🟩 TypeScript | 11 | Web apps, browser tools, interactive games, full-stack applications |
| 🟧 Java | 0* | Available as alternate for any challenge; especially strong for enterprise patterns |

*\*Java is kept as an option for learners who prefer it. Any challenge can be done in Java — it's particularly well-suited for the server/infrastructure challenges (Redis, Memcached, NATS) where its threading model and type system shine.*

---

## Dependency Graph (Key Orderings)

These orderings matter — later challenges assume knowledge from earlier ones:

```
JSON Parser (1) ──→ jq (16) ──→ yq (17)
                ──→ Redis Server (32) ──→ Redis CLI (47)
                ──→ Web Server (30) ──→ Load Balancer (31)
                                    ──→ HTTP Forward Proxy (29)
                                    ──→ URL Shortener (37)

wc (5) ──→ cat/head/cut (6-8) ──→ sort/grep/sed (11-13) ──→ Shell (22)

DNS Resolver (23) ──→ DNS Forwarder (24)
curl (21) ──→ Load Tester (48)
Huffman (2) ──→ tar (19)
Calculator (58) ──→ Lisp Interpreter (59)
Snake (52) ──→ Pong (54) ──→ Tetris (55) ──→ Space Invaders (56)
```

---

## README-First Policy 📖

**Every single challenge** follows the README-first learning mandate:

> Before writing any code, create a comprehensive `README.md` with this structure:
>
> 1. **What We're Building** — What is this tool? What problem does it solve?
> 2. **Core Concepts** — The CS/systems concepts that make it work
> 3. **How It Works in the Real World** — How does the production version work?
> 4. **Architecture** — High-level design of our implementation
> 5. **Step-by-Step Implementation** — Incremental build plan
> 6. **Testing Strategy** — How to verify correctness
> 7. **Key Takeaways** — What you learned, distilled
> 8. **Further Reading** — Links to specs, papers, and deeper dives

The README is the most important artifact of each challenge. Code without a teaching README will not be accepted.

---

## Suggested Learning Paths

### 🏃 Sprint Path (Top 10 for maximum learning)
If you only have time for a few, these give the broadest coverage:
1. JSON Parser → 2. wc → 3. grep → 4. Shell → 5. DNS Resolver → 6. Web Server → 7. Redis Server → 8. URL Shortener → 9. Git → 10. Lisp Interpreter

### 🎯 Systems Focus
Phases 1–5, then Git and Docker from Phase 7.

### 🌐 Full-Stack Focus
Phases 1–2 (foundations), then jump to Phases 5–6 (servers + applications).

### 🎮 Fun First
Start with Phase 8 games, then circle back to Phase 2–3 for rigor.

---

*Curriculum designed 2026-06-08. Covers all 65 challenges from [codingchallenges.fyi](https://codingchallenges.fyi/).*
*Progress: ⬜ Not started*
