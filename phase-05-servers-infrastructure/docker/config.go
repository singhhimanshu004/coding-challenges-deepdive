package main

// config.go is PLATFORM-NEUTRAL. It holds the container configuration struct
// and all the CLI/argument parsing. None of this touches Linux syscalls, so it
// compiles everywhere and is fully unit-tested on macOS (see config_test.go).

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Config is the fully-resolved description of one container to launch.
//
// 🐍 A Go struct is like a Python @dataclass: a named bundle of typed fields.
// MemoryLimit is stored in BYTES (we parse "100m" into 104857600 here) so the
// rest of the program never has to think about units again.
type Config struct {
	MemoryLimit int64    // bytes; 0 means "no limit"
	PidsLimit   int      // max processes; 0 means "no limit"
	Hostname    string   // hostname seen inside the container
	RootFS      string   // directory to become the container's "/"
	Command     string   // program to run inside the container
	Args        []string // arguments to that program
}

// parseRunArgs parses the user-facing `run` form:
//
//	run [--mem 100m] [--pids 50] [--hostname web] <rootfs> <cmd> [args...]
//
// We use the standard library `flag` package. A subtlety worth knowing: `flag`
// stops parsing at the FIRST non-flag argument. That is exactly what we want —
// everything after <rootfs> belongs to the container command, even if it looks
// like a flag (e.g. `/bin/ls -la` — the `-la` is the container's, not ours).
func parseRunArgs(args []string, stderr io.Writer) (*Config, error) {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)

	mem := fs.String("mem", "", "memory limit, e.g. 100m, 512k, 1g (0/empty = unlimited)")
	pids := fs.Int("pids", 0, "max number of processes (0 = unlimited)")
	hostname := fs.String("hostname", "container", "hostname inside the container")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	rest := fs.Args()
	if len(rest) < 2 {
		return nil, errors.New("usage: gocker run [--mem 100m] [--pids 50] [--hostname name] <rootfs-dir> <cmd> [args...]")
	}

	memBytes, err := parseSize(*mem)
	if err != nil {
		return nil, fmt.Errorf("invalid --mem: %w", err)
	}
	if *pids < 0 {
		return nil, errors.New("--pids must be >= 0")
	}

	return &Config{
		MemoryLimit: memBytes,
		PidsLimit:   *pids,
		Hostname:    *hostname,
		RootFS:      rest[0],
		Command:     rest[1],
		Args:        rest[2:],
	}, nil
}

// parseChildArgs parses the INTERNAL `child` form that gocker re-executes on
// itself (see the re-exec trick in main.go). The key difference from the user
// form is that --membytes is already an integer number of bytes — we don't
// re-parse "100m" in the child, we pass the resolved value straight through.
// This keeps the parent and child perfectly in sync.
func parseChildArgs(args []string, stderr io.Writer) (*Config, error) {
	fs := flag.NewFlagSet("child", flag.ContinueOnError)
	fs.SetOutput(stderr)

	memBytes := fs.Int64("membytes", 0, "memory limit in bytes")
	pids := fs.Int("pids", 0, "max number of processes")
	hostname := fs.String("hostname", "container", "hostname inside the container")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	rest := fs.Args()
	if len(rest) < 2 {
		return nil, errors.New("internal error: child mode requires <rootfs> <cmd>")
	}

	return &Config{
		MemoryLimit: *memBytes,
		PidsLimit:   *pids,
		Hostname:    *hostname,
		RootFS:      rest[0],
		Command:     rest[1],
		Args:        rest[2:],
	}, nil
}

// childArgs serialises a Config back into the argv used to re-exec ourselves as
// `/proc/self/exe child ...`. parseChildArgs(childArgs(cfg)) must round-trip to
// the same Config — config_test.go asserts exactly that.
func childArgs(cfg *Config) []string {
	out := []string{
		"child",
		"--membytes", strconv.FormatInt(cfg.MemoryLimit, 10),
		"--pids", strconv.Itoa(cfg.PidsLimit),
		"--hostname", cfg.Hostname,
		cfg.RootFS,
		cfg.Command,
	}
	return append(out, cfg.Args...)
}

// parseSize converts a human size string ("100m", "512k", "1g", "2048") into a
// byte count. Empty string or "0" means "unlimited" (0).
//
// 🐍 Go has no try/except: ParseInt returns (value, error) and we forward the
// error up with %w so callers can inspect the cause. The suffix is binary
// (k=1024, m=1024², g=1024³) to match how cgroup limits are usually reasoned
// about.
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0, nil
	}

	mult := int64(1)
	num := s
	switch s[len(s)-1] {
	case 'k', 'K':
		mult, num = 1<<10, s[:len(s)-1]
	case 'm', 'M':
		mult, num = 1<<20, s[:len(s)-1]
	case 'g', 'G':
		mult, num = 1<<30, s[:len(s)-1]
	case 'b', 'B':
		num = s[:len(s)-1]
	}

	n, err := strconv.ParseInt(strings.TrimSpace(num), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("not a valid size: %q", s)
	}
	if n < 0 {
		return 0, fmt.Errorf("size must be >= 0: %q", s)
	}
	return n * mult, nil
}
