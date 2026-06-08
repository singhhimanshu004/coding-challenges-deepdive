package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunStdin checks the stdin-fallback path: no file args means read stdin,
// and the output carries no trailing filename.
func TestRunStdin(t *testing.T) {
	in := strings.NewReader("hello world\nsecond line\n")
	var out, errOut bytes.Buffer

	code := run(nil, in, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, errOut.String())
	}

	// Default columns: lines words bytes -> 2 4 24
	want := " 2  4 24\n"
	if out.String() != want {
		t.Errorf("stdin output = %q, want %q", out.String(), want)
	}
}

// TestRunSingleFile checks reading a real file with a specific flag.
func TestRunSingleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	if err := os.WriteFile(path, []byte("one two three\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	code := run([]string{"-w", path}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, errOut.String())
	}

	want := "3 " + path + "\n"
	if out.String() != want {
		t.Errorf("file output = %q, want %q", out.String(), want)
	}
}

// TestRunMultipleFiles checks per-file rows plus the aggregated total row.
func TestRunMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(a, []byte("foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("bar baz\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	code := run([]string{"-l", a, b}, strings.NewReader(""), &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, errOut.String())
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 output lines, got %d: %q", len(lines), out.String())
	}
	if !strings.HasSuffix(lines[2], "total") {
		t.Errorf("last line should be the total, got %q", lines[2])
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[2]), "2") {
		t.Errorf("total line count should be 2, got %q", lines[2])
	}
}

// TestRunMissingFile checks the exit code and error message for a bad path.
func TestRunMissingFile(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"/no/such/file/here.txt"}, strings.NewReader(""), &out, &errOut)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "wc:") {
		t.Errorf("expected error message on stderr, got %q", errOut.String())
	}
}

// TestRunBadFlag checks usage errors return exit code 2.
func TestRunBadFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run([]string{"-z"}, strings.NewReader(""), &out, &errOut)
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

// TestRunAllFlags checks every column appears in the fixed order l, w, m, c.
func TestRunAllFlags(t *testing.T) {
	in := strings.NewReader("héllo\n") // 1 line, 1 word, 6 chars, 7 bytes
	var out, errOut bytes.Buffer

	code := run([]string{"-lwmc"}, in, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%q", code, errOut.String())
	}

	fields := strings.Fields(out.String())
	want := []string{"1", "1", "6", "7"}
	if len(fields) != len(want) {
		t.Fatalf("got %v, want %v", fields, want)
	}
	for i := range want {
		if fields[i] != want[i] {
			t.Errorf("field %d = %s, want %s (full=%q)", i, fields[i], want[i], out.String())
		}
	}
}
