package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// response is the parsed form of an HTTP/1.1 response message.
type response struct {
	proto      string   // "HTTP/1.1"
	statusCode int      // 200, 404, 301, ...
	statusText string   // "OK", "Not Found", ...
	headers    []header // in the order the server sent them
	body       []byte   // fully decoded (de-chunked) body bytes
}

// get returns the FIRST value for a header name, case-insensitively (HTTP
// header names are case-insensitive: "Content-Length" == "content-length").
func (r response) get(name string) string {
	for _, h := range r.headers {
		if strings.EqualFold(h.name, name) {
			return h.value
		}
	}
	return ""
}

// statusLine reconstructs the original first line, handy for -v output.
func (r response) statusLine() string {
	return fmt.Sprintf("%s %d %s", r.proto, r.statusCode, r.statusText)
}

// isRedirect reports whether this is a 3xx that we should follow when -L is set.
func (r response) isRedirect() bool {
	return r.statusCode >= 300 && r.statusCode < 400 && r.get("Location") != ""
}

// parseResponse reads and decodes a complete HTTP/1.1 response from r.
//
// A response message mirrors the request's shape:
//
//	HTTP/1.1 200 OK            CRLF   ← status line
//	Header: value             CRLF   ┐
//	Header: value             CRLF   ├ header section
//	...                              ┘
//	                          CRLF   ← blank line = headers end here
//	<body bytes>                     ← framed by Content-Length OR chunked
//
// headOnly = true (for the -I / HEAD case) means the server promises NO body,
// so we stop right after the headers without trying to read one.
func parseResponse(r *bufio.Reader, headOnly bool) (*response, error) {
	resp := &response{}

	// --- Status line ----------------------------------------------------
	line, err := readLine(r)
	if err != nil {
		return nil, fmt.Errorf("reading status line: %w", err)
	}
	if err := parseStatusLine(line, resp); err != nil {
		return nil, err
	}

	// --- Headers --------------------------------------------------------
	// Read "Name: value" lines until we hit the blank line that ends the
	// header section.
	for {
		line, err := readLine(r)
		if err != nil {
			return nil, fmt.Errorf("reading headers: %w", err)
		}
		if line == "" { // the blank line — headers are done
			break
		}
		i := strings.IndexByte(line, ':')
		if i < 0 {
			return nil, fmt.Errorf("malformed header line: %q", line)
		}
		name := strings.TrimSpace(line[:i])
		value := strings.TrimSpace(line[i+1:])
		resp.headers = append(resp.headers, header{name: name, value: value})
	}

	// HEAD responses (and 1xx/204/304) carry no body by definition.
	if headOnly || resp.statusCode == 204 || resp.statusCode == 304 {
		return resp, nil
	}

	// --- Body -----------------------------------------------------------
	// The server tells us how the body is framed. There are two common ways,
	// and we must support both:
	//
	//   1. Transfer-Encoding: chunked  → length-prefixed pieces (size unknown
	//      up front, e.g. streamed/generated content).
	//   2. Content-Length: N           → exactly N bytes follow.
	//
	// chunked TAKES PRECEDENCE if both are present (RFC 7230 §3.3.3). If
	// neither is present, the body runs until the server closes the connection
	// (valid because we sent Connection: close).
	te := strings.ToLower(resp.get("Transfer-Encoding"))
	switch {
	case strings.Contains(te, "chunked"):
		body, err := readChunked(r)
		if err != nil {
			return nil, fmt.Errorf("decoding chunked body: %w", err)
		}
		resp.body = body

	case resp.get("Content-Length") != "":
		n, err := strconv.Atoi(resp.get("Content-Length"))
		if err != nil || n < 0 {
			return nil, fmt.Errorf("invalid Content-Length %q", resp.get("Content-Length"))
		}
		body := make([]byte, n)
		// io.ReadFull keeps reading until it has filled exactly n bytes or hits
		// an error — the right primitive when you know the length in advance.
		if _, err := io.ReadFull(r, body); err != nil {
			return nil, fmt.Errorf("reading body: %w", err)
		}
		resp.body = body

	default:
		// No length signal: read to EOF.
		body, err := io.ReadAll(r)
		if err != nil {
			return nil, fmt.Errorf("reading body until close: %w", err)
		}
		resp.body = body
	}

	return resp, nil
}

// parseStatusLine parses "HTTP/1.1 200 OK" into the response struct.
func parseStatusLine(line string, resp *response) error {
	// SplitN with 3 keeps the reason phrase intact even if it has spaces
	// ("HTTP/1.1 404 Not Found" → ["HTTP/1.1", "404", "Not Found"]).
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 2 || !strings.HasPrefix(parts[0], "HTTP/") {
		return fmt.Errorf("malformed status line: %q", line)
	}
	code, err := strconv.Atoi(parts[1])
	if err != nil {
		return fmt.Errorf("malformed status code in %q", line)
	}
	resp.proto = parts[0]
	resp.statusCode = code
	if len(parts) == 3 {
		resp.statusText = parts[2]
	}
	return nil
}

// readLine reads one CRLF-terminated line and returns it WITHOUT the trailing
// "\r\n". We read up to '\n' then strip the '\r', which tolerates a stray bare
// '\n' from a sloppy server while still being correct for proper CRLF.
func readLine(r *bufio.Reader) (string, error) {
	s, err := r.ReadString('\n')
	if err != nil {
		// A line with content but no newline at EOF is still usable.
		if err == io.EOF && s != "" {
			return strings.TrimRight(s, "\r\n"), nil
		}
		return "", err
	}
	return strings.TrimRight(s, "\r\n"), nil
}

// readChunked decodes a Transfer-Encoding: chunked body.
//
// THE CHUNKED WIRE FORMAT (this is the trickiest parse in the challenge):
//
//	1b\r\n                       ← chunk size in HEXADECIMAL (0x1b = 27), then CRLF
//	<27 bytes of data>\r\n       ← exactly that many data bytes, then a CRLF
//	9\r\n                        ← next chunk: 9 bytes
//	<9 bytes of data>\r\n
//	0\r\n                        ← a zero-size chunk marks THE END
//	\r\n                         ← (optional trailer headers, then) final CRLF
//
// Why it exists: when a server generates content on the fly it doesn't know the
// total length up front, so it can't send Content-Length. Instead it ships the
// body in self-describing pieces, each announcing its own size, and signals the
// end with a 0-length chunk. We reassemble all the data bytes into one slice.
func readChunked(r *bufio.Reader) ([]byte, error) {
	var body []byte

	for {
		// Read the chunk-size line. It may carry ";ext" chunk extensions after
		// the size, which we ignore — only the hex number before ';' matters.
		sizeLine, err := readLine(r)
		if err != nil {
			return nil, err
		}
		if i := strings.IndexByte(sizeLine, ';'); i >= 0 {
			sizeLine = sizeLine[:i]
		}
		sizeLine = strings.TrimSpace(sizeLine)

		// Base 16! Chunk sizes are hex, a frequent source of bugs if you assume
		// decimal. ParseUint(.., 16, ..) does the conversion.
		size, err := strconv.ParseInt(sizeLine, 16, 64)
		if err != nil || size < 0 {
			return nil, fmt.Errorf("invalid chunk size %q", sizeLine)
		}

		// Size 0 == last chunk. Consume the trailing (possibly empty) trailer
		// section up to the final blank line, then we're done.
		if size == 0 {
			for {
				trailer, err := readLine(r)
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}
					return nil, err
				}
				if trailer == "" {
					break
				}
			}
			return body, nil
		}

		// Read exactly `size` data bytes.
		chunk := make([]byte, size)
		if _, err := io.ReadFull(r, chunk); err != nil {
			return nil, fmt.Errorf("reading chunk data: %w", err)
		}
		body = append(body, chunk...)

		// Each chunk's data is followed by its own CRLF, which we must consume
		// before the next size line. Skipping this throws the parser off by two
		// bytes — a notoriously confusing bug.
		if _, err := readLine(r); err != nil {
			return nil, fmt.Errorf("reading chunk trailer CRLF: %w", err)
		}
	}
}
