# Traceroute

> **Phase:** 4 — Networking Fundamentals
> **Difficulty:** 🔵
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** M

**Status:** ✅ Completed

> 🐍➡️🐹 **New to Go?** Read the project's
> [**Go Quickstart for a Python Developer**](../../docs/go-quickstart.md) first.
> This README assumes you've skimmed it, and adds 🐍 callouts wherever Go does
> something a Python or Java developer wouldn't expect.

---

## 🎯 What We're Building

`traceroute` is the classic network-diagnostics tool that answers a deceptively
simple question: **"What path do my packets take to reach a host?"**

When you `ping example.com` you only learn *whether* you can reach it and *how
long* the round trip takes. Traceroute goes further — it lists **every router
(hop) between you and the destination, in order**, with the round-trip time to
each one:

```
$ traceroute 8.8.8.8
traceroute to 8.8.8.8, 30 hops max, 3 probes per hop
 1  10.2.0.1  1.2ms  1.1ms  1.3ms
 2  * 79.127.146.125  9.8ms  10.1ms
 3  79.127.195.177  10.0ms  9.9ms  10.2ms
 ...
 8  8.8.8.8  11.4ms  11.2ms  11.3ms
```

This is the tool network engineers reach for first when "the internet is slow"
or "I can't reach that server": it shows *where* on the path latency spikes or
packets vanish. We'll build it from scratch in Go, and along the way learn the
single most elegant trick in the IP toolbox — using the **Time-To-Live** field
for something it was never designed for.

---

## 📚 Core Concepts

### 1. IP TTL (Time To Live) — the heart of the whole thing

Every IP packet header contains an 8-bit field called **TTL** (Time To Live).
Its original purpose is *loop prevention*: without it, a misconfigured network
could bounce a packet between two routers forever. So the rule is:

> **Every router that forwards a packet decrements its TTL by 1. When a router
> receives a packet whose TTL is already 1 (so decrementing makes it 0), it does
> NOT forward it. Instead it DROPS the packet and sends an ICMP "Time Exceeded"
> message back to the original sender.**

Here's the brilliant part. That "Time Exceeded" error message comes *from the
router that dropped the packet* — so its **source IP address is that router's
address**. Traceroute weaponises this:

```
Send a packet with TTL = 1:
    Router #1 receives it, decrements TTL 1 → 0, drops it,
    and mails us back "Time Exceeded" — FROM router #1.   →  that's hop 1!

Send a packet with TTL = 2:
    Router #1 decrements 2 → 1, forwards it.
    Router #2 decrements 1 → 0, drops it, sends "Time Exceeded". →  hop 2!

Send a packet with TTL = 3:  →  reveals hop 3.
... and so on ...

Eventually a packet's TTL is large enough to reach the DESTINATION.
    The destination doesn't send "Time Exceeded" — it answers our probe
    directly (an ICMP Echo Reply, or a "Port Unreachable" for UDP probes).
    That different reply is our signal: "you've arrived. Stop."
```

By sweeping the TTL from 1 upward, **each router on the path is forced to
announce itself, in order.** That's traceroute. No special routing protocol, no
cooperation from the network — just a clever (ab)use of a loop-prevention
counter.

```
   You                R1          R2          R3       8.8.8.8
    |    TTL=1 ─────►  ✗ drop
    |  ◄──────────────  "Time Exceeded" (from R1)        hop 1
    |    TTL=2 ─────►  ──────────►  ✗ drop
    |  ◄───────────────────────────  "Time Exceeded" (R2) hop 2
    |    TTL=3 ─────►  ──────────►  ──────────►  ✗ drop
    |  ◄──────────────────────────────────────── "TimeEx"(R3) hop 3
    |    TTL=4 ─────►  ──────────►  ──────────►  ─────────►  ✓
    |  ◄──────────────────────────────────────────────── "Echo Reply"  DONE
```

### 2. ICMP — the "control plane" of IP

**ICMP** (Internet Control Message Protocol) is the protocol IP uses to send
*error and diagnostic* messages — it's how routers and hosts say "something went
wrong" or "are you there?". The two ICMP message types we care about:

| Type | Name | When it's sent | What it tells traceroute |
|------|------|----------------|--------------------------|
| 11   | **Time Exceeded** | A router dropped a packet because TTL hit 0 | "I'm an intermediate hop — here's my IP" |
| 0    | **Echo Reply** | The destination answered our ping (Echo Request, type 8) | "You reached me — the trace is over" |
| 3    | **Destination Unreachable** | Host/port can't be reached (e.g. UDP "Port Unreachable") | Also a "you arrived" signal (used by UDP-probe traceroute) |

Our probes are **ICMP Echo Requests** (type 8) — exactly what `ping` sends. The
only difference from `ping` is that we deliberately set a *small TTL* so the
probe expires partway and we collect the "Time Exceeded" replies.

> **Two flavours of traceroute exist.** The original Unix tool sends **UDP**
> packets to high, unused ports, and detects arrival via "Port Unreachable".
> Windows `tracert` and our implementation send **ICMP Echo Requests** and
> detect arrival via "Echo Reply". Both rely on the *same* TTL-expiry mechanism
> for the intermediate hops — only the "I arrived" signal differs. We classify
> all three reply types (`Time Exceeded`, `Echo Reply`, `Dest Unreachable`) so
> the logic is robust.

### 3. Raw sockets vs. unprivileged ICMP datagram sockets

To *send* an ICMP packet you normally need a **raw socket** — a socket that lets
you hand-craft packets below the TCP/UDP layer. The catch:

> **Raw sockets require root / administrator privileges** (`CAP_NET_RAW` on
> Linux). That's why the system `traceroute` and `ping` are often installed
> setuid-root.

We don't want to demand `sudo`. Fortunately, modern macOS and Linux offer a
middle ground: an **unprivileged ICMP datagram socket**.

```go
// "udp4" here does NOT mean UDP — it asks for a DATAGRAM-style ICMP socket
// (SOCK_DGRAM + IPPROTO_ICMP) instead of a RAW one. The kernel handles the
// ICMP id/checksum bookkeeping for us, and crucially: NO ROOT NEEDED.
conn, _ := icmp.ListenPacket("udp4", "0.0.0.0")
```

| | Raw socket (`ip4:icmp`) | Datagram socket (`udp4`) |
|--|------------------------|--------------------------|
| Privileges | **root / CAP_NET_RAW** | **none** (macOS & Linux) |
| You control | full IP header | ICMP payload only; kernel fills the rest |
| Best for | crafting arbitrary packets | ping/traceroute clients |

We use the datagram approach via [`golang.org/x/net/icmp`](https://pkg.go.dev/golang.org/x/net/icmp).
To set the TTL **per packet**, we reach for the IPv4 control wrapper
[`golang.org/x/net/ipv4`](https://pkg.go.dev/golang.org/x/net/ipv4):

```go
pkt := conn.IPv4PacketConn()
pkt.SetTTL(ttl)        // ← this is what makes each probe expire at a chosen hop
conn.WriteTo(echoBytes, dst)
```

> 🍎 **macOS note:** unprivileged ICMP datagram sockets are exactly what Apple's
> own `ping` uses, so this works out of the box without `sudo`. On Linux it
> depends on the `net.ipv4.ping_group_range` sysctl (open by default on most
> distros). If your platform refuses the socket, the program reports a clear
> error rather than crashing.

### 4. RTT (Round-Trip Time)

For each probe we record a timestamp right before sending and subtract it from
the time the reply arrives. That delta is the **round-trip time** — how long the
packet took to reach that hop *and* for the reply to come back. We send several
probes per hop (default 3) because a single sample is noisy: routers prioritise
forwarding real traffic over answering ICMP, so RTTs jitter. Three samples give
you a feel for the typical latency and reveal packet loss (a `*` means that probe
got no reply within the timeout).

---

## 🏗️ Architecture & Design

The program is deliberately split so that **all the protocol logic is testable
without a network or root**, and only a thin layer actually touches a socket.

```
main.go ───────────► parse CLI flags, call trace(), set exit code
   │
   ▼
traceroute.go
   ├── trace()        glue: open socket, stream hops to stdout
   ├── runTrace()     ◄── PURE ALGORITHM: TTL loop + hop aggregation
   │                      depends only on the `prober` interface
   ├── prober (iface) ◄── the seam we inject a fake into for tests
   ├── icmpProber     ◄── the REAL prober: socket + SetTTL + send/recv
   └── formatHop()    render one hop as a traceroute line
   │
   ▼
icmp.go
   ├── buildEchoRequest()  marshal an ICMP echo request → bytes
   └── parseICMPReply()    raw bytes → replyKind (TimeExceeded/EchoReply/…)
```

### The three factored-out, independently testable pieces

The challenge with networking code is that you can't unit-test a live trace
deterministically. So we carved out the three pieces that *are* pure logic:

1. **Building the probe bytes** (`buildEchoRequest`) — given an id/seq/payload,
   produce valid ICMP echo-request bytes. Test: build, then parse back, assert
   round-trip. No socket.
2. **Parsing/classifying a reply** (`parseICMPReply`) — given raw bytes, decide
   *Time Exceeded* vs *Echo Reply* vs *Dest Unreachable* vs *ignore*. Test: feed
   crafted bytes for each type, assert the classification. No socket.
3. **The TTL-iteration / hop-aggregation loop** (`runTrace`) — depends only on a
   small `prober` interface. Test: inject a **fake prober** that returns a
   scripted path (two routers then a destination) and assert the loop stops at
   the destination, records timeouts as `*`, and respects `--max-hops`. No
   socket, no network, no root.

> 🐍➡️🐹 **The interface seam.** `prober` is a one-method Go interface. The real
> `icmpProber` satisfies it by *having* a `probe` method — Go interfaces are
> implemented **implicitly**, there's no `implements` keyword. In tests we pass a
> `fakeProber` with the same method. This is exactly dependency injection / mock
> objects from Python and Java, but enforced by the compiler and free of any
> mocking framework.

---

## 🔨 Step-by-Step Implementation

### Step 1 — Parse the command line (`main.go`)

A small hand-rolled flag parser produces an `options` struct. `main()` stays
trivial (`os.Exit(run(...))`) and `run()` takes its output streams as
parameters, so tests can call it with fakes and inspect the bytes written.

```
traceroute [--max-hops 30] [--probes 3] [--timeout 1s] [--resolve] <host>
```

Exit codes: `0` success, `1` runtime error (socket/DNS), `2` usage error.

### Step 2 — Build an ICMP echo request (`icmp.go`)

```go
msg := icmp.Message{
    Type: ipv4.ICMPTypeEcho,           // type 8 = echo request (a ping)
    Body: &icmp.Echo{ID: id, Seq: seq, Data: payload},
}
b, _ := msg.Marshal(nil)               // x/net computes the checksum & layout
```

> 🐍 In Python you'd build these bytes by hand with `struct.pack` and compute the
> checksum yourself. `x/net/icmp` does that grunt work; `Marshal` returns the
> exact bytes that go on the wire.

### Step 3 — Open an unprivileged socket and set the TTL (`traceroute.go`)

```go
conn, _ := icmp.ListenPacket("udp4", "0.0.0.0")  // no root needed
pkt := conn.IPv4PacketConn()                      // lets us set TTL per packet
```

### Step 4 — Probe one hop

For each probe: `pkt.SetTTL(ttl)`, record `start := time.Now()`, `conn.WriteTo`,
then wait for a reply. The timeout is enforced with a **read deadline**:

```go
conn.SetReadDeadline(time.Now().Add(timeout))   // 🐍 like sock.settimeout(t)
n, peer, err := conn.ReadFrom(buf)
```

A deadline-expired read surfaces as a `net.Error` whose `Timeout()` is true — we
treat that as a `*` (no answer). Otherwise we classify the bytes with
`parseICMPReply` and record the RTT (`time.Since(start)`) and the responder
(`peer`).

> 🐍➡️🐹 **Deadlines, not timeouts-per-call.** Go sockets don't take a timeout
> argument on each read. Instead you set an absolute *deadline*; the blocked read
> returns an error once that instant passes. It's the idiomatic Go way to bound
> any blocking I/O.

### Step 5 — The TTL loop (`runTrace`)

```go
for ttl := 1; ttl <= maxHops; ttl++ {
    h := hop{ttl: ttl}
    for i := 0; i < probes; i++ {
        h.results = append(h.results, p.probe(ttl, seq))   // p is the interface
    }
    report(h)                       // stream the finished hop to the printer
    if h.reachedDest() { break }    // Echo Reply / Dest Unreachable → done
}
```

Because `runTrace` talks only to the `prober` interface, the real socket and the
test fake are interchangeable.

### Step 6 — Render each hop (`formatHop`)

Repeated probes from the *same* router collapse to one address label (matching
system `traceroute`); timeouts print as `*`; the `udp4` peer's meaningless `:0`
port is stripped. `--resolve` adds an optional, best-effort reverse-DNS lookup
(errors swallowed — a missing PTR record is normal, not a failure).

---

## 🧪 Testing Strategy

ICMP/raw-network behaviour can't be unit-tested deterministically without root
or a live, stable network. So we test the **logic**, and *guard* the one live
test so the suite always passes offline.

Run them:

```bash
go vet ./...
CGO_ENABLED=0 go test ./...     # see the toolchain note below
```

**What the offline tests cover (no root, no network):**

- **`icmp_test.go`** — `buildEchoRequest` round-trips through a parser; crafted
  bytes for *Time Exceeded*, *Echo Reply*, *Dest Unreachable*, and a stray *Echo
  Request* are each classified correctly; garbage bytes return an error; the
  `terminal()` rule is verified.
- **`trace_test.go`** — a **`fakeProber`** drives `runTrace` through scripted
  paths: stops exactly at the destination, records all-timeout hops as `*` and
  keeps going, and respects `--max-hops` instead of looping forever.
  `formatHop` is checked for hop number, single-address collapsing, `*`, and RTT
  rendering.
- **`main_test.go`** — flag parsing defaults, valid flags, and every error path
  (missing host, bad numbers, bad duration, unknown flag, extra positional).
- **`integration_test.go`** — a **guarded live trace** to `8.8.8.8`. It *skips*
  (never fails) when offline, in `-short` mode, or if the platform denies the
  socket — so it can never break CI. Run it on purpose with:
  `go test -run TestTraceIntegration -v`.

### ⚠️ Environment / toolchain note (read this if `go test` aborts)

On this macOS dev box, a plain `go test ./...` aborts before any test runs with:

```
dyld: missing LC_UUID load command
signal: abort trap
```

This is **not a bug in our code** — it's a known mismatch between the Go
toolchain's external linker and the installed Xcode Command-Line Tools `ld`,
triggered because the package transitively imports `net` (which pulls in **cgo**
for the system resolver). The fix is to build the tests in pure-Go mode, which
uses Go's internal linker:

```bash
CGO_ENABLED=0 go test ./...     # ✅ all tests pass
```

`go vet ./...` passes either way. The binary also builds cleanly with
`CGO_ENABLED=0 go build`. (This mirrors the `curl` challenge in this repo, which
hit and documented the same linker bug.)

### Running it for real

Actually *running* a trace needs outbound network access, but **the tests do
not** — they're designed to pass fully offline. With network available, a live
run was confirmed reaching the destination:

```
$ go build -o traceroute . && ./traceroute --max-hops 8 --probes 3 8.8.8.8
traceroute to 8.8.8.8, 8 hops max, 3 probes per hop
 1  10.2.0.1  1.2ms  1.1ms  1.3ms
 2  * 79.127.146.125  9.8ms  10.1ms
 ...
 8  8.8.8.8  11.4ms  11.2ms  11.3ms
```

---

## 💡 Key Takeaways

- **TTL expiry is the whole trick.** A field built to *prevent loops* is
  repurposed to *map the path*: by sweeping TTL 1→N you force each router to
  unmask itself via an ICMP "Time Exceeded" whose source address *is* that hop.
- **ICMP is IP's control plane.** "Time Exceeded" (type 11), "Echo Reply" (type
  0), and "Destination Unreachable" (type 3) are the three messages that drive
  intermediate-hop discovery and arrival detection.
- **You rarely need root for ICMP anymore.** Unprivileged **datagram** ICMP
  sockets (`icmp.ListenPacket("udp4", …)`) let `ping`/`traceroute`-style tools
  run without `sudo` on macOS and Linux — a far better default than setuid raw
  sockets.
- **Isolate protocol logic from I/O to make it testable.** Byte-building,
  byte-parsing, and the control loop are pure functions/an interface; only a thin
  `icmpProber` touches the network. A one-method Go interface (`prober`) is all
  it takes to inject a fake and test the algorithm offline.
- **Go idioms learned:** `x/net/icmp` + `x/net/ipv4` for portable ICMP and
  per-packet TTL; **read deadlines** (`SetReadDeadline`) to bound blocking reads;
  `iota` enums for classifying replies; **implicit interface satisfaction** for
  dependency injection without a mocking framework.

---

## 📖 Further Reading

- 🐍➡️🐹 [Go Quickstart for a Python Developer](../../docs/go-quickstart.md) — the project Go primer
- [RFC 792 — Internet Control Message Protocol (ICMP)](https://www.rfc-editor.org/rfc/rfc792) — defines Time Exceeded, Echo/Echo Reply, Destination Unreachable
- [RFC 791 — Internet Protocol](https://www.rfc-editor.org/rfc/rfc791) — the IP header and the TTL field
- [RFC 1393 — Traceroute Using an IP Option](https://www.rfc-editor.org/rfc/rfc1393) — background on path discovery
- [Van Jacobson's original `traceroute`](https://en.wikipedia.org/wiki/Traceroute) — the tool's history and design
- [`golang.org/x/net/icmp`](https://pkg.go.dev/golang.org/x/net/icmp) and [`golang.org/x/net/ipv4`](https://pkg.go.dev/golang.org/x/net/ipv4) — the Go packages we build on
- [Coding Challenges — Traceroute](https://codingchallenges.fyi/challenges/intro) — the challenge series this repo follows
