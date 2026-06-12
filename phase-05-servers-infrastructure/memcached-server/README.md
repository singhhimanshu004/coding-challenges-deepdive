# Memcached Server

> **Phase:** 5 — Servers & Infrastructure
> **Difficulty:** 🟠
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** L

**Status:** ✅ Completed

> 🐍➡️🐹 **New to Go?** Read the project's
> [**Go Quickstart for a Python Developer**](../../docs/go-quickstart.md) first.
> It maps every Go idiom used here (`net.Listener`, `bufio`, `container/list`,
> `sync.Mutex`, goroutines, struct methods, error returns) back to the Python you
> already know. This README assumes you've skimmed it and adds 🐍 callouts where
> Go does something surprising.

---

## 🎯 What We're Building

A working **memcached server** — the in-memory caching daemon that sits in front
of databases at almost every large web company (Facebook, Twitter, Wikipedia,
Reddit…) to absorb read traffic. When your app asks "what's user 42's profile?"
ten thousand times a second, you don't hammer Postgres ten thousand times — you
ask memcached, which keeps recent answers in RAM and replies in microseconds.

We implement the **classic memcached TEXT protocol** over raw TCP, from scratch:

```
memcached-server [--addr :11211] [--max-items N] [--verbose]

--addr        host:port to listen on (default :11211)
--max-items   evict least-recently-used items beyond this count; 0 = unlimited
--verbose     log connections and each received command line to stderr
```

The commands we speak (enough to use a real memcached client, or just `telnet`):

| Category | Commands |
| --- | --- |
| Store | `set` `add` `replace` `append` `prepend` `cas` |
| Retrieve | `get` `gets` |
| Mutate | `incr` `decr` |
| Remove | `delete` `flush_all` |
| Misc | `version` `quit` |

You can talk to it with nothing but a terminal:

```
$ telnet 127.0.0.1 11211
set greeting 0 0 5      ← key, flags, exptime, byte-length
hello                   ← the 5 data bytes
STORED
get greeting
VALUE greeting 0 5
hello
END
```

This challenge is the **caching counterpart to the Redis server** (built
separately in this phase). The two look similar from the outside but teach
deliberately different lessons — see the [Redis contrast](#-memcached-vs-redis)
at the end.

---

## 📚 Core Concepts

### 1. The memcached TEXT protocol: line framing + a length-prefixed data block

memcached has two protocols (text and binary). We build the **text** one because
it is human-readable and teaches the two framing ideas you'll meet everywhere.

Every interaction is one of two shapes:

**(a) A single command line, terminated by `CRLF` (`\r\n`).**

```
get foo\r\n
delete foo\r\n
incr counter 2\r\n
```

We read a line, split it on spaces, and dispatch on the first word. (🐍 This is
basically a REPL over a socket: `line.split()` then a big `match`/`switch`.)

**(b) A storage command line + a DATA BLOCK.** Storage commands carry an
arbitrary binary value, so the command line first *announces the exact byte
length*, and the value follows on the next line:

```
set foo 0 0 3\r\n      ← set <key> <flags> <exptime> <bytes>
bar\r\n                ← exactly 3 bytes "bar", then CRLF
```

This is **length-prefixed framing**, and it's the crucial idea:

```
   set    foo    0      0       3      \r\n
    │      │     │      │       │
  command key  flags exptime  bytes ─────┐ "read exactly this many
                                          │  raw bytes next, THEN expect CRLF"
                                          ▼
                                       b a r \r\n
```

> **Why announce the length instead of reading until a delimiter?** Because the
> value is *opaque bytes* — it might itself contain `\r\n`, or null bytes, or a
> serialized image. If we searched for a terminator we'd corrupt binary data.
> By stating the length up front, the server reads *precisely* that many bytes
> with `io.ReadFull` and never guesses. (This is the same lesson HTTP teaches
> with `Content-Length`; see the [curl challenge](../../phase-03-advanced-cli/curl/).)

The trailing `\r\n` after the data block is **not** counted in `<bytes>` — it's
pure framing. Forgetting to consume it leaves you two bytes out of sync forever.

**Reply vocabulary** (each line ends with `\r\n`):

| Reply | Meaning |
| --- | --- |
| `STORED` | a store succeeded |
| `NOT_STORED` | `add` on an existing key, or `replace`/`append`/`prepend` on a missing one |
| `EXISTS` | `cas` failed: the item changed since you read it |
| `NOT_FOUND` | `cas`/`delete`/`incr` on a missing key |
| `DELETED` | `delete` succeeded |
| `VALUE <key> <flags> <bytes> [<cas>]` | header line of a `get`/`gets` hit, followed by the data block |
| `END` | end of a `get`/`gets` response (and the *only* reply when nothing matched) |
| `OK` | `flush_all` done |
| `ERROR` / `CLIENT_ERROR <msg>` / `SERVER_ERROR <msg>` | unknown command / your fault / our fault |

Note `get` of a missing key isn't an error — it just returns a bare `END` with
no `VALUE` lines. That's the cache-miss signal.

### 2. Expiry (the `exptime` semantics)

Every item carries an expiry. The wire `exptime` field has three modes — a real
protocol quirk worth memorising:

| `exptime` value | Meaning |
| --- | --- |
| `0` | never expires |
| `1 … 2592000` (≤ 30 days) | **relative**: that many seconds from *now* |
| `> 2592000` | **absolute**: a Unix timestamp (seconds since the epoch) |
| negative | already expired (delete-on-the-spot) |

We use **lazy expiration**: there's no background reaper thread. An item's
deadline is checked the next time someone touches it; if it's stale, we drop it
then and report a miss. Anything never touched again is eventually reclaimed by
LRU eviction (below). This is exactly how real memcached behaves — a sweeper for
millions of keys would burn CPU for little benefit.

### 3. FLAGS and the CAS token

- **FLAGS** is a 32-bit number the *client* stores alongside the value and gets
  back verbatim. The server never interprets it — clients use it to record, say,
  "this value is gzip-compressed" or "this is a Python pickle." We just round-trip it.

- **CAS (Compare-And-Swap)** is memcached's answer to the lost-update race. Every
  item has a monotonic **version token**. `gets` returns it; `cas` lets you say
  "store this value *only if* the version is still the one I read":

  ```
  gets k            → VALUE k 0 2 57   (CAS token = 57)
                       v1
                       END
  cas k 0 0 2 57    → STORED            (token matched: swap done, token bumped)
  cas k 0 0 2 57    → EXISTS            (someone else changed it; 57 is stale now)
  ```

  This is **optimistic concurrency control** — the same idea as an HTTP `ETag` +
  `If-Match`, or a `version` column in a database row. No locks held across the
  round-trip; the loser just retries.

### 4. LRU eviction (a cache must forget)

A cache has finite RAM, so when it fills up it must throw something away. The
canonical policy is **LRU — evict the Least Recently Used item**, betting that
what you haven't touched in a while you won't touch soon.

The textbook O(1) LRU is a **hash map + a doubly-linked list**:

```
   map: key ──────────────► node   (O(1) lookup)

   doubly-linked list (most-recent → least-recent):
   ┌──────┐   ┌──────┐   ┌──────┐   ┌──────┐
   │ FRONT│⇄ │  ...  │⇄ │  ...  │⇄ │ BACK │
   └──────┘                         └──────┘
     hot                              cold  ← evict from here
```

- **On every access** (`get`, or any successful store), move that node to the
  **front** — "just used."
- **On insert past the cap**, remove from the **back** — the coldest item — and
  delete its map entry.

Both moves are O(1) because the map gives us the node directly (no list scan) and
splicing a node out of a doubly-linked list is constant time. We implement this
with Go's standard-library [`container/list`](https://pkg.go.dev/container/list)
(a ready-made doubly-linked list) plus a `map[string]*item`.

> 🐍 Python devs: this is exactly the data structure behind
> `functools.lru_cache` and `collections.OrderedDict.move_to_end()`. We're
> building the OrderedDict by hand so you can see the moving parts.

### 5. Slab allocation (what *real* memcached does — and what we deliberately don't)

Real memcached does **not** `malloc` a fresh chunk for every item. That would
fragment the heap badly: store a 50-byte value, free it, try to reuse the hole
for a 60-byte value — doesn't fit, gap wasted. Over time the heap becomes Swiss
cheese.

memcached's fix is the **slab allocator**:

```
 Memory is carved into 1 MB PAGES. Each page belongs to a SLAB CLASS,
 and a class hands out fixed-size CHUNKS:

  class 1 chunks: [ 96B ][ 96B ][ 96B ] …      ← tiny items
  class 2 chunks: [ 120B ][ 120B ][ 120B ] …
  class 3 chunks: [ 152B ][ 152B ] …           ← each class ≈ 1.25× the previous
  …
  class N chunks: [ up to 1 MB ]               ← the max item size
```

- To store an item, pick the **smallest class whose chunk fits it**, and use a
  free chunk from that class. Freeing returns the chunk to its class's free list.
- Because chunks within a class are identical in size, **a freed chunk can always
  be reused** by the next item of that class — no fragmentation, O(1) alloc/free.
- The trade-off is **internal waste**: a 100-byte value in a 120-byte chunk
  wastes 20 bytes. The 1.25× growth factor keeps that bounded.
- LRU is actually maintained **per slab class**, so eviction picks the coldest
  item *of the right size*.

**What we built instead:** plain Go heap allocation — each value is a `[]byte`,
each item a struct, and Go's garbage collector reclaims them. We get clarity and
correctness; we give up the fragmentation control and predictable memory ceiling
that slabs provide. For a learning server that's the right call. The README
points at slabs so you know what the production system does differently and *why*
(performance under churn at scale), without drowning the core protocol lesson in
allocator code.

---

## 🏗️ Architecture & Design

One concern per file, so each idea has an obvious home:

```
store.go    The in-memory store: item struct (value, flags, CAS, expiry),
            the map + container/list LRU, sync.Mutex safety, expiry math,
            incr/decr, and eviction. This is the brain.
conn.go     The TEXT protocol: read a command line, parse it, read the data
            block for storage commands, call the store, write the reply.
server.go   The TCP plumbing: net.Listener, accept loop, goroutine-per-conn,
            graceful Close().
main.go     The CLI: parse --addr/--max-items/--verbose, wire it together.
```

Dependency flow is a straight line: `main → server → conn → store`. Each layer is
testable on its own — the store against direct method calls, the protocol+server
against a real local socket.

> 🐍 **Concurrency model, for a Python dev.** Go's idiom is *one goroutine per
> connection*. A goroutine is a function scheduled by the Go runtime onto a small
> pool of OS threads — vastly cheaper than a thread, so "thousands of concurrent
> connections, one goroutine each" is normal and cheap (no `asyncio` event loop,
> no callbacks; you write straight-line blocking code and the runtime multiplexes
> it). All those goroutines share **one `Store`**, so the store guards every
> operation with a `sync.Mutex` (like `threading.Lock`) — taken at the top of each
> method, released by `defer`. That single lock is what keeps the map and the LRU
> list moving in lock-step. (A production cache would *shard* the map into N
> independently-locked pieces to reduce contention; we note that but keep one lock
> for clarity.)

> 🐍 **Injectable clock, for testability.** The store's "current time" is a field
> `now func() time.Time`, defaulting to `time.Now`. Tests swap in a fake clock and
> *advance* it instantly, so expiry tests are deterministic and take zero
> wall-clock time — no flaky `time.Sleep`. Same dependency-injection trick the
> curl challenge uses for its I/O streams.

---

## 🔨 Step-by-Step Implementation

1. **`store.go` — the store first.** Define `item` (value, flags, CAS, expiresAt,
   and a back-pointer into the LRU list). Build `Store` with a `map[string]*item`,
   a `container/list.List`, a `sync.Mutex`, a CAS counter, and the injectable
   clock. Implement `normalizeExpiry` (the three exptime modes), then the public
   API: `Set/Add/Replace/Append/Prepend/CAS/Get/Delete/IncrDecr/FlushAll`. Each
   public method locks, calls a small `*Locked` helper (`liveLocked`,
   `touchLocked`, `insertLocked`, `evictIfNeededLocked`), and unlocks via `defer`.
2. **`server.go` — the socket.** `net.Listen("tcp", addr)`, then an `Accept` loop
   that spawns `go handle(conn)` per connection. Track open conns so `Close()` can
   shut down cleanly (used by tests). Binding to `:0` lets tests grab a free port.
3. **`conn.go` — the protocol.** Wrap the connection in `bufio.Reader`/`Writer`.
   Loop: `readLine` → split on spaces → `dispatch`. Storage commands additionally
   call `readDataBlock(n)` which does `io.ReadFull` for exactly `n` bytes then
   verifies the trailing CRLF. Map each store result to its reply word. Honour the
   optional `noreply` token (suppresses the reply — clients use it for pipelined
   bulk loads).
4. **`main.go` — the CLI.** Stdlib `flag` for `--addr/--max-items/--verbose`
   (long flags → `flag` is idiomatic here; the Unix-filter challenges hand-roll
   their parsers only for exotic short-flag bundling). A thin `main()` calls a
   testable `run(args, logw)`.

> 🐍 **`container/list` gotcha.** It stores `any` (Go's `interface{}`), so reading
> a node's payload needs a *type assertion*: `node.Value.(*item)`. That's the Go
> equivalent of trusting `isinstance` — checked at runtime. We always push
> `*item` in, so the assertion is safe.

---

## 🧪 Testing Strategy

Two layers, **fully self-contained — no external memcached, no network, no
sleeping**:

1. **`store_test.go` — unit tests against the store directly.** set/get, the
   add/replace exist-vs-missing matrix, append/prepend, the full **CAS** dance
   (stale token → `EXISTS`, fresh token → `STORED`, reused token rejected),
   **expiry via the injected fake clock** (set a 2s TTL, `advance(2s)`, assert the
   item is gone *and* reaped), negative-exptime dead-on-arrival, incr/decr
   (including the decr-floors-at-zero and non-numeric→error rules), delete,
   flush, and two **LRU eviction** tests (touch one key so a *different* one is
   evicted; and oldest-evicted-when-never-touched).

2. **`server_test.go` — end-to-end over a real TCP socket.** A tiny `testClient`
   dials `127.0.0.1:0` (kernel-chosen port) and speaks the raw protocol by hand —
   exactly like `telnet`. It exercises the whole `accept → parse → store → reply`
   path: set-then-get returns the value; **get of a missing key returns just
   `END`**; gets returns a CAS token; add/replace; append/prepend; CAS over the
   wire; delete → `DELETED`/`NOT_FOUND`; incr/decr; flush_all; `noreply`; unknown
   command → `ERROR`; **expiry** (injected clock again, but driven through the
   socket); and **LRU eviction** (fill past a `--max-items 2` cap, assert the
   coldest key is evicted). Table-driven where it pays off; raw byte assertions
   where framing matters.

Run them:

```bash
go vet ./...
CGO_ENABLED=0 go test ./...     # see the toolchain note below
```

Result on this machine: **`go vet` clean; all 24 tests pass** under
`CGO_ENABLED=0`. A live smoke test of the built binary (driven with `nc`)
confirmed `set/get`, `incr` on a non-numeric value → `CLIENT_ERROR`, and LRU
eviction under `--max-items 2`.

### ⚠️ Environment / toolchain note (read this if `go test` aborts)

On this macOS dev box, a plain `go test ./...` can abort before any test runs
with:

```
dyld: missing LC_UUID load command
signal: abort trap
```

This is **not a bug in our code** — it's a known mismatch between the Go
toolchain's external linker and the installed Xcode Command-Line Tools `ld`,
triggered because this package imports `net` (which pulls in **cgo** for the
system DNS resolver). The fix is to build in pure-Go mode, which uses Go's
internal linker and native resolver:

```bash
CGO_ENABLED=0 go test ./...     # ✅ all tests pass
```

`go vet ./...` passes either way, and `CGO_ENABLED=0 go build` produces a clean
static binary. (The same note appears in the
[curl challenge](../../phase-03-advanced-cli/curl/) — it affects every networking
challenge in this repo.)

---

## 💡 Key Takeaways

- **Two framing styles in one protocol.** Command lines are *delimiter-framed*
  (`\r\n`); values are *length-prefixed* (`<bytes>` then exactly that many raw
  bytes). Length-prefixing is what lets a cache store opaque binary blobs safely —
  the same lesson as HTTP's `Content-Length`.
- **A cache must forget, and LRU is how.** Map + doubly-linked list = O(1)
  lookup, O(1) "mark as used," O(1) "evict the coldest." This is the engine
  behind every `lru_cache` you've ever used.
- **Optimistic concurrency without locks.** A CAS version token lets many clients
  race to update a key and only the one with the current version wins — the rest
  just retry. No lock is held across the network round-trip.
- **Lazy expiry beats a reaper.** Check the deadline on access and let LRU mop up
  the rest; don't burn CPU sweeping millions of keys.
- **Go's server idiom is "goroutine per connection + a shared, mutex-guarded
  store."** Straight-line blocking code, cheap concurrency, one lock keeping the
  map and LRU list consistent.
- **Know what production does differently.** Real memcached uses a **slab
  allocator** to kill fragmentation and bound memory; we use the Go heap for
  clarity. Knowing the gap is the point of the exercise.

### 🆚 Memcached vs Redis

Both are in-memory stores, built side-by-side in this phase, but they teach
opposite lessons:

| | **Memcached (this challenge)** | **Redis (separate challenge)** |
| --- | --- | --- |
| Data model | Opaque **blobs** only (key → bytes) | **Rich types**: strings, lists, hashes, sets, sorted sets |
| Protocol | Simple text: line + length-prefixed block | RESP — a typed, recursively-framed protocol |
| Persistence | None — pure cache, data is disposable | Optional RDB snapshots / AOF log |
| Eviction | LRU is *central* (it's a cache) | Configurable; persistence means it's also a database |
| Memory | **Slab allocator** (fixed-size chunk classes) | jemalloc + per-type encodings (ziplist, intset…) |
| The lesson here | **Caching mechanics**: expiry, CAS, LRU, slabs | **Data-structure server** + serialization + persistence |

Memcached is the *purest* expression of "a cache": forgettable, blob-only, and
all about eviction. That focus is exactly why it's a great vehicle for learning
LRU and the slab idea.

---

## 📖 Further Reading

- 🐍➡️🐹 [Go Quickstart for a Python Developer](../../docs/go-quickstart.md) — the project Go primer
- [Coding Challenges — Build Your Own Memcached Server](https://codingchallenges.fyi/challenges/challenge-memcached)
- [memcached TEXT protocol spec (`protocol.txt`)](https://github.com/memcached/memcached/blob/master/doc/protocol.txt) — the authoritative reference for every command and reply
- [memcached wiki — Slab allocation & memory management](https://github.com/memcached/memcached/wiki/UserInternals) — how slab classes and the 1.25× growth factor work
- [memcached wiki — LRU and the item lifecycle](https://github.com/memcached/memcached/wiki/Overview)
- Go stdlib: [`net`](https://pkg.go.dev/net), [`bufio`](https://pkg.go.dev/bufio), [`container/list`](https://pkg.go.dev/container/list), [`sync`](https://pkg.go.dev/sync)
- Wikipedia: [Cache replacement policies (LRU)](https://en.wikipedia.org/wiki/Cache_replacement_policies#Least_recently_used_(LRU))
