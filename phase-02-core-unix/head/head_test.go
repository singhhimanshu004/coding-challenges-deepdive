package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeLines builds a string of n numbered lines, each newline-terminated.
func makeLines(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		b.WriteString("line")
		b.WriteByte(byte('0' + (i % 10)))
		b.WriteByte('\n')
	}
	return b.String()
}

// headStream is the streaming core; testing it directly avoids any file/stdin
// plumbing and lets us assert exact byte output.
func TestHeadLinesDefault(t *testing.T) {
	in := strings.NewReader(makeLines(20))
	var out bytes.Buffer
	if err := headStream(&out, in, config{count: 10}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if lines := strings.Count(got, "\n"); lines != 10 {
		t.Fatalf("expected 10 lines, got %d:\n%q", lines, got)
	}
}

func TestHeadLinesN(t *testing.T) {
	in := strings.NewReader(makeLines(100))
	var out bytes.Buffer
	if err := headStream(&out, in, config{count: 3}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := strings.Count(out.String(), "\n"), 3; got != want {
		t.Fatalf("expected %d lines, got %d", want, got)
	}
}

func TestHeadBytes(t *testing.T) {
	in := strings.NewReader("hello world this is a test")
	var out bytes.Buffer
	if err := headStream(&out, in, config{count: 5, byteMode: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := out.String(), "hello"; got != want {
		t.Fatalf("byte mode: got %q want %q", got, want)
	}
}

// N larger than the file: head should print the whole file and stop cleanly.
func TestHeadLinesMoreThanFile(t *testing.T) {
	content := makeLines(4)
	in := strings.NewReader(content)
	var out bytes.Buffer
	if err := headStream(&out, in, config{count: 100}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.String() != content {
		t.Fatalf("expected full file, got %q", out.String())
	}
}

// A final line without a trailing newline must still be printed and counted.
func TestHeadLinesNoTrailingNewline(t *testing.T) {
	in := strings.NewReader("a\nb\nc")
	var out bytes.Buffer
	if err := headStream(&out, in, config{count: 10}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := out.String(), "a\nb\nc"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestHeadEmptyInput(t *testing.T) {
	var out bytes.Buffer
	if err := headStream(&out, strings.NewReader(""), config{count: 10}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected empty output, got %q", out.String())
	}
	out.Reset()
	if err := headStream(&out, strings.NewReader(""), config{count: 5, byteMode: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected empty byte output, got %q", out.String())
	}
}

func TestHeadBytesMoreThanFile(t *testing.T) {
	in := strings.NewReader("abc")
	var out bytes.Buffer
	if err := headStream(&out, in, config{count: 100, byteMode: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.String() != "abc" {
		t.Fatalf("got %q", out.String())
	}
}

// --- argument parsing ---

func TestParseArgs(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		count    int
		byteMode bool
		files    []string
		wantErr  bool
	}{
		{"default", nil, 10, false, nil, false},
		{"-n space", []string{"-n", "5"}, 5, false, nil, false},
		{"-n glued", []string{"-n5"}, 5, false, nil, false},
		{"-c space", []string{"-c", "20"}, 20, true, nil, false},
		{"-c glued", []string{"-c100"}, 100, true, nil, false},
		{"files", []string{"a.txt", "b.txt"}, 10, false, []string{"a.txt", "b.txt"}, false},
		{"flag then files", []string{"-n", "2", "a.txt"}, 2, false, []string{"a.txt"}, false},
		{"dash stdin", []string{"-"}, 10, false, []string{"-"}, false},
		{"missing value", []string{"-n"}, 0, false, nil, true},
		{"bad number", []string{"-n", "x"}, 0, false, nil, true},
		{"bad flag", []string{"-z"}, 0, false, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := parseArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.count != tc.count || cfg.byteMode != tc.byteMode {
				t.Fatalf("count/byteMode = %d/%v, want %d/%v", cfg.count, cfg.byteMode, tc.count, tc.byteMode)
			}
			if strings.Join(cfg.files, ",") != strings.Join(tc.files, ",") {
				t.Fatalf("files = %v, want %v", cfg.files, tc.files)
			}
		})
	}
}

// --- multi-file headers via run() against real temp files ---

func TestRunMultipleFilesHeaders(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(a, []byte("a1\na2\na3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("b1\nb2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Redirect stdout to capture run()'s output.
	got := captureStdout(t, func() int {
		return run([]string{"-n", "2", a, b})
	})

	if !strings.Contains(got, "==> "+a+" <==") {
		t.Fatalf("missing header for first file:\n%s", got)
	}
	if !strings.Contains(got, "==> "+b+" <==") {
		t.Fatalf("missing header for second file:\n%s", got)
	}
	if !strings.Contains(got, "a1\na2\n") {
		t.Fatalf("missing first file body:\n%s", got)
	}
	if strings.Contains(got, "a3") {
		t.Fatalf("should not include 3rd line of first file:\n%s", got)
	}
	// Blank separator line should appear before the second header but not the first.
	if strings.HasPrefix(got, "\n") {
		t.Fatalf("output should not start with a blank line:\n%q", got)
	}
}

func TestRunMissingFile(t *testing.T) {
	got := captureStdout(t, func() int {
		code := run([]string{filepath.Join(t.TempDir(), "nope.txt")})
		if code != 1 {
			t.Errorf("expected exit code 1 for missing file, got %d", code)
		}
		return code
	})
	_ = got
}

// captureStdout swaps os.Stdout for a pipe, runs fn, and returns what was
// written. This is the standard Go trick for testing code that writes directly
// to os.Stdout.
func captureStdout(t *testing.T, fn func() int) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	w.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// TestRunStdin verifies the stdin fallback path (no file args).
func TestRunStdin(t *testing.T) {
	old := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	defer func() { os.Stdin = old }()

	go func() {
		w.WriteString("one\ntwo\nthree\nfour\n")
		w.Close()
	}()

	got := captureStdout(t, func() int {
		return run([]string{"-n", "2"})
	})
	if got != "one\ntwo\n" {
		t.Fatalf("stdin head: got %q", got)
	}
}
