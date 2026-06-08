package main

import (
	"bytes"
	"strings"
	"testing"
)

// runUniq is a tiny test helper: feed it input text and options, get back the
// produced output. Keeping this in one place keeps each test case readable.
func runUniq(t *testing.T, input string, opt options) string {
	t.Helper()
	var out bytes.Buffer
	if err := uniqStream(strings.NewReader(input), &out, opt); err != nil {
		t.Fatalf("uniqStream returned error: %v", err)
	}
	return out.String()
}

func TestAdjacentDuplicatesCollapse(t *testing.T) {
	// Three adjacent "a" lines collapse to one; "b" stands alone.
	got := runUniq(t, "a\na\na\nb\n", options{})
	want := "a\nb\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestNonAdjacentDuplicatesAreKept(t *testing.T) {
	// THE KEY SEMANTIC: a...b...a — the two a's are NOT adjacent, so both stay.
	got := runUniq(t, "a\nb\na\n", options{})
	want := "a\nb\na\n"
	if got != want {
		t.Errorf("non-adjacent dups should be kept: got %q, want %q", got, want)
	}
}

func TestCountFlag(t *testing.T) {
	got := runUniq(t, "a\na\nb\nc\nc\nc\n", options{count: true})
	want := "   2 a\n   1 b\n   3 c\n"
	if got != want {
		t.Errorf("-c: got %q, want %q", got, want)
	}
}

func TestOnlyDuplicatedFlag(t *testing.T) {
	// -d keeps only groups that repeated (a, c), drops the singleton b.
	got := runUniq(t, "a\na\nb\nc\nc\n", options{onlyDup: true})
	want := "a\nc\n"
	if got != want {
		t.Errorf("-d: got %q, want %q", got, want)
	}
}

func TestOnlyUniqueFlag(t *testing.T) {
	// -u keeps only groups that appeared exactly once (b), drops a and c.
	got := runUniq(t, "a\na\nb\nc\nc\n", options{onlyUni: true})
	want := "b\n"
	if got != want {
		t.Errorf("-u: got %q, want %q", got, want)
	}
}

func TestCountWithOnlyDuplicated(t *testing.T) {
	// Combining -c and -d: counts, but only for repeated groups.
	got := runUniq(t, "a\na\nb\nc\nc\nc\n", options{count: true, onlyDup: true})
	want := "   2 a\n   3 c\n"
	if got != want {
		t.Errorf("-c -d: got %q, want %q", got, want)
	}
}

func TestEmptyInput(t *testing.T) {
	got := runUniq(t, "", options{})
	if got != "" {
		t.Errorf("empty input should produce empty output, got %q", got)
	}
}

func TestSingleLine(t *testing.T) {
	got := runUniq(t, "only\n", options{})
	want := "only\n"
	if got != want {
		t.Errorf("single line: got %q, want %q", got, want)
	}
}

func TestSingleLineNoTrailingNewline(t *testing.T) {
	// bufio.Scanner yields the final line even without a trailing newline;
	// we always emit one newline-terminated line.
	got := runUniq(t, "only", options{})
	want := "only\n"
	if got != want {
		t.Errorf("no trailing newline: got %q, want %q", got, want)
	}
}

// TestStdinFallback exercises the full run() path with no file arguments,
// proving stdin is used when no positional input is given. (We swap os.Stdin
// for a pipe to simulate piped input, like `sort | uniq`.)
func TestStdinFallback(t *testing.T) {
	r, w, err := osPipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	go func() {
		w.WriteString("x\nx\ny\n")
		w.Close()
	}()

	oldStdin, oldStdout := swapStdin(r), captureStdout(t)
	defer restoreStdin(oldStdin)

	code := run(nil) // no args -> read stdin, write stdout
	out := oldStdout()

	if code != 0 {
		t.Fatalf("run exit code = %d, want 0", code)
	}
	want := "x\ny\n"
	if out != want {
		t.Errorf("stdin fallback: got %q, want %q", out, want)
	}
}
