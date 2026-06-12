# HTTP Forward Proxy

> **Phase:** 4 — Networking Fundamentals
> **Difficulty:** 🟠
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** L

**Status:** ✅ Completed

> 🐍➡️🐹 **New to Go?** Read the project's
> [**Go Quickstart for a Python Developer**](../../docs/go-quickstart.md) first.
> It maps every Go idiom used here (`defer`, `bufio`, interfaces, goroutines,
> error returns, slices vs. maps) back to the Python you already know. This
> README assumes you've skimmed it and adds 🐍 callouts where Go does something
> surprising.

---

## 🏁 The Phase 4 Capstone

This is the final challenge of the networking phase, and it deliberately
**ties everything together**:

- From **`curl` (#21)** you learned that *HTTP is just text on a TCP socket* —
  a request line, headers, a blank line, a body. The proxy **reads and rewrites
  exactly that text**.
- From **`netcat` (#28)** you learned the **bidirectional byte relay** — two
  `io.Copy` loops shoving bytes between sockets. The proxy's HTTPS tunnel **is**
  that relay, generalised from "socket ↔ stdin" to "socket ↔ socket."
- From the whole phase you learned **goroutine-per-connection concurrency**,
  **dial/accept**, and **timeouts**. The proxy uses all three.

A proxy is the perfect capstone because it is simultaneously an HTTP **server**
(it accepts client requests) and an HTTP **client** (it fetches from origins) —
and for HTTPS it stops being either and becomes a **dumb pipe**.

---

## 🎯 What We're Building

A working **forward proxy** that a browser, `curl`, or an `http.Client` can be
configured to send all its traffic through. It listens on `:8080` by default and
serves two kinds of request on the same port:

```
┌────────┐   GET http://site/x      ┌────────┐   GET /x          ┌────────┐
│ client │ ───────────────────────► │ PROXY  │ ────────────────► │ origin │
│ (curl, │   (absolute-form)        │        │   (origin-form)   │ server │
│  http  │ ◄─────────────────────── │  :8080 │ ◄──────────────── │        │
│ Client)│      response relayed     └────────┘    response       └────────┘
└────────┘
```

For HTTPS, the picture changes completely — the proxy can't see anything:

```
┌────────┐  CONNECT site:443  ┌────────┐   dial site:443   ┌────────┐
│ client │ ─────────────────► │ PROXY  │ ────────────────► │ origin │
│        │ ◄─ 200 Established  │ :8080  │                   │ (TLS)  │
│        │ ════ TLS handshake + encrypted bytes (OPAQUE) ══════════► │
│        │ ◄═══════════ proxy blindly relays ciphertext ═══════════► │
└────────┘                    └────────┘                   └────────┘
```

Run it:

```bash
CGO_ENABLED=0 go build -o http-forward-proxy .   # see toolchain note below
./http-forward-proxy --listen :8080 --verbose

# In another terminal — point curl at the proxy with -x:
curl -x http://127.0.0.1:8080 http://example.com    # plain HTTP (proxy SEES it)
curl -x http://127.0.0.1:8080 https://example.com   # CONNECT tunnel (opaque)
```

### Forward proxy vs. reverse proxy

A **forward** proxy acts on behalf of the **client** — the client knows it's
there and is configured to use it (corporate egress filters, caching proxies,
Tor entry). A **reverse** proxy (nginx, a load balancer) acts on behalf of the
**server** and is invisible to clients. This challenge is a *forward* proxy.

---

## 📚 Core Concepts

### 1. The forward proxy role

Normally a client opens a socket directly to `example.com:80`. With a proxy
configured, the client instead opens a socket to the **proxy** and asks it to do
the fetching. The proxy is a man-in-the-middle by design — for plain HTTP it can
read, cache, filter, or log everything; for HTTPS it is reduced to a blind pipe
(see §5).

### 2. Request rewriting: absolute-form → origin-form

This is the subtle bit that makes a proxy a proxy. A normal HTTP request to a
server uses **origin-form** — just the path:

```
GET /path?q=1 HTTP/1.1
Host: example.com
```

But when a client talks to a *proxy*, it uses **absolute-form** — the whole URL
on the request line — because the proxy needs to know *which server* to dial:

```
GET http://example.com/path?q=1 HTTP/1.1
Host: example.com
```

The origin server, however, is **not** a proxy and will choke on absolute-form.
So the proxy must **rewrite** the request line back to origin-form before
forwarding. In our code:

```go
// req.URL.RequestURI() turns "http://example.com/path?q=1" into "/path?q=1"
fmt.Fprintf(&sb, "%s %s HTTP/1.1\r\n", req.Method, req.URL.RequestURI())
fmt.Fprintf(&sb, "Host: %s\r\n", req.Host)
```

### 3. Hop-by-hop vs. end-to-end headers

HTTP headers come in two flavours (RFC 7230 §6.1):

- **End-to-end** headers (`Content-Type`, `Authorization`, custom `X-*` …) are
  meant for the final recipient and **must be forwarded** untouched.
- **Hop-by-hop** headers (`Connection`, `Proxy-Connection`, `Keep-Alive`,
  `Transfer-Encoding`, `Upgrade`, `TE`, `Trailer`, `Proxy-Authenticate`,
  `Proxy-Authorization`) describe **the single TCP hop they arrived on** and must
  **not** be passed to the next hop.

A proxy that blindly forwards `Connection: keep-alive` or the non-standard
`Proxy-Connection` can wedge keep-alive negotiation or leak its own presence. We
strip them with a small lookup set (`isHopByHop`) and inject our own
`Connection: close` for a clean, simple framing.

### 4. CONNECT tunnelling

For `https://`, the client can't send a readable request — it must establish TLS
*end-to-end with the origin*. So instead of a normal request it sends:

```
CONNECT example.com:443 HTTP/1.1
Host: example.com:443
```

The proxy dials `example.com:443`, replies:

```
HTTP/1.1 200 Connection Established
```

…and from that instant the connection stops being HTTP. Whatever bytes the
client sends next (its TLS `ClientHello`) the proxy just **copies straight to
the origin**, and vice-versa. This is **tunnelling**: the proxy provides a raw
byte pipe and otherwise gets out of the way.

### 5. 🔐 TLS opacity — why the proxy CAN'T read HTTPS

This is the single most important takeaway of the challenge.

After the `200 Connection Established`, the client and the **origin** perform a
TLS handshake and derive **session keys that only the two of them possess**. The
proxy never participates in that key exchange. Everything flowing through the
tunnel afterwards is **ciphertext the proxy cannot decrypt, read, or modify** —
it can only see *how many* encrypted bytes went where, and the destination host
(from the CONNECT line). That end-to-end encryption is precisely what stops a
proxy (or any network middlebox) from snooping on HTTPS.

(The only way to "see inside" is **TLS interception**: the proxy terminates TLS
with its own forged certificate, which requires installing the proxy's CA on the
client — that's how corporate MITM appliances and tools like mitmproxy work, and
it's exactly the property HTTPS is designed to make *visible and refusable*.)

### 6. Concurrency, timeouts, teardown

Each accepted connection runs on its own **goroutine**, so one slow client never
blocks the rest. A `DialTimeout` caps how long we wait for an origin so a dead
upstream can't pin a goroutine forever. The tunnel uses a `sync.WaitGroup` to
wait for **both** copy directions to drain, and a TCP **half-close**
(`CloseWrite`) to signal EOF to each peer before the deferred `Close()` runs.

---

## 🏗️ Architecture & Design

```
main.go     CLI (flags --listen / --verbose), bind socket, build Proxy, Serve.
proxy.go    The proxy itself:
              Proxy.Serve         accept loop → goroutine per connection
              Proxy.handleConn    read ONE request, dispatch by method
              Proxy.handlePlainHTTP   absolute→origin rewrite, strip hop-by-hop,
                                      forward, relay response
              Proxy.handleConnect     dial origin, 200, hand off to tunnel
              tunnel                  bidirectional raw byte relay (2× io.Copy)
              isHopByHop / ensurePort / writeError   small helpers
proxy_test.go   Self-contained tests (httptest origins, in-process proxy).
```

### What's hand-rolled vs. library

The challenge says the focus is **proxy mechanics**, not re-implementing an HTTP
parser (we already did that by hand in `curl`). So:

| Concern | How we do it | Why |
|---|---|---|
| Parsing the inbound request line + headers | `http.ReadRequest` (stdlib) | Parsing is fiddly and not the lesson here; `curl` already taught it. |
| Request rewriting (absolute→origin) | **hand-rolled** string building | This *is* the proxy lesson. |
| Hop-by-hop header stripping | **hand-rolled** lookup set | Core proxy correctness rule. |
| Response relay (plain HTTP) | `io.Copy` over the raw socket | We never parse the response — `Connection: close` makes EOF the frame end. |
| CONNECT tunnel | **hand-rolled** raw `net.Conn` byte relay | The whole point — must be opaque, byte-for-byte. |

The CONNECT tunnel is deliberately **not** built on any `net/http` machinery —
it's raw `io.Copy` between `net.Conn`s, the same primitive `netcat` used.

### Testability seam

`Proxy.Serve` takes a `net.Listener`, and `main()` is a thin wrapper over
`run()`. Tests create their own listener on `127.0.0.1:0` (kernel picks a free
port), so there are **no fixed ports and no real internet** involved.

---

## 🔨 Step-by-Step Implementation

1. **Bind and accept (`Serve`).** `net.Listen("tcp", addr)` then a `for` loop
   calling `Accept()`. Each connection → `go p.handleConn(conn)`. A closed
   listener (test teardown) ends the loop cleanly.

2. **Read one request (`handleConn`).** Wrap the socket in a `bufio.Reader`
   (required by `http.ReadRequest`, and — importantly — it also buffers any
   bytes that arrived alongside the request). Parse, then branch on
   `req.Method == CONNECT`.

3. **Plain HTTP (`handlePlainHTTP`).**
   - Work out the origin host (`req.URL.Host`), default port 80, `net.DialTimeout`.
   - Rewrite: request line uses `req.URL.RequestURI()` (origin-form), re-emit the
     `Host` header, copy all **non**-hop-by-hop headers, append `Connection: close`.
   - Stream the request body through with `io.Copy` (handles POST/PUT).
   - Relay the response with a single `io.Copy(client, origin)` — `Connection:
     close` means EOF marks the end, so we never have to parse the response.

4. **HTTPS CONNECT (`handleConnect`).**
   - Dial `req.Host` (default port 443).
   - Write `HTTP/1.1 200 Connection Established\r\n\r\n`.
   - Hand both sockets to `tunnel`, reading the client side **through the
     `bufio.Reader`** so any pipelined tunnel bytes aren't lost.

5. **The tunnel (`tunnel`).** Two goroutines, each an `io.Copy` in one direction;
   on EOF each calls `CloseWrite()` to half-close and signal the peer; a
   `WaitGroup` waits for both before the deferred `Close()`s fire.

6. **CLI (`main`/`run`).** `flag.NewFlagSet` (a private set, so tests can re-run
   `run`) parses `--listen` and `--verbose`; `--verbose` swaps the no-op `logf`
   for a real logger.

---

## 🧪 Testing Strategy

All tests are **self-contained** — they use `httptest` origins and an in-process
proxy on a loopback port, with **zero external network access**.

- **`TestPlainHTTPThroughProxy`** (table-driven): a real `http.Client` configured
  with `Transport.Proxy` sends absolute-form requests through the proxy. The
  `httptest.NewServer` origin **asserts it never sees absolute-form** (proving
  the rewrite happened) and echoes the path; the test checks the body and a
  relayed response header come back. Cases: root, nested path, query string.

- **`TestHTTPSThroughCONNECT`**: an `httptest.NewTLSServer` origin plus an
  `http.Client` whose transport trusts the test cert *and* uses our proxy. The
  transport automatically issues `CONNECT`, completes a TLS handshake **through
  the tunnel**, and fetches a body. If the byte relay were wrong, the handshake
  would simply fail — so a passing test proves the tunnel is byte-accurate.

- **`TestConnectRawHandshake`**: the same path, but **hand-driven over a raw
  socket** — we type the `CONNECT` line, assert the `200 Connection Established`
  reply, then run `tls.Client(...).Handshake()` over the same connection and do a
  manual `GET`. This shows the wire-level steps a browser performs, with no
  `http.Transport` hiding them.

- **`TestIsHopByHop` / `TestEnsurePort`**: focused unit tests of the two pure
  helpers (header classification and host:port defaulting).

```bash
go vet ./...
CGO_ENABLED=0 go test ./...     # ✅ all tests pass — see toolchain note
```

### ⚠️ Environment / toolchain note (read this if `go test` aborts)

On this macOS dev box, a plain `go test ./...` aborts before any test runs with:

```
dyld: missing LC_UUID load command
signal: abort trap
```

This is **not a bug in our code** — it's the same known mismatch documented in
the `curl` challenge, between the Go toolchain's external linker and the
installed Xcode Command-Line Tools `ld`. It triggers only because this package
imports `net`/`net/http`/`crypto/tls`, which pull in **cgo** for the system DNS
resolver. The fix is to build in pure-Go mode (Go's internal linker + native
resolver):

```bash
CGO_ENABLED=0 go test ./...     # ✅ all tests pass
CGO_ENABLED=0 go build .        # ✅ binary builds cleanly
```

`go vet ./...` passes either way.

---

## 💡 Key Takeaways

- A **forward proxy** is an HTTP server and an HTTP client glued together — and
  for HTTPS, neither: just a byte pipe.
- **Request rewriting** (absolute-form → origin-form) is what distinguishes
  proxy traffic from direct traffic, and it's why the request line carries the
  whole URL when you set a proxy.
- **Hop-by-hop headers** must be stripped at each hop; forwarding them is a
  classic proxy bug.
- **CONNECT tunnelling + TLS opacity** is the headline lesson: once TLS is
  established end-to-end, the proxy fundamentally **cannot** read the traffic.
  This is a *protocol guarantee*, not a courtesy.
- **`io.Copy` is the universal streaming primitive** — the same two-direction
  relay powers `netcat` and this proxy's tunnel, using almost no memory at any
  throughput.
- **Goroutine-per-connection** makes a concurrent server almost free to write in
  Go.

### Go idioms for a Python dev (recap)

- `defer conn.Close()` = a `with`-block / `finally` that runs at function exit.
- A `*bufio.Reader` = a buffered wrapper like Python's `io.BufferedReader`, but
  here it also lets us parse the request without losing bytes meant for the tunnel.
- `interface { CloseWrite() error }` = duck typing made explicit and
  compiler-checked — "anything with a `CloseWrite` method."
- `go func(){...}()` + `sync.WaitGroup` = `threading.Thread` + `Thread.join()`,
  but goroutines are cheap enough to spawn per connection.
- Errors are **returned values**, not exceptions — you check them inline.

---

## 📖 Further Reading

- **RFC 7230 — HTTP/1.1 Message Syntax and Routing**:
  [§5.3 request target forms](https://www.rfc-editor.org/rfc/rfc7230#section-5.3)
  (absolute-form vs origin-form),
  [§6.1 Connection / hop-by-hop headers](https://www.rfc-editor.org/rfc/rfc7230#section-6.1).
- **RFC 7231 — HTTP/1.1 Semantics**:
  [§4.3.6 the CONNECT method](https://www.rfc-editor.org/rfc/rfc7231#section-4.3.6).
- **RFC 8446 — TLS 1.3** — why the session keys are end-to-end and the proxy
  can't have them: <https://www.rfc-editor.org/rfc/rfc8446>.
- **Go stdlib**: [`net/http.ReadRequest`](https://pkg.go.dev/net/http#ReadRequest),
  [`net.Conn`](https://pkg.go.dev/net#Conn),
  [`io.Copy`](https://pkg.go.dev/io#Copy).
- **mitmproxy docs** — a real interception proxy, to see what TLS termination
  (and its CA-install requirement) actually involves: <https://docs.mitmproxy.org/>.
- Sibling challenges in this repo: [`curl`](../../phase-03-advanced-cli/curl/)
  (the HTTP client) and [`netcat`](../netcat/) (the byte relay).
