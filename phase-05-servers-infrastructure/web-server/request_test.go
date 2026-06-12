package main

import (
	"bufio"
	"strings"
	"testing"
)

// TestParseRequest covers the request-line + header + body parsing with a
// table of cases, including the keep-alive default rules that differ between
// HTTP/1.0 and HTTP/1.1.
func TestParseRequest(t *testing.T) {
	tests := []struct {
		name          string
		raw           string
		wantMethod    string
		wantPath      string
		wantVersion   string
		wantBody      string
		wantKeepAlive bool
	}{
		{
			name:          "simple GET 1.1 defaults to keep-alive",
			raw:           "GET /index.html HTTP/1.1\r\nHost: x\r\n\r\n",
			wantMethod:    "GET",
			wantPath:      "/index.html",
			wantVersion:   "HTTP/1.1",
			wantKeepAlive: true,
		},
		{
			name:          "1.1 with Connection: close",
			raw:           "GET / HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n",
			wantMethod:    "GET",
			wantPath:      "/",
			wantVersion:   "HTTP/1.1",
			wantKeepAlive: false,
		},
		{
			name:          "1.0 defaults to close",
			raw:           "GET / HTTP/1.0\r\nHost: x\r\n\r\n",
			wantMethod:    "GET",
			wantPath:      "/",
			wantVersion:   "HTTP/1.0",
			wantKeepAlive: false,
		},
		{
			name:          "1.0 opts in to keep-alive",
			raw:           "GET / HTTP/1.0\r\nConnection: keep-alive\r\n\r\n",
			wantVersion:   "HTTP/1.0",
			wantMethod:    "GET",
			wantPath:      "/",
			wantKeepAlive: true,
		},
		{
			name:          "POST with Content-Length body",
			raw:           "POST /submit HTTP/1.1\r\nContent-Length: 5\r\n\r\nhello",
			wantMethod:    "POST",
			wantPath:      "/submit",
			wantVersion:   "HTTP/1.1",
			wantBody:      "hello",
			wantKeepAlive: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			br := bufio.NewReader(strings.NewReader(tc.raw))
			req, err := parseRequest(br)
			if err != nil {
				t.Fatalf("parseRequest: %v", err)
			}
			if req.method != tc.wantMethod {
				t.Errorf("method = %q; want %q", req.method, tc.wantMethod)
			}
			if req.path != tc.wantPath {
				t.Errorf("path = %q; want %q", req.path, tc.wantPath)
			}
			if req.version != tc.wantVersion {
				t.Errorf("version = %q; want %q", req.version, tc.wantVersion)
			}
			if string(req.body) != tc.wantBody {
				t.Errorf("body = %q; want %q", req.body, tc.wantBody)
			}
			if got := req.wantsKeepAlive(); got != tc.wantKeepAlive {
				t.Errorf("wantsKeepAlive = %v; want %v", got, tc.wantKeepAlive)
			}
		})
	}
}

// TestParseRequestErrors proves malformed input is rejected cleanly rather than
// panicking or hanging.
func TestParseRequestErrors(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"missing fields in request line", "GET /\r\n\r\n"},
		{"unsupported version", "GET / HTTP/2.0\r\n\r\n"},
		{"malformed header", "GET / HTTP/1.1\r\nBadHeaderNoColon\r\n\r\n"},
		{"invalid Content-Length", "POST / HTTP/1.1\r\nContent-Length: abc\r\n\r\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			br := bufio.NewReader(strings.NewReader(tc.raw))
			if _, err := parseRequest(br); err == nil {
				t.Errorf("expected an error for %q, got nil", tc.raw)
			}
		})
	}
}

// TestContentTypeFor checks extension → MIME mapping, including the unknown
// fallback.
func TestContentTypeFor(t *testing.T) {
	tests := map[string]string{
		"/index.html":  "text/html; charset=utf-8",
		"/css/app.CSS": "text/css; charset=utf-8", // case-insensitive extension
		"/logo.png":    "image/png",
		"/data.bin":    "application/octet-stream", // unknown → generic
	}
	for path, want := range tests {
		if got := contentTypeFor(path); got != want {
			t.Errorf("contentTypeFor(%q) = %q; want %q", path, got, want)
		}
	}
}
