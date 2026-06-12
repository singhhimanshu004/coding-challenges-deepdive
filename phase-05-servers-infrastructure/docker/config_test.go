package main

// config_test.go is PLATFORM-NEUTRAL (no build tag), so these tests run on
// macOS, Linux, and Windows alike. They cover CLI/arg parsing and the
// parent→child argv round-trip — exactly the logic that does NOT depend on any
// Linux syscall.

import (
	"bytes"
	"reflect"
	"testing"
)

func TestParseSize(t *testing.T) {
	cases := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"", 0, false},
		{"0", 0, false},
		{"512", 512, false},
		{"512b", 512, false},
		{"1k", 1 << 10, false},
		{"1K", 1 << 10, false},
		{"100m", 100 << 20, false},
		{"100M", 100 << 20, false},
		{"2g", 2 << 30, false},
		{"  256m  ", 256 << 20, false},
		{"abc", 0, true},
		{"-5m", 0, true},
		{"1.5g", 0, true},
	}
	for _, c := range cases {
		got, err := parseSize(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseSize(%q): expected error, got %d", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseSize(%q): unexpected error %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseRunArgs(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseRunArgs(
		[]string{"--mem", "100m", "--pids", "50", "--hostname", "web", "/rootfs", "/bin/ls", "-la", "/etc"},
		&stderr,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MemoryLimit != 100<<20 {
		t.Errorf("MemoryLimit = %d, want %d", cfg.MemoryLimit, 100<<20)
	}
	if cfg.PidsLimit != 50 {
		t.Errorf("PidsLimit = %d, want 50", cfg.PidsLimit)
	}
	if cfg.Hostname != "web" {
		t.Errorf("Hostname = %q, want web", cfg.Hostname)
	}
	if cfg.RootFS != "/rootfs" || cfg.Command != "/bin/ls" {
		t.Errorf("RootFS/Command = %q/%q", cfg.RootFS, cfg.Command)
	}
	// Flags AFTER the command must belong to the command, not to gocker.
	if !reflect.DeepEqual(cfg.Args, []string{"-la", "/etc"}) {
		t.Errorf("Args = %v, want [-la /etc]", cfg.Args)
	}
}

func TestParseRunArgsDefaults(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseRunArgs([]string{"/rootfs", "/bin/sh"}, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MemoryLimit != 0 || cfg.PidsLimit != 0 {
		t.Errorf("expected unlimited defaults, got mem=%d pids=%d", cfg.MemoryLimit, cfg.PidsLimit)
	}
	if cfg.Hostname != "container" {
		t.Errorf("default Hostname = %q, want container", cfg.Hostname)
	}
	if len(cfg.Args) != 0 {
		t.Errorf("Args = %v, want empty", cfg.Args)
	}
}

func TestParseRunArgsTooFew(t *testing.T) {
	var stderr bytes.Buffer
	if _, err := parseRunArgs([]string{"/rootfs"}, &stderr); err == nil {
		t.Error("expected error when command is missing")
	}
}

// TestChildArgsRoundTrip is the important one: it proves the re-exec serialises
// a Config and parses it back identically. If this breaks, containers would be
// launched with the wrong limits/command.
func TestChildArgsRoundTrip(t *testing.T) {
	orig := &Config{
		MemoryLimit: 100 << 20,
		PidsLimit:   25,
		Hostname:    "demo",
		RootFS:      "/img/root",
		Command:     "/bin/sh",
		Args:        []string{"-c", "echo hi"},
	}

	args := childArgs(orig)
	if args[0] != "child" {
		t.Fatalf("childArgs[0] = %q, want child", args[0])
	}

	var stderr bytes.Buffer
	// parseChildArgs receives everything AFTER the "child" token.
	got, err := parseChildArgs(args[1:], &stderr)
	if err != nil {
		t.Fatalf("parseChildArgs error: %v", err)
	}
	if !reflect.DeepEqual(orig, got) {
		t.Errorf("round-trip mismatch:\n orig = %+v\n got  = %+v", orig, got)
	}
}

func TestDispatchUnknownCommand(t *testing.T) {
	var out, errBuf bytes.Buffer
	if err := dispatch([]string{"bogus"}, &out, &errBuf); err == nil {
		t.Error("expected error for unknown command")
	}
}

func TestDispatchHelp(t *testing.T) {
	var out, errBuf bytes.Buffer
	if err := dispatch([]string{"--help"}, &out, &errBuf); err != nil {
		t.Errorf("help should not error: %v", err)
	}
	if out.Len() == 0 {
		t.Error("expected usage text on stdout")
	}
}
