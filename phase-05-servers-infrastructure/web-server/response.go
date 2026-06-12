package main

import (
	"bytes"
	"io"
	"sort"
	"strconv"
)

// statusText maps the status codes we emit to their canonical reason phrases.
// The reason phrase ("OK", "Not Found") is purely informational — clients key
// off the numeric code — but a well-formed status line includes it.
var statusText = map[int]string{
	200: "OK",
	400: "Bad Request",
	403: "Forbidden",
	404: "Not Found",
	405: "Method Not Allowed",
	408: "Request Timeout",
	413: "Payload Too Large",
	500: "Internal Server Error",
	501: "Not Implemented",
}

// response is one HTTP/1.1 response message we are about to send back.
//
// 🐍 Compare with Flask: returning `("body", 404, {"X": "y"})` builds the same
// three things — a status code, headers, and a body — except we serialise them
// to bytes by hand instead of letting the framework do it.
type response struct {
	statusCode int
	headers    map[string]string
	body       []byte
}

// newResponse builds a response with the given status and body, pre-filling the
// Content-Length header. Announcing Content-Length is what lets the client know
// where the body ends WITHOUT us closing the connection — which is exactly what
// makes keep-alive possible (see the server's connection loop).
func newResponse(status int, body []byte) *response {
	return &response{
		statusCode: status,
		headers:    map[string]string{"Content-Length": strconv.Itoa(len(body))},
		body:       body,
	}
}

// setHeader sets a response header, overwriting any existing value.
func (r *response) setHeader(name, value string) {
	r.headers[name] = value
}

// write serialises the response to w in the exact HTTP/1.1 wire format:
//
//	HTTP/1.1 <code> <reason> CRLF      ← status line
//	Header-Name: value CRLF            ┐
//	...                                ├ headers
//	CRLF                               ┘ ← blank line = end of headers
//	<body bytes>
//
// We build the whole message in a buffer and do a single Write so the status
// line, headers, and body cannot be interleaved with another goroutine's output
// on a shared writer, and so the client sees one contiguous message.
func (r *response) write(w io.Writer) error {
	reason := statusText[r.statusCode]
	if reason == "" {
		reason = "Status " + strconv.Itoa(r.statusCode)
	}

	var b bytes.Buffer
	// Status line.
	b.WriteString("HTTP/1.1 ")
	b.WriteString(strconv.Itoa(r.statusCode))
	b.WriteByte(' ')
	b.WriteString(reason)
	b.WriteString(crlf)

	// Headers. We sort the names so the output is deterministic, which keeps
	// the tests exact and the verbose logs readable. (Header order is not
	// semantically significant in HTTP.)
	names := make([]string, 0, len(r.headers))
	for name := range r.headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		b.WriteString(name)
		b.WriteString(": ")
		b.WriteString(r.headers[name])
		b.WriteString(crlf)
	}

	// Blank line: end of headers.
	b.WriteString(crlf)

	// Body.
	b.Write(r.body)

	_, err := w.Write(b.Bytes())
	return err
}

// textResponse is a convenience for the common "send this string with this
// status and Content-Type" case used by error pages and dynamic routes.
func textResponse(status int, contentType, body string) *response {
	resp := newResponse(status, []byte(body))
	resp.setHeader("Content-Type", contentType)
	return resp
}
