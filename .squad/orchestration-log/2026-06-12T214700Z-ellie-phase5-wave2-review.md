# Ellie — Phase 5 Wave 2 Review (Challenges 31, 32, 36)

**Date:** 2026-06-13 03:01 (UTC 2026-06-12 21:31)  
**Agent:** Ellie (Reviewer, claude-opus-4.8)  
**Scope:** Phase 5 Wave 2 (load-balancer #31, redis-server #32, docker #36)  

## Review Summary

✅ **ALL THREE APPROVED**

Verified each challenge via:
- `go vet ./...` (clean)
- `CGO_ENABLED=0 go test ./...` (all tests pass)
- Docker additionally: `GOOS=linux go build ./...` and `GOOS=linux go vet ./...`
- Key logic spot-checked by reading source

## Challenge Verdicts

### #31 Load Balancer — ✅ APPROVED

- Scheduling + health hand-written; `httputil.ReverseProxy` used for mechanics only (allowed).
- Round-robin distributes in order (unit test asserts 0,1,2,0,1,2; end-to-end via httptest confirms).
- Least-connections prefers least-busy, shifts as in-flight counts change.
- Health: active probes `/health`, backend marked DOWN on recovery, reused; passive markdown via 502.
- Weighted RR (3:1), random, CLI parsing all covered.
- Tests deterministic (no sleeps), use httptest backends, toggleable `/health`.

### #32 Redis Server (XL) — ✅ APPROVED

- RESP2 encoder/decoder: Simple String, Error, Integer, Bulk String, Array, null bulk ($-1), null array (*-1), nested arrays. Crafted-byte tests for Marshal/Decode, round-trip property tests (binary-safe payloads with CRLF/NUL).
- TCP server, goroutine-per-connection, background active-expiry sweeper.
- **15 commands:** PING, ECHO, SET (EX/PX/NX/XX), GET, DEL, EXISTS, EXPIRE, TTL, INCR, DECR, KEYS, APPEND, GETSET, MSET, SAVE.
- Expiry: lazy (delete-on-touch) + active sweep; TTL semantics (-2 missing, -1 no-expiry, seconds).
- Persistence: SAVE snapshot; round-trip test populates, SAVEs, shuts down, starts fresh server, confirms values AND TTL survived.
- No real redis library — go.mod has zero third-party deps.
- .gitignore excludes *.rdb and dump.rdb.

### #36 Docker (CAPSTONE, Linux-only) — ✅ APPROVED

- Build-tag split correct: `run_linux.go` + `run_linux_test.go` + `cgroup_linux.go` have `//go:build linux`; `run_other.go` is `//go:build !linux` (stub returns clear error naming GOOS/GOARCH + README pointer); platform-neutral files (no tag) compiled everywhere.
- `GOOS=linux go build ./...` and `GOOS=linux go vet ./...` succeed (Linux implementation cross-compiles).
- Default macOS `go test ./...` PASSES (neutral tests cover CLI parsing, parent→child argv round-trip, OverlayFS path math).
- Linux code correct-by-reading: re-exec pattern via `/proc/self/exe` in child mode; Cloneflags = CLONE_NEWUTS|NEWPID|NEWNS|NEWNET|NEWIPC; pivot_root with self bind-mount; cgroup memory/pids limits (v1+v2); optional user-namespace documented.
- README prominently flags Linux-only at top, gives exact Linux recipe (export rootfs, build, run privileged, integration test via hostname/ps/ls/).

## README Quality Gate — ✅ PASS (all three)

All three READMEs have 7 mandated sections (What We're Building → Core Concepts → Architecture → Step-by-Step → Testing → Key Takeaways → Further Reading), link `docs/go-quickstart.md`, explain Go idioms for Python dev (🐍 markers).

## Non-blocking nice-to-haves

- Load Balancer: "Try it live" could mention /health endpoint behavior; bundled demo backend would smooth UX.
- Redis: consider ADel, SETEX, TYPE for breadth; AOF stub already mentioned in README as future work.
- Docker: `make` target for rootfs fetch would lower Linux-user barrier (cosmetic only).

## Overall Verdict

**All three APPROVED. Phase 5 (Servers & Infrastructure) is COMPLETE.**

Notes: Did not reject over go1.22.2 LC_UUID linker abort (used `CGO_ENABLED=0`); did not reject Docker being un-runnable on macOS (expected and by design).
