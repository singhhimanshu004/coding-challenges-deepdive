// Command port-scanner is a concurrent TCP port scanner built around a worker
// pool of goroutines. It performs "connect scans": for each port it attempts a
// full TCP handshake with net.DialTimeout; a successful connect means the port
// is OPEN, an error/timeout means closed or filtered.
//
// Usage:
//
//	port-scanner <host> [--ports SPEC] [--workers N] [--timeout DUR]
//
//	--ports SPEC     ports to scan. SPEC is one of:
//	                   a range   "1-1024"
//	                   a list    "22,80,443"
//	                   a single  "8080"
//	                 (default: 1-1024)
//	--workers N      number of concurrent probes in flight (default: 100)
//	--timeout DUR    per-connection timeout, e.g. 500ms, 1s, 2s (default: 1s)
//
// Examples:
//
//	port-scanner scanme.nmap.org
//	port-scanner 127.0.0.1 --ports 22,80,443
//	port-scanner 10.0.0.5 --ports 1-65535 --workers 500 --timeout 750ms
//
// Note on scan type: this is a CONNECT scan, which needs no special privileges.
// A SYN ("half-open") scan — sending a raw SYN and never completing the
// handshake — is stealthier and faster but requires crafting raw packets, which
// needs root/CAP_NET_RAW. That is deliberately out of scope here; we use the
// portable, unprivileged net.Dial path.
//
// Exit codes:
//
//	0  scan completed
//	2  usage error (bad flags or missing host)
package main

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// options is the parsed command line. Grouping flags in a struct keeps run()'s
// signature tidy (🐍 think of it as a small @dataclass of settings).
type options struct {
	host    string
	ports   []int
	workers int
	timeout time.Duration
}

// run is the real entry point. Keeping main() trivial and passing the output
// streams in makes the whole program testable: a test can call run() with fake
// args and assert on the bytes written. (🐍 dependency injection — same reason
// you'd pass a file object into a function instead of touching sys.stdout.)
func run(args []string, stdout, stderr *os.File) int {
	opt, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "port-scanner: %v\n", err)
		fmt.Fprintf(stderr, "usage: port-scanner <host> [--ports 1-1024|22,80,443] [--workers 100] [--timeout 1s]\n")
		return 2
	}

	fmt.Fprintf(stdout, "Scanning %s — %d port(s), %d workers, %s timeout\n",
		opt.host, len(opt.ports), opt.workers, opt.timeout)

	start := time.Now()
	open := scan(opt.host, opt.ports, opt.workers, opt.timeout)
	elapsed := time.Since(start)

	if len(open) == 0 {
		fmt.Fprintf(stdout, "No open ports found.\n")
	} else {
		fmt.Fprintf(stdout, "\nPORT\tSTATE\tSERVICE\n")
		for _, r := range open {
			fmt.Fprintf(stdout, "%d\topen\t%s\n", r.port, r.service)
		}
	}
	fmt.Fprintf(stdout, "\n%d open port(s) found in %s\n", len(open), elapsed.Round(time.Millisecond))
	return 0
}

// parseArgs is a small hand-rolled flag parser (matching the ergonomics of the
// other tools in this repo). The first non-flag argument is the host.
func parseArgs(args []string) (options, error) {
	opt := options{
		workers: 100,
		timeout: time.Second,
	}
	var portSpec string

	i := 0
	for i < len(args) {
		arg := args[i]
		switch arg {
		case "--ports", "-p":
			val, err := nextValue(args, &i, arg)
			if err != nil {
				return opt, err
			}
			portSpec = val
		case "--workers", "-w":
			val, err := nextValue(args, &i, arg)
			if err != nil {
				return opt, err
			}
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				return opt, fmt.Errorf("invalid --workers %q: must be a positive integer", val)
			}
			opt.workers = n
		case "--timeout", "-t":
			val, err := nextValue(args, &i, arg)
			if err != nil {
				return opt, err
			}
			d, err := time.ParseDuration(val)
			if err != nil || d <= 0 {
				return opt, fmt.Errorf("invalid --timeout %q: use a duration like 500ms, 1s, 2s", val)
			}
			opt.timeout = d
		default:
			if strings.HasPrefix(arg, "-") {
				return opt, fmt.Errorf("unknown flag %q", arg)
			}
			if opt.host != "" {
				return opt, fmt.Errorf("unexpected extra argument %q", arg)
			}
			opt.host = arg
		}
		i++
	}

	if opt.host == "" {
		return opt, fmt.Errorf("missing host")
	}

	if portSpec == "" {
		portSpec = "1-1024"
	}
	ports, err := parsePorts(portSpec)
	if err != nil {
		return opt, err
	}
	opt.ports = ports
	return opt, nil
}

// nextValue consumes the value following a flag, advancing the index.
func nextValue(args []string, i *int, flag string) (string, error) {
	if *i+1 >= len(args) {
		return "", fmt.Errorf("flag %s needs a value", flag)
	}
	*i++
	return args[*i], nil
}

// parsePorts turns a port SPEC into a sorted, de-duplicated slice of ports.
// Accepted forms (which may be mixed with commas, e.g. "22,80,8000-8010"):
//
//	"1-1024"        an inclusive range
//	"22,80,443"     a comma-separated list
//	"8080"          a single port
func parsePorts(spec string) ([]int, error) {
	set := make(map[int]struct{})

	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if lo, hi, ok := strings.Cut(part, "-"); ok {
			start, err1 := strconv.Atoi(strings.TrimSpace(lo))
			end, err2 := strconv.Atoi(strings.TrimSpace(hi))
			if err1 != nil || err2 != nil {
				return nil, fmt.Errorf("invalid port range %q", part)
			}
			if start > end {
				start, end = end, start
			}
			if !validPort(start) || !validPort(end) {
				return nil, fmt.Errorf("port range %q out of bounds (1-65535)", part)
			}
			for p := start; p <= end; p++ {
				set[p] = struct{}{}
			}
		} else {
			p, err := strconv.Atoi(part)
			if err != nil || !validPort(p) {
				return nil, fmt.Errorf("invalid port %q (must be 1-65535)", part)
			}
			set[p] = struct{}{}
		}
	}

	if len(set) == 0 {
		return nil, fmt.Errorf("no ports to scan")
	}

	ports := make([]int, 0, len(set))
	for p := range set {
		ports = append(ports, p)
	}
	sort.Ints(ports)
	return ports, nil
}

func validPort(p int) bool { return p >= 1 && p <= 65535 }
