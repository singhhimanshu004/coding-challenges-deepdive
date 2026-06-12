# DNS Resolver

> **Phase:** 4 — Networking Fundamentals
> **Difficulty:** 🔵
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** M

**Status:** ✅ Done

> 🐍➡️🐹 **New to Go?** Read the project's
> [**Go Quickstart for a Python Developer**](../../docs/go-quickstart.md) first.
> It maps every Go idiom used here (`defer`, slices, structs + methods, error
> returns, `encoding/binary`) back to the Python you already know. This README
> assumes you've skimmed it and adds 🐍 callouts where Go does something a
> Python dev would find surprising.

---

## 🎯 What We're Building

A working **DNS resolver** — the thing that turns `example.com` into
`93.184.216.34` — but with a strict rule that makes it a *learning* project
instead of a one-line library call:

> **We build it on a RAW UDP socket and hand-encode / hand-decode every byte of
> the DNS message ourselves.** No `net.Resolver`, no `LookupHost`. We pack the
> 12-byte header, the question, and parse the resource records by hand —
> including the infamous **name-compression pointer**.

This is the natural next step after the `curl` challenge. There we learned that
"HTTP" is just agreed-upon *text* on a TCP byte-pipe. DNS is the same idea one
layer more raw: a tight **binary** protocol on a UDP byte-pipe. Once you've
written the header bits and walked a delegation chain from a root server, the
"magic" of name resolution disappears.

What our resolver supports:

```
dns-resolver <domain> [--type A] [--server 8.8.8.8] [--trace]

--type T      record type: A (default), AAAA, NS, CNAME, MX
--server IP   recursive resolver to query (default 8.8.8.8; :port optional, 53 assumed)
--trace       resolve ITERATIVELY from a root server, printing each referral hop
```

Examples:

```bash
dns-resolver example.com                 # A record via 8.8.8.8
dns-resolver example.com --type AAAA     # IPv6
dns-resolver gmail.com --type MX         # mail servers
dns-resolver example.com --server 1.1.1.1
dns-resolver example.com --trace         # walk root -> .com -> example.com yourself
```

---

## 📚 Core Concepts

### 1. The DNS namespace is a tree; resolution walks down it

Domain names are read right-to-left as a path down a tree:

```
                         . (the root)
                        /      |      \
                     com      org      net        ← Top-Level Domains (TLDs)
                    /   \
            example     google                    ← second-level domains
              |
             www                                   ← a hostname (a leaf)
```

No single server knows everything. Instead each zone **delegates** the level
below it to other name servers. Resolving `www.example.com` means asking:

1. a **root** server → "who runs `.com`?" → *referral* to the `.com` servers
2. a **`.com`** server → "who runs `example.com`?" → *referral* to example's NS
3. **example.com**'s server → "what's the A record for `www.example.com`?" →
   the **answer**

That top-down walk is **iterative resolution** (the resolver does the asking).
Alternatively you can hand the whole job to a **recursive resolver** (like
`8.8.8.8`) that walks the tree for you and returns just the final answer. We
implement **both** — see Architecture.

### 2. DNS rides on UDP (mostly)

A query is small and a single round-trip is cheap, so DNS uses **UDP** port 53
by default. UDP is *connectionless* and *unreliable*: you fire a datagram and
hope a reply comes back. That has consequences we must handle:

- **No "connection" to drop**, so a dead server just means *silence* → we set a
  **read deadline** and treat a timeout as failure.
- **A reply is one datagram.** One `Read` returns one whole message — we don't
  stitch a byte stream together like we did for TCP/HTTP in `curl`.
- **Size limits.** Classic DNS caps a UDP message at 512 bytes; if a response is
  bigger, the server sets the **TC (truncated)** bit and you're supposed to
  retry over TCP. We read into a 1232-byte buffer (a safe modern size) and note
  the TC bit, but don't implement TCP fallback — it's rarely needed for the
  record types here.

> 🐍 `net.DialUDP("udp", nil, raddr)` is
> `s = socket.socket(AF_INET, SOCK_DGRAM); s.connect((ip, 53))`. `nil` for the
> local address = "OS, pick me an ephemeral source port." The returned
> `*net.UDPConn` is just a thing with `Read([]byte)` and `Write([]byte)`.

### 3. The DNS wire format — byte by byte

Every DNS message has the same five-part shape. Everything is **big-endian**
("network byte order").

```
+---------------------+
|       Header        |   12 bytes, fixed
+---------------------+
|      Question       |   what we're asking (usually 1)
+---------------------+
|       Answer        |   resource records that answer it
+---------------------+
|      Authority      |   NS records (used in referrals)
+---------------------+
|     Additional      |   extra records, e.g. "glue" IPs for the NS hosts
+---------------------+
```

**The 12-byte header** packs a lot into two flag bytes:

```
 0  1  2  3  4  5  6  7  8  9 10 11 12 13 14 15
+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+
|QR|   Opcode  |AA|TC|RD|RA|   Z    |   RCODE   |
+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+--+

ID      (16 bits) — random per query; the reply echoes it so we can match them
QR      query (0) / response (1)
RD      Recursion Desired  — set by the CLIENT  ("please do the walk for me")
RA      Recursion Available — set by the SERVER ("I can / can't do recursion")
RCODE   result: 0 NOERROR, 3 NXDOMAIN (no such name), 2 SERVFAIL, ...
QDCOUNT/ANCOUNT/NSCOUNT/ARCOUNT — how many entries are in each section below
```

**The question** is a name plus the type/class we want:

```
QNAME (label-encoded)   QTYPE(2)   QCLASS(2)
```

**A resource record (RR)** adds a TTL and a length-prefixed payload:

```
NAME   TYPE(2)   CLASS(2)   TTL(4)   RDLENGTH(2)   RDATA(RDLENGTH bytes)
```

Record types we decode: **A** (4-byte IPv4), **AAAA** (16-byte IPv6),
**CNAME**/**NS** (a name), **MX** (2-byte preference + a name).

### 4. QNAME encoding — labels, not a string

A name is **not** stored as `"www.example.com\0"`. Each dot-separated label is
length-prefixed, and a single `0x00` byte terminates the name:

```
www.example.com  ->  3 w w w  7 e x a m p l e  3 c o m  0
                     └label┘  └───label─────┘  └label┘  └ root (end)
```

The root domain (`.`) is just a single `0x00`. A label is at most 63 bytes —
because the top two bits of the length byte are reserved for the next concept.

### 5. ⚠️ The classic gotcha: NAME COMPRESSION

DNS responses repeat the same domain names over and over (the question name,
every answer's owner name, the NS host names…). To save space, RFC 1035 lets a
name **end with a pointer to a name that appeared earlier in the same message.**

A length byte whose **top two bits are set** (`byte & 0xC0 == 0xC0`) is *not* a
length — that byte plus the next form a **14-bit offset from the start of the
message**. You jump there and keep reading labels:

```
message offset 12:  7 e x a m p l e  3 c o m  0          ← "example.com"
...
later:              3 w w w  C0 0C                        ← "www" + pointer→12
                             └────┘
                    0xC0 0x0C  =  1100_0000 0000_1100
                                  ^^                       top 2 bits = pointer flag
                                    \____ 14-bit offset = 12
                    decodes to:  www.example.com
```

This trips up **everyone** writing a resolver. Two subtleties our decoder gets
right (and the tests pin down):

1. **The offset you RETURN to the caller is the position right after the 2-byte
   pointer in the original stream — not wherever the jump landed.** The pointer
   "borrows" earlier bytes but the cursor in the stream only advanced by two.
2. **Pointers can chain, and a malicious packet can loop.** We cap the number of
   jumps so a self-referential pointer errors out instead of hanging forever.

A second trap: names inside **RDATA** (NS, CNAME, MX hosts) can *also* be
compressed back into the message — so RDATA decoding must run against the
**full** message buffer, never just the isolated RDATA slice.

---

## 🏗️ Architecture & Design

One concern per file, so each idea has an obvious home:

```
message.go    the WIRE FORMAT: pack/unpack the header, question, and RRs;
              encode/decode QNAME labels; and the name-compression decoder.
              Pure byte work — no network, fully unit-testable in isolation.
resolver.go   the NETWORK: open the UDP socket (net.DialUDP), send one query,
              read one datagram. Plus the two resolution strategies:
                - resolveRecursive: ask 8.8.8.8 with RD=1, trust its answer
                - resolveIterative: walk root -> TLD -> zone, following NS
                  referrals and using "glue" A records, RD=0 (this is --trace)
main.go       the CLI: hand-rolled flag parsing, mode selection, dig-like output
*_test.go     table-driven tests on crafted byte slices (no network)
```

**Which resolution did we implement?** *Both.*

- **Default** = recursive-resolver mode: we set the **RD** bit and let a
  configured resolver (`8.8.8.8:53`, override with `--server`) do the tree walk.
  One question, one answer.
- **`--trace`** = iterative mode: we act like a resolver ourselves, starting at a
  **root server** with **RD=0** and following the **NS referrals** down the
  delegation chain, printing each hop. This is the "recursive resolution from
  the root" the challenge asks for, viewed from the client's seat.

> 🐍 Python-dev note on testability: `run()` takes its output streams as
> parameters instead of hard-coding `os.Stdout`, and the wire-format functions
> operate on plain `[]byte` you can craft inline. That's dependency injection —
> the same reason you'd pass a file object into a function rather than `open()`
> inside it. The unit tests build byte slices by hand and assert on the decoded
> structs; no internet required.

---

## 🔨 Step-by-Step Implementation

1. **`message.go` — the header.** Define a `Header` struct of six `uint16`s.
   `pack` writes them with `binary.BigEndian.PutUint16` (🐍 `struct.pack(">H")`);
   `unpackHeader` reads them back. Helpers pull the **RCODE** (low 4 bits) and
   set the **RD** flag bit.
2. **QNAME encode/decode.** `encodeName` splits on `.` and length-prefixes each
   label, ending with `0x00`. `decodeName` is the star: it reads labels, and on
   hitting a `0xC0` byte follows the **compression pointer**, remembering the
   real continuation offset and capping jumps to avoid loops.
3. **Question + resource records.** `Question.pack` appends the encoded name plus
   QTYPE/QCLASS. `unpackRR` reads NAME→TYPE→CLASS→TTL→RDLENGTH→RDATA, then
   `decodeRData` renders the payload per type (A→dotted IPv4, AAAA→IPv6,
   NS/CNAME→a name, MX→`pref host`) — always decoding inner names against the
   full message so compression works.
4. **Build & parse whole messages.** `buildQuery` assembles a query (header +
   one question, RD optional). `parseMessage` decodes a full reply, advancing a
   single running offset through questions → answers → authority → additional.
5. **`resolver.go` — the socket.** `exchange` does `net.DialUDP`, sets a
   deadline, `Write`s the query bytes, `Read`s one datagram, and parses it.
6. **Two strategies.** `resolveRecursive` sends RD=1 to the configured server.
   `resolveIterative` loops from the root: if the response has the answer →
   done; a CNAME → restart for the alias; otherwise gather the **referral** NS
   servers (preferring **glue** A records, else resolving an NS name) and ask
   one level deeper.
7. **`main.go` — the CLI.** A small hand-rolled flag parser (consistent with the
   other tools in this repo), mode selection (`--trace` vs default), and
   `dig`-style output to stdout with trace lines to stderr.

---

## 🧪 Testing Strategy

**All unit tests run on crafted byte slices — zero network**, so they're fast
and never flake. They are **table-driven** where it helps (encode cases, type
round-trips):

- **Header** (`TestHeaderRoundTrip`): exact 12-byte big-endian layout, plus a
  pack→unpack round-trip, and a too-short error case.
- **QNAME encode** (`TestEncodeName`): `example.com`, multi-label, the **root**
  (`""` → single `0x00`), and a trailing-dot name.
- **QNAME decode + the gotcha** (`TestDecodeNameSimple`, `…Compression`,
  `…PointerLoop`): a plain name with continuation-offset check; a real
  **0xC0 compression pointer** that must decode to `www.example.com` while
  returning the offset *past the pointer*; and a self-referential pointer that
  must **error** instead of looping.
- **Resource records** (`TestUnpackRR_A`, `TestUnpackRR_MX`): full RR decode with
  a **compressed owner name**, checking every field; MX RDATA = preference +
  a compressed mail host.
- **End-to-end decode** (`TestParseFullResponse`): a realistic response with a
  question and two answers that *both* use compression.
- **Round-trips** (`TestBuildQueryAndParse`, `TestParseTypeRoundTrip`): a built
  query parses back identically; CLI type mnemonics convert both ways.

A **network integration test** (`integration_test.go`) exercises the real UDP
round-trip against `8.8.8.8` and the iterative root walk, but it is **guarded**
so it never fails offline or in CI — it only runs when you opt in:

```bash
DNS_NETWORK_TEST=1 CGO_ENABLED=0 go test -run Network -v ./...
```

Run the hermetic suite:

```bash
go vet ./...
CGO_ENABLED=0 go test ./...     # see the toolchain note below
```

### ⚠️ Environment / toolchain note (read this if `go test` aborts)

On this macOS dev box (go1.22.2, darwin/arm64), a plain `go test ./...` aborts
**before any test runs** with:

```
dyld: missing LC_UUID load command
signal: abort trap
```

This is **not a bug in our code.** It's a known mismatch between the Go
toolchain's external linker and the installed Xcode Command-Line Tools `ld`,
triggered only because this package imports `net` (which pulls in **cgo** for
the system resolver). The fix — identical to the `curl` challenge — is to build
in pure-Go mode, which uses Go's internal linker:

```bash
CGO_ENABLED=0 go test ./...     # ✅ all tests pass
CGO_ENABLED=0 go build          # ✅ binary builds cleanly
```

`go vet ./...` passes either way. **Network verification:** with outbound access
available, real lookups were confirmed working — `A`, `AAAA`, and `MX` queries
returned correct records via `8.8.8.8`, and `--trace` walked
`root → .com → example.com` and returned the A records.

---

## 💡 Key Takeaways

- **DNS is a tight binary protocol on a UDP datagram.** After `curl` showed HTTP
  is text on a TCP stream, this shows the other half: fixed-width big-endian
  fields you pack and unpack by hand with `encoding/binary`.
- **Names are label-encoded, not strings.** Length-prefixed labels terminated by
  a zero byte — and the root is just `0x00`.
- **Name compression is THE gotcha.** A `0xC0` pointer jumps to an earlier offset
  in the *same* message. The cursor only advances past the 2-byte pointer, inner
  RDATA names can be compressed too, and loops must be capped. Get these three
  right and the decoder is solid.
- **Iterative vs recursive resolution are different jobs.** Setting RD=1 hands
  the tree-walk to a resolver; RD=0 means *you* follow the NS referrals from the
  root, using glue records to avoid chicken-and-egg lookups. We did both.
- **UDP needs a deadline.** With no connection to drop, a missing reply is just
  silence — a read deadline is the only thing standing between you and a hang.
- **Inject I/O and operate on `[]byte` for testability.** Crafted byte slices let
  us pin down every wire-format edge case — including compression — with no
  network at all.

---

## 📖 Further Reading

- 🐍➡️🐹 [Go Quickstart for a Python Developer](../../docs/go-quickstart.md) — the project Go primer
- [Coding Challenges — Build Your Own DNS Resolver](https://codingchallenges.fyi/challenges/challenge-dns-resolver)
- [RFC 1035 — Domain Names: Implementation and Specification](https://datatracker.ietf.org/doc/html/rfc1035) (header §4.1.1, name compression §4.1.4, RR format §3.2)
- [RFC 3596 — DNS Extensions to Support IPv6 (AAAA)](https://datatracker.ietf.org/doc/html/rfc3596)
- [Julia Evans — Implement DNS in a Weekend](https://implement-dns.wizardzines.com/) (the friendliest walkthrough of exactly this build)
- [Root servers](https://www.iana.org/domains/root/servers) — where the iterative walk begins
- Go stdlib: [`net`](https://pkg.go.dev/net), [`encoding/binary`](https://pkg.go.dev/encoding/binary)
