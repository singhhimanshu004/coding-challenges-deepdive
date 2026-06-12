# Port Scanner

> **Phase:** 4 — Networking
> **Difficulty:** 🔵
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** M

**Status:** ✅ Completed

> 🐍➡️🐹 **New to Go?** Read the project's
> [**Go Quickstart for a Python Developer**](../../docs/go-quickstart.md) first.
> It maps every Go idiom used here (`defer`, slices vs. maps, error returns,
> structs) back to the Python you already know. This README assumes you've
> skimmed it, and adds 🐍 callouts where Go does something surprising — especially
> around **goroutines and channels**, which are the whole point of this challenge.

---

## 🎯 What We're Building

A concurrent **TCP port scanner** — a tool that, given a host, tells you which of
its ports are accepting connections (and what service usually lives there):

```
$ port-scanner 127.0.0.1 --ports 20-450
Scanning 127.0.0.1 — 431 port(s), 100 workers, 1s timeout

PORT    STATE   SERVICE
22      open    ssh
80      open    http
443     open    https

3 open port(s) found in 412ms
```

This is the same job `nmap` does. A scanner asks a deceptively simple question —
"is anyone listening on this port?" — but answering it for **thousands of ports
quickly** forces you to confront the central topic of Phase 4-and-beyond Go:
**how do you run many slow, network-bound operations at once without melting the
machine?** The answer is Go's signature concurrency pattern, the **worker pool**,
and this challenge is the cleanest possible setting to learn it.

What our scanner supports:

```
port-scanner <host> [--ports SPEC] [--workers N] [--timeout DUR]

--ports SPEC     ports to scan (default 1-1024). SPEC may be:
                   a range   "1-1024"
                   a list    "22,80,443"
                   a single  "8080"
                   mixed     "22,80,8000-8010"
--workers N      concurrent probes in flight   (default 100)
--timeout DUR    per-connection timeout         (default 1s; e.g. 500ms, 2s)
```

Examples:

```bash
port-scanner scanme.nmap.org                                  # default 1-1024
port-scanner 127.0.0.1 --ports 22,80,443
port-scanner 10.0.0.5 --ports 1-65535 --workers 500 --timeout 750ms
port-scanner localhost -p 1-1024 -w 200 -t 500ms              # short flags
```

---

## 📚 Core Concepts

### 1. How do you tell if a port is open? Two kinds of scan.

A TCP connection is established with a **three-way handshake**:

```
   client                         server
     │   ── SYN ───────────────►    │     "I'd like to connect"
     │   ◄────────── SYN/ACK ──     │     "sure, go ahead"   ← port is OPEN
     │   ── ACK ───────────────►    │     "connection established"
```

There are two classic ways to exploit this to probe a port:

**(a) Connect scan — what we build.** Ask the operating system to perform a
*full* handshake (all three packets) using a normal `connect()` call. In Go that
is `net.DialTimeout`. The interpretation is dead simple:

| Outcome of the dial | What it means |
| --- | --- |
| **Succeeds** | Something completed the handshake → the port is **OPEN**. |
| **Fails fast (RST)** | The OS replied "nobody's home" → port is **closed**. |
| **No reply, hits timeout** | A firewall silently dropped our packets → **filtered**. |

A connect scan can't reliably tell *closed* from *filtered* apart, so we bucket
both as simply "not open." Its big advantage: it uses ordinary sockets, so it
needs **no special privileges** and works identically everywhere.

**(b) SYN scan ("half-open") — deliberately out of scope.** A SYN scan sends a
lone `SYN` packet and, the moment it sees `SYN/ACK`, declares the port open and
*never sends the final ACK* — it tears the half-open connection down with a
`RST`. This is stealthier (many systems never log the incomplete connection) and
a touch faster, but it requires **hand-crafting raw TCP packets**. Forging
packets bypasses the OS's normal socket machinery, which is a privileged
operation: it needs **root / `CAP_NET_RAW`**. That's why `nmap`'s default SYN
scan must be run with `sudo`. We intentionally use the portable, unprivileged
connect path instead.

### 2. Why a timeout is non-negotiable

A *filtered* port sends **no reply at all**. Without a deadline, the dial would
block until the OS's default TCP timeout — which can be **dozens of seconds** per
port. Scanning 1,024 ports like that would take minutes. The per-connection
`timeout` is what makes a scan **bounded and fast**: "if there's no answer in 1s,
give up on this port and move on." It's the single most important knob for tuning
speed-vs-accuracy (too short and you'll miss slow hosts; too long and the scan
crawls).

### 3. The worker-pool concurrency model — the heart of this challenge

Scanning is **embarrassingly parallel**: every port probe is independent, and
each one spends ~100% of its time *waiting on the network*, not using the CPU.
That's the textbook case for concurrency.

The naive idea is "launch one goroutine per port." But firing **65,535**
simultaneous dials would exhaust the OS's file-descriptor limit and flood the
network. We want **bounded** concurrency: at most *N* probes in flight at any
moment. That structure is a **worker pool**, and Go builds it out of two
primitives — **channels** and a **`sync.WaitGroup`**:

```
                  jobs channel  (ports waiting to be scanned)
                    │   │   │   │   │
                    ▼   ▼   ▼   ▼   ▼          a FIXED number (N) of worker
                 ┌────┐┌────┐┌────┐  ...       goroutines, each looping:
                 │ w1 ││ w2 ││ w3 │            "take a port, probe it,
                 └─┬──┘└─┬──┘└─┬──┘             send the result"
                    │   │   │   │   │
                    ▼   ▼   ▼   ▼   ▼
                 results channel  (open / closed verdicts)
```

- **`jobs`** is a channel of port numbers. We push every port onto it, then
  **close** it. Closing means "no more work is coming."
- Each of the *N* workers runs `for port := range jobs { ... }`. Ranging over a
  channel pulls values until the channel is closed *and* drained, then the loop
  exits cleanly — so closing `jobs` is what tells the whole pool to wind down.
- Each worker sends its verdict on the **`results`** channel.
- A **`sync.WaitGroup`** counts the live workers. When the count hits zero, every
  worker has finished, so it's safe to close `results` and stop reading.

#### 🐍 For a Python developer

This is the part that feels most foreign coming from Python, so here is the
direct mapping:

| Go | Python equivalent | The key difference |
| --- | --- | --- |
| `go f()` (a **goroutine**) | starting a coroutine / a thread | A goroutine is scheduled by the Go *runtime*, not the OS. It starts at ~a few KB of stack, so **hundreds or thousands are routine**. |
| **blocking** `net.DialTimeout` inside a goroutine | `await reader.read()` in asyncio | In Go you **don't need `async`/`await`**. A plain *blocking* call is fine: when a goroutine blocks on I/O, the runtime parks it and runs another goroutine on the same OS thread. |
| a **channel** (`chan int`) | `queue.Queue` | A channel is a typed, thread-safe queue that *also* handles synchronisation. `range`-ing it until it's closed is the idiomatic "consume until the producer is done." |
| the **worker pool** here | `concurrent.futures.ThreadPoolExecutor(max_workers=N)` | Same idea — bounded fan-out — but assembled from language primitives instead of a library class. |
| no **GIL** | the GIL | Goroutines genuinely run in parallel across CPU cores. (For I/O-bound work like scanning, that matters less, but there's no global lock in your way.) |

The mental model: **goroutines are cheap, channels move data *and* synchronise,
and a `WaitGroup` answers "are we all done yet?"** Master those three and you've
got Go concurrency.

---

## 🏗️ Architecture & Design

Three small files, one concern each:

```
main.go       CLI: flag parsing, the "1-1024 / 22,80,443" port-spec parser,
              output formatting. Keeps main() trivial; the real entry point is
              run(args, stdout, stderr) so tests can drive it directly.
scanner.go    the actual scanning: scanPort() (one connect probe) and
              scan() (the worker pool that fans out across all ports).
services.go   a small port→service-name lookup table (80→"http", 22→"ssh", …)
              to make output friendlier.
```

The dependency flow is a straight line: `main` parses input → calls
`scan` → which calls `scanPort` per port → which consults `serviceName`.

> 🐍 **Testability via dependency injection.** `main()` does nothing but call
> `run(os.Args[1:], os.Stdout, os.Stderr)`. Because `run` takes its output
> streams as *arguments* rather than hard-coding `os.Stdout`, tests can call it
> with fakes and assert on the bytes — exactly like passing a file object into a
> Python function instead of touching `sys.stdout` inside it. Likewise, `scan`
> and `scanPort` take a host/port/timeout, so tests point them at **local
> listeners** (see Testing Strategy) and never touch the public internet.

---

## 🔨 Step-by-Step Implementation

Walking through the actual functions:

### 1. `parseArgs` / `parsePorts` (`main.go`) — turn CLI text into a plan

`parseArgs` is a small hand-rolled flag parser (matching the ergonomics of the
other tools in this repo): it walks the argument list, recognises
`--ports/-p`, `--workers/-w`, `--timeout/-t`, and treats the first non-flag
token as the host. It defaults `workers` to 100 and `timeout` to 1s, validates
that workers is a positive integer and the timeout parses as a Go duration, and
errors clearly on anything malformed.

`parsePorts` is where the `"1-1024"` / `"22,80,443"` / `"8080"` SPEC becomes a
clean `[]int`. It splits on commas; each piece is either a single port or a
`lo-hi` range (detected with `strings.Cut(part, "-")`). It collects everything
into a `map[int]struct{}` — a **set** — which gives **de-duplication for free**,
then sorts the result. Nice touches: a reversed range like `25-20` is swapped
into order, whitespace is trimmed, and every value is bounds-checked to `1–65535`.

> 🐍 `map[int]struct{}` is Go's idiom for a set. `struct{}` is a zero-byte value,
> so the map stores *keys only* — the equivalent of Python's `set()`.

### 2. `scanPort` (`scanner.go`) — probe exactly one port

```go
conn, err := net.DialTimeout("tcp", address, timeout)
if err != nil {
    return result{port: port, open: false}   // closed / filtered / timed out
}
_ = conn.Close()                              // open! we only needed the handshake
return result{port: port, open: true, service: serviceName(port)}
```

This is the whole connect scan in three lines. We ask for a full handshake with a
deadline; success means **open** and we immediately `Close()` the socket (we never
wanted to *talk* to the service — only to know it's there — and closing promptly
stops us from exhausting file descriptors across thousands of ports). Any error
means **not open**.

### 3. `scan` (`scanner.go`) — the worker pool

This is the function the challenge exists to teach. In order:

1. **Make two buffered channels.** `jobs` (ports in) and `results` (verdicts out),
   both buffered to `len(ports)` so producers never block waiting on consumers.
2. **Launch N workers.** A `for i := 0; i < workers; i++` loop starts `workers`
   goroutines. Each does `wg.Add(1)` before launching, `defer wg.Done()` on exit,
   and loops `for port := range jobs { results <- scanPort(...) }`.
3. **Feed the queue, then close it.** Push every port onto `jobs`, then
   `close(jobs)`. The close is the signal that lets every worker's `range` loop
   terminate once the queue drains.
4. **Close `results` once the workers are done — in a separate goroutine:**
   ```go
   go func() { wg.Wait(); close(results) }()
   ```
   This is the crucial idiom. `wg.Wait()` blocks until all workers return; doing
   it in its **own** goroutine means the main goroutine can start *reading*
   `results` immediately below, instead of deadlocking (a worker could block
   trying to send a result while main is stuck waiting on `wg.Wait()`).
5. **Collect.** `for r := range results` drains every verdict, keeping the open
   ones. The loop ends naturally when `results` is closed and empty.
6. **Sort** the open ports ascending before returning — workers finish in
   *non-deterministic* order, so results arrive scrambled and must be sorted for
   a stable, readable report.

### 4. `serviceName` (`services.go`) — friendlier output

A `map[int]string` literal mapping well-known ports to IANA service names
(`80→"http"`, `22→"ssh"`, `6379→"redis"`, …). It's intentionally small — just
enough to annotate output without shipping the full 14,000-line `/etc/services`
database. Unknown ports return `""`.

### 5. `run` (`main.go`) — tie it together

Parse args → print a one-line summary of the plan → time the `scan` call with
`time.Now()`/`time.Since` → print the open ports as a `PORT/STATE/SERVICE` table
(or "No open ports found.") → print the count and elapsed time. Exit code `0` on
success, `2` on a usage error.

---

## 🧪 Testing Strategy

The tests are **fully self-contained** — they never touch the public internet,
which keeps them fast and deterministic. The trick is to scan **local listeners**
we spin up inside the test itself.

Two helpers in `scanner_test.go` create known-state ports:

- **`startListener`** opens a TCP listener on `127.0.0.1:0`. The `:0` tells the
  kernel to pick any free **ephemeral** port (so the test never collides with
  something already running), and it returns the port the OS assigned. A
  background goroutine `Accept()`s and immediately closes connections, so a
  connect scan sees the port as **OPEN**.
- **`freeClosedPort`** opens such a listener just to learn a real port number,
  then **closes it immediately** — yielding a port that is (almost certainly) not
  accepting connections: a reliable **known-closed** port.

With those, the tests cover:

| Test | What it proves |
| --- | --- |
| `TestScanPortOpen` | `scanPort` reports a live listener as open. |
| `TestScanPortClosed` | `scanPort` reports a closed port as not open. |
| `TestScanReportsOpenAndClosed` | The pool scans a mix of 2 open + 2 closed ports and returns **exactly** the open ones, sorted. |
| `TestScanResultsAreSorted` | Results come back ascending regardless of the order 5 workers finish in. |
| `TestScanSingleWorker` | Correctness is independent of pool size — even with `workers=1`, every open port is still found. |
| `TestScanPortServiceName` | `serviceName` returns `ssh`/`http` for known ports and `""` for unknown ones. (We can't bind privileged port 22 in a test, so we verify the lookup helper directly.) |
| `TestParsePorts` | The SPEC parser: single, list, range, mixed, dedup+sort, reversed range, whitespace, and a batch of invalid inputs (`0`, `70000`, `abc`, empty). |
| `TestParseArgs` | Flag parsing: defaults, all-flags, short flags, and error cases (missing host, bad workers/timeout, unknown flag, flag without value). |

Run them:

```bash
go vet ./...
CGO_ENABLED=0 go test ./...     # see the toolchain note below
```

### ⚠️ Environment / toolchain note (read this if `go test` aborts)

On this macOS dev box, a plain `go test ./...` can abort *before any test runs*
with:

```
dyld: missing LC_UUID load command
signal: abort trap
```

This is **not a bug in our code.** It's a known mismatch between the Go
toolchain's external linker and the installed Xcode Command-Line Tools `ld`,
triggered because this package imports `net` — which pulls in **cgo** for the
system DNS resolver, forcing the external linker. The fix is to build the tests
in pure-Go mode, which uses Go's internal linker and native resolver:

```bash
CGO_ENABLED=0 go test ./...     # ✅ all tests pass
```

`go vet ./...` passes either way. This is a project-wide gotcha for Phase 4
networking challenges (it also affects the `curl` build), so **default to
`CGO_ENABLED=0` for test runs** here.

---

## 💡 Key Takeaways

- **A connect scan is just "did the dial succeed?"** Asking the OS for a full
  handshake and interpreting success/failure is all there is to it — no privileges
  required. A stealthier **SYN scan** needs raw packets and therefore **root**,
  which is why we don't build it.
- **The timeout is what makes scanning practical.** Filtered ports never reply;
  without a deadline the scan would stall for ages on every dead port.
- **The worker pool is *the* Go concurrency pattern.** A `jobs` channel + N
  goroutines + a `results` channel + a `sync.WaitGroup` gives you **bounded**
  parallelism for any "many slow, independent tasks" problem — scanning today,
  crawling or fan-out RPC tomorrow.
- **`close(channel)` is a broadcast "we're done."** Closing `jobs` ends every
  worker's `range` loop; closing `results` (after `wg.Wait()`, in its own
  goroutine) ends the collector loop. Getting *who closes what, when* right is the
  whole game.
- **Coming from Python:** goroutines replace threads/asyncio, blocking calls are
  fine (no `async`/`await`), channels are `queue.Queue` with built-in
  synchronisation, and there's no GIL. The pool is a `ThreadPoolExecutor` built
  from language primitives.
- **Inject your I/O for testability.** Passing output streams into `run`, and
  pointing `scan` at local `net.Listener`s, let the whole tool be tested with zero
  internet dependency.

---

## 📖 Further Reading

- 🐍➡️🐹 [Go Quickstart for a Python Developer](../../docs/go-quickstart.md) — the project Go primer
- [Coding Challenges — Build Your Own Port Scanner](https://codingchallenges.fyi/challenges/challenge-port-scanner)
- [A Tour of Go — Goroutines & Channels](https://go.dev/tour/concurrency/1)
- [Go blog — Pipelines and cancellation](https://go.dev/blog/pipelines) (the worker-pool / fan-out pattern in depth)
- [Effective Go — Concurrency](https://go.dev/doc/effective_go#concurrency)
- Go stdlib: [`net`](https://pkg.go.dev/net), [`sync`](https://pkg.go.dev/sync) (`WaitGroup`), [`time`](https://pkg.go.dev/time)
- [Nmap — Port Scanning Techniques](https://nmap.org/book/man-port-scanning-techniques.html) (connect vs. SYN scans, and why SYN needs root)
- [RFC 9293 — TCP](https://datatracker.ietf.org/doc/html/rfc9293) (the three-way handshake)
