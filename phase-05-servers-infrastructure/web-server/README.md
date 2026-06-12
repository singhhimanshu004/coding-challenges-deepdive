# Web Server

> **Phase:** 5 — Servers & Infrastructure
> **Difficulty:** 🟠
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** L

**Status:** ✅ Completed

> 🐍➡️🐹 **New to Go?** Read the project's
> [**Go Quickstart for a Python Developer**](../../docs/go-quickstart.md) first.
> It maps the Go idioms used here (`net.Listener`, goroutines, `bufio.Reader`,
> `defer`, interfaces, error returns, slices vs. maps) back to the Python you
> already know. This README assumes you've skimmed it and adds 🐍 callouts where
> Go does something a Python/Java dev wouldn't expect.

---

## 🎯 What We're Building

A working **HTTP/1.1 web server**, built from a **raw TCP socket** — the exact
counterpart to the Phase 3 [`curl`](../../phase-03-advanced-cli/curl/) client.
`curl` *wrote* HTTP request bytes and *parsed* the response; this server *reads*
those request bytes and *writes* the response. Same protocol, opposite ends of
the pipe.

> **We build the server on `net.Listen` / `net.Conn`, NOT `net/http`.** We run
> our own accept loop, parse the request line + headers + body by hand, and
> frame the response bytes (status line, headers, CRLFs, body) ourselves. That's
> the whole point — once you've served HTTP by hand, every other server (a proxy,
> a load balancer, a Redis server) is "the same accept loop with a different
> protocol on the socket."

We *do* reference `net/http` in one place: the **tests** use its client to make
requests, because a server's job is to satisfy a real client. But nothing on the
serving path imports it.

What our server supports:

```
web-server [--addr :8080] [--root ./public] [--verbose]

--addr      address to listen on (host:port; default ":8080")
--root      directory of static files to serve (default "./public")
--verbose   log every connection and request
```

```bash
web-server                                  # serve ./public on :8080
web-server --addr 127.0.0.1:9000 --root /srv/www --verbose
curl -i http://localhost:8080/              # -> 200, serves index.html
curl -I http://localhost:8080/css/style.css # HEAD: headers only, text/css
curl http://localhost:8080/hello            # a dynamic route
```

---

## 📚 Core Concepts

### 1. A server is just the other end of the byte pipe

In the `curl` challenge the key idea was: *a TCP connection is a two-way stream
of bytes, and HTTP is just agreed-upon text flowing over it.* A server is the
mirror image:

```
 ┌──────────┐   request bytes   ┌──────────┐
 │  client  │ ────────────────► │  server  │   net.Listen + Accept()
 │ (curl,   │                   │ (us!)    │   → one net.Conn per client
 │  browser)│ ◄──────────────── │          │
 └──────────┘   response bytes  └──────────┘
```

`net.Listen("tcp", ":8080")` binds the port and returns a `net.Listener`.
Calling `ln.Accept()` blocks until a client connects, then hands back a
`net.Conn` — the **same** `Read`/`Write` byte pipe `curl` dialed from the other
side.

> 🐍 `net.Listen` + `Accept()` is Python's `s = socket(); s.bind(); s.listen();
> conn, addr = s.accept()`. Java devs: it's `new ServerSocket(8080).accept()`.
> `net.Conn` is the accepted connection — there is **no "HTTP request object"**
> handed to us; we build one by parsing bytes.

### 2. The accept loop + goroutine-per-connection concurrency

The heart of every server is a tiny loop:

```
for {
    conn := ln.Accept()   // wait for the next client
    go handleConn(conn)   // serve it on its OWN goroutine; loop immediately
}
```

That one `go` keyword is the entire concurrency model. Each connection is served
independently and in parallel, so a slow client can never block the others.

> 🐍 In Python you'd reach for `threading`, `asyncio`, or a process pool to get
> this — and you'd worry about the GIL and thread overhead. A Go **goroutine** is
> so cheap (a few KB of stack, multiplexed by the runtime onto a small pool of OS
> threads) that "one goroutine per connection" is the *normal, idiomatic* design,
> not something to optimise away. This is Go's headline feature for servers.

### 3. HTTP/1.1 request framing — what we parse

A request is plain ASCII with a rigid layout (every line ends in `CRLF` =
`\r\n`):

```
GET /index.html HTTP/1.1\r\n      ← request line: METHOD  target  version
Host: localhost:8080\r\n          ← mandatory in HTTP/1.1
User-Agent: curl/8.0\r\n          ┐
Accept: */*\r\n                   ├ headers
Connection: keep-alive\r\n        ┘
\r\n                              ← BLANK LINE = headers finished
<optional body, Content-Length bytes>
```

Parsing steps (`request.go`):

1. Read the request line, split into exactly **three** fields.
2. Read header lines until the **blank line**; store names **lower-cased** so
   lookups are case-insensitive (`Content-Type` == `content-type`).
3. If `Content-Length: N` is present, read **exactly N** body bytes with
   `io.ReadFull` (a single `Read` may return fewer bytes — a naive read truncates
   the body).

> 🐍 We wrap the socket in a `bufio.Reader`. Its `ReadString('\n')` is like
> Python's `file.readline()` — read up to the next newline. Buffering matters: an
> unbuffered socket read of one line could mean one syscall per byte.

### 4. HTTP/1.1 response framing — what we write

The response mirrors the request shape (`response.go`):

```
HTTP/1.1 200 OK\r\n               ← status line: version  code  reason
Content-Type: text/html; charset=utf-8\r\n
Content-Length: 342\r\n           ← lets the client find the body's end
Connection: keep-alive\r\n
\r\n                              ← blank line = headers end
<342 body bytes>
```

We build the whole message in a buffer and do **one** `Write`, so the status
line, headers, and body arrive as one contiguous message.

**Why `Content-Length` is the linchpin:** it tells the client exactly where the
body ends *without* the server closing the connection. That single header is what
makes **keep-alive** possible (next section).

### 5. Routing & static file serving

A **router (mux)** maps `METHOD + path` → handler (`router.go`). If no dynamic
route matches, we fall back to serving **static files** from a configurable web
root:

- **Content-Type by extension** — a browser decides how to render a response
  from its `Content-Type`, *not* the file extension. Send `text/css` and the CSS
  applies; send the wrong type and the browser downloads it instead.
- **`/` → `index.html`** — the universal web convention.
- **404** for a missing file, **405** when the path exists for a different
  method, **501** for a method we don't implement.

### 6. 🔒 Path traversal — the security gotcha you must not skip

A naïve static server does `filepath.Join(root, requestPath)` and reads the
result. A malicious client sends:

```
GET /../../../../etc/passwd HTTP/1.1
```

Joined onto the root, those `..` segments **climb out of the web root** and hand
the attacker any file the server process can read. This is
[CWE-22](https://cwe.mitre.org/data/definitions/22.html), one of the oldest and
most common web vulnerabilities.

Our defence is **two layers** (`fileServer.serve`):

1. **Lexical cleaning** — `filepath.Clean("/" + path)` collapses `.` and `..`
   *before* joining, so `/a/../../etc` resolves to `/etc` relative to the root.
2. **Containment check** — after joining to an absolute path, verify the result
   is still **inside** the resolved absolute root; if not, return **403
   Forbidden**.

> ⚠️ Never rely on layer 1 alone: lexical cleaning can be defeated by symlinks,
> so the containment re-check against the real absolute root is the one that
> actually has to hold. We also compare against `root + separator` so a sibling
> directory like `/srv/www-evil` can't sneak past a `/srv/www` prefix test.

### 7. Keep-alive vs. close — persistent connections

In **HTTP/1.0** the model was *one request per TCP connection*: connect, ask,
answer, disconnect — paying a full TCP (and for HTTPS, TLS) handshake **every
single time**. **HTTP/1.1** made **persistent connections the default**: after
sending a response we loop back and read the *next* request on the *same* socket,
amortising the handshake across many requests. This is a huge real-world latency
win (a page pulls dozens of assets).

The rules our server honours (`handleConn` loop):

| Version | `Connection` header | Connection behaviour |
| --- | --- | --- |
| HTTP/1.1 | *(absent)* | **keep-alive** (the default) |
| HTTP/1.1 | `close` | close after this response |
| HTTP/1.0 | *(absent)* | **close** (the default) |
| HTTP/1.0 | `keep-alive` | keep-alive (explicit opt-in) |

That **default flip** between 1.0 and 1.1 is the detail everyone misremembers.

We also arm a **read timeout** (`SetReadDeadline`) before each request. Without
it, a client that connects and goes silent would pin a goroutine and a file
descriptor forever — a trivial denial-of-service. Every real server sets one
(nginx's `keepalive_timeout` defaults to ~75s; ours is 10s for demonstration).

---

## 🏗️ Architecture & Design

One concern per file, so each protocol idea has an obvious home:

```
request.go    parse raw bytes → request{method, path, version, headers, body}
              ── incl. the HTTP/1.0-vs-1.1 keep-alive default rules
response.go   build response{status, headers, body} → exact wire bytes (CRLFs)
router.go     mux (method+path → handler) + static file server (+ traversal guard)
server.go     net.Listen, the ACCEPT LOOP, goroutine-per-conn, keep-alive loop
main.go       CLI flags (--addr/--root/--verbose); wires it all together
```

Dependency flow is a straight line: `main → server → (router → response, request)`.

> 🐍 **Testability via dependency injection.** `server.serve(ln net.Listener)` is
> split out from `listenAndServe()` so tests bind their *own* `127.0.0.1:0`
> listener (the OS picks a free port), read the chosen address from `ln.Addr()`,
> and drive real requests through the server. The logger is an `io.Writer`, so
> tests send logs to `io.Discard`. Same idea as passing a file object into a
> Python function instead of `open()`-ing inside it — no globals, no fixed ports,
> no flaky network.

---

## 🔨 Step-by-Step Implementation

1. **`request.go` — parse the request.** `bufio.NewReader(conn)`, read the
   request line (split into 3), read headers until the blank line (lower-case the
   names), then read exactly `Content-Length` body bytes with `io.ReadFull`. Add
   `wantsKeepAlive()` encoding the 1.0/1.1 default rules. Cap total header bytes
   so a slow-loris client can't buffer forever.
2. **`response.go` — frame the reply.** A `response` struct with a status code,
   header map, and body. `newResponse` pre-fills `Content-Length`. `write()`
   serialises status line → sorted headers → blank line → body into one buffer
   and does a single `Write`.
3. **`router.go` — route + serve files.** Exact `method+path` map for dynamic
   routes; fall back to the `fileServer`. The file server cleans the path,
   maps `/` → `index.html`, **checks containment against the absolute root**
   (the security step), sets `Content-Type` by extension, and drops the body for
   `HEAD`.
4. **`server.go` — the engine.** `net.Listen` → accept loop → `go handleConn`.
   `handleConn` is the keep-alive loop: arm a read deadline, parse a request,
   route it, write the response with the right `Connection` header, and either
   loop (keep-alive) or return (close / timeout / EOF / error). A clean EOF or a
   timeout on a kept-alive connection is *normal*, not an error to report.
5. **`main.go` — the CLI.** Stdlib `flag` for `--addr`/`--root`/`--verbose`,
   register the demo `/hello` route, and start serving.

---

## 🧪 Testing Strategy

Two layers, **fully self-contained — no external network** (tests must be
hermetic and fast):

1. **Pure unit tests** (`request_test.go`):
   - **Table-driven request parsing**: request line, headers, `Content-Length`
     body, and the keep-alive default rules across HTTP/1.0 and HTTP/1.1.
   - **Error cases**: malformed request line, bad version, header with no colon,
     non-numeric `Content-Length` — proving we reject cleanly instead of hanging
     or panicking.
   - **Content-Type mapping**, including the unknown-extension fallback.
2. **End-to-end through a real listener** (`integration_test.go`): each test
   starts the server on `127.0.0.1:0` and makes **real requests through it**,
   asserting:
   - a static file is served with the right **status + Content-Type** (and `/`
     serves `index.html`);
   - a missing file returns **404**;
   - a **path-traversal** attempt (`/../secret.txt`, sent over a raw socket so a
     well-behaved client can't normalise the `..` away first) does **not** leak
     the planted secret file;
   - **keep-alive reuses one TCP connection** for two sequential requests, and
     `Connection: close` is honoured;
   - the dynamic route and **405 Method Not Allowed** behave correctly.

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
triggered only because this package imports `net`/`net/http` (which pull in
**cgo** for the system DNS resolver). The fix is to build in pure-Go mode, which
uses Go's internal linker:

```bash
CGO_ENABLED=0 go test ./...     # ✅ all tests pass
```

`go vet ./...` passes either way, and the binary builds cleanly with
`CGO_ENABLED=0 go build`. (The Phase 3 `curl` challenge documents the identical
workaround.) **Manual verification:** the server was run live and confirmed
serving `/` (200, `text/html`), `/css/style.css` (200, `text/css`, HEAD = no
body), `/hello` (dynamic), a 404 for a missing path, a rejected traversal
attempt, and **two requests over a single keep-alive connection**.

---

## 💡 Key Takeaways

- **A server is the accept loop.** `net.Listen` → `Accept()` → `go
  handleConn(conn)`. Everything else is protocol detail layered on a byte pipe.
- **Goroutine-per-connection is idiomatic Go.** The cheap `go` keyword replaces
  the thread pools / async machinery you'd reach for in Python or Java.
- **`Content-Length` is what unlocks keep-alive.** Tell the client exactly how
  many body bytes to expect and you no longer need to close the connection to
  signal "body done" — so the next request can reuse the same socket.
- **The keep-alive default flips between HTTP/1.0 (close) and 1.1 (keep-alive).**
  Honour the `Connection` header, advertise your decision back, and always set a
  read timeout so idle clients can't exhaust you.
- **Path traversal is the static-server footgun.** Clean the path *and* verify
  the resolved file stays inside the web root. Never trust client-supplied paths.
- **Inject your I/O (listener, logger) for testability.** A `127.0.0.1:0`
  listener + `io.Discard` logger let us test the whole server against real
  sockets with zero external network.

---

## 📖 Further Reading

- 🐍➡️🐹 [Go Quickstart for a Python Developer](../../docs/go-quickstart.md) — the project Go primer
- 🔁 [The `curl` challenge](../../phase-03-advanced-cli/curl/) — the HTTP/1.1 **client** counterpart to this server
- [Coding Challenges — Build Your Own Web Server](https://codingchallenges.fyi/challenges/challenge-webserver)
- [RFC 7230 — HTTP/1.1 Message Syntax and Routing](https://datatracker.ietf.org/doc/html/rfc7230) (message framing; persistent connections: §6)
- [RFC 7231 — HTTP/1.1 Semantics and Content](https://datatracker.ietf.org/doc/html/rfc7231) (methods, status codes)
- [MDN — Connection management in HTTP/1.x](https://developer.mozilla.org/en-US/docs/Web/HTTP/Connection_management_in_HTTP_1.x) (keep-alive vs close)
- [CWE-22 — Path Traversal](https://cwe.mitre.org/data/definitions/22.html)
- Go stdlib: [`net`](https://pkg.go.dev/net), [`bufio`](https://pkg.go.dev/bufio), [`path/filepath`](https://pkg.go.dev/path/filepath)
