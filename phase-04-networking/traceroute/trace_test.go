package main

import (
	"strings"
	"testing"
	"time"
)

// fakeProber is a scripted prober used to drive runTrace with no network.
//
// 🐍➡️🐹 Because `prober` is an interface, any type with a matching `probe`
// method satisfies it — we don't declare "implements prober" anywhere. This fake
// returns canned results per call, letting us simulate an entire path.
type fakeProber struct {
	// script maps a probe sequence number to the result it should return. We key
	// on seq (which runTrace bumps monotonically: 1,2,3,...) so each probe across
	// all hops gets an independent scripted answer.
	results []probeResult
	calls   int
}

func (f *fakeProber) probe(ttl, seq int) probeResult {
	r := f.results[f.calls]
	f.calls++
	return r
}

// TestRunTraceStopsAtDestination simulates a 3-hop path where the third hop is
// the destination (echo reply). With 1 probe per hop we expect exactly 3 hops
// and a stop, never reaching maxHops.
func TestRunTraceStopsAtDestination(t *testing.T) {
	f := &fakeProber{results: []probeResult{
		{from: ipAddr("10.0.0.1"), rtt: time.Millisecond, kind: replyTimeExceeded},
		{from: ipAddr("10.0.0.2"), rtt: 2 * time.Millisecond, kind: replyTimeExceeded},
		{from: ipAddr("93.184.216.34"), rtt: 3 * time.Millisecond, kind: replyEchoReply},
	}}

	hops := runTrace(f, 30, 1, nil)
	if len(hops) != 3 {
		t.Fatalf("got %d hops, want 3 (should stop at destination)", len(hops))
	}
	if !hops[2].reachedDest() {
		t.Errorf("final hop should be the destination")
	}
	if f.calls != 3 {
		t.Errorf("prober called %d times, want 3", f.calls)
	}
}

// TestRunTraceTimeoutThenStar checks that a hop where every probe times out is
// still recorded (it shows as "*"), and that the trace continues past it.
func TestRunTraceTimeoutThenStar(t *testing.T) {
	f := &fakeProber{results: []probeResult{
		// hop 1: two timeouts
		{timeout: true},
		{timeout: true},
		// hop 2: destination answers on the first probe, second also answers
		{from: ipAddr("8.8.8.8"), rtt: 5 * time.Millisecond, kind: replyEchoReply},
		{from: ipAddr("8.8.8.8"), rtt: 6 * time.Millisecond, kind: replyEchoReply},
	}}

	hops := runTrace(f, 30, 2, nil)
	if len(hops) != 2 {
		t.Fatalf("got %d hops, want 2", len(hops))
	}
	if hops[0].reachedDest() {
		t.Errorf("hop 1 should not reach destination")
	}
	if !hops[1].reachedDest() {
		t.Errorf("hop 2 should reach destination")
	}
}

// TestRunTraceRespectsMaxHops ensures that if the destination never answers, we
// stop after exactly maxHops and don't loop forever. Every probe times out.
func TestRunTraceRespectsMaxHops(t *testing.T) {
	const maxHops = 5
	const probes = 3
	results := make([]probeResult, maxHops*probes)
	for i := range results {
		results[i] = probeResult{timeout: true}
	}
	f := &fakeProber{results: results}

	hops := runTrace(f, maxHops, probes, nil)
	if len(hops) != maxHops {
		t.Fatalf("got %d hops, want %d", len(hops), maxHops)
	}
	if f.calls != maxHops*probes {
		t.Errorf("prober called %d times, want %d", f.calls, maxHops*probes)
	}
}

// TestFormatHop verifies the rendered line: hop number, the address shown once
// for repeated same-source probes, RTTs, and "*" for timeouts. resolve=false to
// avoid DNS in tests.
func TestFormatHop(t *testing.T) {
	h := hop{
		ttl: 3,
		results: []probeResult{
			{from: ipAddr("10.0.0.1"), rtt: 1200 * time.Microsecond, kind: replyTimeExceeded},
			{timeout: true},
			{from: ipAddr("10.0.0.1"), rtt: 1300 * time.Microsecond, kind: replyTimeExceeded},
		},
	}

	got := formatHop(h, false)

	if !strings.HasPrefix(got, " 3 ") {
		t.Errorf("line %q should start with hop number ' 3 '", got)
	}
	if !strings.Contains(got, "10.0.0.1") {
		t.Errorf("line %q should contain the router IP", got)
	}
	if strings.Count(got, "10.0.0.1") != 1 {
		t.Errorf("repeated same-source address should appear once; line = %q", got)
	}
	if !strings.Contains(got, "*") {
		t.Errorf("timed-out probe should render as '*'; line = %q", got)
	}
	if !strings.Contains(got, "ms") {
		t.Errorf("line %q should contain millisecond RTTs", got)
	}
}

// TestFormatHopAllTimeouts renders a hop where nothing answered.
func TestFormatHopAllTimeouts(t *testing.T) {
	h := hop{ttl: 7, results: []probeResult{{timeout: true}, {timeout: true}, {timeout: true}}}
	got := formatHop(h, false)
	if strings.Count(got, "*") != 3 {
		t.Errorf("want three '*'; line = %q", got)
	}
}
