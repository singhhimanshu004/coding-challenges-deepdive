# NATS Message Broker

> **Phase:** 5 — Servers & Infrastructure
> **Difficulty:** 🟠
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** L

**Status:** ✅ Completed

> 🐍➡️🐹 **New to Go?** Read the project's
> [**Go Quickstart for a Python Developer**](../../docs/go-quickstart.md) first.
> It maps every Go idiom used here (`defer`, goroutines, channels, `bufio`,
> maps, interfaces, `sync.Mutex`) back to the Python you already know. This
> README assumes you've skimmed it and adds 🐍 callouts where Go does something
> a Python/Java developer would find surprising.

---

## 🎯 What We're Building

A working **message broker** that speaks the core
[**NATS**](https://nats.io) protocol — the small, fast pub/sub system used at
companies like Mastercard, Walmart, and Tesla to glue microservices together.

A broker is a **post office for software**. Publishers drop a message addressed
to a *subject* (`orders.us.created`); subscribers register interest in subjects
(possibly with wildcards) and the broker routes each message to everyone who
cares — without publishers and subscribers ever knowing about each other. That
decoupling is the whole point: you can add a new "send-email-on-order" service
without touching the code that creates orders.

We build the **server side** of NATS from a raw TCP socket. No `net/http`, and —
importantly — **we do not import the real `nats.go` library**. We implement the
text protocol ourselves so the lessons are visible:

What our broker supports:

```
nats-broker [--addr :4222] [--verbose]

--addr     TCP address to listen on (default :4222, the real NATS port)
--verbose  log connection events and reply +OK to every command
```

Client verbs it understands (all plain text over TCP):

```
CONNECT {options}                              client handshake / options
PING                          → PONG           keepalive
PUB <subject> [reply] <#bytes>\r\n<payload>\r\n  publish a message
SUB <subject> [queue] <sid>                    subscribe (optionally in a queue group)
UNSUB <sid> [max_msgs]                         cancel / auto-cancel a subscription
```

Server replies it produces:

```
INFO {options}\r\n                             sent once, on connect
MSG <subject> <sid> [reply] <#bytes>\r\n<payload>\r\n   a delivered message
+OK\r\n                                        success (only in verbose mode)
-ERR '<reason>'\r\n                            protocol error
PONG\r\n                                       reply to PING
```

You can drive it by hand with `nc` once it's running:

```bash
nats-broker --addr :4222 &
printf 'SUB foo.* 1\r\n' | nc localhost 4222   # subscribe in one terminal
printf 'PUB foo.bar 5\r\nhello\r\n' | nc localhost 4222  # publish in another
```

---

## 📚 Core Concepts

### 1. The NATS text protocol — line-based framing

NATS, like HTTP and Redis's RESP, is **just agreed-upon text flowing over a TCP
byte-pipe**. The unit of communication is a **control line** terminated by
`\r\n` (carriage-return + line-feed). The first word of the line is the *verb*;
the rest are space-separated arguments:

```
SUB foo.bar 1\r\n
└┬┘ └──┬──┘ │
 verb subject sid
```

Most verbs are *one line and done*. The exception is `PUB` (and the server's
`MSG`), because the payload is **arbitrary binary** that might itself contain
`\r\n`. You cannot find the end of binary data by scanning for a newline. So the
protocol uses **length-prefixed framing**: the control line announces the exact
byte count, and the broker then reads *precisely that many bytes*, followed by a
trailing `\r\n`:

```
PUB foo.bar 11\r\n        ← "the payload is exactly 11 bytes"
hello world\r\n           ← 11 bytes, then the terminator
```

This is the single most important framing idea in the whole challenge:
**newline-delimited for control lines, length-prefixed for payloads.** Get this
right and everything else follows.

### 2. Subjects — a routing hierarchy

A **subject** is a dot-separated name made of *tokens*: `time.us.east`. Subjects
form a hierarchy you design for your domain, e.g. `orders.<region>.<event>`.
Subscribers don't have to name a subject exactly — they can use **wildcards**:

| Wildcard | Meaning | `foo.bar` | `foo.bar.baz` | `foo` |
|----------|---------|:---------:|:-------------:|:-----:|
| `foo.*`  | `*` matches **exactly one** token | ✅ | ❌ (two tokens) | ❌ (zero tokens) |
| `foo.>`  | `>` matches **one or more** trailing tokens; must be last | ✅ | ✅ | ❌ (needs ≥1) |
| `>`      | matches **any** non-empty subject | ✅ | ✅ | ✅ |
| `foo.bar`| literal — no wildcards | ✅ | ❌ | ❌ |

The matching algorithm (in `subject.go`) is a left-to-right token walk:

```
pattern:  foo   *    baz
subject:  foo   bar  baz
          ───  ───  ───
          ==   any   ==     → match

'>' short-circuits: if we hit '>', it must be the last pattern token and there
must be at least one remaining subject token → match the rest.
```

Wildcards live **only on the subscription side**. A *published* subject is
always concrete; you publish to `foo.bar`, never to `foo.*`.

### 3. Publish / subscribe fan-out

The broker keeps a **registry** of every active subscription. On each publish it
walks the registry, tests the published subject against each subscription's
pattern, and delivers a `MSG` to every match. One publish can become **zero, one,
or many** deliveries — *fan-out*. Publishers never block on or know about
subscribers; this is the decoupling that makes pub/sub scale.

### 4. Queue groups — load balancing

Sometimes you want the opposite of fan-out. If you run three identical "order
processor" workers, you want each order handled **once**, not three times.

That's a **queue group**. When subscribers join the same group name for a
subject (`SUB orders.* workers 1`), the broker treats the group as a *single
logical subscriber*: a matching message is delivered to **exactly one** member,
chosen by load balancing (we use round-robin). Plain (non-group) subscribers
still each get their own copy. So you can mix both: a metrics service gets every
message (plain sub) while a pool of workers shares the load (queue group).

### 5. Delivery semantics — at-most-once, no persistence

NATS core is deliberately simple and **fire-and-forget**:

- **At-most-once.** A message is delivered to a connected, matching subscriber
  zero or one times. If a subscriber is slow or offline, the message is dropped
  for them — never queued, never retried.
- **No persistence.** The broker holds nothing on disk. If no one is subscribed
  when you publish, the message simply evaporates. (NATS's *JetStream* layer
  adds persistence and at-least-once on top — we implement core only.)

This trade-off buys enormous speed and simplicity, and it's why our broker can
drop a frame to a slow consumer without ceremony.

---

## 🏗️ Architecture & Design

One concern per file, so each idea has an obvious home:

```
subject.go    the subject matcher: tokenize + wildcard matching (the lesson)
client.go     one connected client: its send-channel + writer goroutine
server.go     the broker: listener, subscription registry, routing & delivery
main.go       the CLI: flag parsing, start the server
```

### The concurrency model

```
                    ┌─────────────────── Server ───────────────────┐
                    │  subs:  map[*client]map[sid]*subscription     │
   TCP accept       │  guarded by a single sync.Mutex               │
   loop ──────┐     └───────────────────────────────────────────────┘
              │              ▲ (lock)            │ (lock)
              ▼              │                   ▼
   ┌── handleClient ──┐   register a SUB    deliver() walks subs,
   │ (1 goroutine     │                     matches subject, enqueues
   │  per connection) │                     MSG frames to recipients
   │                  │                            │
   │  bufio.Reader ───┼── reads & parses           │  c.enqueue(frame)
   │  command lines   │   commands                 ▼
   └──────────────────┘                  ┌── client.out (chan) ──┐
                                          │  writeLoop goroutine   │
                                          │  drains channel →      │
                                          │  writes socket bytes   │
                                          └────────────────────────┘
```

Three design choices worth calling out:

1. **Goroutine per connection.** Go's goroutines are cheap (a few KB of stack),
   so the classic "one thread per client" model that would be wasteful in
   Java/Python is idiomatic and simple here. `go s.handleClient(conn)` and you're
   done. 🐍 A goroutine is like a thread, but you can have hundreds of thousands.

2. **A channel + dedicated writer goroutine per client.** Multiple publishers
   can deliver to the same subscriber at the same time, and a `net.Conn` is *not*
   safe for concurrent writes. Rather than lock the socket, each client has an
   `out chan []byte`; publishers `enqueue` onto it and a single `writeLoop`
   goroutine owns the actual socket writes. This is the Go mantra in action:
   *"Don't communicate by sharing memory; share memory by communicating."*
   🐍 A channel is a thread-safe `queue.Queue`; `writeLoop` is its consumer.

3. **One `sync.Mutex` around the registry.** Subscribes, unsubscribes,
   disconnects and the read-side of every publish all touch the `subs` map, and
   Go maps are not safe for concurrent use. A single mutex guarding the registry
   is simple, correct, and fast enough — we hold it only long enough to *build*
   the recipient list, then release it *before* doing the (potentially blocking)
   channel sends. 🐍 `sync.Mutex` is `threading.Lock()`; `defer mu.Unlock()` is
   the `with lock:` idiom.

### The subscription registry shape

```go
subs map[*client]map[string]*subscription   // client → sid → subscription
```

Keying first by client makes **disconnect** O(1) — drop one top-level key and
all that client's subscriptions vanish. Routing a publish iterates every
subscription (O(n)); a production broker builds a *subject trie* for sublinear
matching, but the flat scan is far easier to read and is the right call for a
learning implementation.

---

## 🔨 Step-by-Step Implementation

1. **`subject.go` — the matcher first.** This is the core lesson, and it's pure
   logic with no networking, so it's the easiest to get right and test. Split on
   `.`, walk the pattern, handle `*` (one token) and `>` (tail). Equal length
   required when no `>` is present.

2. **`client.go` — the per-connection plumbing.** Define the `client` with its
   `out` channel, `quit` channel, and a `sync.Once` so teardown happens exactly
   once. `enqueue` selects on `quit` so a send to a departed client is *dropped*,
   not a panic (at-most-once in action). `writeLoop` is the sole socket writer.

3. **`server.go` — the broker.**
   - `Listen()` opens the `net.Listener`; split from `Serve()` so tests can read
     the OS-assigned port (`:0`) before accepting.
   - `handleClient` runs per connection: send `INFO`, then loop reading control
     lines with `bufio.Reader.ReadString('\n')` and dispatch on the verb.
   - `handlePub` does the length-prefixed read: `strconv.Atoi` the size, then
     `io.ReadFull` exactly that many bytes, then `Discard(2)` the trailing CRLF.
   - `deliver` is the routing core: under the mutex, scan subscriptions, collect
     plain matches and bucket queue-group matches by group name, pick one member
     per group (round-robin), build `MSG` frames, then enqueue *after* unlocking.
   - `removeClient` (called via `defer`) drops the client's subscriptions and
     tears down the connection.

4. **`main.go` — the CLI.** `flag.String`/`flag.Bool` for `--addr`/`--verbose`,
   then `Listen` + `Serve`. Tiny on purpose: all the logic lives in testable
   functions.

---

## 🧪 Testing Strategy

Two layers, **both hermetic** — no real NATS server, no `nats.go`, no network
beyond loopback:

1. **Table-driven unit tests for the matcher** (`subject_test.go`). The wildcard
   rules are subtle (does `foo.*` match `foo.bar.baz`? does `foo.>` match `foo`?),
   so every rule gets an explicit row. 🐍 Table-driven tests are Go's idiom for
   what you'd write as `@pytest.mark.parametrize` in Python.

2. **End-to-end tests over raw TCP** (`integration_test.go`). Each test starts a
   broker on `127.0.0.1:0` and connects raw `net.Dial` clients that speak the
   protocol by hand. The cases mirror the requirements exactly:
   - a `foo.*` subscriber **receives** a `PUB` to `foo.bar`;
   - a non-matching subject (`bar.baz`, and the too-long `foo.bar.baz`) is **not**
     delivered (asserted via a read timeout);
   - a `foo.>` subscriber receives a multi-token `foo.bar.baz`;
   - two members of one queue group → a publish reaches **exactly one**;
   - after `UNSUB`, further publishes **stop** arriving;
   - `PING` is answered with `PONG`.

   A small `flush()` helper (send `PING`, wait for `PONG`) solves the classic
   pub/sub test race: because the broker processes one connection's commands in
   order, a returned `PONG` proves the earlier `SUB` is already registered before
   we publish from another connection.

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

This is **not a bug in our code** — it's a known mismatch between the Go
toolchain's external linker and the installed Xcode Command-Line Tools `ld`,
triggered because this package imports `net` (which can pull in **cgo** for the
system resolver). The fix is to build in pure-Go mode, which uses Go's internal
linker:

```bash
CGO_ENABLED=0 go test ./...     # ✅ all tests pass
```

`go vet ./...` passes either way, and the binary builds cleanly with
`CGO_ENABLED=0 go build`. (This is the same workaround documented for the
Phase 3 `curl` challenge.)

---

## 💡 Key Takeaways

- **A broker is text on a TCP pipe — with two framing styles.** Control lines are
  newline-delimited; payloads are length-prefixed because binary data can contain
  newlines. Mixing the two correctly is the heart of the protocol.
- **Subject routing with `*` and `>` is the core lesson.** Tokenize on `.`, walk
  left to right: `*` consumes one token, `>` consumes the tail (and only ever
  appears last). Wildcards exist only on the subscribe side.
- **Pub/sub decouples producers from consumers.** One publish fans out to every
  matching subscriber; nobody needs to know who else exists.
- **Queue groups invert fan-out into load balancing.** Same group → exactly one
  member gets each message. You can run fan-out and load-balanced delivery side
  by side.
- **Core delivery is at-most-once with no persistence.** That simplicity is a
  feature; dropping to a slow consumer needs no ceremony.
- **Go's concurrency toolkit fits this problem perfectly:** a goroutine per
  connection, a channel + writer goroutine to serialise per-client output, and a
  single `sync.Mutex` to guard the shared registry — "share memory by
  communicating" in practice.

---

## 📖 Further Reading

- [NATS Protocol Specification](https://docs.nats.io/reference/reference-protocols/nats-protocol) — the authoritative description of every verb (`CONNECT`, `PUB`, `SUB`, `MSG`, `PING`/`PONG`).
- [NATS Subject-Based Messaging & Wildcards](https://docs.nats.io/nats-concepts/subjects) — the official explanation of `*` and `>`.
- [NATS Queue Groups](https://docs.nats.io/nats-concepts/core-nats/queue) — load-balanced subscriptions.
- [Go: `net` package](https://pkg.go.dev/net) — `Listener`, `Conn`, `Dial`.
- [Go: `bufio` package](https://pkg.go.dev/bufio) — buffered `ReadString`/`Discard`.
- [Effective Go: Concurrency](https://go.dev/doc/effective_go#concurrency) — goroutines, channels, and the "share by communicating" philosophy.
- [Go Quickstart for a Python Developer](../../docs/go-quickstart.md) — this repo's idiom map.
