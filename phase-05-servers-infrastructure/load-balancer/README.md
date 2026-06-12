# Load Balancer

> **Phase:** 5 — Servers & Infrastructure
> **Difficulty:** 🟠
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** L

**Status:** ✅ Completed

> 🐍➡️🐹 **New to Go?** Read the project's
> [**Go Quickstart for a Python Developer**](../../docs/go-quickstart.md) first.
> It maps the Go idioms used here (interfaces, goroutines, `time.Ticker`,
> `sync/atomic`, `sync.RWMutex`, `defer`, error returns, slices vs. maps) back to
> the Python you already know. This README assumes you've skimmed it and adds 🐍
> callouts wherever Go does something a Python/Java dev wouldn't expect.

---

## 🎯 What We're Building

A **HTTP reverse-proxy load balancer**: one process that listens on a single
address, and for every incoming request **picks one of several backend servers,
forwards the request to it, and streams the backend's response back to the
client.** The client only ever talks to the balancer; it never knows (or cares)
which backend actually served it.

This is the direct sequel to **Challenge 30 — [Web Server](../web-server/)**.
That challenge built a single HTTP server from a raw TCP socket. One server is a
single point of failure and a single point of capacity. The moment you want
*two* of them — for redundancy or for more throughput — you need something in
front that spreads traffic across them and routes around the ones that fall
over. That something is a load balancer.

```
                          ┌──────────────────────────────┐
                          │        LOAD BALANCER          │
   client  ─── HTTP ───▶  │  pick a healthy backend, then │  ─── HTTP ──▶  backend A  (web-server :9001)
   client  ─── HTTP ───▶  │  reverse-proxy the request    │  ─── HTTP ──▶  backend B  (web-server :9002)
   client  ─── HTTP ───▶  │  and stream the reply back    │  ─── HTTP ──▶  backend C  (web-server :9003)
                          │                               │
                          │  + background health checks   │
                          └──────────────────────────────┘
```

**Reverse vs forward proxy — how this differs from Challenge 29.** In Phase 4 we
built a [forward proxy](../../phase-04-networking/http-forward-proxy/): it sits
next to the *client* and reaches *arbitrary* origins the client names. A
**reverse** proxy sits next to the *servers*: clients address *it* as if it were
the origin, and it fans requests out to a *fixed, known* pool of backends it
controls. Same plumbing (copy a request upstream, copy the response back);
opposite side of the relationship and opposite knowledge of who's who.

What our balancer supports:

```
load-balancer [--addr :8080]
              [--backends http://127.0.0.1:9001,http://127.0.0.1:9002]
              [--algo round-robin|least-conn|random|weighted]
              [--health-interval 5s] [--health-path /health] [--health-timeout 2s]
              [--verbose]
```

- **Reverse-proxy forwarding** built on `net/http` + `httputil.ReverseProxy`.
- **Pluggable scheduling**: round-robin, least-connections, random, weighted —
  all behind one `Scheduler` interface.
- **Active health checks**: a background goroutine probes `GET /health` on every
  backend and marks the dead ones DOWN, then brings them back when they recover.
- **Passive health checks**: a forward that fails at the transport level
  (connection refused/reset/timeout) instantly fails that backend out.
- **Concurrency-safe backend pool** shared by the request goroutines and the
  health-check goroutine.

> **What we build vs. what we borrow.** We deliberately use
> `httputil.ReverseProxy` for the *mechanics* of proxying — we already hand-rolled
> HTTP request rewriting and tunnelling in Challenge 29, so re-doing it would
> teach nothing new. **The lesson of this challenge is everything *around* the
> proxy:** the scheduling algorithms and the health-check loop. Those are 100%
> hand-written.

---

## 📚 Core Concepts

### 1. Reverse proxy (and what `httputil.ReverseProxy` does for us)

A reverse proxy receives a client request, **re-issues it to an upstream origin**,
and **copies the upstream's response back** to the client. `httputil.NewSingleHostReverseProxy(target)`
gives us exactly that for one target: it rewrites the request's scheme + host to
point at `target`, manages the hop-by-hop headers (`Connection`, `Keep-Alive`,
…) that must *not* be forwarded verbatim, appends the client IP to
`X-Forwarded-For`, and **streams** the response body back (so a 1 GB download
doesn't buffer in memory).

> 🐍 Think of `ReverseProxy` as `requests.request(...)` *plus* a streaming
> copy of the answer straight back to your own caller — but written to be safe
> under heavy concurrency and correct about HTTP's fiddly header rules. We give
> each backend its **own** `ReverseProxy` so the per-backend target is baked in.

The one hook we customise is `proxy.ErrorHandler` — see passive health checks
below.

### 2. Scheduling: how to choose a backend

The whole job of a load balancer reduces to one question asked on every request:
*which backend gets this one?* Different answers suit different workloads.

| Algorithm | How it chooses | Best when… | Cost / caveat |
|---|---|---|---|
| **Round-robin** | next backend in order, wrapping around: A,B,C,A,B,C… | requests are cheap and roughly **equal** in cost | ignores that one backend may be stuck on a slow request |
| **Least-connections** | the backend with the **fewest in-flight** requests right now | request durations **vary a lot** (some slow, some fast) | must track live connection counts (state) |
| **Weighted (round-robin)** | like round-robin but a backend of weight *W* is chosen *W*× as often | backends have **unequal capacity** (big box + small box) | you must supply sensible weights |
| **Random** | a uniformly random backend | simple, stateless; statistically even at scale | unlucky short-term clumping |

**Round-robin vs least-connections — the key intuition.** Round-robin is *fair
by count*: everyone gets the same *number* of requests. Least-connections is
*fair by load*: everyone gets the same amount of *work in flight*. If every
request takes the same time these are identical. The moment one request blocks
for 10 seconds, round-robin keeps piling new requests onto that busy backend on
its turn, while least-connections routes around it until it catches up.

To track "in flight" we keep an **active-connection counter per backend**:
increment when a request starts, decrement when it finishes. Because many
request goroutines touch that counter at once, it must be updated atomically
(see concept 4).

### 3. Health checks: active vs passive

A backend can die at any time. A balancer that keeps sending traffic to a dead
backend is worse than useless. So we continuously answer: *which backends are
healthy right now?* Two complementary techniques:

- **Active health checking** — the balancer *initiates* probes on a schedule
  (e.g. `GET /health` every 5 s, or a bare TCP dial). Pros: detects death even
  when no client traffic is flowing, and — crucially — detects **recovery**, so
  a backend that was down can be brought back. Cons: a probe interval of *N*
  seconds means up to *N* seconds of blindness.
- **Passive health checking** — the balancer infers health from **real**
  traffic: if forwarding a live request fails (connection refused, reset,
  timeout), mark that backend down *immediately*. Pros: instant, zero extra
  traffic. Cons: it can't tell when a dead backend **recovers**, because it has
  stopped sending it requests to learn from.

> **Production systems run BOTH** — passive to fail a backend *out* in
> milliseconds, active to fail it *back in* once it's healthy again. We
> implement both: a `time.Ticker` probe loop (active) and a `ReverseProxy.ErrorHandler`
> that marks a backend down on a failed forward (passive).

### 4. Concurrency-safe state (the Go-specific part)

An HTTP server in Go runs **every request on its own goroutine**. So the backend
pool and the per-backend counters are read and written by *many goroutines at
once*, plus the health-check goroutine. Two tools keep that race-free:

- **`sync/atomic`** for single values that are hammered on the hot path: each
  backend's `alive` flag (`atomic.Bool`) and `active` connection count
  (`atomic.Int64`). Atomics give lock-free, race-free reads/writes of one value.
- **`sync.RWMutex`** for the backend *list* in the pool: many readers (request
  goroutines asking "who's healthy?") share the lock; a writer (adding/removing
  a backend) takes it exclusively.

> 🐍 In Python the GIL makes `x += 1` *look* atomic, and you reach for
> `threading.Lock`. Go has **real parallelism** (goroutines run on multiple OS
> threads), so an unguarded `count++` from two goroutines is a genuine data race
> the race detector will flag. `atomic.Int64.Add(1)` is the race-free version;
> `sync.RWMutex` is `threading.RLock`/`Lock` with a reader/writer split.

### 5. Connection draining (graceful shutdown)

When you take a balancer (or a backend) out of rotation, you don't want to sever
requests mid-flight. **Draining** means: stop sending *new* requests to the
target, but let the *in-flight* ones finish before you shut it down. Go's
`http.Server.Shutdown(ctx)` does exactly this for the balancer's own listener —
it stops accepting new connections and waits (up to a deadline) for active ones
to complete. The same idea applies to a backend: mark it DOWN (no new traffic),
then wait for its `active` counter to reach zero before you redeploy it.

---

## 🏗️ Architecture & Design

Five small files, each one responsibility — the same "one stage per file" layout
the earlier Go challenges use.

```
load-balancer/
├── main.go        CLI flags → build pool + scheduler + health checker → http.Server
├── backend.go     Backend (URL + ReverseProxy + alive + active counter) and the Pool
├── scheduler.go   Scheduler interface + RoundRobin / LeastConn / Random / WeightedRoundRobin
├── health.go      HealthChecker: background time.Ticker probe loop (active checks)
├── proxy.go       LoadBalancer: the http.Handler — pick, account, forward
└── loadbalancer_test.go   self-contained tests (httptest backends in front of the LB)
```

**Request lifecycle:**

```
            ┌─────────────── LoadBalancer.ServeHTTP (one goroutine per request) ───────────────┐
client ───▶ │ 1. pool.HealthyBackends()   → snapshot of backends marked alive                  │
            │ 2. scheduler.Next(healthy)  → choose one (round-robin / least-conn / …)           │
            │ 3. backend.acquire()        → active++   (defer backend.release() → active--)     │
            │ 4. backend.Proxy.ServeHTTP  → forward upstream, stream response back to client    │ ───▶ backend
            └──────────────────────────────────────────────────────────────────────────────────┘
                              ▲
            background:  HealthChecker goroutine — every interval, GET /health on each backend,
                         SetAlive(true/false). Passive: a failed forward → ErrorHandler → SetAlive(false).
```

**Key design choices:**

1. **`Scheduler` is an interface, not a flag.** The balancer holds a
   `Scheduler` and calls `Next(healthy)`; it never branches on the algorithm
   name. Adding a new strategy = add a type with a `Next` method. This is the
   Strategy pattern, Go-style (structural interfaces — no `implements`).
2. **The pool filters health; the scheduler never sees a dead backend.**
   `HealthyBackends()` returns only live backends in pool order, so each
   scheduler stays tiny and oblivious to health. Round-robin over the filtered
   slice automatically "skips" down backends.
3. **The `active` counter spans the whole request via `defer`.**
   `acquire()` then `defer release()` guarantees the count is high for exactly
   as long as the backend is busy — even if the proxy panics or the client
   disconnects mid-stream. That correctness is what makes least-connections
   meaningful.
4. **Health state lives on the backend as atomics**, so the probe goroutine and
   the request goroutines never need a shared lock for the hot path.
5. **A test seam on the health loop.** `CheckAll(ctx)` runs one probe sweep and
   is called directly by tests, so health transitions are driven
   *deterministically* — no sleeping for a ticker to fire.

---

## 🔨 Step-by-Step Implementation

### Step 1 — Model a backend (`backend.go`)

A `Backend` bundles the target `*url.URL`, its own `*httputil.ReverseProxy`, an
`atomic.Bool alive`, and an `atomic.Int64 active`. `newBackend` builds the
reverse proxy and installs an `ErrorHandler` that, on a failed forward, marks
the backend down (passive health check) and returns `502`.

> 🐍 `atomic.Bool`/`atomic.Int64` are "a bool/int that's safe to touch from many
> goroutines." `Store`/`Load`/`Add` are the race-free verbs.

### Step 2 — A concurrency-safe pool (`backend.go`)

`Pool` wraps `[]*Backend` behind a `sync.RWMutex`. `Backends()` returns a copy
(for the health loop) and `HealthyBackends()` returns only the live ones, in
order (for the scheduler). Returning a *copy/snapshot* means a caller can iterate
without holding the lock.

### Step 3 — The scheduler interface and strategies (`scheduler.go`)

```go
type Scheduler interface {
    Next(backends []*Backend) *Backend // choose one healthy backend (or nil)
    Name() string
}
```

- **RoundRobin** holds an `atomic.Uint64` counter; `Next` returns
  `backends[counter.Add(1)-1 % len]`. `Add(1)` returns the *new* value so two
  concurrent calls get two different slots — the reason it must be atomic.
- **LeastConn** scans for the smallest `ActiveConnections()`.
- **Random** picks `backends[rand.IntN(len)]` (injectable for tests).
- **WeightedRoundRobin** expands each backend by its weight across the cycle.

`newScheduler(algo)` maps the CLI string (with aliases like `rr`, `lc`) to a
concrete type — the *only* place the algorithm name is interpreted.

### Step 4 — Active health checks (`health.go`)

`HealthChecker.Start(ctx)` launches the idiomatic Go background loop:

```go
go func() {
    h.CheckAll(ctx)                  // probe once immediately (don't start blind)
    ticker := time.NewTicker(h.interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done(): return    // clean shutdown — no goroutine leak
        case <-ticker.C:  h.CheckAll(ctx)
        }
    }
}()
```

> 🐍 `time.Ticker` is "fire repeatedly on an interval," and `select` lets one
> goroutine wait for *either* a tick *or* a cancel signal — there's no clean
> Python equivalent in one construct. Always `Stop()` the ticker and return on
> `ctx.Done()` so the goroutine ends when the program does.

`CheckAll` does `GET <url>/health` on each backend and `SetAlive(2xx)`. It's
package-visible specifically so tests can trigger a sweep on demand.

### Step 5 — The balancer handler (`proxy.go`)

`LoadBalancer.ServeHTTP` ties it together: snapshot healthy backends → `Next` →
`503` if none → `acquire()` + `defer release()` → `backend.Proxy.ServeHTTP`.
Implementing `ServeHTTP` is all it takes to *be* an `http.Handler`.

### Step 6 — Wire up the CLI (`main.go`)

Parse flags, `parseBackends` the CSV (each URL must be absolute; an optional
`#weight` suffix is supported), build the pool/scheduler/health checker, start
health checks under a cancellable context, and run an `http.Server` with a
SIGINT/SIGTERM handler that cancels the context and calls `srv.Shutdown` to
**drain** in-flight requests.

---

## 🧪 Testing Strategy

Run them (note the `CGO_ENABLED=0` — see below):

```bash
cd phase-05-servers-infrastructure/load-balancer
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go test ./...
```

Everything is **self-contained — no external network**. Tests spin up two or
three `httptest.Server` backends (each echoes an `X-Served-By` label and exposes
a toggleable `/health`), put the load balancer in front of them, and assert real
behaviour. 11 tests:

| Test | What it proves |
|---|---|
| `TestRoundRobinOrder` | round-robin returns backends strictly in order, wrapping |
| `TestRoundRobinEmpty` | empty healthy set → `nil` (no panic) |
| `TestLeastConnPrefersLeastBusy` | with crafted in-flight counts, picks the least-busy, and re-picks as load shifts |
| `TestWeightedRoundRobinDistribution` | a weight-3 vs weight-1 backend split 3:1 over a cycle |
| `TestProxyingPreservesResponse` | **body, custom header, AND status (201)** flow through the proxy untouched |
| `TestRoundRobinSpreadsAcrossBackends` | end-to-end A,B,C,A,B,C across three real backends |
| `TestUnhealthyBackendSkippedAndRecovered` | toggle B's `/health` off → a probe marks it DOWN → all traffic goes to A → toggle on → a probe brings it back → both serve again |
| `TestNoHealthyBackendReturns503` | every backend down → `503`, not a hang |
| `TestPassiveMarkDownOnTransportError` | forwarding to a *closed* origin trips the `ErrorHandler` → backend marked DOWN + `502` |
| `TestParseBackends` / `TestNewScheduler` | table-driven CLI parsing (URLs, weights, errors; algorithm aliases) |

**The determinism trick.** Health transitions are normally timing-dependent and
flaky. We avoid sleeps by (a) giving each test backend a `/health` toggle and (b)
calling `HealthChecker.CheckAll(ctx)` *directly* to run exactly one probe sweep
when the test wants one. Least-connections is tested by setting the `active`
counters explicitly rather than racing real concurrent requests. The result is
fast and 100% deterministic.

### ⚠️ macOS / go1.22 toolchain note (`CGO_ENABLED=0`)

On go1.22 / darwin-arm64, any package that imports `net`/`net/http` pulls in the
cgo DNS resolver, and a plain `go test` aborts with
`dyld: missing LC_UUID load command`. The fix — used across every Go challenge in
this repo — is to disable cgo:

```bash
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go build -o load-balancer .
```

### Try it live

```bash
# Terminals 1-2: start two backends. The Challenge 30 web server works, or any
# HTTP server. The stdlib server below won't answer /health, so probe root with
# --health-path /:
python3 -m http.server 9001
python3 -m http.server 9002

# Terminal 3: balance across them
CGO_ENABLED=0 go run . --backends http://127.0.0.1:9001,http://127.0.0.1:9002 \
    --algo round-robin --health-path / --verbose

# Terminal 4: watch requests fan out
for i in $(seq 6); do curl -s localhost:8080/ -o /dev/null -w "%{http_code}\n"; done
```

---

## 💡 Key Takeaways

- **A load balancer is "pick a backend, then reverse-proxy to it."** Strip away
  the proxying (which the standard library handles) and the entire intellectual
  content is *which* backend and *is it alive* — scheduling and health.
- **Round-robin is fair by count; least-connections is fair by load.** They're
  identical until request durations diverge; then least-connections routes
  around the slow backend and round-robin doesn't.
- **Active + passive health checks are complementary**, not alternatives. Passive
  fails a backend *out* instantly; active fails it *back in* after recovery.
  Neither alone is enough.
- **Pluggable behaviour = a small interface.** `Scheduler` with one real method
  lets the balancer stay ignorant of the algorithm — the Go idiom for the
  Strategy pattern, no inheritance required.
- **Shared mutable state under real parallelism needs `sync/atomic` (hot single
  values) and `sync.RWMutex` (the list).** Unlike Python's GIL world, `count++`
  from two goroutines is a real race.
- **`time.Ticker` + `select { ctx.Done() / ticker.C }`** is *the* shape for a
  background periodic task with clean shutdown.
- **Design a test seam for time-dependent logic.** Exposing `CheckAll` and
  toggling backend health turned flaky, sleep-based tests into deterministic ones.
- **Reverse vs forward proxy** is about *whose* side you're on and *who* you
  know: forward = next to the client, reaches anywhere; reverse = next to the
  servers, fans out to a known pool.

---

## 📖 Further Reading

- 🐍➡️🐹 [Go Quickstart for a Python Developer](../../docs/go-quickstart.md) — the project Go primer
- [`net/http/httputil.ReverseProxy`](https://pkg.go.dev/net/http/httputil#ReverseProxy) — what we use for the proxying mechanics
- [`sync/atomic`](https://pkg.go.dev/sync/atomic) and [`sync.RWMutex`](https://pkg.go.dev/sync#RWMutex) — the concurrency primitives used here
- [`time.Ticker`](https://pkg.go.dev/time#Ticker) and [`context`](https://pkg.go.dev/context) — the background-loop + cancellation pattern
- [NGINX: HTTP load balancing](https://docs.nginx.com/nginx/admin-guide/load-balancer/http-load-balancer/) — round-robin / least-conn / weighted in a production LB
- [HAProxy configuration manual](https://docs.haproxy.org/) — `balance` algorithms and `option httpchk` active health checks
- [Envoy: load balancing](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/load_balancing/load_balancing) and [outlier detection](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/upstream/outlier) — modern active + passive health checking
- John Crickett's [Coding Challenges — Load Balancer](https://codingchallenges.fyi/challenges/challenge-load-balancer)
- Sibling challenges: [Web Server (30)](../web-server/) (the backend you balance) · [HTTP Forward Proxy (29)](../../phase-04-networking/http-forward-proxy/) (reverse vs forward)
