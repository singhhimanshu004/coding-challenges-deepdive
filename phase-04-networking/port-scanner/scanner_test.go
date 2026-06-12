package main

import (
	"net"
	"reflect"
	"testing"
	"time"
)

// startListener opens a TCP listener on an ephemeral 127.0.0.1 port and returns
// it together with the port the OS assigned. Using ":0" lets the kernel pick a
// free port, so the test never collides with whatever else is running on the
// box — the standard way to write hermetic network tests in Go.
func startListener(t *testing.T) (net.Listener, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not open listener: %v", err)
	}
	// Accept and immediately drop connections in the background so a connect
	// scan sees the port as OPEN (a completed handshake).
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			_ = conn.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	return ln, port
}

// freeClosedPort grabs an ephemeral port and then immediately closes its
// listener, leaving a port number that is (almost certainly) NOT accepting
// connections — a reliable "known closed" port for the test.
func freeClosedPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not open listener: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func TestScanPortOpen(t *testing.T) {
	ln, port := startListener(t)
	defer ln.Close()

	got := scanPort("127.0.0.1", port, time.Second)
	if !got.open {
		t.Fatalf("expected port %d to be OPEN, got closed", port)
	}
}

func TestScanPortClosed(t *testing.T) {
	port := freeClosedPort(t)

	got := scanPort("127.0.0.1", port, time.Second)
	if got.open {
		t.Fatalf("expected port %d to be CLOSED, got open", port)
	}
}

// TestScanReportsOpenAndClosed spins up two live listeners and mixes in two
// known-closed ports, then asserts the worker pool reports exactly the open
// ones (sorted) and none of the closed ones.
func TestScanReportsOpenAndClosed(t *testing.T) {
	ln1, openA := startListener(t)
	defer ln1.Close()
	ln2, openB := startListener(t)
	defer ln2.Close()

	closedA := freeClosedPort(t)
	closedB := freeClosedPort(t)

	ports := []int{openA, openB, closedA, closedB}
	open := scan("127.0.0.1", ports, 10, 500*time.Millisecond)

	gotPorts := make([]int, 0, len(open))
	for _, r := range open {
		gotPorts = append(gotPorts, r.port)
		if !r.open {
			t.Errorf("result for port %d has open=false in the open list", r.port)
		}
	}

	want := []int{openA, openB}
	if want[0] > want[1] {
		want[0], want[1] = want[1], want[0]
	}
	if !reflect.DeepEqual(gotPorts, want) {
		t.Fatalf("scan reported %v open, want %v (closed ports %d,%d must be excluded)",
			gotPorts, want, closedA, closedB)
	}
}

// TestScanResultsAreSorted verifies the worker pool returns results in ascending
// port order regardless of the order workers finish in.
func TestScanResultsAreSorted(t *testing.T) {
	var listeners []net.Listener
	var ports []int
	for i := 0; i < 5; i++ {
		ln, p := startListener(t)
		defer ln.Close()
		listeners = append(listeners, ln)
		ports = append(ports, p)
	}

	open := scan("127.0.0.1", ports, 3, 500*time.Millisecond)
	if len(open) != len(ports) {
		t.Fatalf("expected %d open ports, got %d", len(ports), len(open))
	}
	for i := 1; i < len(open); i++ {
		if open[i-1].port > open[i].port {
			t.Fatalf("results not sorted: %d came before %d", open[i-1].port, open[i].port)
		}
	}
}

// TestScanSingleWorker proves correctness is independent of pool size: with a
// single worker the channel is fully serialised, yet every open port is still
// found.
func TestScanSingleWorker(t *testing.T) {
	ln, port := startListener(t)
	defer ln.Close()
	closed := freeClosedPort(t)

	open := scan("127.0.0.1", []int{port, closed}, 1, 500*time.Millisecond)
	if len(open) != 1 || open[0].port != port {
		t.Fatalf("single-worker scan returned %v, want just %d open", open, port)
	}
}

func TestScanPortServiceName(t *testing.T) {
	// scanPort attaches the well-known service name for recognised ports. We
	// can't bind port 22 in a test, so verify the lookup helper directly.
	if got := serviceName(22); got != "ssh" {
		t.Errorf("serviceName(22) = %q, want ssh", got)
	}
	if got := serviceName(80); got != "http" {
		t.Errorf("serviceName(80) = %q, want http", got)
	}
	if got := serviceName(54321); got != "" {
		t.Errorf("serviceName(54321) = %q, want empty", got)
	}
}
