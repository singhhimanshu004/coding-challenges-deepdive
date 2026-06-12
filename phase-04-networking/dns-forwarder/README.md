# DNS Forwarder

> **Phase:** 4 — Networking Fundamentals
> **Difficulty:** 🔵
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** M

**Status:** ✅ Completed

> 🐍➡️🐹 **New to Go?** Read the project's
> [**Go Quickstart for a Python Developer**](../../docs/go-quickstart.md) first.
> It maps every Go idiom used here (`defer`, goroutines, slices vs. maps, error
> returns, structs/methods) back to the Python you already know. This README
> assumes you've skimmed it and adds 🐍 callouts where Go does something
> surprising.

This challenge is the direct sequel to **[DNS Resolver](../dns-resolver/)**
(#23). There you built a DNS *client* that hand-encoded a query and parsed the
reply. Here you reuse that exact wire-format knowledge to build a DNS *server*.

---

## 🎯 What We're Building

A **caching, forwarding DNS server.** It listens on a UDP port, and for every
query a client sends it does one of two things:

- **Cache hit:** it already knows the answer (and that answer hasn't expired), so
  it replies instantly from memory — the upstream is never contacted.
- **Cache miss:** it **forwards** the query to an upstream recursive resolver
  (default `8.8.8.8:53`), relays the reply back to the client, and **caches** it
  for as long as the record's TTL allows.

```
dns-forwarder [--listen :1053] [--upstream 8.8.8.8:53] [--verbose]

--listen ADDR     UDP address to listen on        (default :1053)
--upstream ADDR   resolver to forward to           (default 8.8.8.8:53)
--verbose         log cache hits / misses / forwards to stderr
```

This is exactly what a home router, a corporate DNS box, or `dnsmasq` /
`systemd-resolved` does on your machine: it doesn't resolve names from the root
itself — it **forwards to a smarter resolver and caches the results** so your
laptop doesn't re-ask "where is google.com?" a thousand times a day.

```bash
# Terminal 1: start the forwarder (no root needed on :1053)
dns-forwarder --verbose

# Terminal 2: point dig at it and watch the cache
dig @127.0.0.1 -p 1053 example.com        # MISS → forwarded upstream
dig @127.0.0.1 -p 1053 example.com        # HIT  → served from cache, instant
```

### Running on the real DNS port (`:53`)

Port 53 is privileged (below 1024), so binding it needs elevated rights. We
default to `:1053` so you can experiment without `sudo`. To use the real port:

```bash
sudo dns-forwarder --listen :53            # macOS / Linux: bind a privileged port
```

On Linux you can instead grant the binary the capability once
(`sudo setcap 'cap_net_bind_service=+ep' ./dns-forwarder`) and then run it as a
normal user.

---

## 📚 Core Concepts

### 1. Recursive vs. forwarding resolver

There are three jobs in the DNS world; don't confuse them:

| Role | What it does | Example |
| --- | --- | --- |
| **Authoritative server** | Holds the real records for a zone | `ns1.example.com` |
| **Recursive resolver** | Walks root → TLD → authoritative for you | `8.8.8.8`, your ISP |
| **Forwarding resolver** | Doesn't walk anything — just **forwards** to a recursive resolver and **caches** | this challenge, your home router |

The dns-resolver challenge built a *recursive* resolver (with `--trace` it walked
the delegation chain from the root itself). A **forwarder is deliberately
lazier**: it never talks to root servers. It trusts an upstream to do the hard
work and focuses on two things a recursive resolver also needs — **a UDP server
loop** and **a cache**.

```
         ┌──────────────┐                          ┌──────────────┐
 client  │              │   cache MISS: forward    │   upstream   │
 ───────►│ dns-forwarder│ ───────────────────────► │  8.8.8.8:53  │
 ◄───────│ (this code)  │ ◄─────────────────────── │ (recursive)  │
         │   + cache    │      relay the reply      └──────────────┘
         └──────────────┘
              ▲   │
   cache HIT  │   │  no upstream traffic at all
              └───┘
```

### 2. A UDP server is `ListenUDP` + a read loop

In the resolver you were a **client**: `net.DialUDP` gave you a socket already
pointed at a server. A server is the mirror image:

```go
addr, _ := net.ResolveUDPAddr("udp", ":1053")
conn, _ := net.ListenUDP("udp", addr)   // bind the port; now we receive

for {
    n, client, _ := conn.ReadFromUDP(buf) // who sent it? `client`
    // ... figure out the answer ...
    conn.WriteToUDP(answer, client)        // reply to that exact peer
}
```

The key difference from TCP: **UDP has no connection.** There's no `Accept()`,
no per-client socket. One socket receives datagrams from *everyone*, and each
read also tells you the sender's address so you know where to send the reply.

> 🐍 `conn.ReadFromUDP` / `conn.WriteToUDP` are Python's
> `sock.recvfrom()` / `sock.sendto((ip, port))`. The address travels with every
> datagram because there's no fixed peer.

### 3. TTL-based caching

Every DNS record carries a **TTL** (time-to-live) — a number of seconds set by
whoever owns the record, meaning *"you may reuse this answer for up to this
long."* `example.com`'s A record might have `TTL=300`, i.e. "good for 5
minutes." Caching is simply respecting that promise:

1. On a cache miss, parse the upstream reply and find the TTL.
2. Store `(question) → (reply bytes, expiry = now + TTL)`.
3. On the next identical question, if `now < expiry`, serve the stored bytes.
4. Once `now ≥ expiry`, the entry is stale — drop it and forward again.

**Which TTL do we use when an answer has several records?** The **minimum**. The
answer set is only as fresh as its shortest-lived member, so caching for the
smallest TTL is the safe, conservative choice.

**The cache key is the question, not just the name.** Two clients can ask about
the same domain but want different record types (`A` vs `AAAA` vs `MX`). The key
must be the full triple **(QNAME, QTYPE, QCLASS)** or you'd hand back an IPv4
address to someone who asked for mail servers.

```
key   = ("example.com", A, IN)
value = { responseBytes, expiry: 2026-06-13T01:57:00 }
```

> One subtlety we handle: the cached bytes still contain the **transaction ID**
> of the *first* client that populated the cache. DNS clients reject a reply
> whose ID doesn't match their query, so before serving from cache we overwrite
> the first two header bytes with the *current* client's ID. (We serve the
> original TTL value rather than decrementing it on the wire — a deliberate
> simplification; see Key Takeaways.)

### 4. Concurrency safety

A server handles many clients at once. We spawn **one goroutine per request** so
a slow upstream for one client never blocks everyone else:

```go
go s.handle(packet, client)   // fire-and-forget worker
```

But those goroutines all touch the **same cache map** — and a plain Go map is
**not safe for concurrent use** (concurrent writes panic outright). We protect it
with a `sync.RWMutex`:

```go
c.mu.RLock();  e, ok := c.m[k]; c.mu.RUnlock()   // many readers in parallel
c.mu.Lock();   c.m[k] = entry;  c.mu.Unlock()    // one exclusive writer
```

> 🐍 A `RWMutex` is a readers-writer lock. **Many** goroutines may hold the read
> lock simultaneously; the write lock is exclusive. Because lookups vastly
> outnumber inserts, this lets the common path run fully in parallel — more
> permissive than Python's single `threading.Lock`. (Go also has `sync.Map`, but
> a `RWMutex` over a typed map is clearer for teaching and lets us reap expired
> entries cleanly.)

---

## 🏗️ Architecture & Design

One concern per file, so each idea has an obvious home:

```
message.go    DNS wire format: parse the question + answer TTLs.
              (Adapted from the dns-resolver sibling; self-contained — its own
               module `dnsforwarder`, nothing imported from the resolver.)
cache.go      Concurrency-safe TTL cache: RWMutex map + injectable clock.
forwarder.go  The server: ListenUDP loop, goroutine-per-request, forward,
              cache lookup/store, reply (with per-client ID patching).
main.go       CLI: flag parsing, bind the socket, start serving.
```

The dependency flow is a straight line: `main` → `forwarder` → (`cache`,
`message`). Each lower layer is testable on its own.

> 🐍 **Two dependency-injection seams make this testable without the internet:**
> (1) the cache takes a `now func() time.Time` clock, so tests simulate TTL
> expiry *instantly* instead of sleeping; (2) the server's `dialUpstream` is a
> field, and tests simply point `--upstream` at a **local** fake resolver. Same
> idea as passing a file object into a function instead of `open()`-ing inside
> it.

---

## 🔨 Step-by-Step Implementation

1. **`message.go` — parse only what we need.** We reuse the resolver's header,
   QNAME label encoding/decoding (including the dreaded **compression
   pointers**), and RR parsing — but stop after the answer section. Two helpers
   matter: extracting the `Question` (for the cache key) and `minTTL()` (for the
   expiry). We relay reply bytes **verbatim**, so we don't need to decode every
   record type — only enough to read TTLs.
2. **`cache.go` — the TTL map.** A `struct{name, qtype, qclass}` makes a clean,
   comparable map key. `set` records `expiry = now + TTL` (skipping `TTL=0`);
   `get` takes the read lock, and on finding an expired entry upgrades to the
   write lock to delete it (**lazy expiration** — reap on access, no background
   sweeper). The injectable clock makes all of this deterministically testable.
3. **`forwarder.go` — the server loop.** `ReadFromUDP` into a buffer, **copy the
   bytes** (the buffer is reused on the next read!), then `go s.handle(...)`.
   `handle` parses the question, checks the cache, forwards on a miss via a fresh
   UDP socket to the upstream, caches the reply at its min TTL, and writes back —
   patching the transaction ID on cache hits.
4. **`main.go` — wire up the CLI.** Hand-rolled flag parser (consistent with the
   other tools here), `net.ListenUDP` to bind, then `Serve()`. Closing the socket
   (Ctrl-C) makes the read loop return cleanly.

---

## 🧪 Testing Strategy

Every test is **hermetic — no public internet**, fast, and deterministic.

The trick: stand up a **fake upstream** — a local `net.ListenUDP` that returns a
canned A-record response and **counts how many queries it received** — and point
the forwarder at it. Watching that counter is how we prove caching works.

1. **`TestForwardAndRelay`** — one query in; assert the upstream was hit exactly
   once, the answer was relayed back, and the reply echoes the client's
   transaction ID.
2. **`TestSecondQueryServedFromCache`** — two *identical* queries (with
   **different** transaction IDs). Assert the upstream counter stays at **1**
   (second served from cache) and the cached reply carries the *second* client's
   ID (proving the per-client ID patch).
3. **`TestCacheExpiresAfterTTL`** — inject a fake clock. Query (miss, counter=1);
   advance to just inside the TTL and query again (still cache, counter=1);
   advance past the TTL and query again — assert the counter climbs to **2**
   (entry expired → upstream re-queried).
4. **`TestCacheTTL`** — table-driven unit test of the cache in isolation (no
   sockets): fresh hit, just-before-expiry hit, at-expiry miss, after-expiry
   miss, and `TTL=0` never cached.

Run them:

```bash
go vet ./...
CGO_ENABLED=0 go test ./...     # see the toolchain note below
```

### ⚠️ Environment / toolchain note (read this if `go test` aborts)

On this macOS dev box, a plain `go test ./...` can abort before any test runs
with:

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
`CGO_ENABLED=0 go build`.

---

## 💡 Key Takeaways

- **A forwarder is a recursive resolver minus the hard part, plus a cache.** It
  never walks the root — it leans on an upstream and earns its keep through
  caching. This is what your router and `dnsmasq` actually do.
- **A UDP server is `ListenUDP` + a read loop.** No `Accept`, no per-client
  socket; one socket serves everyone and every datagram carries its sender's
  address.
- **Cache the question, not the name.** The key is **(QNAME, QTYPE, QCLASS)** —
  cache by name alone and you'll serve IPv6 answers to IPv4 questions.
- **TTL is a promise; honour the minimum.** Expiry = `now + min(answer TTLs)`.
  Lazy expiration (reap on access) keeps it simple.
- **Goroutine-per-request demands a lock.** Concurrent goroutines + a shared map
  = a panic without a `sync.RWMutex` (or `sync.Map`). Reads are parallel; writes
  are exclusive.
- **The two gotchas that bite everyone:** (1) **copy the read buffer** before
  handing it to a goroutine — it's reused on the next `ReadFromUDP`; (2) **patch
  the transaction ID** when serving from cache, or the client rejects the reply.
- **Inject your clock (and your upstream).** A `now func()` and a swappable
  upstream address made the whole thing testable instantly and offline.

### Honest scope (deliberately left out)

- We don't **decrement TTLs on the wire** as time passes in the cache; we serve
  the original TTL. A production resolver counts the remaining seconds down.
- No **negative caching** (NXDOMAIN/empty answers aren't cached), no EDNS0, no
  DNS-over-TCP fallback for truncated (`TC=1`) replies, no cache size cap /
  eviction beyond TTL expiry. Each is a natural next step.

---

## 📖 Further Reading

- 🐍➡️🐹 [Go Quickstart for a Python Developer](../../docs/go-quickstart.md) — the project Go primer
- [DNS Resolver (challenge #23)](../dns-resolver/) — the client this server builds on; full wire-format walkthrough
- [Coding Challenges — Build Your Own DNS Forwarder](https://codingchallenges.fyi/challenges/challenge-dns-forwarder)
- [RFC 1035 — Domain Names: Implementation and Specification](https://datatracker.ietf.org/doc/html/rfc1035) (message format §4, TTL §3.2.1)
- [RFC 2181 — Clarifications to the DNS Specification](https://datatracker.ietf.org/doc/html/rfc2181) (TTL handling, §5.2 on differing TTLs)
- [RFC 1034 — Domain Concepts and Facilities](https://datatracker.ietf.org/doc/html/rfc1034) (resolvers, caching §3.6–§4)
- Go stdlib: [`net`](https://pkg.go.dev/net), [`sync`](https://pkg.go.dev/sync), [`encoding/binary`](https://pkg.go.dev/encoding/binary)
