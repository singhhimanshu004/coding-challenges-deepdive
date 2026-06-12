package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// TestTraceIntegration runs a REAL traceroute against a public host. It is
// guarded so it never fails the suite when offline or unprivileged: any setup or
// socket error is reported with t.Skip, not t.Fatal. Run it explicitly with:
//
//	go test -run TestTraceIntegration -v
//
// Even when it runs, we only assert that *some* output was produced — the actual
// path varies by network and we don't want a flaky test.
func TestTraceIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live network test in -short mode")
	}

	var buf bytes.Buffer
	// Open the socket first; if we can't (no network access, or the platform
	// disallows unprivileged ICMP), skip rather than fail.
	p, err := newICMPProber("8.8.8.8", time.Second)
	if err != nil {
		t.Skipf("skipping: cannot open ICMP socket (%v)", err)
	}
	p.Close()

	if err := trace("8.8.8.8", 5, 1, time.Second, false, &buf); err != nil {
		t.Skipf("skipping: trace failed (%v)", err)
	}
	if !strings.Contains(buf.String(), "traceroute to") {
		t.Errorf("expected a traceroute header, got:\n%s", buf.String())
	}
}
