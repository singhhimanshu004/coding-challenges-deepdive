package main

import (
	"testing"
	"time"
)

// TestParseArgsDefaults confirms the documented defaults.
func TestParseArgsDefaults(t *testing.T) {
	opt, err := parseArgs([]string{"example.com"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if opt.host != "example.com" {
		t.Errorf("host = %q, want example.com", opt.host)
	}
	if opt.maxHops != 30 {
		t.Errorf("maxHops = %d, want 30", opt.maxHops)
	}
	if opt.probes != 3 {
		t.Errorf("probes = %d, want 3", opt.probes)
	}
	if opt.timeout != time.Second {
		t.Errorf("timeout = %v, want 1s", opt.timeout)
	}
	if opt.resolve {
		t.Errorf("resolve should default to false")
	}
}

func TestParseArgsFlags(t *testing.T) {
	opt, err := parseArgs([]string{"--max-hops", "10", "--probes", "5", "--timeout", "500ms", "--resolve", "1.1.1.1"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if opt.maxHops != 10 || opt.probes != 5 || opt.timeout != 500*time.Millisecond || !opt.resolve || opt.host != "1.1.1.1" {
		t.Errorf("unexpected parse result: %+v", opt)
	}
}

func TestParseArgsErrors(t *testing.T) {
	cases := [][]string{
		{},                         // missing host
		{"--max-hops", "0", "h"},   // non-positive
		{"--probes", "x", "h"},     // not a number
		{"--timeout", "nope", "h"}, // bad duration
		{"--bogus", "h"},           // unknown flag
		{"a", "b"},                 // extra positional
		{"--timeout"},              // flag without value
	}
	for _, args := range cases {
		if _, err := parseArgs(args); err == nil {
			t.Errorf("parseArgs(%v) = nil error, want error", args)
		}
	}
}
