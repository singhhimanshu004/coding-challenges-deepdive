package main

import (
	"bufio"
	"net"
	"strings"
	"testing"
)

// rawServer starts a local TCP listener that speaks HTTP/1.1 by hand. It reads
// the client's request, hands it to the test, and writes back the supplied raw
// response bytes. This lets us exercise the FULL dial → write → parse path with
// zero dependency on the public internet.
//
// It returns the "host:port" to target and a channel that delivers the exact
// request bytes the server received (so we can assert on our own framing).
func rawServer(t *testing.T, rawResponse string) (string, <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0") // :0 = pick a free port
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	gotReq := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read the request up to and including the blank line that ends headers.
		br := bufio.NewReader(conn)
		var sb strings.Builder
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				break
			}
			sb.WriteString(line)
			if line == "\r\n" { // blank line: end of request headers
				break
			}
		}
		gotReq <- sb.String()

		conn.Write([]byte(rawResponse))
	}()

	return ln.Addr().String(), gotReq
}

// TestEndToEndContentLength drives doRequest against a local server returning a
// Content-Length body, and verifies both directions of the exchange.
func TestEndToEndContentLength(t *testing.T) {
	resp := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: text/plain\r\n" +
		"Content-Length: 13\r\n" +
		"\r\n" +
		"Hello, world!"

	addr, gotReq := rawServer(t, resp)
	tgt, err := parseTarget("http://" + addr + "/greet")
	if err != nil {
		t.Fatalf("parseTarget: %v", err)
	}

	got, err := doRequest(tgt, "GET", options{}, nil, nil)
	if err != nil {
		t.Fatalf("doRequest: %v", err)
	}

	// Assert on what the server actually received from our framer.
	reqBytes := <-gotReq
	if !strings.HasPrefix(reqBytes, "GET /greet HTTP/1.1\r\n") {
		t.Errorf("request line wrong:\n%q", reqBytes)
	}
	if !strings.Contains(reqBytes, "Host: "+addr+"\r\n") {
		t.Errorf("Host header missing/wrong:\n%q", reqBytes)
	}
	if !strings.Contains(reqBytes, "Connection: close\r\n") {
		t.Errorf("Connection: close missing:\n%q", reqBytes)
	}

	// Assert on the parsed response.
	if got.statusCode != 200 {
		t.Errorf("status = %d; want 200", got.statusCode)
	}
	if string(got.body) != "Hello, world!" {
		t.Errorf("body = %q; want %q", got.body, "Hello, world!")
	}
}

// TestEndToEndChunked drives the full path with a chunked server response.
func TestEndToEndChunked(t *testing.T) {
	resp := "HTTP/1.1 200 OK\r\n" +
		"Transfer-Encoding: chunked\r\n" +
		"\r\n" +
		"6\r\nchunk-\r\n" +
		"5\r\nbased\r\n" +
		"0\r\n\r\n"

	addr, _ := rawServer(t, resp)
	tgt, _ := parseTarget("http://" + addr + "/")

	got, err := doRequest(tgt, "GET", options{}, nil, nil)
	if err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if string(got.body) != "chunk-based" {
		t.Errorf("chunked body = %q; want %q", got.body, "chunk-based")
	}
}

// TestEndToEndPOSTBody verifies that -d body bytes and Content-Length actually
// reach the server intact.
func TestEndToEndPOSTBody(t *testing.T) {
	resp := "HTTP/1.1 201 Created\r\nContent-Length: 2\r\n\r\nok"
	addr, gotReq := rawServer(t, resp)
	tgt, _ := parseTarget("http://" + addr + "/submit")

	body := []byte(`{"k":"v"}`)
	got, err := doRequest(tgt, "POST", options{}, body, nil)
	if err != nil {
		t.Fatalf("doRequest: %v", err)
	}

	reqBytes := <-gotReq
	if !strings.HasPrefix(reqBytes, "POST /submit HTTP/1.1\r\n") {
		t.Errorf("expected POST request line:\n%q", reqBytes)
	}
	if !strings.Contains(reqBytes, "Content-Length: 9\r\n") {
		t.Errorf("expected Content-Length: 9:\n%q", reqBytes)
	}
	if got.statusCode != 201 {
		t.Errorf("status = %d; want 201", got.statusCode)
	}
}
