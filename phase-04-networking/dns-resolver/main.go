// Command dns-resolver is a from-scratch DNS resolver. It speaks the DNS wire
// protocol over RAW UDP sockets (net.DialUDP) and hand-encodes/decodes every
// byte of the message — header, question, and resource records — instead of
// using net.Resolver/LookupHost.
//
// Usage:
//
//	dns-resolver <domain> [--type A] [--server 8.8.8.8] [--trace]
//
//	--type T      record type: A (default), AAAA, NS, CNAME, MX
//	--server IP   recursive resolver to query (default 8.8.8.8). A :port may be
//	              appended; 53 is assumed otherwise.
//	--trace       resolve ITERATIVELY from a root server, printing each referral
//	              hop down the delegation chain (ignores --server).
//
// Exit codes:
//
//	0  success (we got and printed an answer)
//	1  runtime failure (network error, SERVFAIL/NXDOMAIN, bad response)
//	2  usage error (bad flags or missing domain)
package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// options is the parsed command line. 🐍 think of it as a small @dataclass.
type options struct {
	domain string
	qtype  uint16
	server string
	trace  bool
}

// run is the real entry point. Passing the output streams in (instead of using
// os.Stdout directly) keeps the program testable, matching the pattern used by
// the other tools in this repo.
func run(args []string, stdout, stderr io.Writer) int {
	opt, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "dns-resolver: %v\n", err)
		return 2
	}

	var resp *Message
	if opt.trace {
		// Iterative mode: walk the delegation chain ourselves from the root.
		resp, err = resolveIterative(opt.domain, opt.qtype, func(line string) {
			fmt.Fprintf(stderr, ";; %s\n", line)
		})
	} else {
		// Default mode: let a recursive resolver do the work.
		resp, err = resolveRecursive(opt.server, opt.domain, opt.qtype)
	}
	if err != nil {
		fmt.Fprintf(stderr, "dns-resolver: %v\n", err)
		return 1
	}

	printResponse(stdout, opt, resp)
	return 0
}

// printResponse renders the answer in a compact, dig-like form.
func printResponse(w io.Writer, opt options, resp *Message) {
	fmt.Fprintf(w, ";; status: %s, id: %d\n", rcodeText(resp.Header.rcode()), resp.Header.ID)
	fmt.Fprintf(w, ";; QUESTION: %s %s\n", opt.domain, typeName(opt.qtype))

	if len(resp.Answers) == 0 {
		fmt.Fprintln(w, ";; (no answer records)")
		// In iterative mode the useful info may be in the authority section.
		for _, rr := range resp.Authority {
			fmt.Fprintf(w, ";; AUTHORITY %s\t%d\t%s\t%s\n", rr.Name, rr.TTL, typeName(rr.Type), rr.Data)
		}
		return
	}

	fmt.Fprintln(w, ";; ANSWER:")
	for _, rr := range resp.Answers {
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", rr.Name, rr.TTL, typeName(rr.Type), rr.Data)
	}
}

// parseArgs is a small hand-rolled flag parser (authentic CLI ergonomics, and
// consistent with the other challenges in this repo).
func parseArgs(args []string) (options, error) {
	opt := options{qtype: TypeA, server: "8.8.8.8"}

	i := 0
	next := func(flag string) (string, error) {
		i++
		if i >= len(args) {
			return "", fmt.Errorf("option %s requires a value", flag)
		}
		return args[i], nil
	}

	for ; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--type", "-t":
			v, err := next(a)
			if err != nil {
				return opt, err
			}
			t, err := parseType(v)
			if err != nil {
				return opt, err
			}
			opt.qtype = t
		case "--server", "-s":
			v, err := next(a)
			if err != nil {
				return opt, err
			}
			opt.server = v
		case "--trace":
			opt.trace = true
		default:
			if strings.HasPrefix(a, "-") {
				return opt, fmt.Errorf("unknown option %q", a)
			}
			if opt.domain != "" {
				return opt, fmt.Errorf("multiple domains given (%q and %q)", opt.domain, a)
			}
			opt.domain = a
		}
	}

	if opt.domain == "" {
		return opt, fmt.Errorf("no domain specified\nusage: dns-resolver <domain> [--type A] [--server 8.8.8.8] [--trace]")
	}

	// Default the resolver port to 53 if the user didn't give one.
	if !strings.Contains(opt.server, ":") {
		opt.server += ":53"
	}
	return opt, nil
}
