# Docker — a container runtime from scratch (`gocker`)

> **Phase:** 5 — Servers & Infrastructure · **Phase Capstone** 🏁
> **Difficulty:** 🔴 · **Recommended Language:** 🟦 Go · **Effort Estimate:** XL

**Status:** ✅ Completed

> 🐧 **LINUX-ONLY RUNTIME.** This challenge uses Linux kernel primitives
> (namespaces, cgroups, `pivot_root`, OverlayFS) that **do not exist on macOS or
> Windows**. The code is written so that it **builds, vets, and tests cleanly on
> macOS** — but the container can only actually *run* on Linux. The "[Why macOS
> can't run this](#-why-this-is-linux-only-and-macos-cant-run-it)" section
> explains exactly why and how the build tags make cross-platform development
> work.

> 🐍➡️🐹 **New to Go?** Read the project's
> [**Go Quickstart for a Python Developer**](../../docs/go-quickstart.md) first.
> It maps the Go idioms used here — `syscall.SysProcAttr`, build tags, the
> `(value, error)` return pattern, structs, `flag` parsing — back to the Python
> you already know. This README adds 🐍 callouts wherever Go does something
> surprising.

---

## 🎯 What We're Building

`gocker` — a minimal container runtime, i.e. a tiny **Docker** you can read in an
afternoon. The headline insight this challenge teaches:

> **A container is not a virtual machine.** There's no guest kernel, no
> emulated hardware. A container is just a **normal Linux process** that the
> kernel has been asked to show a *different view of the world* — its own
> hostname, its own process tree, its own filesystem, its own network — and to
> *cap how much it can consume*.

Everything Docker does to make that happen is built from a handful of Linux
kernel features. We implement the core ones by hand:

```
gocker run [--mem 100m] [--pids 50] [--hostname web] <rootfs-dir> <command> [args...]

--mem       memory limit (100m, 512k, 1g…); 0/empty = unlimited
--pids      max number of processes; 0 = unlimited
--hostname  hostname seen inside the container (default "container")
```

Example (on a Linux host, as root):

```bash
sudo ./gocker run --mem 100m --pids 50 /path/to/alpine-rootfs /bin/sh
# you're now in a shell with its OWN hostname, PID 1, /proc, and root filesystem,
# limited to 100 MiB of RAM and 50 processes.
```

What you get inside:

- `hostname` → `container` (not the host's name) — **UTS namespace**
- `ps aux` → shows only the container's processes, your shell is **PID 1** — **PID namespace + fresh /proc**
- `/` → the image's root filesystem, the host's files are gone — **pivot_root**
- `ip addr` → an isolated, empty network stack — **network namespace**
- a fork bomb dies at 50 procs; a memory hog gets OOM-killed at 100 MiB — **cgroups**

On macOS/Windows the same binary prints:

```
gocker: this container runtime only runs on Linux — namespaces, cgroups and
pivot_root are Linux-only kernel features; you are on darwin/arm64. Build and
run it inside a Linux VM, container, or host (see README.md)
```

---

## 📚 Core Concepts

A container is the sum of **four kernel ideas**: *namespaces* (what a process can
**see**), *cgroups* (how much it can **use**), *root-filesystem switching* (what
its **`/`** is), and *layered filesystems* (how images are **stored and shared**).

### 1. Namespaces — isolating what a process can *see*

A **namespace** wraps a global system resource so that the processes inside it
have their own private instance. Linux has several kinds; we use five:

| Namespace | Clone flag | What it isolates | What you notice |
| --- | --- | --- | --- |
| **UTS** | `CLONE_NEWUTS` | hostname / domainname | `hostname` differs inside |
| **PID** | `CLONE_NEWPID` | process IDs | your command is **PID 1** |
| **Mount** | `CLONE_NEWNS` | the mount table | you can mount `/proc` without touching the host |
| **Network** | `CLONE_NEWNET` | interfaces, routes, ports | isolated, empty network stack |
| **IPC** | `CLONE_NEWIPC` | SysV IPC / message queues | no shared shared-memory with host |

There's also the **user namespace** (`CLONE_NEWUSER`), which maps *container
root* (uid 0) to an *unprivileged host uid*. It lets non-root users run
containers, but it interacts awkwardly with mounting `/proc` and networking on
some kernels, so we include it as **documented, commented-out code** rather than
on by default.

Namespaces are requested at process-creation time by passing `CLONE_NEW*` flags
to the `clone(2)` syscall. In Go we don't call `clone` directly — we set them on
`exec.Cmd`:

```go
cmd.SysProcAttr = &syscall.SysProcAttr{
    Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID |
        syscall.CLONE_NEWNS  | syscall.CLONE_NEWNET | syscall.CLONE_NEWIPC,
}
```

> 🐍 `syscall.SysProcAttr` is Go's "extra OS-specific knobs for the new
> process". In Python the closest cousin is the `preexec_fn`/`subprocess`
> machinery, but namespaces specifically would need `os.unshare`/`ctypes` — Go
> gives you a clean, typed struct for it.

#### The re-exec ("child mode") trick — the cleverest part

Here's the problem: **some setup must happen from *inside* the new namespaces**
(setting the hostname, mounting a private `/proc`, pivoting root). In C you'd
`fork()` and do that work in the child. But Go's runtime is **multi-threaded**,
and `fork()` without an immediate `exec` in a multi-threaded program is unsafe —
only one thread survives the fork, and the Go scheduler may be mid-flight on
another.

The idiomatic Go solution is to **re-execute the program itself**:

```
┌─────────────────────────┐         ┌──────────────────────────────────────┐
│ gocker run <rootfs> sh  │         │ (same binary) gocker child <rootfs> sh │
│                         │ clone() │                                        │
│ set Cloneflags ─────────┼────────▶│ now INSIDE new namespaces:             │
│ exec /proc/self/exe     │  with   │   1. Sethostname()                     │
│        child …          │  new ns │   2. apply cgroups                     │
│ wait for child          │         │   3. pivot_root into <rootfs>          │
└─────────────────────────┘         │   4. mount fresh /proc                 │
                                     │   5. exec the user's command (PID 1)   │
                                     └──────────────────────────────────────┘
```

1. The **parent** (`gocker run …`) sets the clone flags and launches
   `/proc/self/exe child …` — `/proc/self/exe` is a kernel symlink to *the
   currently running binary*, so it re-runs **us**.
2. The kernel creates the new namespaces *for that child*.
3. The **child** (`gocker child …`) wakes up already inside the isolation and
   finishes the setup, then becomes the user's command.

Both modes are the same executable; `dispatch()` in `main.go` decides which path
to take based on the first argument. This is precisely how real runtimes like
`runc` and Liz Rice's "containers from scratch" demo do it.

### 2. cgroups — limiting how much a process can *use*

Namespaces hide things; they don't *limit* things. A container with all five
namespaces can still fork-bomb the host or eat all RAM. **Control groups
(cgroups)** are the kernel feature that caps CPU, memory, process count, and I/O
for a group of tasks.

cgroups are driven through a **pseudo-filesystem** under `/sys/fs/cgroup`. The
recipe is delightfully simple — it's just files:

```bash
mkdir /sys/fs/cgroup/gocker            # creating the directory creates the group
echo 104857600 > .../gocker/memory.max # 100 MiB memory cap (cgroup v2)
echo 50        > .../gocker/pids.max    # at most 50 processes
echo $PID      > .../gocker/cgroup.procs# move this process into the group
```

There are **two incompatible layouts** in the wild, and we detect and support
both (`cgroup_linux.go`):

| | cgroup **v2** (modern default) | cgroup **v1** (legacy) |
| --- | --- | --- |
| Layout | one unified tree | one tree **per controller** |
| Memory file | `memory.max` | `memory/…/memory.limit_in_bytes` |
| Pids file | `pids.max` | `pids/…/pids.max` |
| Detect via | `/sys/fs/cgroup/cgroup.controllers` exists | (no v2 marker) |

> 🐍 No special API — you literally `os.WriteFile` integers into kernel files.
> It feels like writing to `/proc` in Python with `open(path, "w")`.

### 3. Root filesystem isolation — `chroot` vs `pivot_root`

A container needs its own `/`. Two options:

- **`chroot(2)`** — changes what `/` resolves to. Simple, but historically
  *escapable* (a process that keeps an open fd to the old root, or re-chroots,
  can break out). Fine for a demo, weak for real isolation.
- **`pivot_root(2)`** — *the real-runtime choice*. It swaps the **mount** that
  backs `/`, moves the old root aside, and lets us **unmount it entirely** so the
  container literally has no handle on the host filesystem.

We implement `pivot_root` (`run_linux.go`), the canonical four-step dance:

1. make the whole mount tree **private** (`MS_REC|MS_PRIVATE`) so our changes
   don't leak back to the host,
2. **bind-mount** the new root onto itself (`pivot_root` requires the new root to
   be a mount point),
3. `pivot_root(newRoot, newRoot/.pivot_old)` — new root becomes `/`, old root
   moves under `.pivot_old`,
4. `chdir("/")`, then **detach and remove** the old root.

After pivoting we mount a fresh `/proc` so tools like `ps` see only the
container's PID namespace.

### 4. Layered filesystems — OverlayFS and why images are cheap

A Docker **image** isn't one big blob — it's a **stack of read-only layers**
(base OS → `apt install nginx` → `COPY app` → …). **OverlayFS** unifies that
stack plus a **writable scratch layer** into a single directory, with
**copy-on-write** semantics:

```
        ┌────────────── merged (what the container sees as "/") ──────────────┐
        │  /etc/nginx.conf (from app layer)   /bin/sh (from base)   /tmp/run … │
        └─────────────────────────────────────────────────────────────────────┘
                 ▲                  ▲                    ▲
   upperdir (RW) │   lowerdir (RO, highest priority first)
   ┌─────────────┴───┐  ┌──────────┴────────┐  ┌─────────┴─────────┐
   │ container writes │  │  app layer (RO)   │  │   base OS (RO)    │
   └──────────────────┘  └───────────────────┘  └───────────────────┘
```

- **`lowerdir`** — the read-only image layers. **Order matters**: leftmost wins,
  so the *newest* layer is listed first.
- **`upperdir`** — the writable layer; every file the container creates or
  modifies lands here (copy-on-write).
- **`workdir`** — internal scratch space OverlayFS requires (same filesystem,
  must be empty).
- **`merged`** — the unified view that becomes the container's root.

Because the read-only layers are **shared** between every container made from the
same image, launching the 100th container costs almost nothing — only its tiny
`upperdir` is new. That's why `docker run` is fast.

In this challenge the **path/layer math is implemented and unit-tested**
(`layers.go`, platform-neutral) — `BuildOverlayLayout` reverses image order
correctly and `MountOptions()` renders the exact `lowerdir=…,upperdir=…,
workdir=…` string the `mount(2)` syscall needs. The actual `mount -t overlay`
only runs on Linux.

### 🐧 Why this is Linux-only (and macOS can't run it)

Namespaces, cgroups, `pivot_root`, and OverlayFS are **features of the Linux
kernel**. macOS runs the **XNU/Darwin** kernel and Windows runs the **NT**
kernel — neither has these syscalls. (This is exactly why "Docker Desktop" on a
Mac quietly runs a **Linux VM** under the hood; the containers live in that VM,
not on macOS itself.)

So we can't *run* the runtime on the dev machine — but we can still **develop**
it there, thanks to Go **build tags**:

```
run_linux.go      //go:build linux    real namespaces/cgroups/pivot_root
cgroup_linux.go   //go:build linux    real cgroup writes
run_other.go      //go:build !linux   stub: "this only runs on Linux" error

main.go, config.go, layers.go         NO tag → compiled everywhere, fully tested
```

The Go toolchain compiles **exactly one** of `run_linux.go` / `run_other.go`
depending on the target OS, so `runParent`/`runChild` are always defined exactly
once. On macOS the `!linux` stub is compiled, so `go build` and `go vet` succeed;
the Linux-requiring tests carry `//go:build linux` and are simply skipped.

> 🐍 A build tag is a compile-time `if platform == "linux"`. Python has nothing
> exactly like it — the closest is `if sys.platform == "linux":` at *runtime*,
> but Go does it at *compile* time so platform-specific syscalls never even get
> compiled on the wrong OS.

---

## 🏗️ Architecture & Design

```
docker/
├── main.go            # PLATFORM-NEUTRAL: entrypoint + dispatch (run vs child)
├── config.go          # PLATFORM-NEUTRAL: Config struct, CLI/arg parsing, size parsing
├── layers.go          # PLATFORM-NEUTRAL: OverlayFS path/layer math
├── run_linux.go       # //go:build linux:  namespaces, re-exec, pivot_root, /proc
├── cgroup_linux.go    # //go:build linux:  cgroup v1/v2 memory & pids limits
├── run_other.go       # //go:build !linux: clear "Linux only" stub
├── config_test.go     # PLATFORM-NEUTRAL tests (run on macOS)
├── layers_test.go     # PLATFORM-NEUTRAL tests (run on macOS)
├── run_linux_test.go  # //go:build linux: integration tests (skip unless root)
├── go.mod             # module gocker
└── .gitignore
```

**Design principles:**

- **Platform-neutral core, platform-specific edges.** Everything that *can* be
  OS-independent (parsing, path math, dispatch) lives in untagged files and is
  testable on any machine. Only true syscalls sit behind `//go:build linux`.
- **One binary, two modes.** The same executable is both the `run` driver and the
  re-exec'd `child`. `dispatch()` picks the mode; `childArgs()`/`parseChildArgs()`
  serialise the `Config` across the re-exec boundary (with a **round-trip test**
  guaranteeing they agree).
- **Dependency injection for testability.** Functions take `io.Writer` for
  stdout/stderr instead of using globals, so tests assert on a `bytes.Buffer` —
  the same pattern used across the other Phase 5 server challenges.
- **Limits resolved early.** `--mem 100m` is parsed into a byte count *once* in
  the parent; the child receives `--membytes 104857600` and never re-parses
  units.

---

## 🔨 Step-by-Step Implementation

1. **CLI & config (`config.go`).** Parse `run [--mem --pids --hostname] <rootfs>
   <cmd> [args…]` with the `flag` package. Key subtlety: `flag` stops at the
   first positional, so flags *after* `<cmd>` (like `/bin/ls -la`) correctly
   belong to the **container's** command, not to gocker.
2. **Size parsing (`parseSize`).** Convert `100m`/`512k`/`1g` to bytes (binary
   units). Empty/`0` means unlimited.
3. **Dispatch & re-exec (`main.go`).** `dispatch()` routes `run` → `runParent`
   and `child` → `runChild`. `childArgs()` serialises the config for the re-exec.
4. **Parent: create namespaces (`run_linux.go`).** Build
   `exec.Command("/proc/self/exe", "child", …)`, set `Cloneflags` for UTS/PID/
   mount/net/IPC, set `Unshareflags` to keep mounts private, and `Run()`.
5. **Child: hostname.** `syscall.Sethostname` — only affects the new UTS
   namespace.
6. **Child: cgroups (`cgroup_linux.go`).** Detect v1 vs v2, create the `gocker`
   group, write `memory.max`/`pids.max` (or the v1 equivalents), and add our PID.
7. **Child: pivot_root.** Make mounts private → bind-mount new root → `pivot_root`
   → `chdir("/")` → detach & remove the old root.
8. **Child: fresh `/proc`.** `syscall.Mount("proc", "/proc", "proc", …)` so `ps`
   sees only the container.
9. **Child: become the command.** Run `<cmd> [args…]` as PID 1 with a clean
   `PATH`/`HOME` environment.
10. **OverlayFS layout (`layers.go`).** Compute `lowerdir/upperdir/workdir/merged`
    and render mount options — the storage half of the image story.
11. **Non-Linux stub (`run_other.go`).** Return a clear "Linux only" error so the
    binary is friendly on macOS/Windows.

---

## 🧪 Testing Strategy

The tests are split exactly along the platform boundary so that **`go test
./...` passes on the macOS dev machine** while the real isolation is still
covered on Linux.

**Platform-neutral tests (run everywhere, incl. macOS):**

- `config_test.go` — `parseSize` (units, errors, whitespace), `parseRunArgs`
  (flags after the command stay with the command, defaults, missing-arg errors),
  the **`childArgs` ↔ `parseChildArgs` round-trip** (proves the re-exec carries
  the config faithfully), and `dispatch` (unknown command, help).
- `layers_test.go` — `BuildOverlayLayout` reverses image order correctly, cleans
  paths, and `MountOptions()` renders the exact `lowerdir=…,upperdir=…,workdir=…`
  string.

**Linux-only tests (`//go:build linux`, skipped on macOS):**

- `run_linux_test.go` — integration tests that need real namespaces/mounts.
  They `t.Skip()` unless running as **root**, so they're safe in unprivileged CI
  and meaningful when run with `sudo` on Linux.

### Running it

**On macOS (this dev machine) — build/vet/test only:**

```bash
cd phase-05-servers-infrastructure/docker
go vet ./...                 # ✅ passes (the !linux stub compiles)
CGO_ENABLED=0 go test ./...  # ✅ passes (Linux tests are tag-excluded)
GOOS=linux go build ./...    # ✅ cross-compiles the real runtime for Linux
```

> ⚠️ **macOS go1.22 LC_UUID linker note.** On some macOS toolchain combinations a
> plain `go test ./...` can abort with an LC_UUID/linker error when cgo is
> involved. If you hit it, prefix with **`CGO_ENABLED=0`** (the same workaround
> documented in the Phase 3 `curl` and Phase 5 web-server challenges). This
> package is pure-Go with no cgo, so `CGO_ENABLED=0` is always safe here.

**On Linux — actually run a container (needs root):**

```bash
# 1. Get a root filesystem to use as the image. Easiest: export one from Docker.
mkdir -p /tmp/alpine && docker export $(docker create alpine) | tar -C /tmp/alpine -xf -

# 2. Build and run.
go build -o gocker .
sudo ./gocker run --mem 100m --pids 50 /tmp/alpine /bin/sh

# Inside the container, prove the isolation:
hostname             # -> container
ps aux               # -> only your shell + ps, you are PID 1
ls /                 # -> the alpine image, not your host
cat /etc/os-release  # -> Alpine, even on an Ubuntu host

# 3. Run the privileged integration tests:
sudo go test ./...
```

---

## 💡 Key Takeaways

- **A container is a process, not a VM.** No guest kernel — just a Linux process
  with a remapped view (namespaces) and a budget (cgroups).
- **Namespaces = what you can see; cgroups = how much you can use.** You need
  *both* — namespaces alone don't stop a fork bomb.
- **The re-exec/"child mode" trick** is the idiomatic Go way to "fork into" new
  namespaces safely from a multi-threaded runtime: set `Cloneflags`, re-run
  `/proc/self/exe`, finish setup inside.
- **`pivot_root` > `chroot`** for real isolation because it lets you *unmount*
  the host root entirely.
- **OverlayFS is why images are cheap**: read-only layers are shared; only the
  thin writable `upperdir` is per-container (copy-on-write).
- **Build tags let one codebase target many OSes.** Linux syscalls live behind
  `//go:build linux`; a `//go:build !linux` stub keeps the project building and
  testing on macOS — exactly how Docker Desktop sidesteps the same problem with a
  hidden Linux VM.
- **Go idioms learned:** `syscall.SysProcAttr`/`Cloneflags`, `/proc/self/exe`
  re-exec, `flag` parsing that stops at positionals, `(value, error)` returns,
  and `io.Writer` dependency injection for testable output.

---

## 📖 Further Reading

- **Liz Rice — "Containers From Scratch" (GOTO/CNCF talk & code)** — the
  canonical walkthrough this challenge follows: <https://github.com/lizrice/containers-from-scratch>
- **`man 7 namespaces`**, **`man 2 clone`**, **`man 2 pivot_root`**,
  **`man 2 unshare`** — the primary kernel docs.
- **Linux cgroups v2 documentation** — <https://docs.kernel.org/admin-guide/cgroup-v2.html>
- **OverlayFS documentation** — <https://docs.kernel.org/filesystems/overlayfs.html>
- **`runc`** (the OCI reference runtime Docker actually uses) — <https://github.com/opencontainers/runc>
- **"What even is a container?" — Julia Evans** — <https://jvns.ca/blog/2016/10/10/what-even-is-a-container/>
- Project primer: [**Go Quickstart for a Python Developer**](../../docs/go-quickstart.md)
