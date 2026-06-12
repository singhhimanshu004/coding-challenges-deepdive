# NTP Client

> **Phase:** 4 — Networking & Protocols
> **Difficulty:** 🔵
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** S

**Status:** ✅ Completed

> 🐍➡️🐹 **New to Go?** Read the project's
> [**Go Quickstart for a Python Developer**](../../docs/go-quickstart.md) first.
> It maps every Go idiom used here (`struct` layout, `encoding/binary`,
> `time.Time`/`time.Duration`, `net.Dial`, error returns) back to the Python you
> already know. This README assumes you've skimmed it, and adds 🐍 callouts where
> Go does something surprising.

---

## 🎯 What We're Building

A small command-line **NTP client** — the thing that asks "what time is it,
really?" The program sends one tiny UDP packet to a public time server (e.g.
`pool.ntp.org`), reads the reply, and prints:

```
Server:      pool.ntp.org:123
Stratum:     2
Server time: 2026-06-13T01:30:53.482113Z
Local time:  2026-06-13T01:30:53.501920Z
Offset:      -19.807ms
Delay:       24.113ms
```

This is the protocol your laptop runs quietly in the background to keep its
clock honest. NTP (the **Network Time Protocol**, RFC 5905) is how virtually
every networked device on Earth agrees on the time — and the wire format is so
compact it's a perfect first binary-over-UDP challenge.

Two numbers are the whole point of the exercise:

- **Offset** — how far your local clock is from the server's clock (you'd *add*
  the offset to your clock to correct it).
- **Delay** — the round-trip time the request/reply spent travelling the network.

What our client supports:

```
ntp-client [flags]

-server HOST   NTP server hostname   (default pool.ntp.org)
-port   N      server UDP port       (default 123)
```

```bash
ntp-client                          # query pool.ntp.org:123
ntp-client -server time.apple.com   # a different server
ntp-client -server 127.0.0.1 -port 1230   # a local test server
```

We use the **client/server (SNTP) subset** of NTP: one request, one reply, four
timestamps, two formulas. No daemon, no clock discipline loop — just the
measurement.

---

## 📚 Core Concepts

### 1. UDP — fire a datagram, hope for a reply

NTP rides on **UDP**, not TCP. There's no handshake and no connection: you throw
a single self-contained packet at the server and (usually) one comes back.

```
 ┌──────────┐   one 48-byte datagram   ┌──────────┐
 │  client  │ ───────────────────────► │  server  │
 │          │ ◄─────────────────────── │ (port 123)│
 └──────────┘   one 48-byte datagram   └──────────┘
```

UDP is the right tool here: a time query is tiny, stateless, and if a packet is
lost you'd rather just ask again than pay for TCP's connection setup. The
trade-off is that **UDP gives you no delivery guarantee** — so the client *must*
set a timeout, or a single dropped packet would hang it forever.

> 🐍 In Python this is `socket.socket(AF_INET, SOCK_DGRAM)` + `sendto`/`recvfrom`.
> Go's `net.Dial("udp", addr)` gives you a *connected* UDP socket: there's still
> no handshake, but the OS remembers the peer so you can use plain `Write`/`Read`
> instead of `WriteTo`/`ReadFrom`.

### 2. The 48-byte NTP packet

Every NTP message — request and reply — is **exactly 48 bytes**. (There are
optional authentication fields after byte 48; we never send or expect them.)
Here's the layout, by byte offset:

```
 byte 0    Settings  →  LI(2 bits) | VN(3 bits) | Mode(3 bits)
 byte 1    Stratum      (distance from a reference clock; 1 = atomic/GPS)
 byte 2    Poll
 byte 3    Precision
 bytes 4–7    Root Delay
 bytes 8–11   Root Dispersion
 bytes 12–15  Reference ID
 bytes 16–23  Reference Timestamp
 bytes 24–31  Originate Timestamp   ← T1 (echoed back from our request)
 bytes 32–39  Receive Timestamp     ← T2 (server got our request)
 bytes 40–47  Transmit Timestamp    ← T3 (server sent its reply)
```

A **client request is almost entirely zeros.** The only byte we must set is byte
0, and we don't even need to fill in our timestamps for a basic query.

### 3. The first byte — packing LI / VN / Mode

Byte 0 squeezes three fields into eight bits, most-significant bits first:

```
   bit:  7 6   5 4 3   2 1 0
        ┌─────┬───────┬───────┐
        │ LI  │  VN   │ Mode  │
        └─────┴───────┴───────┘
          │       │       └─ Mode  : 3 = client (we always send this)
          │       └───────── VN    : NTP version (3 or 4)
          └───────────────── LI    : Leap Indicator (0 = no warning)
```

To build it we **bit-pack**: slide the version into bits 5–3 and OR in the mode:

```go
b[0] = (0 << 6) | (version << 3) | modeClient   // LI=0, VN=version, Mode=3
```

For version 4 that's `0b00_100_011` = `0x23`; for version 3, `0b00_011_011` =
`0x1B`. The server replies with **Mode 4** ("server").

> 🐍 Identical to Python's `(version << 3) | mode`. Decoding pulls the fields
> back out with shifts and masks: `li = b0 >> 6`, `vn = (b0 >> 3) & 0b111`,
> `mode = b0 & 0b111`.

### 4. NTP timestamp format — 64-bit fixed point

This is the concept that trips everyone up. An NTP timestamp is **not** a Unix
time. It's a **64-bit fixed-point number**:

```
        ┌───────────── 64 bits ─────────────┐
        │  32 bits seconds │ 32 bits fraction │
        └──────────────────┴──────────────────┘
            since 1900           fraction of a
            (the NTP epoch)      second × 2³²
```

- **High 32 bits** = whole seconds since the **NTP epoch, 1900-01-01 00:00:00
  UTC**.
- **Low 32 bits** = the *fraction* of a second, scaled by 2³². So a fraction of
  `0x80000000` (half of 2³²) means exactly **0.5 s**; `0x40000000` means 0.25 s.

In Go we model it as two `uint32`s, which is exactly the 8 wire bytes:

```go
type ntpTime struct {
    Seconds  uint32
    Fraction uint32
}
```

### 5. The 1900 → 1970 epoch offset (2208988800)

Go's `time.Time` and Unix time count from **1970**; NTP counts from **1900**.
The gap is **2,208,988,800 seconds** — 70 years, which is 25,567 days (70 × 365
+ 17 leap days) × 86,400 s/day. To turn an NTP timestamp into a Go time we
**subtract** the offset; to go the other way we **add** it:

```go
const ntpEpochOffset = 2208988800
unixSeconds := int64(t.Seconds) - ntpEpochOffset
```

The fractional part becomes nanoseconds with integer math (`>> 32` divides by
2³², keeping us out of floating point):

```go
nanos := (int64(t.Fraction) * 1_000_000_000) >> 32
```

### 6. Network byte order (big-endian)

Every multi-byte integer on the wire is **big-endian** ("network byte order") —
most-significant byte first. Your CPU is probably little-endian, so reading the
bytes raw would scramble them. We tell Go to interpret the packet as big-endian
and it does the swapping for us (`binary.BigEndian`).

### 7. The four timestamps and the math

A complete transaction collects four moments:

```
   client                                   server
     │                                        │
  T1 ●  send request ───────────────────────► │
     │                                  T2 ●  receive
     │                                  T3 ●  transmit
  T4 ●  ◄─────────────────────────── reply    │
     │                                        │
```

- **T1** = originate — local clock, just before we send.
- **T2** = receive — server clock, when it got our request.
- **T3** = transmit — server clock, when it sent the reply.
- **T4** = destination — local clock, just after the reply arrives.

T1 and T4 we measure locally; T2 and T3 come out of the server's reply packet.
From these four:

```
offset = ((T2 − T1) + (T3 − T4)) / 2
delay  =  (T4 − T1) − (T3 − T2)
```

- **offset** averages the apparent error on the outbound leg (`T2 − T1`) and the
  inbound leg (`T3 − T4`). Averaging *cancels the network travel time* — **as
  long as the path is roughly symmetric** (same latency each way). That symmetry
  assumption is the core idea of NTP, and also its main source of error.
- **delay** is the round trip: total observed time (`T4 − T1`) minus the time the
  server spent thinking (`T3 − T2`), leaving only time spent on the wire.

> 🐍 In Go, `time.Duration` is just an `int64` count of nanoseconds with pretty
> printing, so `+`, `−`, and `/` work exactly as you'd expect — no special
> "timedelta" type juggling.

---

## 🏗️ Architecture & Design

Two source files, split by concern:

```
ntp.go    the PROTOCOL — packet layout, build/parse, timestamp conversion,
          and the offset/delay math. Zero networking — pure bytes & time.
main.go   the TRANSPORT + CLI — open the UDP socket, do the four-timestamp
          round trip, parse flags, print the result.
```

The clean seam matters: **everything in `ntp.go` is pure and deterministic**, so
it's unit-testable against crafted byte slices with no network at all. `main.go`
is the only place that touches a socket, and the live round trip is exercised by
one guarded integration test.

The wire format maps **1:1 onto Go structs**. Because every field is a
fixed-size integer (or an `ntpTime`, which is two `uint32`s), `encoding/binary`
can read or write the *entire* 48-byte packet in a single call — the Go struct
layout **is** the wire layout. There's no hand-rolled byte fiddling.

> 🐍 This is the payoff over Python's `struct.unpack(">II...")`: instead of a
> format string you'd have to keep in sync with the layout by counting bytes, you
> declare a struct once and `binary.Read`/`binary.Write` walks it field by field.

---

## 🔨 Step-by-Step Implementation

Walking through the actual functions:

### `buildRequest(version)` — *ntp.go*

Allocates a 48-byte slice (all zeros) and sets only byte 0 via bit-packing
(`(0 << 6) | (version << 3) | modeClient`). Returns the slice. That's the whole
request — a client query carries no meaningful payload beyond "I'm a client,
here's my version."

### `parseResponse(b)` — *ntp.go*

Guards against a short read (`len(b) < 48` → error, so we never read past the
buffer), then `binary.Read(bytes.NewReader(b), binary.BigEndian, &p)` fills the
`packet` struct in one shot. Big-endian decoding and field offsets are handled
by the struct definition.

### `ntpTime.toTime()` — *ntp.go*

Converts the fixed-point timestamp to a Go `time.Time` in UTC:
- A zero timestamp (both halves 0) is NTP's "no value," so it returns the zero
  `time.Time{}`.
- Otherwise: `seconds − ntpEpochOffset` for the 1900→1970 rebase, and
  `(fraction × 1e9) >> 32` for the nanosecond part. Returns
  `time.Unix(secs, nanos).UTC()`.

### `timeToNTP(t)` — *ntp.go*

The inverse: `t.Unix() + ntpEpochOffset` for the seconds, and
`(nanos << 32) / 1e9` for the fraction. Used to stamp the transmit time; it
exists mainly so we can prove the conversion round-trips in tests.

### `clockMetrics(t1, t2, t3, t4)` — *ntp.go*

The two formulas, verbatim:
```go
offset = ((t2.Sub(t1)) + (t3.Sub(t4))) / 2
delay  =  (t4.Sub(t1)) - (t3.Sub(t2))
```
Returns two `time.Duration`s.

### `query(server, port, version, timeout)` — *main.go*

The orchestration — the four-timestamp dance over UDP:
1. `net.JoinHostPort(server, port)` builds the address (handles IPv6 bracketing
   so we never concatenate `host:port` by hand).
2. `net.Dial("udp", addr)` opens the connected UDP socket; `defer conn.Close()`.
3. `conn.SetDeadline(now + timeout)` — **one deadline covers both the write and
   the read.** This is the safety net that turns a lost UDP packet into a clean
   error instead of a hang.
4. Record **T1**, `conn.Write(req)`.
5. `conn.Read(resp)` into a 48-byte buffer, then record **T4** immediately.
6. `parseResponse` the reply; pull **T2** and **T3** out of it
   (`RecvTimestamp.toTime()`, `TxTimestamp.toTime()`).
7. `clockMetrics(t1, t2, t3, t4)` → offset and delay. Bundle everything into a
   `Result` (server time, offset, delay, stratum).

### `main()` — *main.go*

`flag` parsing for `-server`/`-port`, calls `query`, prints the report (or writes
the error to stderr and exits non-zero). Times are formatted with
`time.RFC3339Nano`.

---

## 🧪 Testing Strategy

The tests split into **fast, hermetic unit tests** (no network) and **one
guarded live integration test**.

**Unit tests (`ntp_test.go`)** — all against crafted bytes/values:

- **`TestBuildRequest`** — table-driven over versions 3 and 4: the request is
  exactly 48 bytes, byte 0 matches the expected `0x1B`/`0x23`, the LI/VN/Mode
  subfields decode correctly, and **every other byte is zero**.
- **`TestNTPTimeToTime`** — the decode side: the epoch offset alone maps to Unix
  time 0; `+1` second → Unix 1; fraction `0x80000000` → 0.5 s; `0x40000000` →
  0.25 s; and a zero timestamp → the zero `time.Time`. These pin down both the
  1900→1970 shift and the fractional-seconds math.
- **`TestTimeToNTPRoundTrip`** — converts a real date to NTP and back, asserting
  the drift is under a microsecond (sub-nanosecond rounding in fixed point is
  unavoidable, so a tiny tolerance is allowed).
- **`TestParseResponse`** — crafts a 48-byte packet with `binary.Write`, parses
  it back, and confirms the fields land at the right offsets in big-endian.
- **`TestParseResponseShort`** — a 10-byte buffer must error, not panic.
- **`TestClockMetrics`** — feeds hand-picked timestamps (server 10 s ahead,
  symmetric 1 s legs) and checks `offset == 10s`, `delay == 2s` exactly.

**Integration test (`integration_test.go`)** — `TestQueryIntegration` does a
**real** query against `pool.ntp.org`. It's deliberately **non-flaky**: it skips
(rather than fails) in `-short` mode and skips on any network error, so an
offline run never breaks the suite. When the network *is* available it asserts
the server time is within a year of "now" and logs the live stratum/offset/delay.

Run them:

```bash
go vet ./...
CGO_ENABLED=0 go test ./...     # see the toolchain note below
```

### ⚠️ Environment / toolchain note (`CGO_ENABLED=0`)

On this macOS dev box (Go 1.22), a plain `go test ./...` can abort *before any
test runs* with:

```
dyld: missing LC_UUID load command
signal: abort trap
```

This is **not a bug in our code.** It's a known mismatch between the Go
toolchain's external linker and the installed Xcode Command-Line Tools `ld`,
triggered because this package imports `net` (which pulls in **cgo** for the
system DNS resolver). The fix is to build in pure-Go mode, which uses Go's
internal linker and native resolver:

```bash
CGO_ENABLED=0 go test ./...     # ✅ all tests pass
```

`go vet ./...` passes either way, and the binary builds cleanly with
`CGO_ENABLED=0 go build`. (This is the same workaround documented for `curl` in
Phase 3 — expect it across the networking phase.)

**Verified:** `go vet` clean; `CGO_ENABLED=0 go test ./...` — all unit tests
pass, and the live integration test succeeded against `pool.ntp.org`.

---

## 💡 Key Takeaways

- **UDP is fire-and-forget, so own the timeout.** No connection, no
  retransmission — a deadline is the only thing standing between you and an
  infinite hang on a dropped packet.
- **The struct *is* the wire format.** Declare fixed-size fields in the right
  order and `encoding/binary` reads/writes the whole 48-byte packet in one call.
  No format strings, no manual byte counting.
- **NTP time ≠ Unix time.** It's 64-bit fixed point counting from **1900**, not
  1970. The `2208988800`-second offset and the `fraction / 2³²` math are the two
  conversions you must get right.
- **Always big-endian on the wire.** Network byte order is non-negotiable for
  multi-byte integers; let the library do the swapping.
- **Four timestamps, two formulas.** `offset = ((T2−T1)+(T3−T4))/2` and
  `delay = (T4−T1)−(T3−T2)` — and the offset is only as good as the *symmetric
  path* assumption it rests on.
- **Keep protocol logic pure.** With all the byte/time math in `ntp.go` and only
  the socket in `main.go`, the tricky parts are testable with zero network.

---

## 📖 Further Reading

- 🐍➡️🐹 [Go Quickstart for a Python Developer](../../docs/go-quickstart.md) — the project Go primer
- [Coding Challenges — Build Your Own NTP Client](https://codingchallenges.fyi/challenges/challenge-ntp)
- [RFC 5905 — Network Time Protocol Version 4](https://datatracker.ietf.org/doc/html/rfc5905) (packet format §7.3, timestamp format §6, the clock-offset/delay equations §8)
- [RFC 4330 — Simple NTP (SNTP) v4](https://datatracker.ietf.org/doc/html/rfc4330) — the one-shot client/server subset we actually implement
- [NTP timestamp format & the 1900 epoch](https://docs.ntpsec.org/latest/ntp.html) — background on the fixed-point representation
- Go stdlib: [`encoding/binary`](https://pkg.go.dev/encoding/binary), [`net`](https://pkg.go.dev/net), [`time`](https://pkg.go.dev/time)
