// Command nc is a from-scratch clone of netcat — the networking
// "Swiss-army knife." It wires a TCP or UDP socket to local stdin/stdout and
// relays bytes in both directions at the same time.
//
// Two modes:
//
//	nc <host> <port>      CONNECT: dial a TCP server, then relay
//	nc -l <port>          LISTEN:  accept ONE TCP connection, then relay
//
// Flags:
//
//	-l            listen mode (server) instead of connect mode (client)
//	-u            use UDP instead of TCP
//	-p PORT       port to use (alternative to the positional port)
//	-w SECONDS    I/O timeout: quit if the read side is idle this long
//	              (effectively REQUIRED for UDP, which never sees an EOF)
//
// Examples:
//
//	nc example.com 80           # connect to a TCP server
//	nc -l 9000                  # listen on TCP :9000 for one client
//	nc -u 127.0.0.1 9000        # send/receive UDP datagrams
//	nc -u -l -w 5 9000          # UDP listener, quit after 5s of silence
//
// Exit codes:
//
//	0  the relay finished cleanly (peer closed, or -w deadline reached)
//	1  runtime failure (dial/listen/accept/IO error)
//	2  usage error (bad flags or missing port)
package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// options is the parsed command line.
//
// 🐍 Think of this as a small @dataclass that holds the flag values. Grouping
// them in a struct keeps run()'s signature short and makes the parsing logic
// easy to unit-test in isolation.
type options struct {
	listen  bool
	udp     bool
	host    string
	port    string
	timeout time.Duration
}

// run is the real entry point. main() only calls run() and os.Exit() — every
// stream (stdin/stdout/stderr) is passed in as an interface, so a test can hand
// run() in-memory buffers and assert on the bytes with no subprocess at all.
//
// 🐍 In Python you'd patch sys.stdin/sys.stdout; here we just inject the
// io.Reader/io.Writer dependencies, which the compiler type-checks for us.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	opt, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "nc: %v\n", err)
		return 2
	}

	// "tcp" / "udp" are the network names net.Dial and net.Listen understand.
	network := "tcp"
	if opt.udp {
		network = "udp"
	}

	var relayErr error
	if opt.listen {
		// In listen mode we bind every local interface on the chosen port.
		addr := net.JoinHostPort("", opt.port)
		relayErr = listenAndRelay(network, addr, opt.timeout, stdin, stdout)
	} else {
		addr := net.JoinHostPort(opt.host, opt.port)
		relayErr = dialAndRelay(network, addr, opt.timeout, stdin, stdout)
	}
	if relayErr != nil {
		fmt.Fprintf(stderr, "nc: %v\n", relayErr)
		return 1
	}
	return 0
}

// parseArgs turns the raw argv slice into an options struct. We hand-roll the
// parser (instead of the stdlib flag package) so the flags can appear in any
// order and mix freely with the positional host/port, matching real netcat's
// ergonomics (e.g. both `nc -l 9000` and `nc -lu -w5 9000` feel natural).
func parseArgs(args []string) (options, error) {
	var opt options
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-l":
			opt.listen = true
		case arg == "-u":
			opt.udp = true
		case arg == "-p":
			i++
			if i >= len(args) {
				return opt, fmt.Errorf("-p requires a port")
			}
			opt.port = args[i]
		case arg == "-w":
			i++
			if i >= len(args) {
				return opt, fmt.Errorf("-w requires a number of seconds")
			}
			secs, err := strconv.Atoi(args[i])
			if err != nil || secs < 0 {
				return opt, fmt.Errorf("invalid -w value %q", args[i])
			}
			opt.timeout = time.Duration(secs) * time.Second
		case len(arg) > 0 && arg[0] == '-' && arg != "-":
			return opt, fmt.Errorf("unknown flag %q", arg)
		default:
			positional = append(positional, arg)
		}
	}

	// Sort the positional arguments into host/port depending on the mode.
	if opt.listen {
		// Listen mode takes only a port (host is implicitly "all interfaces").
		if opt.port == "" {
			if len(positional) != 1 {
				return opt, fmt.Errorf("listen mode needs a port: nc -l <port>")
			}
			opt.port = positional[0]
		}
	} else {
		// Connect mode needs host + port. The port may come from -p instead.
		switch {
		case opt.port != "" && len(positional) == 1:
			opt.host = positional[0]
		case len(positional) == 2:
			opt.host = positional[0]
			opt.port = positional[1]
		default:
			return opt, fmt.Errorf("connect mode needs host and port: nc <host> <port>")
		}
	}

	if opt.port == "" {
		return opt, fmt.Errorf("missing port")
	}
	return opt, nil
}
