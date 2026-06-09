// Command curl is a from-scratch HTTP/1.1 client built directly on top of raw
// TCP sockets (and crypto/tls for https). It does NOT use net/http for the
// protocol work: it opens a socket with net.Dial, writes the HTTP request bytes
// by hand, and parses the response bytes by hand — including Content-Length and
// chunked Transfer-Encoding decoding.
//
// Usage:
//
//	curl [options] URL
//
//	-X METHOD        request method (default GET, or POST when -d is given)
//	-H 'Name: val'   add a request header (repeatable)
//	-d DATA          send DATA as the request body (implies POST + Content-Length)
//	-o FILE          write the response body to FILE instead of stdout
//	-I               HEAD request: fetch headers only, print them
//	-v               verbose: print the request and response headers (to stderr)
//	-L               follow 3xx redirects (Location header)
//
// Exit codes:
//
//	0  success (2xx, or any completed exchange we could print)
//	1  runtime failure (DNS/connect/TLS/IO/protocol error)
//	2  usage error (bad flags or missing URL)
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// options is the parsed command line. Grouping flags in a struct keeps run()'s
// signature tidy (🐍 think of it as a small @dataclass of settings).
type options struct {
	method      string
	url         string
	headers     []header
	data        string
	hasData     bool
	outFile     string
	verbose     bool
	headOnly    bool
	followRedir bool
}

const maxRedirects = 10

// run is the real entry point. Keeping main() trivial and passing the output
// streams in makes the whole program testable: a test can call run() with fake
// args and assert on bytes.Buffers, no subprocess needed.
func run(args []string, stdout, stderr *os.File) int {
	opt, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "curl: %v\n", err)
		return 2
	}

	t, err := parseTarget(opt.url)
	if err != nil {
		fmt.Fprintf(stderr, "curl: %v\n", err)
		return 1
	}

	// Decide the method. -I forces HEAD; -d defaults to POST; else GET.
	method := opt.method
	if method == "" {
		if opt.headOnly {
			method = "HEAD"
		} else if opt.hasData {
			method = "POST"
		} else {
			method = "GET"
		}
	}

	var body []byte
	if opt.hasData {
		body = []byte(opt.data)
	}

	// Redirect loop. Without -L we make exactly one request.
	for redirects := 0; ; redirects++ {
		resp, err := doRequest(t, method, opt, body, stderr)
		if err != nil {
			fmt.Fprintf(stderr, "curl: %v\n", err)
			return 1
		}

		// Follow a redirect if asked and we haven't looped too many times.
		if opt.followRedir && resp.isRedirect() {
			if redirects >= maxRedirects {
				fmt.Fprintf(stderr, "curl: too many redirects (>%d)\n", maxRedirects)
				return 1
			}
			next, err := resolveLocation(t, resp.get("Location"))
			if err != nil {
				fmt.Fprintf(stderr, "curl: %v\n", err)
				return 1
			}
			if opt.verbose {
				fmt.Fprintf(stderr, "* Following redirect to %s\n", next.scheme+"://"+next.authority()+next.path)
			}
			// Per RFC 7231: 303 (and, as curl does, 301/302) turn the follow-up
			// into a bodyless GET; 307/308 preserve method and body.
			if resp.statusCode != 307 && resp.statusCode != 308 {
				method = "GET"
				body = nil
			}
			t = next
			continue
		}

		// Terminal response: emit it.
		return emit(resp, opt, stdout, stderr)
	}
}

// doRequest performs ONE request/response exchange against t and returns the
// parsed response. It owns the socket lifecycle (dial → write → parse → close).
func doRequest(t target, method string, opt options, body []byte, stderr *os.File) (*response, error) {
	conn, err := dial(t)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	req := request{method: method, target: t, headers: opt.headers, body: body}
	raw := req.frame()

	// -v prints what we're about to send, prefixed with ">" like real curl.
	if opt.verbose {
		printVerbose(stderr, raw, ">")
	}

	if _, err := conn.Write(raw); err != nil {
		return nil, fmt.Errorf("writing request: %w", err)
	}

	// Wrap the socket in a buffered reader so the parser can read line by line
	// and do precise ReadFull calls without lots of tiny syscalls.
	br := bufio.NewReader(conn)
	resp, err := parseResponse(br, method == "HEAD")
	if err != nil {
		return nil, err
	}

	// -v prints the response status + headers, prefixed with "<".
	if opt.verbose {
		fmt.Fprintf(stderr, "< %s\n", resp.statusLine())
		for _, h := range resp.headers {
			fmt.Fprintf(stderr, "< %s: %s\n", h.name, h.value)
		}
		fmt.Fprintln(stderr, "<")
	}

	return resp, nil
}

// emit writes the final response to the right place and returns the exit code.
func emit(resp *response, opt options, stdout, stderr *os.File) int {
	// -I (HEAD) prints the status line + headers to stdout and stops.
	if opt.headOnly {
		fmt.Fprintf(stdout, "%s\r\n", resp.statusLine())
		for _, h := range resp.headers {
			fmt.Fprintf(stdout, "%s: %s\r\n", h.name, h.value)
		}
		fmt.Fprint(stdout, "\r\n")
		return 0
	}

	// -o FILE sends the body to a file; otherwise to stdout.
	if opt.outFile != "" {
		if err := os.WriteFile(opt.outFile, resp.body, 0o644); err != nil {
			fmt.Fprintf(stderr, "curl: writing %s: %v\n", opt.outFile, err)
			return 1
		}
	} else {
		stdout.Write(resp.body)
	}
	return 0
}

// printVerbose prints a raw request blob line by line with a prefix, stopping at
// the blank line so we never dump a (possibly binary) body to the terminal.
func printVerbose(w *os.File, raw []byte, prefix string) {
	text := string(raw)
	if i := strings.Index(text, crlf+crlf); i >= 0 {
		text = text[:i]
	}
	for _, line := range strings.Split(text, crlf) {
		fmt.Fprintf(w, "%s %s\n", prefix, line)
	}
	fmt.Fprintf(w, "%s\n", prefix)
}

// parseArgs is a small hand-rolled flag parser. We avoid stdlib `flag` because,
// like the other tools in this repo, we want authentic curl ergonomics: short
// flags that take the next arg, a repeatable -H, and a single positional URL.
func parseArgs(args []string) (options, error) {
	var opt options

	// next() pulls the value that follows a flag like -X / -H / -d / -o.
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
		case "-X", "--request":
			v, err := next(a)
			if err != nil {
				return opt, err
			}
			opt.method = strings.ToUpper(v)
		case "-H", "--header":
			v, err := next(a)
			if err != nil {
				return opt, err
			}
			h, err := parseHeaderArg(v)
			if err != nil {
				return opt, err
			}
			opt.headers = append(opt.headers, h)
		case "-d", "--data":
			v, err := next(a)
			if err != nil {
				return opt, err
			}
			opt.data = v
			opt.hasData = true
		case "-o", "--output":
			v, err := next(a)
			if err != nil {
				return opt, err
			}
			opt.outFile = v
		case "-I", "--head":
			opt.headOnly = true
		case "-v", "--verbose":
			opt.verbose = true
		case "-L", "--location":
			opt.followRedir = true
		default:
			if strings.HasPrefix(a, "-") && a != "-" {
				return opt, fmt.Errorf("unknown option %q", a)
			}
			if opt.url != "" {
				return opt, fmt.Errorf("multiple URLs given (%q and %q)", opt.url, a)
			}
			opt.url = a
		}
	}

	if opt.url == "" {
		return opt, fmt.Errorf("no URL specified")
	}
	return opt, nil
}
