# Decisions Log

## Decision: Learning Curriculum Structure

**Date:** 2026-06-08
**Author:** Grant (Lead)
**Status:** Proposed

### Decision

Created `CURRICULUM.md` organizing all 65 challenges into 8 progressive learning phases.

### Phases

1. **Foundations: Parsing & Data** (4 challenges) — JSON parser, compression, data structures
2. **Core Unix: Text Processing** (11 challenges) — wc through diff/xxd
3. **Advanced CLI & Scripting** (7 challenges) — jq, yq, tar, curl, shell
4. **Networking Fundamentals** (7 challenges) — DNS, NTP, traceroute, port scanner, netcat, proxy
5. **Servers & Infrastructure** (7 challenges) — web server, load balancer, Redis, Memcached, NATS, Docker
6. **Applications & Full-Stack** (9 challenges) — URL shortener through Dropbox
7. **Developer Tools & Internals** (6 challenges) — Git, Redis CLI, load tester, visualizer, extension
8. **Games, Interpreters & Creative** (14 challenges) — games, Lisp interpreter, bots, API tools

### Progression Rationale

- **Parsing first:** JSON Parser is the foundational skill that unlocks structured-data tools (jq, yq) and protocol parsing (Redis RESP, HTTP, DNS).
- **CLI before networking:** Stdin/stdout, buffered I/O, and file handling are prerequisites for socket programming.
- **Networking before servers:** Understanding protocols as a client is simpler than implementing them as a server.
- **Servers before applications:** Applications build on HTTP server/database patterns established in Phase 5.
- **Games and interpreters last:** These are self-contained and require varied skills from all prior phases. They serve as capstone projects.

### Language Strategy

- Go for systems work (CLI, networking, servers) — 36 challenges
- Python for data, scripting, games, interpreters — 18 challenges
- TypeScript for web/full-stack applications — 11 challenges
- Java available as alternate for any challenge
