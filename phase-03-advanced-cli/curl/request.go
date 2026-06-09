package main

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// crlf is the line terminator HTTP uses EVERYWHERE: carriage-return + line-feed
// ("\r\n", bytes 13 and 10). This is a classic gotcha — a bare "\n" is NOT a
// valid HTTP line ending and a strict server will reject it.
const crlf = "\r\n"

// defaultUserAgent identifies our client, the way real curl sends "curl/8.x".
const defaultUserAgent = "cc-curl/1.0"

// header is one ordered "Name: value" pair. We keep request headers in a slice
// (not a map) so the bytes we put on the wire come out in a predictable order —
// which makes the output deterministic and the unit tests exact.
type header struct {
	name  string
	value string
}

// request is everything needed to frame one HTTP/1.1 request message.
type request struct {
	method  string   // "GET", "POST", "HEAD", ...
	target  target   // host/port/path
	headers []header // user-supplied -H headers, in order
	body    []byte   // -d data; nil means no body
}

// frame renders the request into the exact bytes that travel down the socket.
//
// An HTTP/1.1 request message has a rigid shape:
//
//	┌──────────────────────────────────────────────┐
//	│ METHOD <SP> request-target <SP> HTTP/1.1 CRLF │  ← request line
//	├──────────────────────────────────────────────┤
//	│ Header-Name: value CRLF                        │  ┐
//	│ Header-Name: value CRLF                        │  ├ header section
//	│ ...                                            │  ┘
//	├──────────────────────────────────────────────┤
//	│ CRLF                                           │  ← blank line = end of headers
//	├──────────────────────────────────────────────┤
//	│ <optional body bytes>                          │  ← entity body (for POST etc.)
//	└──────────────────────────────────────────────┘
//
// The single blank line (an "empty" CRLF right after the last header) is how
// the server knows the headers are finished and the body begins. Forget it and
// the server waits forever.
func (r request) frame() []byte {
	var b bytes.Buffer

	// --- Request line ---------------------------------------------------
	// e.g. "GET /index.html?q=go HTTP/1.1"
	b.WriteString(r.method)
	b.WriteByte(' ')
	b.WriteString(r.target.path)
	b.WriteByte(' ')
	b.WriteString("HTTP/1.1")
	b.WriteString(crlf)

	// --- Headers --------------------------------------------------------
	// We build a canonical-name set as we go so user -H headers can OVERRIDE
	// our defaults instead of duplicating them (curl behaves this way).
	seen := map[string]bool{}
	write := func(name, value string) {
		b.WriteString(name)
		b.WriteString(": ")
		b.WriteString(value)
		b.WriteString(crlf)
		seen[strings.ToLower(name)] = true
	}

	// Host is MANDATORY in HTTP/1.1 — it's what lets one IP serve many sites
	// (virtual hosting). A request without it is malformed.
	write("Host", r.target.authority())

	// User -H headers come next so they can pre-empt the defaults below.
	for _, h := range r.headers {
		write(h.name, h.value)
	}

	// Sensible defaults, only if the user didn't already set them.
	if !seen["user-agent"] {
		write("User-Agent", defaultUserAgent)
	}
	if !seen["accept"] {
		write("Accept", "*/*")
	}

	// A body means we MUST tell the server how many bytes to expect, via
	// Content-Length. Without it the server can't know where the body ends on
	// a Connection: close stream.
	if r.body != nil && !seen["content-length"] {
		write("Content-Length", strconv.Itoa(len(r.body)))
	}

	// We always close the connection after one exchange. This keeps the
	// response parser simple: "read until EOF" is a valid body-termination
	// signal once the server agrees to close. (Real curl reuses connections;
	// we trade that performance for clarity.)
	if !seen["connection"] {
		write("Connection", "close")
	}

	// --- Blank line: end of headers ------------------------------------
	b.WriteString(crlf)

	// --- Body -----------------------------------------------------------
	if r.body != nil {
		b.Write(r.body)
	}

	return b.Bytes()
}

// parseHeaderArg splits a "-H" value like "Content-Type: application/json" into
// its name and value. We trim a single optional space after the colon, matching
// how curl forwards the header.
func parseHeaderArg(s string) (header, error) {
	i := strings.IndexByte(s, ':')
	if i < 0 {
		return header{}, fmt.Errorf("invalid header %q (expected 'Name: value')", s)
	}
	name := strings.TrimSpace(s[:i])
	value := strings.TrimSpace(s[i+1:])
	if name == "" {
		return header{}, fmt.Errorf("invalid header %q (empty name)", s)
	}
	return header{name: name, value: value}, nil
}
