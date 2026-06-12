# Redis Server

> **Phase:** 5 — Servers & Infrastructure
> **Difficulty:** 🟠
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** XL

**Status:** ✅ Completed

> 🐍➡️🐹 **New to Go?** Read the
> [**Go Quickstart for a Python Developer**](../../docs/go-quickstart.md) first.
> It covers goroutines, slices, `bufio`, interfaces, error handling, and the
> `go test` workflow that this challenge leans on heavily. Throughout this README,
> **🐍 callouts** translate a Go idiom into its Python/Java equivalent.

---

## 🎯 What We're Building

We're building a **Redis-compatible server from scratch in Go** — a networked,
in-memory key/value store that speaks the real **RESP** protocol over TCP. By the
end you can point tools and clients at it, run `SET`/`GET`/`INCR`/`EXPIRE`, watch
keys expire, and persist the dataset to disk so it survives a restart.

[Redis](https://redis.io) ("**RE**mote **DI**ctionary **S**erver") is one of the
most deployed pieces of infrastructure on earth — used for caching, session
storage, rate limiting, leaderboards, queues, and pub/sub. Underneath the rich
feature set, its core is surprisingly approachable: a giant hash map, a simple
text-ish wire protocol, and a single-threaded command loop. Building a subset
teaches you the parts that matter:

- A **line/length-framed binary protocol** (RESP) you encode and decode by hand.
- A **concurrency-safe in-memory store** shared across many client connections.
- **Key expiry** with two cooperating strategies (lazy + active).
- **Persistence** — snapshotting the whole dataset to disk and reloading it.

This is the XL capstone of Phase 5. It reuses the **parsing** muscle from Phase 1
(the JSON Parser) and the **TCP-socket + concurrency** muscle from the Phase 5 Web
Server. Later, the **Phase 7 Redis CLI (Challenge 47)** is a RESP *client* that
will talk to **this very server** — so the protocol we implement here is the
contract the client depends on. Build the server first, exactly as the curriculum's
dependency graph (`Redis Server (32) → Redis CLI (47)`) prescribes.

### What it can do

```text
$ redis-server --addr :6379 --rdb dump.rdb
redis-server listening on [::]:6379
```

```text
SET name malcolm          -> OK
GET name                  -> "malcolm"
SET token abc EX 60       -> OK          (expires in 60s)
TTL token                 -> 60
INCR visits               -> 1
INCR visits               -> 2
MSET a 1 b 2              -> OK
MGET a missing b          -> ["1", nil, "2"]
SAVE                      -> OK          (snapshot written to dump.rdb)
```

### Supported commands

| Group        | Commands                                                            |
| ------------ | ------------------------------------------------------------------- |
| Connection   | `PING`, `ECHO`                                                       |
| Strings      | `SET` (with `EX`/`PX`/`NX`/`XX`), `GET`, `GETSET`, `APPEND`, `MGET`, `MSET` |
| Keys         | `DEL`, `EXISTS`, `KEYS`, `EXPIRE`, `TTL`                             |
| Numbers      | `INCR`, `DECR`                                                       |
| Persistence  | `SAVE`, `BGSAVE`                                                     |

---

## 📚 Core Concepts

### 1. RESP — the REdis Serialization Protocol

Everything a Redis client and server exchange is framed using **RESP** (we
implement **RESP2**). It is a compact, mostly-ASCII protocol where **the first byte
of every payload declares its type**, and **`\r\n` (CRLF) terminates every line**.
That single rule — *type byte first, CRLF-terminated lines* — is the whole game.

RESP2 has exactly **five types**:

| Type            | First byte | Example bytes (CRLF shown as `\r\n`)     | Meaning                          |
| --------------- | ---------- | ---------------------------------------- | -------------------------------- |
| Simple String   | `+`        | `+OK\r\n`                                | short status line, not binary    |
| Error           | `-`        | `-ERR unknown command\r\n`               | like a simple string, but an error |
| Integer         | `:`        | `:1000\r\n`                              | a signed 64-bit integer          |
| Bulk String     | `$`        | `$5\r\nhello\r\n`                        | **length-prefixed**, binary-safe |
| Array           | `*`        | `*2\r\n$1\r\na\r\n$1\r\nb\r\n`           | a count, then that many values   |

Plus two crucial **null** forms:

- **Null bulk string**: `$-1\r\n` — the canonical *"key does not exist"* answer.
- **Null array**: `*-1\r\n` — *"no result"* for commands that return collections.

#### Two ways to frame data (the key insight)

RESP mixes **two framing techniques**, and understanding the difference is the
heart of this challenge:

- **Delimiter framing** (simple strings, errors, integers): read bytes until you
  hit `\r\n`. The terminator *is* the boundary.
- **Length-prefix framing** (bulk strings, arrays): a header announces *how many*
  bytes/elements follow, then you read exactly that many.

Why have both? Length-prefixing makes bulk strings **binary-safe**: because the
server is told "the next value is 5 bytes," the payload may itself contain `\r\n`,
NUL bytes, or any binary data without confusing the parser. Simple strings can't
do that (a `\r\n` inside them would end the line early), which is why Redis uses
them only for short, controlled status words like `OK` and `PONG`.

> 🐍 This is the same lesson as HTTP's `Content-Length` (Phase 4 curl / Phase 5
> Web Server) and Memcached's value framing (Challenge 33): *structured headers
> describe an opaque, length-delimited body.* Once you've seen it in three
> protocols it becomes second nature.

#### How a client sends a command

A client **always** sends a command as a **RESP array of bulk strings**. So
`SET name malcolm` goes on the wire as:

```text
*3\r\n              <- array of 3 elements
$3\r\nSET\r\n       <- bulk string "SET"
$4\r\nname\r\n      <- bulk string "name"
$7\r\nmalcolm\r\n   <- bulk string "malcolm"
```

The server decodes that array, dispatches on the first element (`SET`), and
replies with a single RESP value (`+OK\r\n`). That request→reply rhythm repeats
for the life of the connection. Because we implement the genuine protocol,
RESP-speaking clients (and the Phase 7 Redis CLI) work conceptually against this
server with no special-casing.

### 2. The in-memory store

At its core Redis is a **hash map** from string keys to values. Ours maps
`string → entry`, where an `entry` holds the value and an optional expiry deadline.
Keeping everything in RAM is what makes Redis microsecond-fast — and is also why
persistence matters (RAM is volatile).

Because many client goroutines touch this one map **in parallel**, every access is
guarded by a `sync.RWMutex`: read-heavy commands take a shared *read* lock and run
concurrently; writers take the exclusive *write* lock. Real Redis sidesteps locking
by being single-threaded with an event loop; we choose the idiomatic-Go
goroutine-per-connection model instead and protect the shared map. (A production
clone would **shard** the keyspace into N independently-locked maps to reduce
contention — noted but not required here.)

> 🐍 Go has **no GIL**. Two goroutines really can write the same map at the same
> instant, and Go treats a concurrent map write as a *fatal crash*, not a subtle
> race. The mutex isn't optional politeness — it's mandatory correctness.

### 3. Key expiry — lazy vs. active

A key with a TTL must eventually disappear. Redis (and we) use **two strategies
together**, because each alone is insufficient:

- **Lazy expiry (on access):** when someone reads a key, check its deadline; if
  it's past, delete it then and report a miss. Cheap and precise — you only pay for
  keys you actually touch. **Problem:** a key that's set-with-TTL and then *never
  read again* would linger in memory forever.
- **Active expiry (background sweep):** a timer periodically scans the keyspace and
  evicts expired keys. This bounds the memory wasted by forgotten keys. **Problem:**
  scanning everything constantly is wasteful, so it runs on an interval, meaning a
  key can briefly outlive its TTL between sweeps.

Used together they cover each other's blind spots: lazy expiry gives correct
results *the instant* you read, and the sweeper reclaims the silent stragglers. We
implement both — lazy deletion inside `Store.Get`/`Set`, and a `SweepExpired` method
driven by a background goroutine on a ticker.

### 4. Persistence — RDB snapshots (and a word on AOF)

In-memory data vanishes on restart, so Redis offers two durability mechanisms:

- **RDB (Redis Database) snapshots:** dump the *entire dataset* to a single file at
  a point in time. Compact, fast to load, perfect for backups — but you lose any
  writes made since the last snapshot if you crash.
- **AOF (Append-Only File):** log *every write command* to a file as it happens;
  rebuild state on startup by replaying the log. More durable (you can lose at most
  a second of writes) but the file grows and must be periodically rewritten/compacted.

We implement a **simple RDB-style snapshot**. ⚠️ **Our file format is our own** —
it is *not* binary-compatible with real Redis's `.rdb` files. Genuine RDB has
opcodes, a bespoke length encoding, optional LZF compression, and a CRC64 checksum;
reproducing that byte-for-byte would dwarf the rest of the challenge and teach
little extra. Our format is a tiny **length-prefixed** text format (so values stay
binary-safe) — enough to demonstrate the *idea*: serialise the keyspace, then
rebuild it on startup. `SAVE` writes it synchronously; `BGSAVE` writes it from a
goroutine; and with `--rdb` set we also **load-on-start** and **save-on-shutdown**.
AOF is described here but left as an optional extension.

---

## 🏗️ Architecture & Design

```text
                    ┌──────────────────────────────────────────────┐
   TCP clients ───▶ │  net.Listener (Accept loop)                  │
   (redis-cli,      │     │  goroutine-per-connection               │
    Phase 7 CLI)    │     ▼                                         │
                    │  handleConn(conn)                             │
                    │   ├─ bufio.Reader ──▶ DecodeValue ──▶ request │
                    │   ├─ dispatch(request) ──▶ commandTable[name] │
                    │   └─ reply.Marshal() ──▶ bufio.Writer ──▶ net │
                    │                    │                          │
                    │                    ▼                          │
                    │            ┌───────────────┐   ┌────────────┐ │
                    │            │  Store        │◀──│ active     │ │
                    │            │  RWMutex      │   │ expiry     │ │
                    │            │  map[str]entry│   │ goroutine  │ │
                    │            └───────┬───────┘   └────────────┘ │
                    │                    │ snapshot/load            │
                    │                    ▼                          │
                    │            persistence (RDB file)            │
                    └──────────────────────────────────────────────┘
```

The code is split into focused files, each owning one concern. This separation is
deliberate: the store knows nothing about sockets, the protocol knows nothing about
commands, and the commands know nothing about bytes on the wire. That layering is
what makes each piece independently testable.

| File             | Responsibility                                                             |
| ---------------- | -------------------------------------------------------------------------- |
| `resp.go`        | RESP2 encoder (`Marshal`) + decoder (`DecodeValue`) — the protocol.        |
| `store.go`       | Concurrency-safe map, TTLs, lazy + active expiry, INCR/APPEND/etc.         |
| `commands.go`    | The dispatch table and one handler per command.                            |
| `server.go`      | TCP listener, accept loop, per-connection reader, background sweeper.      |
| `persistence.go` | RDB-style snapshot `Save`/`Load`.                                          |
| `main.go`        | CLI flag parsing, startup, signal-driven graceful shutdown.                |
| `*_test.go`      | Table-driven protocol tests + end-to-end TCP tests (no external deps).     |

### Why these choices

- **Goroutine-per-connection** (not an event loop): it's the idiomatic Go shape and
  far simpler than hand-rolling epoll. Goroutines are cheap enough that thousands of
  connections are routine.
- **One tagged `Value` struct** for RESP (not an interface hierarchy): keeps the
  encoder a single `switch` and makes test assertions trivial.
- **Dispatch via a `map[string]handler`** (not a giant `switch`): adding a command
  is one map entry; it's the canonical Go pattern for command routers.
- **Injectable clock** (`Store.now`): lets expiry tests advance time instantly with
  zero `time.Sleep`, so they're fast and never flaky.

---

## 🔨 Step-by-Step Implementation

The whole phase follows the same server arc: **(1) read the protocol spec → (2)
build a minimal single-connection version → (3) add concurrency → (4) add
persistence.** Here's how that maps to our build.

### Step 1 — The RESP codec (`resp.go`)

Start with the protocol, because everything else depends on it.

- Define the five type-byte constants and a `Value` tagged struct.
- Write `Marshal()` as a `switch` over the type, appending bytes to a slice.
  Handle the null bulk (`$-1\r\n`) and null array (`*-1\r\n`) special cases.
- Write `DecodeValue(*bufio.Reader)` as the mirror image. The subtle parts:
  - **Bulk strings** read the length, then `io.ReadFull` exactly `len+2` bytes
    (payload + framing CRLF). `io.ReadFull` is the idiomatic "give me exactly N
    bytes or error" call — vital because one TCP read may return a short slice.
  - **Arrays** read a count and **recurse** `DecodeValue` that many times, which is
    how nested arrays just work.

> 🐍 `bufio.Reader.ReadString('\n')` is like reading a line from a buffered file,
> but we drive the framing ourselves rather than trusting a line-based library —
> because bulk-string bodies can contain `\n`.

### Step 2 — The store (`store.go`)

- `Store` wraps `map[string]entry` with a `sync.RWMutex` and an injectable `now`.
- `Get` applies **lazy expiry**: under the write lock, if the key is past its
  deadline, delete it and report a miss.
- `Set` handles `NX`/`XX` guards and optional TTL; `Expire`/`TTL` follow Redis's
  `-2`(missing)/`-1`(no-expiry)/`≥0`(seconds) convention.
- `INCR`/`DECR` parse-mutate-format the value, erroring on non-integers.
- `SweepExpired` implements **active expiry**: scan and delete every expired key.

### Step 3 — Commands + server (`commands.go`, `server.go`)

- `commandTable` maps uppercased names to handlers; `dispatch` validates the
  request is a non-empty array and routes on element 0.
- `server.go` binds a `net.Listener`, and the **accept loop** spawns
  `go handleConn(conn)` per client. `handleConn` loops: decode a request, dispatch,
  write the marshalled reply, flush.
- A **background goroutine** (`activeExpiryLoop`) ticks every `sweepInterval` and
  calls `SweepExpired`. A `quit` channel + `sync.WaitGroup` give clean shutdown.

> 🐍 `go handleConn(conn)` is "start a thread per client," but goroutines are
> lightweight green threads. The `quit` channel is Go's idiomatic stop signal —
> closing it makes every `<-s.quit` in every goroutine fire at once.

### Step 4 — Persistence + CLI (`persistence.go`, `main.go`)

- `Save` snapshots the live keyspace to a temp file then **atomically renames** it
  over the target (a crash mid-write can't corrupt a good snapshot). `Load` reads it
  back, skipping any keys already expired on disk.
- `main.go` uses stdlib `flag` for `--addr`/`--rdb`, loads the snapshot on start,
  serves, and on `SIGINT`/`SIGTERM` saves the snapshot before exiting.

> 🐍 `flag` is `argparse`-lite. The temp-file-then-`os.Rename` trick is the standard
> way to get crash-safe file writes on POSIX filesystems.

---

## 🧪 Testing Strategy

All tests are **self-contained** — they import **no real Redis client library** and
spin up servers on `127.0.0.1:0` (an OS-assigned free port). Run them with:

```bash
cd phase-05-servers-infrastructure/redis-server
go vet ./...
CGO_ENABLED=0 go test ./...
```

> ⚠️ **macOS / go1.22 toolchain note.** On some macOS + Xcode CLT combinations,
> linking a test binary that imports `net` aborts with
> `dyld: ... missing LC_UUID load command`. The fix — used across this repo (curl,
> Web Server, Memcached) — is to disable cgo so Go uses its internal linker and pure
> Go resolver: **`CGO_ENABLED=0 go test ./...`**. Plain `go test` may work for you;
> if it aborts at link time, add the prefix.

The suite has three layers:

1. **RESP codec unit tests (`resp_test.go`)** — table-driven over **crafted bytes**,
   covering *all five types, both null forms, empty values, binary-safe payloads
   containing CRLF/NUL, and nested arrays*. There's a `Marshal` table, a
   `DecodeValue` table, a `RoundTrip` property check (encode→decode is identity), and
   negative tests asserting malformed frames are rejected.
2. **Store unit tests (`store_test.go`)** — drive expiry deterministically via the
   **injected clock**: set a TTL, advance the mock time, assert the key vanishes
   (lazy) and that `SweepExpired` reaps stragglers (active) without touching
   no-expiry keys. Also covers NX/XX, INCR/DECR on non-integers, APPEND/GETSET/MSET.
3. **End-to-end TCP tests (`server_test.go`)** — a tiny in-test RESP client (reusing
   *our own* codec) drives a live server over a real socket: `SET`/`GET`/`DEL`,
   `EXISTS`, `INCR`/`DECR`, `EXPIRE`/`TTL`, a `PX`-based expiry that waits for the
   background sweeper, `MGET`/`MSET`/`APPEND`/`GETSET`, and the headline
   **SAVE-then-reload round-trip**: populate a server, `SAVE`, shut it down, start a
   **brand-new server** pointed at the same file, and assert every key (TTL included)
   was restored.

The whole suite also passes under the **race detector**
(`CGO_ENABLED=0 go test -race ./...`), proving the locking is sound.

---

## 💡 Key Takeaways

- **RESP is small once you see the two framing styles.** A type byte plus CRLF
  lines for the simple types; length prefixes for the binary-safe ones (bulk
  strings, arrays). Clients send commands as arrays of bulk strings; servers reply
  with one value. That's the entire protocol.
- **Length-prefix framing buys binary safety** — the same idea as HTTP
  `Content-Length`. You'll keep meeting it.
- **A cache is a guarded hash map.** The interesting engineering is everything
  *around* the map: concurrency, expiry, persistence.
- **Expiry wants two strategies.** Lazy gives correctness on access; active bounds
  wasted memory. Neither suffices alone.
- **Snapshots are just careful serialization** — and atomic rename is how you make
  the write crash-safe.
- **Go idioms you practised:** `bufio` stream parsing, `io.ReadFull` for exact
  reads, a `map[string]func` dispatch table, `sync.RWMutex` over shared state, a
  background goroutine + `time.Ticker` + `quit` channel, an injectable clock for
  testable time, and `os.Rename` for atomic file swaps.
- **This server is a contract.** The **Phase 7 Redis CLI (Challenge 47)** is a RESP
  client that talks to *this* server — which is exactly why the curriculum has you
  build the server first.

---

## 📖 Further Reading

- 🐍➡️🐹 [Go Quickstart for a Python Developer](../../docs/go-quickstart.md) — the project Go primer
- [RESP protocol specification](https://redis.io/docs/latest/develop/reference/protocol-spec/) — the authoritative wire-format reference
- [Redis command reference](https://redis.io/commands/) — exact semantics of every command we implemented
- [Redis persistence (RDB & AOF)](https://redis.io/docs/latest/operate/oss_and_stack/management/persistence/) — how the real thing durably stores data
- [Redis key expiration](https://redis.io/docs/latest/develop/use/keyspace/#key-expiration) — the lazy + active strategy, from the source
- [Build Your Own Redis (codingchallenges.fyi)](https://codingchallenges.fyi/challenges/challenge-redis/) — the challenge brief
- Go stdlib: [`bufio`](https://pkg.go.dev/bufio), [`net`](https://pkg.go.dev/net), [`sync`](https://pkg.go.dev/sync) — the packages this build leans on
