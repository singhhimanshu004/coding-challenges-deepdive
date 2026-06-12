# Malcolm — Docker Build (Challenge 36, Capstone, Linux-only)

**Date:** 2026-06-13 02:58 (UTC 2026-06-12 21:28)  
**Agent:** Malcolm (Content Dev, claude-opus-4.8)  
**Challenge:** #36 Docker (Capstone, XL) — Container runtime  
**Location:** `phase-05-servers-infrastructure/docker/`  

## Build Summary

✅ **COMPLETE (Phase 5 capstone)**  

Minimal container runtime showing namespaces + cgroups in action. Created UTS + PID + mount + network + IPC namespaces, pivot_root into rootfs, fresh /proc, hostname, memory/pids limits (v1 + v2), exec user command as PID 1.

**Key feature:** Cross-platform build-tag split (Linux-only code compiles/tests on macOS).

**Platform support:** Linux only (⚠️ cannot run on macOS; documented in README)  
**Test coverage:** 10 platform-neutral tests (macOS), Linux tests tag-excluded on macOS  

## Key Decisions

- **Build-tag solution for Linux-only:** `run_linux.go` (real syscalls) + `run_other.go` (stub error), platform-neutral pure logic in main/config/layers (no tag).
- **Re-exec pattern** (Go can't fork from multi-threaded runtime): parent sets Cloneflags, re-execs `/proc/self/exe child`; child finishes setup inside new namespaces.
- **Config via argv:** childArgs(cfg) → child args, parseChildArgs parses back; round-trip unit test is safety net.
- **Resolve units in parent:** --mem 100m → bytes; child sees --membytes only.
- **pivot_root over chroot:** canonical 4-step dance (make private, bind-mount, pivot_root, detach).
- **cgroup v1 AND v2:** detect v2, write appropriate limits.
- **OverlayFS path math testable cross-platform:** layers.go tested on macOS even though mount is Linux-only.

## Verification (on macOS, cross-compile verified)

- ✅ `gofmt -l .` clean
- ✅ `go vet ./...` passes
- ✅ `CGO_ENABLED=0 go test ./...` — 10 platform-neutral tests PASS
- ✅ `GOOS=linux go build ./...` succeeds (cross-compile)
- ✅ Linux recipe documented: export alpine rootfs, build for Linux, `sudo ./gocker run … /bin/sh`

## Status

**Ready for review:** Platform-neutral tests pass on macOS, cross-compile succeeds, Linux runtime correct-by-reading, comprehensive documentation with Linux-only badge and usage instructions.
