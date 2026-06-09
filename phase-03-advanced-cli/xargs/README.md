# xargs

> **Phase:** 3 — Advanced CLI & Orchestration  
> **Difficulty:** 🔵  
> **Recommended Language:** 🟦 Go  
> **Effort Estimate:** M

**Status:** ✅ Done

> 🐍→🐹 **New to Go?** Read [`docs/go-quickstart.md`](../../docs/go-quickstart.md)
> first. It teaches Go *by mapping it to Python*, using this repo's code as the
> running example. Every Go-specific idiom below is also commented inline in the
> source files, so keep them open side by side.

---

## 🎯 What We're Building

`xargs` is the Unix glue that turns **a list of things on standard input** into
**commands that actually run**. On its own, many tools (`find`, `ls`, `grep -l`)
just *print names*. `xargs` reads those names and feeds them as **arguments** to
another program, spawning one or more child processes to do real work.

```bash
# find prints file names; xargs turns them into a single rm invocation
find . -name '*.tmp' | xargs rm

# one echo per line
printf 'a\nb\nc\n' | xargs -n1 echo
```

The reason `xargs` exists at all is a hard limit: you **cannot** put unlimited
arguments on one command line (the OS caps it — `ARG_MAX`). So `find ... | rm` via
a naive `$(...)` can blow up, but `xargs` *batches* the items into right-sized
argument lists and runs the command as many times as needed.

This implementation supports the flags that teach the core ideas:

| Flag | Meaning | Example |
|------|---------|---------|
| *(none)* | Append all items to the command, run once | `echo a b c \| xargs echo` |
| `-n N` | At most **N items per command** (batching) | `xargs -n1 echo` |
| `-I R` | **Replace mode**: run once per item, substituting `R` | `xargs -I {} mv {} bak/{}` |
| `-0` | Items are **NUL-delimited** (safe for spaces/newlines) | `find . -print0 \| xargs -0 ...` |
| `-P N` | **Bounded parallelism**: up to N children at once | `xargs -P4 -n1 curl` |
| `-t` | Echo each command to stderr before running it | `xargs -t echo` |

If you give **no command**, the default is `echo` (just like the real thing).

```bash
echo "Hello, World!" | xargs        # → Hello, World!  (echo is implied)
```

## 📚 Core Concepts

### 1. argv batching — the heart of xargs

Every program receives its arguments as an **argument vector** (`argv`): a list
of strings where `argv[0]` is the program name and the rest are its arguments.
`xargs` builds these vectors. Given base command `echo` and items `a b c d` with
`-n2`, it builds two vectors:

```
[echo a b]
[echo c d]
```

…and runs each one. That batching (how many items per `argv`) is the single most
important knob, controlled by `-n`.

### 2. fork/exec — how a process spawns another

A Unix program starts a child by **fork** (clone the process) + **exec** (replace
the clone's program image with the new command). Go wraps this in the
`os/exec` package: `exec.Command("echo", "a").Run()` does fork + exec + wait for
you. We never call the raw syscalls — `os/exec` is the idiomatic door.

> 🐍 Python analogy: this is `subprocess.run(["echo", "a"])`. Go's `cmd.Run()`
> is the same fork/exec/wait, and `cmd.Stdout = os.Stdout` is like
> `subprocess.run(..., stdout=sys.stdout)`.

### 3. stdin tokenization — turning a stream into items

Input arrives as a raw byte stream. `xargs` splits it into items:

- **Default:** split on *any* whitespace (spaces, tabs, newlines), collapsing
  runs and dropping blanks. (`strings.Fields` in Go ≈ Python's `s.split()`.)
- **`-0`:** split *only* on the NUL byte. Filenames can legally contain spaces
  and even newlines, so `find -print0 | xargs -0` is the only safe way to pass
  arbitrary filenames.

### 4. Exit-status propagation

Children fail. `xargs` reports a meaningful exit code so scripts can react:

| Code | Meaning |
|------|---------|
| `0` | every invocation succeeded |
| `123` | one or more invocations exited 1–125 |
| `126` | a command was found but could not be executed |
| `127` | a command could not be found |

## 🏗️ Architecture & Design

The tool is a clean three-stage pipeline, one file per stage, glued by `run()`:

```
                 ┌──────────────┐     ┌──────────────┐     ┌────────────────────┐
  stdin  ───────▶│  tokenize    │────▶│  buildJobs   │────▶│     runJobs        │────▶ exit code
  (bytes)        │ tokenize.go  │items│  build.go    │ jobs│  run.go (parallel) │
                 └──────────────┘     └──────────────┘     └────────────────────┘
                  split on WS / NUL    batch (-n) or         worker pool of P
                                       replace (-I)          goroutines + semaphore
```

| File | Responsibility |
|------|----------------|
| `tokenize.go` | Read stdin, split into `[]string` items (whitespace or `-0` NUL). |
| `build.go` | Turn items + base command into `[]job` (batch `-n` or replace `-I`). |
| `run.go` | Spawn jobs with **bounded parallelism**, aggregate exit codes. |
| `main.go` | Hand-rolled flag parsing + orchestration (`run`). |

Each stage is a **pure function of its input**, so tests drive them directly with
no subprocesses (except the deliberate end-to-end tests that spawn `echo`/`false`).
The `runner` function type in `run.go` is the seam that lets tests inject a fake
spawner to exercise the concurrency engine deterministically.

## 🔨 Step-by-Step Implementation

1. **Tokenizer** (`tokenize.go`): `io.ReadAll` the stream, then `strings.Fields`
   (default) or split on `\x00` (`-0`), dropping the trailing empty chunk.
2. **Command builder** (`build.go`):
   - *Replace mode* (`-I {}`): one job per item, `strings.ReplaceAll` the
     placeholder inside every base argument.
   - *Batch mode*: chunk items by `-n` (0 = all on one line), appending each
     chunk onto a **fresh copy** of the base command.
3. **Parallel runner** (`run.go`): a semaphore-bounded worker pool (below).
4. **CLI** (`main.go`): parse flags that appear *before* the command; everything
   from the first non-flag token on is the command and its fixed args.

## 🧵 Bounded Parallelism — goroutines, channels, a semaphore

This is the headline lesson of the challenge. `-P N` runs up to **N** children at
once — never more — using the canonical Go concurrency pattern.

A **buffered channel** used as a **counting semaphore** is the throttle. A channel
with capacity `N` holds at most `N` tokens. Sending a token *acquires* a slot;
once `N` are in flight, the next send **blocks** until a running job *releases* by
receiving. A `sync.WaitGroup` lets the parent wait for all children to finish.

```go
sem := make(chan struct{}, parallelism) // N permits
var wg sync.WaitGroup

for _, j := range jobs {
    wg.Add(1)
    sem <- struct{}{}        // ACQUIRE — blocks once N are running
    go func() {
        defer wg.Done()
        defer func() { <-sem }() // RELEASE a permit when this job ends
        run(j.argv)              // fork/exec one child
    }()
}
wg.Wait()                        // wait for every child
```

Visually, with `-P3` and 7 jobs the slots stay at most 3 wide:

```
time ──▶
slot 1: [ job1 ][ job4 ][ job7 ]
slot 2: [ job2 ][ job5 ]
slot 3: [ job3 ][ job6 ]
        └─ at most 3 children alive at any vertical slice ─┘
```

> 🐍 Python analogy: `sem` is `threading.BoundedSemaphore(N)` and the WaitGroup
> is joining a thread pool. The big difference: **goroutines are cheap** (a few KB
> of stack, multiplexed onto OS threads by Go's runtime), so launching one
> goroutine per job — even thousands — is normal and idiomatic. You would not
> casually spawn thousands of OS threads in Python.

**Why it matters:** `-P` is the difference between downloading 100 URLs one at a
time and downloading them 8 at a time, while still **never** overwhelming the
machine with 100 simultaneous processes. Bounded — not unlimited — parallelism.

## 🧪 Testing Strategy

`go test ./...` covers every stage, and `go vet ./...` is clean. Highlights:

- **Tokenization:** whitespace collapsing, blank-line padding, and `-0` keeping
  spaces/newlines inside items.
- **Batching (`-n`):** all-in-one default, `-n1`, and `-n2` with a remainder.
- **Replace (`-I`):** one job per item with substring substitution; empty input
  runs nothing.
- **Exit-status propagation:** the 0/123/126/127 priority ladder, plus a real
  `false` child that makes `xargs` return 123.
- **`-t`:** commands are echoed before running.
- **Bounded parallelism — deterministic:** instead of relying on `sleep` timing,
  a fake runner uses a **barrier** that only opens once the `P`-th goroutine
  arrives. If the pool allowed fewer than `P` concurrent jobs the barrier would
  never open and the test times out; we then assert the observed max concurrency
  is **exactly `P`**. Run under `-race` to prove the worker pool is data-race free.
- **End-to-end:** real `echo`/`false` processes spawned via `os/exec` (child
  output routed through an `os.Pipe` so concurrent children share no in-process
  buffer).

```bash
go test ./...          # all unit + integration tests
go test ./... -race    # prove the parallel runner is race-free
go vet ./...           # static checks

# Try it for real:
printf 'a\nb\nc\n'        | go run . -n1 echo
printf '1 2 3 4 5\n'      | go run . -n2 echo
printf 'x\ny\n'           | go run . -I {} echo item={}
find . -print0            | go run . -0 -n1 echo
seq 1 8 | go run . -P4 -n1 sh -c 'sleep 0.3; echo done'   # ~0.6s, not 2.4s
```

## 💡 Key Takeaways

- **xargs = tokenize → batch into argv → fork/exec.** Three small stages.
- **`-n` controls batching; `-I` switches to one-process-per-item replace mode.**
- **`-0` is the only safe way to pass arbitrary filenames** — spaces and newlines
  survive because NUL is the one byte a filename can't contain.
- **Bounded parallelism is a buffered channel used as a semaphore.** Acquire on
  send, release on receive; `WaitGroup` joins. Goroutines make per-job concurrency
  cheap and idiomatic.
- **Propagate child exit status** so pipelines and scripts can detect failure.

## 📖 Further Reading

- Coding Challenges — *Build Your Own xargs*: <https://codingchallenges.fyi/challenges/challenge-xargs>
- `man xargs` — the real flag reference (GNU and BSD differ in places)
- Go `os/exec` package: <https://pkg.go.dev/os/exec>
- Go blog, *Pipelines and cancellation* (channels & concurrency patterns):
  <https://go.dev/blog/pipelines>
- This repo's [`docs/go-quickstart.md`](../../docs/go-quickstart.md)
