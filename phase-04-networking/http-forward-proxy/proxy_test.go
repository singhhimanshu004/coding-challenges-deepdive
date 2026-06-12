package main

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// startProxy spins up the proxy on a kernel-chosen loopback port and returns its
// address plus a cleanup func. Everything is in-process — no external network.
func startProxy(t *testing.T) (addr string, cleanup func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0") // :0 → kernel picks a free port
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	p := &Proxy{Logf: func(string, ...any) {}, DialTimeout: 5 * time.Second}
	go p.Serve(ln)
	return ln.Addr().String(), func() { ln.Close() }
}

// proxyClient builds an http.Client whose Transport routes EVERY request through
// our proxy. For https targets the transport automatically issues CONNECT.
func proxyClient(t *testing.T, proxyAddr string, tlsCfg *tls.Config) *http.Client {
	t.Helper()
	pu, err := url.Parse("http://" + proxyAddr)
	if err != nil {
		t.Fatalf("parse proxy url: %v", err)
	}
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(pu),
			TLSClientConfig: tlsCfg,
		},
	}
}

// TestPlainHTTPThroughProxy: a real http.Client sends absolute-form requests to
// our proxy, which must rewrite + forward them and relay the origin's body back.
func TestPlainHTTPThroughProxy(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prove the proxy rewrote absolute-form → origin-form: the origin must
		// see a path, never the full URL on its request line.
		if strings.HasPrefix(r.RequestURI, "http://") {
			t.Errorf("origin saw absolute-form request URI %q (proxy did not rewrite)", r.RequestURI)
		}
		w.Header().Set("X-Origin", "yes")
		io.WriteString(w, "hello from origin: "+r.URL.Path)
	}))
	defer origin.Close()

	proxyAddr, cleanup := startProxy(t)
	defer cleanup()
	client := proxyClient(t, proxyAddr, nil)

	cases := []struct {
		name string
		path string
		want string
	}{
		{"root", "/", "hello from origin: /"},
		{"nested", "/a/b/c", "hello from origin: /a/b/c"},
		{"query", "/search?q=go", "hello from origin: /search"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := client.Get(origin.URL + tc.path)
			if err != nil {
				t.Fatalf("GET via proxy: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			if resp.Header.Get("X-Origin") != "yes" {
				t.Errorf("origin response header not relayed back")
			}
			body, _ := io.ReadAll(resp.Body)
			if string(body) != tc.want {
				t.Errorf("body = %q, want %q", body, tc.want)
			}
		})
	}
}

// TestHTTPSThroughCONNECT: the transport issues CONNECT to our proxy, completes
// a TLS handshake with a LOCAL https origin THROUGH the tunnel, and fetches a
// response. If the byte relay is wrong, the TLS handshake fails outright.
func TestHTTPSThroughCONNECT(t *testing.T) {
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "secret over TLS: "+r.URL.Path)
	}))
	defer origin.Close()

	// Trust the test server's self-signed cert (so the handshake through the
	// tunnel can validate). We are NOT teaching the proxy this cert — the proxy
	// only relays opaque bytes; the CLIENT verifies the cert end-to-end.
	pool := x509.NewCertPool()
	pool.AddCert(origin.Certificate())

	proxyAddr, cleanup := startProxy(t)
	defer cleanup()
	client := proxyClient(t, proxyAddr, &tls.Config{RootCAs: pool})

	resp, err := client.Get(origin.URL + "/vault")
	if err != nil {
		t.Fatalf("HTTPS GET via CONNECT: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if got, want := string(body), "secret over TLS: /vault"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

// TestConnectRawHandshake exercises the CONNECT path at the byte level: we open
// a RAW socket to the proxy, type the CONNECT line ourselves, assert the
// "200 Connection Established" reply, then run a TLS handshake over the same
// socket — exactly what a browser does, with no http.Transport hiding the steps.
func TestConnectRawHandshake(t *testing.T) {
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "tunnelled-ok")
	}))
	defer origin.Close()
	pool := x509.NewCertPool()
	pool.AddCert(origin.Certificate())

	originURL, _ := url.Parse(origin.URL)
	proxyAddr, cleanup := startProxy(t)
	defer cleanup()

	raw, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer raw.Close()

	// 1. Hand-write the CONNECT request.
	io.WriteString(raw, "CONNECT "+originURL.Host+" HTTP/1.1\r\nHost: "+originURL.Host+"\r\n\r\n")

	// 2. Read the proxy's status line and assert the tunnel is open.
	buf := make([]byte, 39) // len("HTTP/1.1 200 Connection Established\r\n\r\n")
	if _, err := io.ReadFull(raw, buf); err != nil {
		t.Fatalf("read CONNECT reply: %v", err)
	}
	if !strings.HasPrefix(string(buf), "HTTP/1.1 200") {
		t.Fatalf("CONNECT reply = %q, want 200 Connection Established", buf)
	}

	// 3. TLS handshake over the now-opaque tunnel, then a normal HTTPS GET.
	tlsConn := tls.Client(raw, &tls.Config{RootCAs: pool, ServerName: "example.com"})
	if err := tlsConn.Handshake(); err != nil {
		t.Fatalf("TLS handshake through tunnel: %v", err)
	}
	io.WriteString(tlsConn, "GET /ping HTTP/1.1\r\nHost: example.com\r\nConnection: close\r\n\r\n")
	got, _ := io.ReadAll(tlsConn)
	if !strings.Contains(string(got), "tunnelled-ok") {
		t.Errorf("tunnelled response = %q, want it to contain %q", got, "tunnelled-ok")
	}
}

// TestIsHopByHop is a focused unit test of the header-stripping rule.
func TestIsHopByHop(t *testing.T) {
	cases := []struct {
		header string
		want   bool
	}{
		{"Connection", true},
		{"Proxy-Connection", true},
		{"proxy-connection", true}, // canonicalised before lookup
		{"Transfer-Encoding", true},
		{"Keep-Alive", true},
		{"Content-Type", false},
		{"Host", false},
		{"X-Custom", false},
	}
	for _, tc := range cases {
		if got := isHopByHop(tc.header); got != tc.want {
			t.Errorf("isHopByHop(%q) = %v, want %v", tc.header, got, tc.want)
		}
	}
}

// TestEnsurePort checks the host:port defaulting used before every net.Dial.
func TestEnsurePort(t *testing.T) {
	cases := []struct {
		addr, def, want string
	}{
		{"example.com", "80", "example.com:80"},
		{"example.com:8080", "80", "example.com:8080"},
		{"example.com", "443", "example.com:443"},
		{"", "80", ""},
	}
	for _, tc := range cases {
		if got := ensurePort(tc.addr, tc.def); got != tc.want {
			t.Errorf("ensurePort(%q, %q) = %q, want %q", tc.addr, tc.def, got, tc.want)
		}
	}
}
