package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// catArgs is a small helper that runs run() with a fake stdin and captured
// stdout/stderr, then returns them. This keeps each test short and readable.
//
// Go note: t.Helper() tells the test runner "blame the caller, not this line"
// when an assertion fails — so failures point at the actual test.
func catArgs(t *testing.T, args []string, stdin string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	code = run(args, strings.NewReader(stdin), &out, &errb)
	return out.String(), errb.String(), code
}

// writeTemp creates a temp file with the given content and returns its path.
// t.TempDir() is auto-removed when the test finishes.
func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeTemp: %v", err)
	}
	return path
}

func TestSingleFile(t *testing.T) {
	f := writeTemp(t, "a.txt", "hello\nworld\n")
	out, errs, code := catArgs(t, []string{f}, "")
	if code != 0 || errs != "" {
		t.Fatalf("code=%d stderr=%q", code, errs)
	}
	if out != "hello\nworld\n" {
		t.Fatalf("got %q", out)
	}
}

func TestMultipleFilesConcatenateInOrder(t *testing.T) {
	a := writeTemp(t, "a.txt", "AAA\n")
	b := writeTemp(t, "b.txt", "BBB\n")
	out, _, code := catArgs(t, []string{a, b}, "")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if out != "AAA\nBBB\n" {
		t.Fatalf("got %q", out)
	}
}

func TestStdinWhenNoFiles(t *testing.T) {
	out, _, code := catArgs(t, nil, "piped input\n")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if out != "piped input\n" {
		t.Fatalf("got %q", out)
	}
}

func TestDashMeansStdinInOrder(t *testing.T) {
	a := writeTemp(t, "a.txt", "file-a\n")
	// Order should be: file, then stdin, then file again.
	c := writeTemp(t, "c.txt", "file-c\n")
	out, _, code := catArgs(t, []string{a, "-", c}, "STDIN\n")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	want := "file-a\nSTDIN\nfile-c\n"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestNumberAllLines(t *testing.T) {
	f := writeTemp(t, "a.txt", "one\n\nthree\n")
	out, _, code := catArgs(t, []string{"-n", f}, "")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	want := "     1\tone\n     2\t\n     3\tthree\n"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestNumberNonBlankLines(t *testing.T) {
	f := writeTemp(t, "a.txt", "one\n\nthree\n")
	out, _, code := catArgs(t, []string{"-b", f}, "")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	// Blank middle line is NOT numbered, and the counter does not advance.
	want := "     1\tone\n\n     2\tthree\n"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestNumberNonBlankOverridesNumberAll(t *testing.T) {
	f := writeTemp(t, "a.txt", "x\n\ny\n")
	// When both -n and -b are given, -b wins (matches GNU cat).
	out, _, _ := catArgs(t, []string{"-nb", f}, "")
	want := "     1\tx\n\n     2\ty\n"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestShowEnds(t *testing.T) {
	f := writeTemp(t, "a.txt", "ab\n\n")
	out, _, _ := catArgs(t, []string{"-E", f}, "")
	want := "ab$\n$\n"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestNumberingIsContinuousAcrossFiles(t *testing.T) {
	a := writeTemp(t, "a.txt", "l1\nl2\n")
	b := writeTemp(t, "b.txt", "l3\n")
	out, _, _ := catArgs(t, []string{"-n", a, b}, "")
	want := "     1\tl1\n     2\tl2\n     3\tl3\n"
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestMissingFileReportsErrorButContinues(t *testing.T) {
	good := writeTemp(t, "good.txt", "data\n")
	missing := filepath.Join(t.TempDir(), "nope.txt")
	out, errs, code := catArgs(t, []string{missing, good}, "")
	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
	if out != "data\n" {
		t.Fatalf("good file should still print, got %q", out)
	}
	if !strings.Contains(errs, "No such file or directory") {
		t.Fatalf("want a not-found message, got %q", errs)
	}
}

func TestEmptyInputProducesNoOutput(t *testing.T) {
	f := writeTemp(t, "empty.txt", "")
	out, _, code := catArgs(t, []string{f}, "")
	if code != 0 || out != "" {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestLastLineWithoutNewlineIsPreserved(t *testing.T) {
	f := writeTemp(t, "a.txt", "no-trailing-newline")
	out, _, _ := catArgs(t, []string{f}, "")
	if out != "no-trailing-newline" {
		t.Fatalf("got %q", out)
	}
	// With -n the partial last line still gets a number.
	out, _, _ = catArgs(t, []string{"-n", f}, "")
	if out != "     1\tno-trailing-newline" {
		t.Fatalf("got %q", out)
	}
}

func TestBinarySafeRawCopy(t *testing.T) {
	// No flags => byte-faithful copy, including NUL and high bytes.
	data := []byte{0x00, 0x01, 0xff, 0x0a, 0x80, 0x7f}
	f := writeTemp(t, "bin", string(data))
	var out, errb bytes.Buffer
	code := run([]string{f}, strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !bytes.Equal(out.Bytes(), data) {
		t.Fatalf("binary copy mismatch: got %v", out.Bytes())
	}
}

func TestUnknownFlagIsUsageError(t *testing.T) {
	_, errs, code := catArgs(t, []string{"-z"}, "")
	if code != 2 {
		t.Fatalf("want exit 2, got %d", code)
	}
	if !strings.Contains(errs, "invalid option") {
		t.Fatalf("want usage error, got %q", errs)
	}
}

func TestDoubleDashStopsFlagParsing(t *testing.T) {
	// After "--", a name that looks like a flag is treated as a file.
	// Create a file literally named "-n".
	dir := t.TempDir()
	path := filepath.Join(dir, "-n")
	if err := os.WriteFile(path, []byte("literal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := catArgs(t, []string{"--", path}, "")
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if out != "literal\n" {
		t.Fatalf("got %q", out)
	}
}
