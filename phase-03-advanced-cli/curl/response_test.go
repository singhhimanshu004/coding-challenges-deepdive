package main

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

// newReader is a tiny helper: wrap a raw response string in the buffered reader
// parseResponse expects, exactly as if it came off a socket.
func newReader(s string) *bufio.Reader {
	return bufio.NewReader(strings.NewReader(s))
}

// TestParseContentLength covers the common case: status line, headers, and a
// body framed by an exact Content-Length.
func TestParseContentLength(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: text/plain\r\n" +
		"Content-Length: 5\r\n" +
		"\r\n" +
		"hello"

	resp, err := parseResponse(newReader(raw), false)
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	if resp.statusCode != 200 || resp.statusText != "OK" {
		t.Errorf("status = %d %q; want 200 OK", resp.statusCode, resp.statusText)
	}
	if resp.proto != "HTTP/1.1" {
		t.Errorf("proto = %q; want HTTP/1.1", resp.proto)
	}
	if got := resp.get("Content-Type"); got != "text/plain" {
		t.Errorf("Content-Type = %q; want text/plain", got)
	}
	if string(resp.body) != "hello" {
		t.Errorf("body = %q; want %q", resp.body, "hello")
	}
}

// TestParseStatusLineReasonWithSpaces guards the SplitN-by-3 logic so multi-word
// reason phrases ("Not Found") survive intact.
func TestParseStatusLineReasonWithSpaces(t *testing.T) {
	raw := "HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n"
	resp, err := parseResponse(newReader(raw), false)
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	if resp.statusCode != 404 || resp.statusText != "Not Found" {
		t.Errorf("status = %d %q; want 404 Not Found", resp.statusCode, resp.statusText)
	}
}

// TestHeaderCaseInsensitiveLookup confirms get() ignores header-name case.
func TestHeaderCaseInsensitiveLookup(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\ncontent-length: 2\r\n\r\nhi"
	resp, err := parseResponse(newReader(raw), false)
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	if resp.get("Content-Length") != "2" {
		t.Errorf("case-insensitive lookup failed: got %q", resp.get("Content-Length"))
	}
}

// TestHeadOnlyNoBody ensures a HEAD-style parse stops after the headers and does
// not block trying to read a body the server won't send.
func TestHeadOnlyNoBody(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\nContent-Length: 1234\r\n\r\n"
	resp, err := parseResponse(newReader(raw), true)
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	if len(resp.body) != 0 {
		t.Errorf("HEAD response should have empty body, got %q", resp.body)
	}
}

// TestParseChunked is THE important one: a chunked body with hex sizes must be
// reassembled into the original bytes, with sizes/CRLFs consumed correctly.
func TestParseChunked(t *testing.T) {
	// "Mozilla" (7 = 0x7) + "Developer" (9 = 0x9) + "Network" (7 = 0x7) + end.
	raw := "HTTP/1.1 200 OK\r\n" +
		"Transfer-Encoding: chunked\r\n" +
		"\r\n" +
		"7\r\n" +
		"Mozilla\r\n" +
		"9\r\n" +
		"Developer\r\n" +
		"7\r\n" +
		"Network\r\n" +
		"0\r\n" +
		"\r\n"

	resp, err := parseResponse(newReader(raw), false)
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	want := "MozillaDeveloperNetwork"
	if string(resp.body) != want {
		t.Errorf("chunked body = %q; want %q", resp.body, want)
	}
}

// TestParseChunkedHexSize uses a chunk whose size needs real hexadecimal parsing
// (0x1A = 26) — catches the classic "parsed size as decimal" bug.
func TestParseChunkedHexSize(t *testing.T) {
	payload := "abcdefghijklmnopqrstuvwxyz" // 26 bytes
	raw := "HTTP/1.1 200 OK\r\n" +
		"Transfer-Encoding: chunked\r\n" +
		"\r\n" +
		"1a\r\n" + payload + "\r\n" +
		"0\r\n\r\n"

	resp, err := parseResponse(newReader(raw), false)
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	if string(resp.body) != payload {
		t.Errorf("hex-sized chunk decode = %q; want %q", resp.body, payload)
	}
}

// TestParseChunkedWithExtensionsAndTrailers exercises chunk extensions (";a=b")
// and a trailer header after the final 0 chunk — both must be ignored cleanly.
func TestParseChunkedWithExtensionsAndTrailers(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\n" +
		"Transfer-Encoding: chunked\r\n" +
		"\r\n" +
		"5;name=value\r\n" +
		"hello\r\n" +
		"0\r\n" +
		"X-Checksum: deadbeef\r\n" +
		"\r\n"

	resp, err := parseResponse(newReader(raw), false)
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	if string(resp.body) != "hello" {
		t.Errorf("body = %q; want %q", resp.body, "hello")
	}
}

// TestReadChunkedDirect tests the decoder in isolation, reading from a buffer
// that contains ONLY the chunked body (no status/headers).
func TestReadChunkedDirect(t *testing.T) {
	body := "4\r\nWiki\r\n5\r\npedia\r\n0\r\n\r\n"
	got, err := readChunked(newReader(body))
	if err != nil {
		t.Fatalf("readChunked error: %v", err)
	}
	if string(got) != "Wikipedia" {
		t.Errorf("readChunked = %q; want %q", got, "Wikipedia")
	}
}

// TestParseBodyUntilEOF covers the no-length, no-chunked case where the body is
// framed only by the connection closing.
func TestParseBodyUntilEOF(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\nstreamed body to EOF"
	resp, err := parseResponse(newReader(raw), false)
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	if string(resp.body) != "streamed body to EOF" {
		t.Errorf("body = %q", resp.body)
	}
}

func TestMalformedStatusLine(t *testing.T) {
	for _, raw := range []string{
		"garbage\r\n\r\n",
		"HTTP/1.1 notanumber OK\r\n\r\n",
	} {
		if _, err := parseResponse(newReader(raw), false); err == nil {
			t.Errorf("expected error for malformed response %q", raw)
		}
	}
}

func TestInvalidChunkSize(t *testing.T) {
	raw := "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\nZZ\r\noops\r\n"
	if _, err := parseResponse(newReader(raw), false); err == nil {
		t.Error("expected error for invalid hex chunk size")
	}
}

// TestRedirectDetection checks isRedirect()'s 3xx + Location logic.
func TestRedirectDetection(t *testing.T) {
	withLoc := response{statusCode: 301, headers: []header{{"Location", "http://x/"}}}
	if !withLoc.isRedirect() {
		t.Error("301 with Location should be a redirect")
	}
	noLoc := response{statusCode: 302}
	if noLoc.isRedirect() {
		t.Error("302 without Location should NOT be a redirect")
	}
	ok := response{statusCode: 200, headers: []header{{"Location", "x"}}}
	if ok.isRedirect() {
		t.Error("200 should never be a redirect")
	}
}

// sanity: make sure a framed request round-trips through a bytes.Buffer write
// without altering the bytes (guards against accidental mutation in frame()).
func TestFrameIsStable(t *testing.T) {
	tgt, _ := parseTarget("http://h/")
	req := request{method: "GET", target: tgt}
	a := req.frame()
	b := req.frame()
	if !bytes.Equal(a, b) {
		t.Error("frame() should be deterministic")
	}
}
