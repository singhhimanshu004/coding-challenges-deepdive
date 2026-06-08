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
