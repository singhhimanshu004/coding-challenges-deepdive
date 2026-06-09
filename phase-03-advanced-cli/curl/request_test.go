package main

import (
	"strings"
	"testing"
)

// TestFrameGET checks the exact bytes of a plain GET request — every CRLF, the
// mandatory Host header, the defaults, and the terminating blank line.
func TestFrameGET(t *testing.T) {
	tgt, _ := parseTarget("http://example.com/index.html")
	req := request{method: "GET", target: tgt}

	got := string(req.frame())
	want := "GET /index.html HTTP/1.1\r\n" +
		"Host: example.com\r\n" +
		"User-Agent: " + defaultUserAgent + "\r\n" +
		"Accept: */*\r\n" +
		"Connection: close\r\n" +
		"\r\n"
	if got != want {
		t.Errorf("GET framing mismatch:\n got=%q\nwant=%q", got, want)
	}
}

// TestFramePOSTWithBody verifies that a body produces POST semantics: the body
// bytes are appended after the blank line and Content-Length is set correctly.
func TestFramePOSTWithBody(t *testing.T) {
	tgt, _ := parseTarget("http://api.test/submit")
	body := []byte(`{"name":"go"}`)
	req := request{method: "POST", target: tgt, body: body}

	got := string(req.frame())

	if !strings.HasPrefix(got, "POST /submit HTTP/1.1\r\n") {
		t.Errorf("expected POST request line, got %q", got)
	}
	if !strings.Contains(got, "Content-Length: 13\r\n") {
		t.Errorf("expected Content-Length: 13, got %q", got)
	}
	// Body must come after the blank line, byte-for-byte.
	if !strings.HasSuffix(got, "\r\n\r\n"+string(body)) {
		t.Errorf("body not appended correctly: %q", got)
	}
}

// TestFrameCustomHeadersOverrideDefaults ensures a user -H can replace a default
// (User-Agent) rather than duplicating it.
func TestFrameCustomHeadersOverrideDefaults(t *testing.T) {
	tgt, _ := parseTarget("http://h/")
	req := request{
		method:  "GET",
		target:  tgt,
		headers: []header{{name: "User-Agent", value: "mybot/9"}, {name: "X-Trace", value: "abc"}},
	}

	got := string(req.frame())

	if strings.Count(got, "User-Agent:") != 1 {
		t.Errorf("User-Agent should appear exactly once, got %q", got)
	}
	if !strings.Contains(got, "User-Agent: mybot/9\r\n") {
		t.Errorf("custom User-Agent not honored: %q", got)
	}
	if strings.Contains(got, defaultUserAgent) {
		t.Errorf("default User-Agent should have been overridden: %q", got)
	}
	if !strings.Contains(got, "X-Trace: abc\r\n") {
		t.Errorf("custom header missing: %q", got)
	}
}

func TestParseHeaderArg(t *testing.T) {
	cases := []struct {
		in        string
		name, val string
		wantErr   bool
	}{
		{in: "Content-Type: application/json", name: "Content-Type", val: "application/json"},
		{in: "X-Empty:", name: "X-Empty", val: ""},
		{in: "no-colon", wantErr: true},
		{in: ": novalue", wantErr: true},
	}
	for _, c := range cases {
		h, err := parseHeaderArg(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseHeaderArg(%q) expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("parseHeaderArg(%q) error: %v", c.in, err)
		}
		if h.name != c.name || h.value != c.val {
			t.Errorf("parseHeaderArg(%q) = %+v; want {%s %s}", c.in, h, c.name, c.val)
		}
	}
}
