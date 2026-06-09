package main

import (
	"fmt"
	"net/url"
	"strings"
)

// target is our own small, flattened view of a URL — exactly the four pieces
// the HTTP layer needs and nothing more.
//
// 🐍 Python analogy: this is like the named tuple you'd build from
// urllib.parse.urlsplit(): (scheme, host, port, path). We deliberately keep it
// tiny so the request writer has an obvious, self-documenting input.
type target struct {
	scheme string // "http" or "https"
	host   string // hostname only, NO port (e.g. "example.com")
	port   string // "80", "443", or whatever the URL specified
	path   string // path + query, ALWAYS begins with "/" (the "request target")
}

// hostport is what you hand to net.Dial: "host:port".
//
// Go note: a method with a value receiver (t target) cannot mutate the struct,
// which is exactly what we want for a read-only accessor. It's the equivalent
// of a Python @property that just formats existing fields.
func (t target) hostport() string {
	return t.host + ":" + t.port
}

// authority is the value of the Host: header. For the default port we omit it,
// matching what real curl/browsers send (Host: example.com, not :80).
func (t target) authority() string {
	if (t.scheme == "http" && t.port == "80") || (t.scheme == "https" && t.port == "443") {
		return t.host
	}
	return t.host + ":" + t.port
}

// parseTarget turns a user-supplied URL string into a target.
//
// We lean on the standard library's net/url ONLY for the lexical parsing
// (splitting scheme/host/path/query). That is a pure string operation and not
// the part of the challenge we're meant to build by hand — the "build it
// yourself" mandate is about the HTTP protocol on the wire, which starts after
// we have these pieces.
func parseTarget(raw string) (target, error) {
	// Be forgiving like curl: if the user types "example.com/foo" with no
	// scheme, assume http:// so net/url doesn't mis-parse the host as a path.
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return target{}, fmt.Errorf("invalid URL %q: %w", raw, err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return target{}, fmt.Errorf("unsupported scheme %q (only http and https)", u.Scheme)
	}

	host := u.Hostname() // strips any ":port" for us
	if host == "" {
		return target{}, fmt.Errorf("URL %q has no host", raw)
	}

	// Pick the port: explicit one from the URL, else the scheme default.
	port := u.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	// The "request target" sent on the first line of the request is the path
	// plus the query string. An empty path MUST become "/".
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}

	return target{scheme: scheme, host: host, port: port, path: path}, nil
}

// resolveLocation computes the absolute URL to follow for a redirect. The
// Location header may be absolute ("https://other/x") or relative ("/x" or
// "x") — RFC 7231 says resolve it against the request URL, which is exactly
// what url.ResolveReference does. We reuse net/url here for the same reason as
// above: pure string resolution, not protocol work.
func resolveLocation(base target, location string) (target, error) {
	baseURL := &url.URL{
		Scheme: base.scheme,
		Host:   base.hostport(),
		// Path/RawQuery come from the request target. We split them back out so
		// relative resolution ("../x", "?y") behaves correctly.
	}
	if i := strings.IndexByte(base.path, '?'); i >= 0 {
		baseURL.Path = base.path[:i]
		baseURL.RawQuery = base.path[i+1:]
	} else {
		baseURL.Path = base.path
	}

	ref, err := url.Parse(location)
	if err != nil {
		return target{}, fmt.Errorf("invalid Location %q: %w", location, err)
	}
	abs := baseURL.ResolveReference(ref)

	return parseTarget(abs.String())
}
