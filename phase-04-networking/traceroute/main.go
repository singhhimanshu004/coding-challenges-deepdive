// Command traceroute discovers the network path to a host hop by hop. It works
// the same way the classic Unix tool does: send probes with a deliberately tiny
// IP TTL and listen for the ICMP "Time Exceeded" complaints that come back from
// each router along the way.
//
// The core trick — TTL expiry:
//
//	Every IP packet carries a Time-To-Live counter. Each router that forwards
//	the packet decrements it by one. When TTL hits 0, the router DROPS the
//	packet and sends an ICMP "Time Exceeded" message back to the sender — and
//	that message's SOURCE address is the router itself. So:
//	  TTL=1 → first router replies "time exceeded"  → that's hop 1
//	  TTL=2 → second router replies                  → that's hop 2
//	  ...
//	  TTL=N → destination finally answers our ping   → done.
//	By incrementing the TTL one at a time we coax each router on the path to
//	reveal itself in order.
//
// Usage:
//
//	traceroute [--max-hops 30] [--probes 3] [--timeout 1s] [--resolve] <host>
//
//	--max-hops N    stop after N hops if the destination isn't reached (def 30)
//	--probes N      probes sent per hop, for RTT sampling (def 3)
//	--timeout DUR   how long to wait for each reply, e.g. 1s, 500ms (def 1s)
//	--resolve       reverse-DNS each router IP to a hostname (slower)
//
// Privileges: this build uses UNPRIVILEGED ICMP datagram sockets
// (icmp.ListenPacket("udp4", ...)), which work without root on macOS and Linux.
// See the README for the raw-vs-datagram-socket discussion.
//
// Exit codes:
//
//	0  trace completed (or ran to max hops)
//	1  runtime error (e.g. could not open the socket / resolve the host)
//	2  usage error (bad flags or missing host)
package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// options is the parsed command line. (🐍 think of it as a small @dataclass of
// settings handed to run().)
type options struct {
	host    string
	maxHops int
	probes  int
	timeout time.Duration
	resolve bool
}

// run is the real entry point. Keeping main() trivial and passing the output
// streams in makes the program testable: a test can call run() with fake args
// and inspect what was written. (🐍 dependency injection, same reason you'd pass
// a file object instead of touching sys.stdout directly.)
func run(args []string, stdout, stderr *os.File) int {
	opt, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "traceroute: %v\n", err)
		fmt.Fprintf(stderr, "usage: traceroute [--max-hops 30] [--probes 3] [--timeout 1s] [--resolve] <host>\n")
		return 2
	}

	if err := trace(opt.host, opt.maxHops, opt.probes, opt.timeout, opt.resolve, stdout); err != nil {
		fmt.Fprintf(stderr, "traceroute: %v\n", err)
		return 1
	}
	return 0
}

// parseArgs is a small hand-rolled flag parser, matching the ergonomics of the
// other tools in this repo. The first non-flag argument is the host.
func parseArgs(args []string) (options, error) {
	opt := options{
		maxHops: 30,
		probes:  3,
		timeout: time.Second,
	}

	i := 0
	for i < len(args) {
		arg := args[i]
		switch arg {
		case "--max-hops", "-m":
			val, err := nextValue(args, &i, arg)
			if err != nil {
				return opt, err
			}
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				return opt, fmt.Errorf("invalid --max-hops %q: must be a positive integer", val)
			}
			opt.maxHops = n
		case "--probes", "-q":
			val, err := nextValue(args, &i, arg)
			if err != nil {
				return opt, err
			}
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 {
				return opt, fmt.Errorf("invalid --probes %q: must be a positive integer", val)
			}
			opt.probes = n
		case "--timeout", "-w":
			val, err := nextValue(args, &i, arg)
			if err != nil {
				return opt, err
			}
			d, err := time.ParseDuration(val)
			if err != nil || d <= 0 {
				return opt, fmt.Errorf("invalid --timeout %q: use a duration like 500ms, 1s, 2s", val)
			}
			opt.timeout = d
		case "--resolve", "-R":
			opt.resolve = true
		default:
			if len(arg) > 0 && arg[0] == '-' {
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
