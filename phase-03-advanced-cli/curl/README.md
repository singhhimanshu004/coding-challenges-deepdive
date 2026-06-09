# curl

> **Phase:** 3 — Advanced CLI & Orchestration
> **Difficulty:** 🔵→🟠
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** L

**Status:** ✅ Done

> 🐍➡️🐹 **New to Go?** Read the project's
> [**Go Quickstart for a Python Developer**](../../docs/go-quickstart.md) first.
> It maps every Go idiom used here (`defer`, `bufio`, interfaces, error returns,
> slices vs. maps) back to the Python you already know. This README assumes
> you've skimmed it, and adds 🐍 callouts where Go does something surprising.

---

## 🎯 What We're Building

A working clone of **`curl`** — the command that fetches a URL — but with a
strict rule that makes it a *learning* project rather than a one-liner:

> **We build the HTTP/1.1 client from a RAW TCP SOCKET.** No `net/http`. We open
> the socket ourselves with `net.Dial`, **type out the HTTP request as text**,
> send it down the wire, and **parse the bytes that come back by hand** —
> including the two different ways a response body can be framed.

This is the **bridge into networking** (Phase 4). Once you've seen that "HTTP"
is just agreed-upon *text* flowing over a TCP byte-pipe, every networking topic
afterwards (DNS, a web server, a proxy) becomes "another protocol on a socket."

What our `curl` supports:

```
curl [options] URL

-X METHOD        request method (default GET; POST when -d is given)
-H 'Name: val'   add a request header (repeatable)
-d DATA          send DATA as the body (implies POST + Content-Length)
-o FILE          write the response body to FILE instead of stdout
-I               HEAD request — fetch and print headers only
-v               verbose — print request (>) and response (<) headers
-L               follow 3xx redirects (Location header)
```

Examples:

```bash
curl http://example.com                       # GET, body to stdout
curl -I https://example.com                    # headers only (HEAD)
curl -v https://example.com                    # see the raw conversation
curl -X POST -d '{"a":1}' -H 'Content-Type: application/json' http://api/x
curl -L http://github.com                       # follow http→https redirect
curl -o page.html https://example.com           # save body to a file
```

---

## 📚 Core Concepts

### 1. A socket is just a byte pipe. HTTP is just text on it.

The single most important idea:

```
 ┌──────────┐   write(bytes)   ┌──────────┐
 │  curl    │ ───────────────► │  server  │
 │ (client) │ ◄─────────────── │          │
 └──────────┘   read(bytes)    └──────────┘
        a TCP connection = a two-way stream of bytes
```

`net.Dial("tcp", "example.com:80")` performs the TCP three-way handshake and
hands you a `net.Conn`. That `Conn` has just two interesting methods:
`Read([]byte)` and `Write([]byte)`. **There is no "HTTP object."** HTTP is an
*agreement* about what text to `Write` and how to interpret the text you `Read`.
We are going to honour that agreement by hand.

> 🐍 Python analogy: `net.Conn` is `socket.create_connection((host, port))`. The
> `Read`/`Write` interface is Go's duck typing — `io.Reader`/`io.Writer` — but
> checked at *compile* time instead of at runtime.

### 2. The HTTP/1.1 request message — byte by byte

A request is plain ASCII text with a rigid layout. Here is a real one,
annotated (every line ends with `CRLF` = `\r\n` = bytes `13 10`):

```
GET /index.html HTTP/1.1\r\n      ← request line: METHOD  target  version
Host: example.com\r\n             ← MANDATORY in 1.1 (virtual hosting)
User-Agent: cc-curl/1.0\r\n       ← who's asking
Accept: */*\r\n                   ← what we'll accept back
Connection: close\r\n             ← "close the socket after one reply"
\r\n                              ← BLANK LINE = "headers are finished"
```

For a request **with a body** (e.g. `POST`), the body bytes come right after
that blank line, and you MUST announce their length:

```
POST /submit HTTP/1.1\r\n
Host: api.test\r\n
Content-Type: application/json\r\n
Content-Length: 13\r\n            ← exactly 13 body bytes follow
Connection: close\r\n
\r\n
{"name":"go"}                    ← the 13-byte body (no trailing CRLF)
```

Three gotchas that bite everyone:

| Gotcha | Why it matters |
| --- | --- |
| **Lines end in `\r\n`, not `\n`** | A bare `\n` is not a valid HTTP line ending; strict servers reject it. |
| **The blank line is required** | It's the *only* signal that headers are done. Forget it and the server waits forever. |
| **A body needs `Content-Length`** | Otherwise the server can't tell where the body ends. |

### 3. The HTTP/1.1 response — and the two ways a body is framed

The response mirrors the request shape:

```
HTTP/1.1 200 OK\r\n               ← status line: version  code  reason
Content-Type: text/html\r\n       ┐
Content-Length: 1256\r\n          ├ headers
Server: cloudflare\r\n            ┘
\r\n                              ← blank line = headers end
<...1256 body bytes...>           ← the body
```

The hard part of *parsing* is: **where does the body end?** HTTP/1.1 gives the
server two ways to tell you, and a real client must handle **both**:

**(a) `Content-Length: N`** — dead simple. Read exactly `N` more bytes.

**(b) `Transfer-Encoding: chunked`** — used when the server generates content on
the fly and *doesn't know the total length up front* (so it can't send a
`Content-Length`). The body arrives as self-describing **chunks**:

```
                chunked body wire format
   ┌────────────────────────────────────────────────┐
   │  7\r\n            ← chunk size in HEXADECIMAL    │
   │  Mozilla\r\n      ← exactly 7 (0x7) data bytes   │
   │  9\r\n            ← next chunk: 9 (0x9) bytes     │
   │  Developer\r\n                                   │
   │  7\r\n                                           │
   │  Network\r\n                                     │
   │  0\r\n            ← a ZERO-size chunk = THE END   │
   │  \r\n            ← (optional trailers,) final CRLF│
   └────────────────────────────────────────────────┘
        decoded body =  "MozillaDeveloperNetwork"
```

The decoding loop:

1. Read a line → parse the size as **hexadecimal** (classic bug: assuming
   decimal). Ignore anything after a `;` (chunk extensions).
2. Size `0`? The body is done — consume any trailer headers + final blank line.
3. Otherwise read exactly that many data bytes, **then consume the CRLF that
   follows each chunk's data** (skip it and you're off by two bytes forever).
4. Append and repeat.

If *neither* header is present, the body simply runs until the server **closes
the connection** — which is valid here precisely because we sent
`Connection: close`.

### 4. TLS (the `s` in https), in one paragraph

For `https` we layer **TLS on top of the TCP socket**. After the TCP handshake
we wrap the connection with `crypto/tls`, which:

1. sends a **ClientHello** (supported versions/cipher suites),
2. receives the server's **certificate chain**,
3. **verifies** that chain against the OS trust store and checks the cert's
   names cover the hostname (this is why we pass `ServerName`),
4. does a **key exchange** so both sides derive the same symmetric keys.

After that, every `Write` is transparently encrypted and every `Read`
decrypted — so **the HTTP code above the TLS layer is byte-for-byte identical**
whether the URL is `http` or `https`. That clean seam is exactly why TLS lives
in its own small wrapper (`conn.go`). We use the stdlib handshake (writing a TLS
stack from scratch is a whole separate universe), but nothing above it.

### 5. Redirects (`-L`)

A `3xx` status with a `Location:` header means "the thing you want is over
there." With `-L` we resolve `Location` (it may be relative, e.g. `/new`)
against the current URL and make a fresh request — up to a sane cap to avoid
loops. Per the spec, `301/302/303` turn the follow-up into a bodyless `GET`;
`307/308` preserve the original method and body.

---

## 🏗️ Architecture & Design

A clean split, one concern per file — so each protocol idea has an obvious home:

```
url.go        parse a URL → {scheme, host, port, path}; resolve redirects
conn.go       net.Dial the TCP socket; wrap with crypto/tls for https
request.go    frame the raw HTTP request bytes (request line, headers, body)
response.go   parse the raw response: status line, headers, body
              ── incl. the Content-Length reader AND the chunked decoder
main.go       CLI: flag parsing, method selection, redirect loop, output
```

The dependency flow is a straight line: `main` → (`url`, `conn`, `request`,
`response`). Each lower file is independently unit-testable against in-memory
bytes, which is exactly how the tests are written.

> 🐍 Python-dev note on testability: `run()` and `doRequest()` take their output
> streams (and the parser takes a `*bufio.Reader`) as parameters instead of
> hard-coding `os.Stdout`/the network. That's dependency injection — the same
> reason you'd pass a file object into a function rather than `open()`-ing
> inside it. Tests feed a `strings.Reader` or a local `net.Listener` and assert
> on a buffer; no real internet required.

---

## 🔨 Step-by-Step Implementation

1. **`url.go` — parse the address.** Use `net/url` for the *lexical* split only
   (that's pure string work, not the protocol). Default the port from the scheme
   (`80`/`443`), default an empty path to `/`, and keep the query on the path
   because the query is part of the "request target" on the first line.
2. **`conn.go` — open the pipe.** `net.DialTimeout` for the TCP handshake. For
   `https`, wrap with `tls.Client` and drive `Handshake()` eagerly so TLS errors
   surface with a clear message.
3. **`request.go` — frame the request.** Write the request line, the mandatory
   `Host`, the user's `-H` headers (so they can *override* our defaults),
   defaults (`User-Agent`, `Accept`), `Content-Length` when there's a body,
   `Connection: close`, the blank line, then the body.
4. **`response.go` — parse the reply.** Read the status line, then headers until
   the blank line, then choose the body framing: **chunked** (takes precedence)
   → **Content-Length** → **read-to-EOF**. The chunked decoder is the star.
5. **`main.go` — wire up the CLI.** Hand-rolled flag parser (authentic curl
   ergonomics, matching the other tools in this repo), method selection
   (`-I`→HEAD, `-d`→POST, else GET), the `-L` redirect loop, and `-v` verbose
   output to stderr.

---

## 🧪 Testing Strategy

Two layers, **neither depends on the public internet** (tests must be
hermetic and fast):

1. **Pure unit tests against in-memory byte fixtures** (`*_test.go`):
   - **Request framing** (`request_test.go`): exact-byte assertions on a GET, a
     POST with body + `Content-Length`, and `-H` overriding a default header.
   - **Response parsing** (`response_test.go`): status line (incl. multi-word
     reason phrases), case-insensitive header lookup, `Content-Length` bodies,
     HEAD/no-body, read-to-EOF, and **the chunked decoder** — hex sizes, chunk
     extensions, trailer headers, and the decoder in isolation. Plus malformed
     inputs (bad status line, bad hex size) to prove we error cleanly.
   - **URL parsing** (`url_test.go`): ports, defaults, query preservation,
     redirect resolution (relative + absolute).
2. **End-to-end against a local `net.Listener`** (`integration_test.go`): a
   tiny in-process server that speaks HTTP/1.1 by hand. It captures the exact
   request bytes our client sent (so we assert on *our own framing over a real
   socket*) and replies with `Content-Length`, chunked, and POST scenarios. This
   exercises the full `dial → write → parse` path with zero external network.

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
triggered only because this package imports `net`/`crypto/tls` (which pull in
**cgo** for the system DNS resolver). The fix is to build the tests in pure-Go
mode, which uses Go's internal linker and native resolver:

```bash
CGO_ENABLED=0 go test ./...     # ✅ all tests pass
```

`go vet ./...` passes either way. The shipped binary also builds cleanly with
`CGO_ENABLED=0 go build`. **Network verification:** with outbound access
available, real requests were confirmed working —
`curl -I http://example.com` (200), `curl -v https://example.com` (TLS
handshake + **chunked** body decoded), and `curl -L http://github.com`
(http→https **redirect followed**).

---

## 💡 Key Takeaways

- **HTTP is text on a TCP pipe.** Once you've written the request line and
  parsed a status line by hand, the mystique is gone. This unlocks all of
  Phase 4.
- **`\r\n` everywhere, and the blank line is sacred.** Line endings and the
  header-terminating blank line are the two things beginners get wrong first.
- **Two body-framing schemes, not one.** A real client *must* handle both
  `Content-Length` and `chunked`. The chunked decoder — **hex** sizes, a
  `0`-chunk terminator, and the easy-to-miss per-chunk trailing CRLF — is the
  trickiest parse in the whole challenge.
- **TLS is a clean wrapper.** Layer it under the HTTP code and the HTTP code
  doesn't change at all between `http` and `https`.
- **Inject your I/O for testability.** Passing readers/writers in (instead of
  reaching for the network or `os.Stdout`) let us test the entire client against
  in-memory bytes and a local listener — no flaky internet dependency.

---

## 📖 Further Reading

- 🐍➡️🐹 [Go Quickstart for a Python Developer](../../docs/go-quickstart.md) — the project Go primer
- [Coding Challenges — Build Your Own curl](https://codingchallenges.fyi/challenges/challenge-curl)
- [RFC 7230 — HTTP/1.1 Message Syntax and Routing](https://datatracker.ietf.org/doc/html/rfc7230) (message format, chunked: §4.1)
- [RFC 7231 — HTTP/1.1 Semantics and Content](https://datatracker.ietf.org/doc/html/rfc7231) (methods, status codes, redirects)
- [MDN — Transfer-Encoding: chunked](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Transfer-Encoding)
- Go stdlib: [`net`](https://pkg.go.dev/net), [`crypto/tls`](https://pkg.go.dev/crypto/tls), [`bufio`](https://pkg.go.dev/bufio)
