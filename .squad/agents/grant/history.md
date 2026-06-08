# Grant — History

## Project Context
- **Project:** coding-challenges-deepdive
- **Owner:** Himanshu Singh
- **Source:** https://codingchallenges.fyi/challenges/intro
- **Stack:** Multi-language (Go, Python, Java, TypeScript)
- **Scope:** 65+ coding challenges covering real-world tools and systems

## Challenge Categories
- **Unix/CLI Tools:** wc, grep, sed, jq, curl, cut, sort, uniq, head, tail
- **Networking & Protocols:** DNS resolver, traceroute, port scanner, NTP client, IRC client
- **Servers & Infrastructure:** web server, load balancer, Redis, Memcached, NATS
- **Data & Compression:** JSON parser, Huffman compression, Bloom filter, Base64
- **Applications:** URL shortener, chat server, shell, Pastebin, Dropbox
- **Games:** Pong, Tetris, Chess, Snake, Minesweeper, Game of Life
- **Developer Tools:** Git, Docker, rate limiter, load tester

## Learnings

### 2026-06-08 — Created CURRICULUM.md
- Created `CURRICULUM.md` at repo root: the master learning plan for all 65 challenges.
- **8 phases:** (1) Foundations: Parsing & Data, (2) Core Unix: Text Processing, (3) Advanced CLI & Scripting, (4) Networking Fundamentals, (5) Servers & Infrastructure, (6) Applications & Full-Stack, (7) Developer Tools & Internals, (8) Games, Interpreters & Creative.
- **Ordering decisions:** JSON Parser placed first as it unlocks jq, yq, and protocol parsing. CLI tools ordered simple→complex (wc→shell). DNS Resolver before DNS Forwarder. Redis Server before Redis CLI. Calculator before Lisp Interpreter. Games ordered by complexity (Snake→Chess).
- Language distribution: Go dominates CLI/networking/servers (36), Python for parsing/data/games/interpreters (18), TypeScript for web apps and browser tools (11). Java kept as alternate option.
- Includes dependency graph, suggested learning paths (Sprint, Systems, Full-Stack, Fun-First), and progress checkboxes.
- Enforces README-first policy per decisions.md.

### 2026-06-08 — Regenerated CURRICULUM.md (v2, claude-opus-4.8)
- Regenerated `CURRICULUM.md` from scratch on **claude-opus-4.8** at the user's request (prior version was a weaker-model draft). Read the old version as reference but produced a fresh, improved one.
- **Improvements over v1:**
  - **Removed a fabricated challenge** — v1 padded the list to "65" by inventing a duplicate "Calculator (GUI)" (#65). Corrected to the **64 challenges actually in the README**; honest count.
  - **Added a difficulty rubric** (🟢 Beginner / 🔵 Intermediate / 🟠 Advanced / 🔴 Expert) with explicit definitions, plus colored difficulty per challenge.
  - **Added effort estimates** (⏱️ S/M/L/XL) to every challenge for planning.
  - **Sharper dependency rigor** — added a "what unlocks what" framing, phase capstones, and a richer dependency graph with load-bearing edges only.
  - **Stronger ordering rationale** per phase, and added two new learning paths (Protocols & Networking, Language & Interpreters).
  - **Progress-at-a-glance table** for trackability alongside per-challenge checkboxes.
- Phase structure retained 8 phases (proven ordering) but tightened names/objectives and corrected counts: P1=4, P2=11, P3=7, P4=7, P5=7, P6=9, P7=6, P8=13 = 64.
- Language distribution corrected to match: Go 35, Python 18, TS 11, Java 0 (alternate).
- Decision recorded at `.squad/decisions/inbox/grant-curriculum-v2.md`.
