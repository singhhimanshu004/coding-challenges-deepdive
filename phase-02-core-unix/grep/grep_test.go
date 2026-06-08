package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runCLI is a small harness around cli(): it feeds stdin from a string, captures
// stdout/stderr, and returns them with the exit code so tests can assert on all
// three the way a shell user would observe them.
func runCLI(t *testing.T, args []string, stdin string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code = cli(args, strings.NewReader(stdin), &out, &errBuf)
	return out.String(), errBuf.String(), code
}

func TestBasicMatch(t *testing.T) {
	in := "apple\nbanana\ncherry\n"
	out, _, code := runCLI(t, []string{"an"}, in)
	if out != "banana\n" {
		t.Errorf("got %q, want %q", out, "banana\n")
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0 (match)", code)
	}
}

func TestIgnoreCase(t *testing.T) {
	in := "Apple\nBANANA\ncherry\n"
	out, _, _ := runCLI(t, []string{"-i", "banana"}, in)
	if out != "BANANA\n" {
		t.Errorf("got %q, want %q", out, "BANANA\n")
	}
}

func TestInvert(t *testing.T) {
	in := "keep\ndrop\nkeep\n"
	out, _, _ := runCLI(t, []string{"-v", "drop"}, in)
	if out != "keep\nkeep\n" {
		t.Errorf("got %q, want %q", out, "keep\nkeep\n")
	}
}

func TestLineNumbers(t *testing.T) {
	in := "alpha\nbeta\ngamma\nbeta\n"
	out, _, _ := runCLI(t, []string{"-n", "beta"}, in)
	want := "2:beta\n4:beta\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestCount(t *testing.T) {
	in := "a1\nb\na2\na3\n"
	out, _, code := runCLI(t, []string{"-c", "a"}, in)
	if out != "3\n" {
		t.Errorf("got %q, want %q", out, "3\n")
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestWordMatch(t *testing.T) {
	in := "foo\nfoobar\na foo b\nbarfoo\n"
	out, _, _ := runCLI(t, []string{"-w", "foo"}, in)
	want := "foo\na foo b\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestContextAfter(t *testing.T) {
	in := "one\ntwo\nhit\nfour\nfive\n"
	out, _, _ := runCLI(t, []string{"-A", "1", "hit"}, in)
	want := "hit\nfour\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestContextBefore(t *testing.T) {
	in := "one\ntwo\nhit\nfour\n"
	out, _, _ := runCLI(t, []string{"-B", "2", "hit"}, in)
	want := "one\ntwo\nhit\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestContextAround(t *testing.T) {
	in := "one\ntwo\nhit\nfour\nfive\n"
	out, _, _ := runCLI(t, []string{"-C", "1", "hit"}, in)
	want := "two\nhit\nfour\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// Non-adjacent matches under context get a "--" group separator, just like GNU.
func TestContextGroupSeparator(t *testing.T) {
	in := "m1\nx\ny\nz\nm2\n"
	out, _, _ := runCLI(t, []string{"-A", "1", "m"}, in)
	want := "m1\nx\n--\nm2\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestNoMatchExitCode(t *testing.T) {
	out, _, code := runCLI(t, []string{"zzz"}, "abc\ndef\n")
	if out != "" {
		t.Errorf("expected no output, got %q", out)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1 (no match)", code)
	}
}

func TestBadPatternExitCode(t *testing.T) {
	_, _, code := runCLI(t, []string{"("}, "abc\n")
	if code != 2 {
		t.Errorf("exit code = %d, want 2 (error)", code)
	}
}

func TestStdinDefault(t *testing.T) {
	// No file operand => read stdin; single source => no filename prefix.
	out, _, _ := runCLI(t, []string{"x"}, "ax\nb\ncx\n")
	want := "ax\ncx\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// Recursive walk over a temp directory tree, asserting filename-prefixed output.
func TestRecursive(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "hello\nworld match\n")
	mustWrite(t, filepath.Join(root, "sub", "b.txt"), "nope\nanother match\n")
	mustWrite(t, filepath.Join(root, "sub", "c.txt"), "nothing here\n")

	out, _, code := runCLI(t, []string{"-r", "match", root}, "")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}

	a := filepath.Join(root, "a.txt") + ":world match"
	b := filepath.Join(root, "sub", "b.txt") + ":another match"
	if !strings.Contains(out, a) {
		t.Errorf("output missing %q\ngot:\n%s", a, out)
	}
	if !strings.Contains(out, b) {
		t.Errorf("output missing %q\ngot:\n%s", b, out)
	}
	if strings.Contains(out, "nothing here") {
		t.Errorf("unexpected non-matching line in output:\n%s", out)
	}
}

func TestRecursiveFilesWithMatches(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "needle\n")
	mustWrite(t, filepath.Join(root, "b.txt"), "haystack\n")

	out, _, _ := runCLI(t, []string{"-r", "-l", "needle", root}, "")
	wantFile := filepath.Join(root, "a.txt")
	if !strings.Contains(out, wantFile) {
		t.Errorf("output missing %q\ngot:\n%s", wantFile, out)
	}
	if strings.Contains(out, "b.txt") {
		t.Errorf("b.txt should not be listed:\n%s", out)
	}
}

// Multiple file operands trigger filename prefixes even without -r.
func TestMultiFilePrefix(t *testing.T) {
	root := t.TempDir()
	f1 := filepath.Join(root, "one.txt")
	f2 := filepath.Join(root, "two.txt")
	mustWrite(t, f1, "x match\n")
	mustWrite(t, f2, "y match\n")

	out, _, _ := runCLI(t, []string{"match", f1, f2}, "")
	if !strings.Contains(out, f1+":x match") || !strings.Contains(out, f2+":y match") {
		t.Errorf("expected filename-prefixed output, got:\n%s", out)
	}
}

func TestDirectoryWithoutRecursiveErrors(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "match\n")

	_, errOut, code := runCLI(t, []string{"match", root}, "")
	if code != 2 {
		t.Errorf("exit code = %d, want 2 (directory without -r)", code)
	}
	if !strings.Contains(errOut, "Is a directory") {
		t.Errorf("expected 'Is a directory' warning, got %q", errOut)
	}
}

func TestMatcherWordAlternation(t *testing.T) {
	// -w must bound the WHOLE alternation, not just one branch.
	m, err := NewMatcher("foo|bar", false, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Match("a bar b") {
		t.Error("expected 'a bar b' to match whole-word foo|bar")
	}
	if m.Match("rebar") {
		t.Error("'rebar' should not match whole-word foo|bar")
	}
}

// mustWrite creates a file (and any parent dirs) with the given content.
func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
