package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestServer starts the web server on 127.0.0.1:0 (a free port) serving the
// given web root, and returns the base URL like "http://127.0.0.1:54321". The
// listener is closed automatically when the test finishes.
//
// This is fully self-contained: a real TCP listener on loopback, real requests
// flowing through our hand-written parser and framer — but no external network.
func newTestServer(t *testing.T, root string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &server{
		router:  newRouter(root),
		verbose: false,
		logger:  log.New(io.Discard, "", 0),
	}
	srv.router.handle("GET", "/hello", helloHandler)

	go srv.serve(ln) // serve() closes ln on return
	t.Cleanup(func() { ln.Close() })

	return "http://" + ln.Addr().String()
}

// writeTestRoot creates a temporary web root with a couple of files and returns
// its path.
func writeTestRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"),
		[]byte("<h1>home</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "css"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "css", "style.css"),
		[]byte("body{color:red}"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A file OUTSIDE the root that a path-traversal attack would try to reach.
	if err := os.WriteFile(filepath.Join(filepath.Dir(root), "secret.txt"),
		[]byte("TOP SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestServeStaticFile checks that a static file is served with the right status
// and Content-Type, using the stdlib http client (which speaks the protocol our
// server must satisfy).
func TestServeStaticFile(t *testing.T) {
	base := newTestServer(t, writeTestRoot(t))

	tests := []struct {
		name        string
		path        string
		wantStatus  int
		wantType    string
		wantContain string
	}{
		{"index via root", "/", 200, "text/html; charset=utf-8", "home"},
		{"explicit html", "/index.html", 200, "text/html; charset=utf-8", "home"},
		{"css file", "/css/style.css", 200, "text/css; charset=utf-8", "color:red"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(base + tc.path)
			if err != nil {
				t.Fatalf("GET %s: %v", tc.path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status = %d; want %d", resp.StatusCode, tc.wantStatus)
			}
			if ct := resp.Header.Get("Content-Type"); ct != tc.wantType {
				t.Errorf("Content-Type = %q; want %q", ct, tc.wantType)
			}
			body, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(body), tc.wantContain) {
				t.Errorf("body = %q; want it to contain %q", body, tc.wantContain)
			}
		})
	}
}

// TestNotFound verifies a missing file returns 404.
func TestNotFound(t *testing.T) {
	base := newTestServer(t, writeTestRoot(t))
	resp, err := http.Get(base + "/does-not-exist.html")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("status = %d; want 404", resp.StatusCode)
	}
}

// TestPathTraversalRejected is the security test: an attempt to escape the web
// root with "../" must NOT return the secret file. We send the raw, un-cleaned
// request over a socket ourselves, because a well-behaved HTTP client (and Go's
// net/url) would normalise the "../" away before it ever reached the server —
// hiding exactly the attack we want to prove we defend against.
func TestPathTraversalRejected(t *testing.T) {
	root := writeTestRoot(t)
	base := newTestServer(t, root)
	addr := strings.TrimPrefix(base, "http://")

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Try to climb out of the root to the sibling secret.txt we planted.
	raw := "GET /../secret.txt HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n"
	if _, err := conn.Write([]byte(raw)); err != nil {
		t.Fatalf("write: %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 200 {
		t.Fatalf("traversal succeeded (status 200) — server is vulnerable!")
	}
	if strings.Contains(string(body), "TOP SECRET") {
		t.Fatalf("leaked secret file contents: %q", body)
	}
	if resp.StatusCode != 403 && resp.StatusCode != 404 {
		t.Errorf("status = %d; want 403 or 404", resp.StatusCode)
	}
}

// TestKeepAliveReusesConnection proves HTTP/1.1 persistent connections work:
// two sequential requests over the SAME TCP socket both get answered. We drive
// the socket by hand so we can guarantee a single connection is reused (rather
// than trusting an HTTP client's connection pool).
func TestKeepAliveReusesConnection(t *testing.T) {
	base := newTestServer(t, writeTestRoot(t))
	addr := strings.TrimPrefix(base, "http://")

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	br := bufio.NewReader(conn)

	for i, path := range []string{"/index.html", "/css/style.css"} {
		req := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: x\r\nConnection: keep-alive\r\n\r\n", path)
		if _, err := conn.Write([]byte(req)); err != nil {
			t.Fatalf("request %d write: %v", i, err)
		}
		// http.ReadResponse reads exactly one response (honouring Content-Length)
		// and leaves the reader positioned for the NEXT response — which only
		// works if the server kept the connection open.
		resp, err := http.ReadResponse(br, nil)
		if err != nil {
			t.Fatalf("request %d read: %v", i, err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Errorf("request %d status = %d; want 200", i, resp.StatusCode)
		}
		// On all but the last request the server must NOT close the connection.
		if resp.Close {
			t.Errorf("request %d: server closed a keep-alive connection", i)
		}
		if got := resp.Header.Get("Connection"); !strings.EqualFold(got, "keep-alive") {
			t.Errorf("request %d Connection header = %q; want keep-alive", i, got)
		}
	}
}

// TestConnectionClose verifies the server honours "Connection: close".
func TestConnectionClose(t *testing.T) {
	base := newTestServer(t, writeTestRoot(t))
	addr := strings.TrimPrefix(base, "http://")

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	req := "GET /index.html HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n"
	conn.Write([]byte(req))

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer resp.Body.Close()
	// Go's response reader folds "Connection: close" into resp.Close rather than
	// leaving it in the header map, so we assert on that.
	if !resp.Close {
		t.Errorf("resp.Close = false; want true (server should close the connection)")
	}
}

// TestDynamicRoute exercises the router with the demo handler.
func TestDynamicRoute(t *testing.T) {
	base := newTestServer(t, writeTestRoot(t))
	resp, err := http.Get(base + "/hello")
	if err != nil {
		t.Fatalf("GET /hello: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(string(body), "Hello, world!") {
		t.Errorf("got status %d body %q; want 200 with greeting", resp.StatusCode, body)
	}
}

// TestMethodNotAllowed verifies that a known path with an unsupported method
// returns 405.
func TestMethodNotAllowed(t *testing.T) {
	base := newTestServer(t, writeTestRoot(t))
	req, _ := http.NewRequest("DELETE", base+"/hello", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /hello: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 405 {
		t.Errorf("status = %d; want 405", resp.StatusCode)
	}
}
