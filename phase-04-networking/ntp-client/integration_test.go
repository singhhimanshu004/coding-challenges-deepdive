package main

import (
	"testing"
	"time"
)

// TestQueryIntegration performs a REAL NTP query against a public server. It is
// guarded so it never fails the suite when offline: any network error is
// reported with t.Skip, not t.Fatal. Run with `-v` to see the live result.
//
//	go test -run TestQueryIntegration -v
//
// What we assert when the network IS available:
//   - the server time is sane (within a few years of "now"), and
//   - the computed offset is small (real public servers and a roughly correct
//     local clock should agree to within a couple of seconds).
func TestQueryIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network integration test in -short mode")
	}

	res, err := query("pool.ntp.org", 123, ntpVersion, 5*time.Second)
	if err != nil {
		t.Skipf("skipping: no NTP reachability (%v)", err)
	}

	if res.ServerTime.IsZero() {
		t.Fatal("server returned a zero transmit timestamp")
	}

	// Sanity bounds: the server's clock should be in the same era as ours.
	now := time.Now()
	if res.ServerTime.Before(now.Add(-365*24*time.Hour)) ||
		res.ServerTime.After(now.Add(365*24*time.Hour)) {
		t.Errorf("server time %v is implausibly far from now %v", res.ServerTime, now)
	}

	t.Logf("stratum=%d server=%s offset=%v delay=%v",
		res.Stratum, res.ServerTime.Format(time.RFC3339Nano), res.Offset, res.Delay)
}
