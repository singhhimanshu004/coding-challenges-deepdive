package main

import (
	"bytes"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// TestRelayTCP drives the core relay over a REAL loopback TCP connection in
// both directions: we send "ping" up, the server echoes "pong" back, then the
// server closes, which must make relay return.
func TestRelayTCP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0") // :0 → kernel picks a free port
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	gotFromClient := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		data, _ := io.ReadAll(conn) // read until the client half-closes (EOF)
		gotFromClient <- string(data)
		conn.Write([]byte("pong"))
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	var stdout bytes.Buffer
	if err := relay(client, strings.NewReader("ping"), &stdout); err != nil {
		t.Fatalf("relay: %v", err)
	}

	if got := <-gotFromClient; got != "ping" {
		t.Errorf("server received %q, want %q", got, "ping")
	}
	if got := stdout.String(); got != "pong" {
		t.Errorf("stdout = %q, want %q", got, "pong")
	}
}

// TestServeTCP exercises LISTEN mode: serveTCP accepts a connection and relays
// its injected stdin to the client while copying the client's bytes to stdout.
func TestServeTCP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	var serverStdout bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- serveTCP(ln, 0, strings.NewReader("server-says-hi"), &serverStdout)
	}()

	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := client.Write([]byte("client-says-hello")); err != nil {
		t.Fatalf("client write: %v", err)
	}
	// Tell the server we're done sending so its conn→stdout copy hits EOF.
	client.(*net.TCPConn).CloseWrite()

	fromServer, _ := io.ReadAll(client) // server's stdin → us, until it closes
	client.Close()

	if err := <-done; err != nil {
		t.Fatalf("serveTCP: %v", err)
	}
	if got := serverStdout.String(); got != "client-says-hello" {
		t.Errorf("server stdout = %q, want %q", got, "client-says-hello")
	}
	if string(fromServer) != "server-says-hi" {
		t.Errorf("client received %q, want %q", fromServer, "server-says-hi")
	}
}

// TestDialAndRelayUDP covers the bidirectional UDP path through CONNECT mode
// against an in-process echo server. UDP never produces EOF, so we rely on the
// -w deadline to end the relay after the echo arrives.
func TestDialAndRelayUDP(t *testing.T) {
	pc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	defer pc.Close()

	go func() {
		buf := make([]byte, 1024)
		n, addr, err := pc.ReadFromUDP(buf)
		if err != nil {
			return
		}
		pc.WriteToUDP(buf[:n], addr) // echo it straight back
	}()

	var stdout bytes.Buffer
	err = dialAndRelay("udp", pc.LocalAddr().String(), 500*time.Millisecond,
		strings.NewReader("ping"), &stdout)
	if err != nil {
		t.Fatalf("dialAndRelay: %v", err)
	}
	if got := stdout.String(); got != "ping" {
		t.Errorf("stdout = %q, want %q (UDP echo)", got, "ping")
	}
}

// TestServePacketInbound covers connectionless LISTEN mode: a datagram sent to
// our bound UDP socket must be relayed to stdout. Local input is empty, so the
// relay's send side finishes immediately and we just verify inbound delivery
// before the -w deadline ends the relay.
func TestServePacketInbound(t *testing.T) {
	pc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	defer pc.Close()

	var stdout bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- servePacket(pc, 500*time.Millisecond, strings.NewReader(""), &stdout)
	}()

	client, err := net.Dial("udp", pc.LocalAddr().String())
	if err != nil {
		t.Fatalf("dial udp: %v", err)
	}
	defer client.Close()
	if _, err := client.Write([]byte("datagram")); err != nil {
		t.Fatalf("client write: %v", err)
	}

	if err := <-done; err != nil {
		t.Fatalf("servePacket: %v", err)
	}
	if got := stdout.String(); got != "datagram" {
		t.Errorf("stdout = %q, want %q", got, "datagram")
	}
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    options
		wantErr bool
	}{
		{
			name: "connect host port",
			args: []string{"example.com", "80"},
			want: options{host: "example.com", port: "80"},
		},
		{
			name: "listen with positional port",
			args: []string{"-l", "9000"},
			want: options{listen: true, port: "9000"},
		},
		{
			name: "udp listen with -p and -w",
			args: []string{"-u", "-l", "-p", "9000", "-w", "5"},
			want: options{udp: true, listen: true, port: "9000", timeout: 5 * time.Second},
		},
		{
			name: "connect with -p",
			args: []string{"-p", "443", "host.test"},
			want: options{host: "host.test", port: "443"},
		},
		{name: "missing port in connect", args: []string{"justhost"}, wantErr: true},
		{name: "unknown flag", args: []string{"-z", "host", "80"}, wantErr: true},
		{name: "bad -w value", args: []string{"-w", "abc", "host", "80"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("parseArgs(%v) = %+v, want %+v", tt.args, got, tt.want)
			}
		})
	}
}

// TestRunConnectTCP exercises the full run() entry point end-to-end against an
// in-process echo server, asserting the exit code and the relayed bytes.
func TestRunConnectTCP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		data, _ := io.ReadAll(conn)
		conn.Write(data) // echo
	}()

	_, port, _ := net.SplitHostPort(ln.Addr().String())
	var stdout, stderr bytes.Buffer
	code := run([]string{"127.0.0.1", port}, strings.NewReader("hello"), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run exit = %d, stderr = %q", code, stderr.String())
	}
	if got := stdout.String(); got != "hello" {
		t.Errorf("stdout = %q, want %q", got, "hello")
	}
}
