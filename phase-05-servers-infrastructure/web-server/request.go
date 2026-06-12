package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// crlf is the line terminator HTTP uses EVERYWHERE: carriage-return + line-feed
// ("\r\n", bytes 13 and 10). A bare "\n" is NOT a valid HTTP line ending, and
// the blank line that separates headers from the body is itself just an empty
// CRLF. Getting these two bytes right is most of what "speaking HTTP" means.
const crlf = "\r\n"

// maxHeaderBytes caps how much of the request head (request line + headers) we
// are willing to buffer before giving up. Without a cap, a client could open a
// connection and dribble header bytes forever, pinning a goroutine and memory.
// Real servers all impose a limit like this (Go's net/http defaults to 1 MB).
const maxHeaderBytes = 64 * 1024

// request is one parsed HTTP/1.1 request message.
//
// 🐍 For a Python dev: think of this as the data you'd get from Flask's
// `request` object — `request.method`, `request.path`, `request.headers`,
// `request.get_data()` — except here we populate it ourselves from raw bytes.
type request struct {
	method  string            // "GET", "POST", ...
	path    string            // request target, e.g. "/index.html"
	version string            // "HTTP/1.1" or "HTTP/1.0"
	headers map[string]string // header names lower-cased for case-insensitive lookup
	body    []byte            // request body (only when Content-Length > 0)
}

// header returns the value of a header by name, case-insensitively. HTTP header
// names are case-insensitive ("Content-Type" == "content-type"), so we always
// look them up by their lower-cased form.
func (r *request) header(name string) string {
	return r.headers[strings.ToLower(name)]
}

// wantsKeepAlive reports whether this request asks us to keep the TCP connection
// open for a follow-up request, per the HTTP/1.1 rules:
//
//   - HTTP/1.1 defaults to keep-alive. The connection stays open UNLESS the
//     client explicitly sends "Connection: close".
//   - HTTP/1.0 defaults to close. The connection stays open ONLY if the client
//     explicitly sends "Connection: keep-alive".
//
// This default flip between 1.0 and 1.1 is one of the most important — and most
// commonly misremembered — details of the protocol.
func (r *request) wantsKeepAlive() bool {
	conn := strings.ToLower(r.header("Connection"))
	if r.version == "HTTP/1.0" {
		return conn == "keep-alive"
	}
	return conn != "close"
}

// parseRequest reads exactly one HTTP/1.1 request message from br and returns it.
//
// The wire shape we are decoding (every line ends in CRLF):
//
//	┌────────────────────────────────────────────┐
//	│ METHOD <SP> request-target <SP> HTTP/1.1 CRLF│  ← request line
//	├────────────────────────────────────────────┤
//	│ Header-Name: value CRLF                      │  ┐
//	│ Header-Name: value CRLF                      │  ├ header section
//	│ ...                                          │  ┘
//	├────────────────────────────────────────────┤
//	│ CRLF                                         │  ← blank line = end of headers
//	├────────────────────────────────────────────┤
//	│ <optional body, exactly Content-Length bytes>│
//	└────────────────────────────────────────────┘
//
// 🐍 A *bufio.Reader is Go's buffered reader. ReadString('\n') is like Python's
// file.readline(): it returns everything up to and including the next '\n'. We
// strip the trailing CRLF ourselves. Buffering matters because reading a socket
// one byte at a time would mean one syscall per byte.
func parseRequest(br *bufio.Reader) (*request, error) {
	// --- Request line ----------------------------------------------------
	line, err := readLine(br)
	if err != nil {
		// io.EOF here means the peer closed the connection cleanly with no new
		// request — that is normal on a kept-alive connection, so pass it
		// through unwrapped for the caller to recognise.
		return nil, err
	}

	// The request line is exactly three space-separated fields. SplitN with a
	// limit of 3 keeps any stray spaces inside the (already unusual) target as
	// part of the target rather than mis-counting fields.
	parts := strings.SplitN(line, " ", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("malformed request line: %q", line)
	}
	req := &request{
		method:  parts[0],
		path:    parts[1],
		version: parts[2],
		headers: make(map[string]string),
	}
	if req.version != "HTTP/1.1" && req.version != "HTTP/1.0" {
		return nil, fmt.Errorf("unsupported HTTP version: %q", req.version)
	}

	// --- Headers ---------------------------------------------------------
	// Read header lines until we hit the blank line that terminates the head.
	read := len(line)
	for {
		hline, err := readLine(br)
		if err != nil {
			return nil, err
		}
		if hline == "" { // the blank line: headers are finished
			break
		}
		read += len(hline)
		if read > maxHeaderBytes {
			return nil, fmt.Errorf("request headers too large (>%d bytes)", maxHeaderBytes)
		}

		// Split "Name: value" on the FIRST colon only; values may contain ':'
		// (think timestamps or URLs).
		i := strings.IndexByte(hline, ':')
		if i < 0 {
			return nil, fmt.Errorf("malformed header line: %q", hline)
		}
		name := strings.ToLower(strings.TrimSpace(hline[:i]))
		value := strings.TrimSpace(hline[i+1:])
		req.headers[name] = value
	}

	// --- Body ------------------------------------------------------------
	// In HTTP/1.1 the body length is announced by Content-Length. (Chunked
	// transfer is the other option; we keep the server simple and only accept
	// Content-Length request bodies — most clients use it.) If there is no
	// Content-Length, there is no body.
	if cl := req.header("Content-Length"); cl != "" {
		n, err := strconv.Atoi(cl)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("invalid Content-Length: %q", cl)
		}
		body := make([]byte, n)
		// io.ReadFull reads EXACTLY n bytes (or errors). This is the correct
		// way to consume a fixed-length body: a single Read may return fewer
		// bytes than asked for, so a naive Read would truncate the body.
		if _, err := io.ReadFull(br, body); err != nil {
			return nil, fmt.Errorf("reading body: %w", err)
		}
		req.body = body
	}

	return req, nil
}

// readLine reads a single CRLF-terminated line and returns it WITHOUT the
// trailing "\r\n". A line that ends in a bare "\n" (no "\r") is tolerated, since
// some clients are sloppy, but we always strip a trailing "\r" if present.
func readLine(br *bufio.Reader) (string, error) {
	line, err := br.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
