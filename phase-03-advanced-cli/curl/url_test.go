package main

import (
	"strings"
	"testing"
)

func TestParseTarget(t *testing.T) {
	cases := []struct {
		name                     string
		raw                      string
		scheme, host, port, path string
		wantErr                  bool
	}{
		{name: "http default port", raw: "http://example.com", scheme: "http", host: "example.com", port: "80", path: "/"},
		{name: "https default port", raw: "https://example.com/", scheme: "https", host: "example.com", port: "443", path: "/"},
		{name: "explicit port and path", raw: "http://localhost:8080/api/v1", scheme: "http", host: "localhost", port: "8080", path: "/api/v1"},
		{name: "query string preserved", raw: "http://h/search?q=go&n=2", scheme: "http", host: "h", port: "80", path: "/search?q=go&n=2"},
		{name: "scheme-less defaults to http", raw: "example.com/x", scheme: "http", host: "example.com", port: "80", path: "/x"},
		{name: "unsupported scheme", raw: "ftp://example.com", wantErr: true},
		{name: "no host", raw: "http://", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseTarget(c.raw)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %+v", c.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.scheme != c.scheme || got.host != c.host || got.port != c.port || got.path != c.path {
				t.Errorf("parseTarget(%q) = %+v; want scheme=%s host=%s port=%s path=%s",
					c.raw, got, c.scheme, c.host, c.port, c.path)
			}
		})
	}
}

func TestAuthority(t *testing.T) {
	// Default ports are omitted from the Host header; non-default ports are kept.
	cases := []struct {
		tgt  target
		want string
	}{
		{target{scheme: "http", host: "example.com", port: "80"}, "example.com"},
		{target{scheme: "https", host: "example.com", port: "443"}, "example.com"},
		{target{scheme: "http", host: "example.com", port: "8080"}, "example.com:8080"},
	}
	for _, c := range cases {
		if got := c.tgt.authority(); got != c.want {
			t.Errorf("authority(%+v) = %q; want %q", c.tgt, got, c.want)
		}
	}
}

func TestResolveLocation(t *testing.T) {
	base, _ := parseTarget("http://example.com/a/b?x=1")
	cases := []struct {
		location string
		want     string // scheme://host:port + path
	}{
		{"/new", "http://example.com:80/new"},
		{"https://other.com/z", "https://other.com:443/z"},
		{"c", "http://example.com:80/a/c"},
	}
	for _, c := range cases {
		got, err := resolveLocation(base, c.location)
		if err != nil {
			t.Fatalf("resolveLocation(%q) error: %v", c.location, err)
		}
		full := got.scheme + "://" + got.hostport() + got.path
		if !strings.HasPrefix(full, c.want) {
			t.Errorf("resolveLocation(%q) = %q; want prefix %q", c.location, full, c.want)
		}
	}
}
