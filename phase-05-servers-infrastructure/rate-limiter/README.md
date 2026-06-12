# Rate Limiter

> **Phase:** 5 — Servers & Infrastructure
> **Difficulty:** 🔵→🟠
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** M

**Status:** ✅ Completed

> 🐍➡️🐹 **New to Go?** Read the project's
> [**Go Quickstart for a Python Developer**](../../docs/go-quickstart.md) first.
> It maps every Go idiom used here (interfaces, `sync.Mutex`, the `http.Handler`
> middleware pattern, struct methods, error returns) back to the Python you
> already know. This README assumes you've skimmed it, and adds 🐍 callouts where
> Go does something a Python/Java developer might not expect.

---

## 🎯 What We're Building

A **reusable rate limiter** — the component that answers one question millions of
times a second: *"has this client already used up its allowance?"* Rate limiting
is how real services protect themselves from abuse, runaway clients, and
thundering-herd traffic, and how APIs enforce their pricing tiers.

The twist that makes this a *learning* project rather than a one-liner:

> **We implement FOUR different limiting algorithms behind a single `Limiter`
> interface** — token bucket, leaky bucket, fixed window, and sliding-window
> log — and expose them as plug-in **`net/http` middleware** that returns
> `429 Too Many Requests` with the standard rate-limit headers.

Implementing several algorithms side by side is the whole point: you *feel* the
trade-offs (burst tolerance vs. smoothness vs. memory vs. accuracy) instead of
just reading about them.

What the demo server supports:

```
rate-limiter [flags]

--algo    token-bucket | leaky-bucket | sliding-window | fixed-window   (default token-bucket)
--rate    allowed rate: tokens/sec for buckets, requests/window for windows (default 10)
--burst   bucket capacity / max instantaneous burst (bucket algorithms)     (default 20)
--window  window length for the window algorithms, e.g. 1s, 1m             (default 1s)
--addr    address for the demo HTTP server to listen on                    (default :8080)
```

Examples:

```bash
# Allow a burst of 20, then a steady 10 requests/sec, per client IP:
rate-limiter --algo token-bucket --rate 10 --burst 20

# Strict, exact 100 requests per rolling minute (no boundary burst):
rate-limiter --algo sliding-window --rate 100 --window 1m

# Watch it work: 3 pass (200), the rest are limited (429):
rate-limiter --algo token-bucket --rate 2 --burst 3 --addr :8080 &
for i in $(seq 1 6); do curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8080/; done
# 200 200 200 429 429 429
```

---

## 📚 Core Concepts

Rate limiting always comes down to the same question — *is this request within
the budget?* — but each algorithm answers it with a different model of "budget."

### 1. Token bucket — *save up, then spend*

```
            refill: rate tokens / second
                 │
                 ▼
        ┌───────────────────┐  capacity = burst
        │  ● ● ● ● ●         │  (bucket holds at most `burst` tokens)
        └───────────────────┘
                 │ each request removes 1 token
                 ▼
      request allowed if a token is available, else rejected
```

A bucket holds up to `burst` tokens and refills at `rate` tokens/sec. Each
request must take one token. An **idle** client lets tokens accumulate up to the
cap, so it can later fire a **burst** of up to `burst` requests instantly — then
it's throttled to the steady refill rate. This "save up then spend" behaviour
makes token bucket the most widely used limiter: it tolerates spiky-but-bounded
traffic gracefully. (It's what Go's own `golang.org/x/time/rate` implements.)

### 2. Leaky bucket — *smooth the output*

```
   requests in  ──►  ┌───────────────────┐
   (bursty)          │ ~ ~ ~ ~ ~ ~       │  capacity = backlog tolerated
                     └─────────┬─────────┘
                               ▼  leaks at a CONSTANT rate
                     steady, smoothed output (drip... drip... drip)
```

Each request pours one unit of water into a bucket with a hole; water leaks out
at a constant rate. If a request would overflow the bucket, it's rejected.
Unlike token bucket, leaky bucket does **not** let you spend a saved-up burst —
it enforces a **smooth, even output rate** no matter how spiky the input. Use it
when a downstream system needs a shaped flow (e.g. feeding a fragile legacy
service) rather than occasional bursts. The capacity just sets how much backlog
you tolerate before shedding load.

> The two bucket algorithms are duals: token bucket bounds the *burst* and lets
> the average float up to the rate; leaky bucket bounds the *output rate* and
> uses capacity only as a queue.

### 3. Fixed window — *the cheap one, and its famous bug*

```
  window 1 [0s──1s)        window 2 [1s──2s)
  count: ▓▓▓▓▓ (5/5)       count resets → ▓▓▓▓▓ (5/5)
                  ▲                 ▲
              t=0.9s            t=1.0s
              5 requests        5 requests   → 10 requests in ~0.2s!
```

Chop time into fixed windows (e.g. each whole second). Keep one counter per key;
increment per request; reset at the start of the next window. It's the cheapest
limiter — one integer and one timestamp per key. But it has the **boundary
burst** flaw: a client can send `limit` requests at the very end of one window
and `limit` more at the very start of the next, sneaking through **2× the limit**
in a span shorter than a single window. Our test
(`TestFixedWindowBoundaryBurst`) demonstrates this directly.

### 4. Sliding-window log — *exact, at a memory cost*

```
        window slides continuously with "now"
   ┌─────────────────────────────┐
   │  •      •   •        •       │   keep a timestamp per request;
   └─────────────────────────────┘   drop any older than (now - window);
   t-window                    now    allow if fewer than `limit` remain
```

Store the **timestamp of every request** in the last `window`. To decide a new
request, discard timestamps older than `now - window` and allow only if fewer
than `limit` remain. The window slides smoothly with the current time, so there
is **no boundary burst** — it's exact. The cost is **memory**: one timestamp per
request in the window. A **sliding-window counter** (a common production
compromise) blends the current and previous fixed-window counts by a weight to
approximate this in O(1) memory — accurate enough, far cheaper. We implement the
exact log here because it's the clearest to learn from.

### Choosing — a cheat sheet

| Algorithm | Burst handling | Output shape | Memory / key | Boundary burst? |
| --- | --- | --- | --- | --- |
| **Token bucket** | allows bursts up to capacity | bursty, bounded average | O(1) | no |
| **Leaky bucket** | absorbs into queue, no burst out | smooth/constant | O(1) | no |
| **Fixed window** | up to 2× at edges | steppy | O(1) | **yes** |
| **Sliding log** | exact, no burst | exact | O(requests in window) | no |

### 5. The HTTP contract: `429` and `Retry-After`

When a client exceeds the limit, the correct HTTP response is
**`429 Too Many Requests`** (RFC 6585). A well-behaved limiter also tells the
client what's going on with advisory headers:

| Header | Meaning |
| --- | --- |
| `Retry-After: 1` | wait this many **seconds** before retrying (on a 429) |
| `X-RateLimit-Limit` | the ceiling for this client |
| `X-RateLimit-Remaining` | requests left in the current allowance |
| `X-RateLimit-Reset` | seconds until the allowance replenishes |

`Retry-After` is standardized; the `X-RateLimit-*` family is a de-facto
convention (GitHub, Twitter, etc.). We always round `Retry-After` **up** to whole
seconds so we never invite a client back too early.

### 6. Distributed considerations (why this gets hard at scale)

Our limiter keeps state in an in-process `map`. That's perfect for a single
instance — but the moment you run **N replicas behind a load balancer**, each
has its own map, so a client effectively gets **N× the limit**. The standard
fixes:

- **Shared store (Redis).** Move the counter/bucket into Redis so all replicas
  share one source of truth. Token-bucket and fixed-window math map neatly onto
  atomic Redis operations (`INCR` + `EXPIRE`, or a small Lua script for
  atomicity). This is by far the most common production design — and a great
  reason this challenge sits right after the **Redis Server** (#32) in Phase 5.
- **Clock skew & atomicity.** Across machines you can't trust local clocks;
  you read time from the shared store, and you must make "check-and-increment"
  atomic (hence Lua scripts) to avoid races.
- **Approximate / local-first.** At very high scale, a sticky-routing or
  per-node "local quota with periodic global reconciliation" trades perfect
  accuracy for latency and resilience.

The key insight: **the algorithm doesn't change — only where the state lives.**
That's exactly why we hid state behind the `Limiter` interface; a `RedisLimiter`
would slot in without touching the middleware or the server.

---

## 🏗️ Architecture & Design

One concern per file, so each algorithm has an obvious home and the shared
abstraction stays tiny:

```
limiter.go        the Limiter interface + the Decision result struct (the contract)
clock.go          the injectable Clock interface + the real wall-clock implementation
tokenbucket.go    token-bucket algorithm
leakybucket.go    leaky-bucket algorithm
slidingwindow.go  sliding-window-log algorithm
fixedwindow.go    fixed-window-counter algorithm
middleware.go     net/http middleware: keys on client IP, sets headers, returns 429
main.go           CLI: flag parsing, algorithm selection, demo server wiring
*_test.go         deterministic, table-driven tests using a fake (manual) clock
```

Everything is built around **one interface**:

```go
type Limiter interface {
    Allow(key string) Decision   // record a request for `key`; is it permitted?
}
```

The middleware, the CLI, and the tests only ever speak to `Limiter` — they never
mention a concrete algorithm. Swapping `--algo` swaps the implementation behind
the interface with zero changes elsewhere. This is the **strategy pattern**, and
it's the central Go-idiom lesson of the challenge.

> 🐍 **Interfaces, the Go way.** In Python you'd write an ABC with an abstract
> `allow()` method and have each algorithm `class TokenBucket(Limiter)`. In Go
> there is **no `implements` keyword** — a type satisfies `Limiter` automatically
> just by having an `Allow(string) Decision` method. This is structural
> ("duck") typing, but verified at **compile time**. The interface is also
> deliberately *small* (one method): Go culture favours tiny interfaces defined
> at the point of use.

Two design decisions worth calling out:

- **Injectable clock.** Every algorithm reads "now" through a `Clock` interface
  rather than calling `time.Now()` directly. Production passes `realClock{}`;
  tests pass a `fakeClock` they advance by hand. This makes time-based behaviour
  **deterministic** — no real sleeps, no flakiness. (🐍 same idea as passing a
  `now` callable into a function, or monkeypatching `time.time`, so tests own
  the clock.)
- **Lazy refill.** Buckets aren't refilled by a background goroutine/timer.
  Instead, on each access we compute how many tokens *would* have accrued since
  the last access (`elapsed × rate`) and add them. It's exact, costs nothing
  while a key is idle, and needs no cleanup goroutine.

---

## 🔨 Step-by-Step Implementation

1. **Define the contract (`limiter.go`).** Write the `Limiter` interface and the
   `Decision` struct (allowed? + limit/remaining/retry-after/reset). Designing
   the return type first forces you to decide what information the middleware
   needs — and keeps every algorithm honest about producing it.
2. **Make time injectable (`clock.go`).** A one-method `Clock` interface and a
   `realClock`. Everything time-related flows through this so tests can control
   it.
3. **Token bucket (`tokenbucket.go`).** Per-key `{tokens float64, last time}`.
   On `Allow`: lazily refill (`min(burst, tokens + elapsed×rate)`), then take a
   token if `tokens >= 1`, else reject and compute `RetryAfter = (1−tokens)/rate`.
   Guard the map with a `sync.Mutex`.
4. **Leaky bucket (`leakybucket.go`).** Per-key `{level float64, last time}`.
   Lazily leak (`max(0, level − elapsed×leakRate)`), then admit if
   `level + 1 <= capacity`. The mirror image of token bucket.
5. **Fixed window (`fixedwindow.go`).** Align windows by truncating `now` to a
   multiple of the window length, keep a counter, reset when the aligned window
   changes. Aligning windows is what makes the boundary-burst behaviour real and
   testable.
6. **Sliding-window log (`slidingwindow.go`).** Per-key slice of timestamps;
   compact out anything older than `now − window` (reusing the backing array via
   `kept := times[:0]`), then admit if the survivors are below the limit.
7. **HTTP middleware (`middleware.go`).** `func(http.Handler) http.Handler` that
   derives the key from the client IP, calls `Allow`, writes the
   `X-RateLimit-*` headers, and on rejection sets `Retry-After` and writes
   `429`. Otherwise it calls the wrapped handler.
8. **CLI & demo (`main.go`).** Parse flags, build the chosen limiter with
   `realClock{}`, wrap a trivial "ok" handler with the middleware, and serve.

> 🐍 **The middleware pattern.** `func(http.Handler) http.Handler` is exactly a
> **decorator**: a function that takes a handler and returns a new handler that
> does some work and then (maybe) calls the original — `def mw(handler): def
> wrapped(req): ...; return wrapped`. `http.HandlerFunc(fn)` is the adapter that
> turns a plain function into something satisfying the `http.Handler` interface
> (it gives the function a `ServeHTTP` method). Wrapping handlers like this is
> how almost all Go web stacks compose logging, auth, and limiting.

> 🐍 **`sync.Mutex` instead of the GIL.** Python's GIL means simple dict updates
> are effectively atomic; Go has true parallelism, so a shared `map` accessed by
> many request goroutines **will** race. We take `mu.Lock()` / `defer
> mu.Unlock()` around every read-modify-write. `defer` (like a `with` block /
> `try/finally`) guarantees the unlock even on an early return.

---

## 🧪 Testing Strategy

Everything is **deterministic and hermetic** — no real sleeps, no real network —
because both time and the HTTP layer are injectable. Tests are **table-driven**,
the idiomatic Go style (one test function, a slice of cases, a `t.Run` subtest
per row).

The injectable clock is the star: instead of `time.Sleep(time.Second)` (slow,
flaky), tests call `clk.Advance(time.Second)` to jump time forward by an exact
amount.

1. **Token bucket** (`tokenbucket_test.go`): a fresh client may burst up to
   `burst`, the bucket then rejects, and after advancing the clock exactly the
   right number of tokens refill (not one more). Also asserts the precise
   `RetryAfter` and per-key isolation.
2. **Sliding window** (`slidingwindow_test.go`): counts correctly within the
   window, rejects beyond it, and — crucially — *slides*: after advancing time
   so the oldest hit ages out (but a newer one survives), a new request is
   allowed again. Exact `RetryAfter` checked too.
3. **Fixed window** (`fixedwindow_test.go`): basic limit, clean rollover into the
   next window, and an explicit **boundary-burst** test proving 2× the limit can
   pass across a window edge.
4. **Leaky bucket** (`leakybucket_test.go`): capacity enforced, one slot frees
   per drain interval (not two), exact `RetryAfter`.
5. **HTTP middleware** (`middleware_test.go`), via `net/http/httptest`: returns
   **200 under the limit and 429 over it**, sets `Retry-After` and
   `X-RateLimit-*` headers on the 429, isolates clients by IP, and **recovers**
   to 200 after the clock advances enough to refill.

Run them:

```bash
go vet ./...
CGO_ENABLED=0 go test ./...     # see the toolchain note below
```

### ⚠️ Environment / toolchain note (read this if `go test` aborts)

On this macOS dev box, a plain `go test ./...` aborts before any test runs with:

```
dyld: missing LC_UUID load command
signal: abort trap
```

This is **not a bug in our code** — it's the same known mismatch documented in
the `curl` challenge between the Go toolchain's external linker and the installed
Xcode Command-Line Tools `ld`. It triggers here because the package imports
`net/http` (which pulls in **cgo** for the system DNS resolver). Building in
pure-Go mode uses Go's internal linker and sidesteps it:

```bash
CGO_ENABLED=0 go test ./...     # ✅ all tests pass
CGO_ENABLED=0 go build          # ✅ binary builds cleanly
```

`go vet ./...` passes either way. **Manual verification:** the demo server was
run with `--algo token-bucket --rate 2 --burst 3`; six rapid `curl`s returned
`200 200 200 429 429 429`, and the 429 carried `Retry-After: 1`,
`X-RateLimit-Limit: 3`, `X-RateLimit-Remaining: 0`.

---

## 💡 Key Takeaways

- **One interface, many strategies.** Hiding four very different algorithms
  behind a single tiny `Limiter` interface is the lesson. The middleware, CLI,
  and tests never change when you swap algorithms — and a future `RedisLimiter`
  would slot in the same way.
- **The four algorithms encode four trade-offs.** Token bucket tolerates
  *bursts*; leaky bucket enforces *smoothness*; fixed window is *cheap* but
  bursts at boundaries; sliding log is *exact* but memory-hungry. Build them
  side by side and the differences stop being abstract.
- **Inject the clock for deterministic time tests.** Reading "now" through a
  `Clock` interface turned every timing test from a flaky `Sleep` into an exact,
  instant `Advance`. This is one of the highest-leverage testing habits in Go.
- **Middleware is just function composition.** `func(http.Handler) http.Handler`
  is the decorator pattern, and it's how cross-cutting concerns (limiting,
  logging, auth) layer cleanly around your real handlers.
- **Concurrency is real in Go.** No GIL means shared state needs a `sync.Mutex`;
  `defer mu.Unlock()` keeps it correct even on early returns.
- **Distributed limiting changes *where state lives*, not the algorithm.** The
  jump from one process to many is solved by a shared store (usually Redis), not
  by inventing new math.

---

## 📖 Further Reading

- 🐍➡️🐹 [Go Quickstart for a Python Developer](../../docs/go-quickstart.md) — the project Go primer
- [Coding Challenges — Build Your Own Rate Limiter](https://codingchallenges.fyi/challenges/challenge-rate-limiter)
- [RFC 6585 §4 — `429 Too Many Requests`](https://datatracker.ietf.org/doc/html/rfc6585#section-4)
- [MDN — `Retry-After`](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Retry-After)
- [Cloudflare — How we built rate limiting (sliding window)](https://blog.cloudflare.com/counting-things-a-lot-of-different-things/)
- [Stripe — Scaling your API with rate limiters](https://stripe.com/blog/rate-limiters)
- Go stdlib: [`net/http`](https://pkg.go.dev/net/http), [`sync`](https://pkg.go.dev/sync), [`net/http/httptest`](https://pkg.go.dev/net/http/httptest), [`golang.org/x/time/rate`](https://pkg.go.dev/golang.org/x/time/rate) (a production token bucket)
