package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runScript drives the CLI run() with the given args and stdin, returning
// stdout and the exit code. This is the same seam main() uses, so tests cover
// the real flag parsing + execution path without spawning a process.
func runScript(t *testing.T, args []string, stdin string) (string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errb)
	if code != 0 && errb.Len() > 0 {
		t.Logf("stderr: %s", errb.String())
	}
	return out.String(), code
}

func TestSubstituteFirstOnly(t *testing.T) {
	out, code := runScript(t, []string{"s/o/0/"}, "foo boo\n")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if want := "f0o boo\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestSubstituteGlobal(t *testing.T) {
	out, _ := runScript(t, []string{"s/o/0/g"}, "foo boo\n")
	if want := "f00 b00\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestSubstituteCaseInsensitive(t *testing.T) {
	out, _ := runScript(t, []string{"s/hello/hi/gi"}, "Hello HELLO hello\n")
	if want := "hi hi hi\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestSubstituteBackreferences(t *testing.T) {
	out, _ := runScript(t, []string{`s/(\w+) (\w+)/\2 \1/`}, "John Smith\n")
	if want := "Smith John\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestSubstituteWholeMatchAmpersand(t *testing.T) {
	out, _ := runScript(t, []string{`s/cat/[&]/`}, "the cat sat\n")
	if want := "the [cat] sat\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestSubstituteAlternateDelimiter(t *testing.T) {
	out, _ := runScript(t, []string{`s|/usr/bin|/opt/bin|`}, "/usr/bin/sed\n")
	if want := "/opt/bin/sed\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestPrintCommandDuplicates(t *testing.T) {
	// p without -n prints the line twice (once via p, once auto-print).
	out, _ := runScript(t, []string{"2p"}, "a\nb\nc\n")
	if want := "a\nb\nb\nc\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestPrintSuppressedWithN(t *testing.T) {
	// -n + p is the canonical "print only selected lines" idiom.
	out, _ := runScript(t, []string{"-n", "2,4p"}, "a\nb\nc\nd\ne\n")
	if want := "b\nc\nd\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestDeleteNumericAddress(t *testing.T) {
	out, _ := runScript(t, []string{"2d"}, "a\nb\nc\n")
	if want := "a\nc\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestDeleteLastLineAddress(t *testing.T) {
	out, _ := runScript(t, []string{"$d"}, "a\nb\nc\n")
	if want := "a\nb\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestDeleteRegexAddress(t *testing.T) {
	out, _ := runScript(t, []string{"/banana/d"}, "apple\nbanana\ncherry\n")
	if want := "apple\ncherry\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestRangeNumericDelete(t *testing.T) {
	out, _ := runScript(t, []string{"2,4d"}, "a\nb\nc\nd\ne\n")
	if want := "a\ne\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestRangeRegexPrint(t *testing.T) {
	in := "x\nBEGIN\nmid\nEND\ny\n"
	out, _ := runScript(t, []string{"-n", "/BEGIN/,/END/p"}, in)
	if want := "BEGIN\nmid\nEND\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestMultipleCommandsSemicolon(t *testing.T) {
	out, _ := runScript(t, []string{"s/a/A/g; 2d"}, "abc\nabc\nabc\n")
	if want := "Abc\nAbc\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestAddressedSubstitution(t *testing.T) {
	// Only line 2 should be substituted.
	out, _ := runScript(t, []string{"2s/x/Y/"}, "x\nx\nx\n")
	if want := "x\nY\nx\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestStdinReading(t *testing.T) {
	out, code := runScript(t, []string{"s/world/Go/"}, "hello world\n")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if want := "hello Go\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestFileArgument(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := runScript(t, []string{"s/t/T/", path}, "")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if want := "one\nTwo\nThree\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestInPlaceEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	original := "red\ngreen\nblue\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := runScript(t, []string{"-i", "s/e/E/g", path}, "")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if out != "" {
		t.Errorf("-i should write nothing to stdout, got %q", out)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "rEd\ngrEEn\nbluE\n"; string(got) != want {
		t.Errorf("file contents = %q, want %q", string(got), want)
	}
}

func TestMissingScriptIsUsageError(t *testing.T) {
	_, code := runScript(t, []string{}, "")
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestBadScriptIsUsageError(t *testing.T) {
	_, code := runScript(t, []string{"s/unterminated"}, "x\n")
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	_, code := runScript(t, []string{"2z"}, "x\n")
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}
