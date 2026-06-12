# netcat (nc)

> **Phase:** 4 — Networking Fundamentals
> **Difficulty:** 🔵→🟠
> **Recommended Language:** 🟦 Go
> **Effort Estimate:** M

**Status:** ✅ Done

> 🐍➡️🐹 **New to Go?** Read the project's
> [**Go Quickstart for a Python Developer**](../../docs/go-quickstart.md) first.
> It maps every Go idiom used here (`defer`, goroutines, interfaces, `io.Reader`/
> `io.Writer`, error returns) back to the Python you already know. This README
> assumes you've skimmed it and adds 🐍 callouts where Go does something
> surprising.

---

## 🎯 What We're Building

A working clone of **`netcat`** (`nc`) — the program people call the *networking
Swiss-army knife*. At its core netcat does one deceptively simple thing:

> **It connects your terminal's stdin/stdout to a network socket and shuttles
> bytes between them in both directions.**

That single idea powers a huge number of tricks: poking at a web server by hand,
transferring a file, chatting between two machines, building a quick port
listener, or piping the output of one command across the network into another.

Our `nc` has two modes and speaks two protocols:

```
nc <host> <port>      CONNECT: dial a TCP server, then relay
nc -l <port>          LISTEN:  accept ONE TCP connection, then relay

-l            listen mode (act as a server) instead of connecting
-u            use UDP instead of TCP
-p PORT       port to use (alternative to the positional port)
-w SECONDS    quit if the read side has been idle this long
              (effectively REQUIRED for UDP, which never sees an EOF)
```

Examples:

```bash
# Talk to a web server by hand — type the HTTP request yourself:
printf 'GET / HTTP/1.0\r\n\r\n' | nc example.com 80

nc -l 9000                 # listen on TCP :9000 for one client
nc -u 127.0.0.1 9000       # send/receive UDP datagrams
nc -u -l -w 5 9000         # UDP listener, give up after 5s of silence

# A two-line "chat": run the listener in one terminal, connect in another.
nc -l 9000                 # terminal A
nc 127.0.0.1 9000          # terminal B — whatever you type appears in A
```

This challenge is the **payoff** for Phase 4: once you've seen that a connection
is "just a two-way pipe of bytes," every higher-level networking tool (HTTP, a
proxy, an RPC framework) is recognizable as *some agreed-upon bytes on a socket*.

---

## 📚 Core Concepts

### 1. A socket is a two-way byte pipe

When you dial a server, the kernel performs the TCP handshake and hands you a
`net.Conn`. That value has just two interesting methods:

```
 ┌──────────┐   conn.Write(bytes)   ┌──────────┐
 │  your nc │ ────────────────────► │  server  │
 │          │ ◄──────────────────── │          │
 └──────────┘   conn.Read(bytes)    └──────────┘
        one TCP connection = a bidirectional stream of bytes
```

There is no "message" framing built in — TCP is a raw stream. netcat doesn't
care what the bytes *mean*; it just moves them.

> 🐍 Python analogy: `net.Dial("tcp", "host:80")` is
> `socket.create_connection(("host", 80))`. `conn.Read`/`conn.Write` are
> `sock.recv`/`sock.sendall`. The difference: Go exposes them through the
> `io.Reader`/`io.Writer` interfaces, so the *same* copy code that works on a
> file or a buffer also works on a socket.

### 2. TCP vs. UDP — connection-oriented vs. connectionless

| | **TCP** | **UDP** |
| --- | --- | --- |
| Model | a *connection* (handshake, then a stream) | *datagrams* — fire and forget |
| Server step | `Listen` then **`Accept`** a connection | `ListenUDP`, then just **read packets** |
| Ordering / retransmit | guaranteed by the kernel | none — packets may drop or reorder |
| End of data | **EOF** when the peer closes | there is **no EOF** — silence ≠ closed |
| Who's my peer? | fixed once connected | learned **per packet** (`ReadFromUDP`) |

The "no EOF" row is the one that bites you. A TCP relay knows it's finished when
the read returns EOF. A UDP relay never gets that signal, so we end it with a
**timeout** (`-w`) instead. This is why our UDP examples always pass `-w`.

> 🐍 In Python this is the `SOCK_STREAM` vs. `SOCK_DGRAM` distinction. With UDP
> you call `recvfrom()` (which tells you the sender) and `sendto(addr)` — exactly
> what our UDP-listener wrapper does under the hood.

### 3. `io.Copy` — the streaming copy loop

`io.Copy(dst, src)` reads chunks from `src` and writes them to `dst` until `src`
hits EOF. It streams: a small fixed buffer, no matter how much data flows. That's
why netcat can pipe a multi-gigabyte file or a never-ending log without using
gigabytes of RAM.

> 🐍 It's `shutil.copyfileobj(src, dst)` — the read-a-chunk/write-a-chunk loop —
> but it works on *anything* implementing the reader/writer interfaces.

### 4. Two concurrent goroutines — copying both ways at once

A connection has two directions and they're **independent**: the server might be
streaming data to you *while* you're still typing. A single-threaded `io.Copy`
can only watch one direction, so we run **two copies concurrently**:

```
        goroutine:  io.Copy(conn, stdin)     ← what you type → network
        main:       io.Copy(stdout, conn)    ← network → your screen
```

> 🐍 This is two threads each running `copyfileobj`, but a **goroutine** is far
> cheaper than an OS thread (a few KB of stack), and the `go` keyword starts one.
> No thread pool, no `async`/`await` ceremony.

### 5. EOF and the TCP half-close

A TCP connection can be shut down **one direction at a time**. When your local
input ends (you press Ctrl-D, or a pipe closes), we want to tell the peer "I'm
done sending" *without* tearing down the whole socket — because the peer may
still have a reply for us.

That one-way shutdown is a **half-close** (`CloseWrite`). The peer sees EOF on
its read side, finishes its work, sends its reply, and closes. Our
network→stdout copy then sees *its* EOF and the relay ends cleanly.

```
echo "GET / HTTP/1.0\r\n\r\n" | nc example.com 80
   │
   └─ stdin reaches EOF ─► we CloseWrite() ─► server sees EOF
                                            ─► server sends the page back
                                            ─► server closes
                                            ─► our read sees EOF ─► nc exits
```

> 🐍 `CloseWrite()` is `socket.shutdown(socket.SHUT_WR)`. Forgetting the
> half-close is the classic "why does my `nc | server` hang forever?" bug — the
> server is still politely waiting for more request bytes that will never come.

---

## 🏗️ Architecture & Design

The whole program is small and splits into two files plus tests:

```
netcat/
├── main.go         CLI parsing + mode dispatch (connect vs. listen)
├── relay.go        the byte-shuttling core + TCP/UDP plumbing
├── relay_test.go   self-contained tests (real loopback sockets, no internet)
├── go.mod          module netcat
└── .gitignore
```

The **one function that matters** is `relay`:

```go
func relay(conn io.ReadWriter, stdin io.Reader, stdout io.Writer) error
```

Notice it takes **interfaces**, not concrete sockets. That single design choice
gives us everything:

- **Testability** — a test passes a real loopback `net.Conn` plus a
  `strings.Reader` and a `bytes.Buffer`; no subprocess, no fixed ports.
- **TCP/UDP reuse** — both protocols funnel into the *same* relay. The only
  difference (half-close vs. timeout) is decided by a tiny interface check
  inside `relay`.

```
                    ┌──────────────── relay(conn, stdin, stdout) ───────────────┐
                    │                                                            │
  net.Dial (TCP)  ──┤  goroutine:  io.Copy(conn, stdin)  ──► CloseWrite on EOF   │
  net.Dial (UDP)  ──┤                                                            │
  Accept   (TCP)  ──┤  main:       io.Copy(stdout, conn)  ──► returns on EOF/-w  │
  ListenUDP(UDP)  ──┘                                                            │
                    └────────────────────────────────────────────────────────────┘
```

**Why `main` → `run` with injected streams?** `main()` does nothing but call
`run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)` and `os.Exit`. Because every
stream is an injected interface, tests call `run()` with in-memory buffers and
assert on bytes — the same pattern used across this repo's Go challenges.

**The UDP-listener adapter.** A UDP listener has no single peer, so it can't
satisfy `io.ReadWriter` directly. `udpListenConn` wraps `*net.UDPConn`: its
`Read` uses `ReadFromUDP` and *remembers the sender*; its `Write` replies to that
remembered address with `WriteToUDP`. With that wrapper, the connectionless case
plugs into the exact same `relay`.

---

## 🔨 Step-by-Step Implementation

**Step 1 — Parse the command line (`parseArgs`).** A small hand-rolled parser
walks argv, sets `-l`/`-u` booleans, reads the values after `-p`/`-w`, and treats
everything else as positional. It then sorts the positionals into host/port
based on the mode (listen needs only a port; connect needs host + port). We
hand-roll instead of using the `flag` package so flags and positionals can mix
freely, matching real netcat's feel.

**Step 2 — Dispatch on mode (`run`).** Pick `"tcp"`/`"udp"`, then call either
`dialAndRelay` (client) or `listenAndRelay` (server). Errors become a message on
stderr and an exit code: `2` for usage mistakes, `1` for runtime failures, `0` on
a clean finish.

**Step 3 — Open the socket.**
- *Connect:* `net.Dial` (or `net.DialTimeout`) returns a ready `net.Conn`.
- *Listen TCP:* `net.Listen` then block in `Accept()` for one client.
- *Listen UDP:* `net.ListenUDP` — no accept; we just start reading datagrams,
  wrapped by `udpListenConn`.

**Step 4 — Arm the timeout.** If `-w` was given, set a read deadline. For TCP
this is a safety net; for UDP it's the *only* way the relay ever stops, since UDP
reads never return EOF.

**Step 5 — Relay (the heart).** Start `io.Copy(conn, stdin)` in a goroutine; run
`io.Copy(stdout, conn)` in the foreground. When stdin ends, half-close TCP
(`CloseWrite`) so the peer sees EOF. When the foreground copy returns (peer
closed, or the `-w` deadline fired), the relay is done. A deadline error is the
expected end-of-UDP signal, so we swallow it and report success.

---

## 🧪 Testing Strategy

Everything is verified with **self-contained tests** — real sockets bound to
`127.0.0.1:0` (the kernel hands us a free ephemeral port) and in-memory
readers/writers standing in for stdin/stdout. **No external network**, no fixed
ports, no flakiness.

- **`TestRelayTCP`** — drives `relay` over a loopback TCP connection: send
  `"ping"` up, the server echoes `"pong"` back and closes; we assert *both*
  directions arrived and that the half-close + EOF made the relay return.
- **`TestServeTCP`** — listen mode: `serveTCP` accepts a client, relays its
  injected stdin to that client, and copies the client's bytes to stdout. Asserts
  both halves of the conversation.
- **`TestDialAndRelayUDP`** — bidirectional UDP via connect mode against an
  in-process echo server, ending on a short `-w` deadline.
- **`TestServePacketInbound`** — connectionless listen: a datagram sent to our
  bound UDP socket must reach stdout; verifies the `ReadFromUDP`-based adapter.
- **`TestParseArgs`** — table-driven coverage of flag/positional combinations and
  the error cases (missing port, unknown flag, bad `-w`).
- **`TestRunConnectTCP`** — the full `run()` entry point end-to-end against an
  echo server, asserting exit code `0` and the relayed bytes.

Run them:

```bash
go vet ./...
CGO_ENABLED=0 go test ./...     # see the toolchain note below
```

### ⚠️ Environment / toolchain note (read this if `go test` aborts)

On this macOS dev box (go1.22.2, darwin/arm64), a plain `go test ./...` aborts
before any test runs with:

```
dyld: missing LC_UUID load command
signal: abort trap
```

This is **not a bug in our code** — it's a known mismatch between the Go
toolchain's external linker and the installed Xcode Command-Line Tools `ld`,
triggered because importing `net` pulls in **cgo** for the system resolver. The
fix is to build in pure-Go mode, which uses Go's internal linker and native
resolver:

```bash
CGO_ENABLED=0 go test ./...     # ✅ all tests pass
```

`go vet ./...` passes either way, and the binary builds cleanly with
`CGO_ENABLED=0 go build -o nc`.

---

## 💡 Key Takeaways

- **A connection is just a two-way byte pipe.** netcat moves bytes; it doesn't
  care what they mean. Every protocol you'll build later is "agreed-upon bytes on
  a socket."
- **`io.Copy` + a goroutine = a concurrent relay.** One copy per direction,
  streaming, with a tiny buffer. This is the canonical Go pattern for bridging
  two streams.
- **Half-close (`CloseWrite`) propagates EOF without killing the socket.**
  Skipping it is the #1 cause of "why does my pipe hang?" Half-close one
  direction, keep reading the reply.
- **TCP has EOF; UDP doesn't.** Connection-oriented streams end naturally;
  connectionless datagrams need a timeout to know when to stop.
- **Design around interfaces (`io.Reader`/`io.Writer`).** Accepting interfaces
  instead of concrete sockets made the core trivially testable *and* let TCP and
  UDP share the same relay.
- **`defer conn.Close()` is function-scoped** (not block-scoped like Python's
  `with`). For one connection per call that's exactly right.

---

## 📖 Further Reading

- Hobbit, *“The GNU Netcat README / man page”* — the original tool's behaviour:
  https://nc110.sourceforge.io/
- Go docs: [`net`](https://pkg.go.dev/net) — `Dial`, `Listen`, `TCPConn.CloseWrite`,
  `ListenUDP`, `UDPConn.ReadFromUDP`/`WriteToUDP`.
- Go docs: [`io.Copy`](https://pkg.go.dev/io#Copy) and the
  [`io.Reader`/`io.Writer`](https://pkg.go.dev/io#Reader) interfaces.
- *The Go Programming Language* (Donovan & Kernighan), Ch. 8 — goroutines,
  channels, and concurrent network servers.
- RFC 793 (TCP) and RFC 768 (UDP) — the protocols themselves, for when you want
  to know exactly what the kernel is doing for you.
- Coding Challenges: [Build Your Own netcat](https://codingchallenges.fyi/) —
  the source brief for this challenge.
