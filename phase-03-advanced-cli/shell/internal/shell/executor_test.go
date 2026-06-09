package shell

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- Integration tests: real process execution ---------------------------
//
// These drive the executor end-to-end by running harmless external programs
// (echo, cat, true, false) plus real pipes and redirections, then asserting on
// captured output and exit codes. No interactive terminal is involved.

func TestExecExternalCommand(t *testing.T) {
	sh, out, _ := newTestShell("")
	st := sh.RunLine("echo from-echo")
	// echo is a builtin here, so prove externals work via a non-builtin: true.
	if st != 0 {
		t.Fatalf("status = %d", st)
	}
	_ = out
	if st := sh.RunLine("true"); st != 0 {
		t.Errorf("true status = %d, want 0", st)
	}
	if st := sh.RunLine("false"); st != 1 {
		t.Errorf("false status = %d, want 1", st)
	}
}

func TestExecCommandNotFound(t *testing.T) {
	sh, _, errb := newTestShell("")
	st := sh.RunLine("definitely-not-a-real-command-xyz")
	if st != 127 {
		t.Errorf("missing command status = %d, want 127", st)
	}
	if !strings.Contains(errb.String(), "not found") {
		t.Errorf("expected 'not found' message, got %q", errb.String())
	}
}

func TestExecTwoStagePipe(t *testing.T) {
	// `echo hi | cat` — the canonical two-stage pipeline. We feed a non-builtin
	// (cat) downstream so it must read from the pipe, not the terminal.
	sh, out, _ := newTestShell("")
	st := sh.RunLine("printf 'a\\nb\\nc\\n' | cat")
	if st != 0 {
		t.Fatalf("pipe status = %d", st)
	}
	if out.String() != "a\nb\nc\n" {
		t.Errorf("pipe output = %q", out.String())
	}
}

func TestExecThreeStagePipeWithCount(t *testing.T) {
	sh, out, _ := newTestShell("")
	st := sh.RunLine("printf 'x\\ny\\nz\\n' | cat | wc -l")
	if st != 0 {
		t.Fatalf("status = %d", st)
	}
	if got := strings.TrimSpace(out.String()); got != "3" {
		t.Errorf("wc -l through pipeline = %q, want 3", got)
	}
}

func TestExecRedirectOutAndReadBack(t *testing.T) {
	sh, out, _ := newTestShell("")
	dir := t.TempDir()
	file := filepath.Join(dir, "data.txt")

	if st := sh.RunLine("printf 'line1\\nline2\\n' > " + file); st != 0 {
		t.Fatalf("redirect-out status = %d", st)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "line1\nline2\n" {
		t.Errorf("file contents = %q", string(data))
	}

	// Read it back through the shell with input redirection.
	out.Reset()
	if st := sh.RunLine("cat < " + file); st != 0 {
		t.Fatalf("redirect-in status = %d", st)
	}
	if out.String() != "line1\nline2\n" {
		t.Errorf("cat < file = %q", out.String())
	}
}

func TestExecAppendRedirect(t *testing.T) {
	sh, _, _ := newTestShell("")
	file := filepath.Join(t.TempDir(), "log.txt")
	sh.RunLine("echo first > " + file)
	sh.RunLine("echo second >> " + file)
	data, _ := os.ReadFile(file)
	if string(data) != "first\nsecond\n" {
		t.Errorf("append result = %q", string(data))
	}
}

func TestExecStderrRedirect(t *testing.T) {
	sh, _, _ := newTestShell("")
	file := filepath.Join(t.TempDir(), "err.txt")
	// `ls` of a missing path writes to stderr; capture it to a file.
	sh.RunLine("ls /no/such/path/xyz 2> " + file)
	data, _ := os.ReadFile(file)
	if len(data) == 0 {
		t.Errorf("expected stderr captured to file, got empty")
	}
}

func TestExecPipelineWithRedirectAtEnd(t *testing.T) {
	// "what `cmd1 | cmd2 > file` actually does": last stage's stdout goes to file.
	sh, _, _ := newTestShell("")
	file := filepath.Join(t.TempDir(), "out.txt")
	st := sh.RunLine("printf 'b\\na\\nc\\n' | sort > " + file)
	if st != 0 {
		t.Fatalf("status = %d", st)
	}
	data, _ := os.ReadFile(file)
	if string(data) != "a\nb\nc\n" {
		t.Errorf("sorted file = %q", string(data))
	}
}

func TestExecSequencing(t *testing.T) {
	sh, out, _ := newTestShell("")
	sh.RunLine("echo one ; echo two ; echo three")
	if out.String() != "one\ntwo\nthree\n" {
		t.Errorf("sequencing output = %q", out.String())
	}
}

func TestExecAndOrShortCircuit(t *testing.T) {
	sh, out, _ := newTestShell("")
	sh.RunLine("true && echo yes")
	if strings.TrimSpace(out.String()) != "yes" {
		t.Errorf("&& after true = %q", out.String())
	}
	out.Reset()
	sh.RunLine("false && echo nope")
	if out.String() != "" {
		t.Errorf("&& after false should print nothing, got %q", out.String())
	}
	out.Reset()
	sh.RunLine("false || echo recovered")
	if strings.TrimSpace(out.String()) != "recovered" {
		t.Errorf("|| after false = %q", out.String())
	}
}

func TestExecBuiltinInPipeline(t *testing.T) {
	// A builtin (echo) as the producer stage of a pipeline must still stream
	// into an external consumer (cat).
	sh, out, _ := newTestShell("")
	st := sh.RunLine("echo piped-builtin | cat")
	if st != 0 {
		t.Fatalf("status = %d", st)
	}
	if strings.TrimSpace(out.String()) != "piped-builtin" {
		t.Errorf("builtin|external = %q", out.String())
	}
}

func TestRunReaderScript(t *testing.T) {
	var out bytes.Buffer
	sh := New(strings.NewReader(""), &out, &out)
	script := "# a comment\necho line-a\n\necho line-b\nexit 3\necho never\n"
	st := sh.RunReader(strings.NewReader(script))
	if st != 3 {
		t.Errorf("script exit status = %d, want 3", st)
	}
	if out.String() != "line-a\nline-b\n" {
		t.Errorf("script output = %q", out.String())
	}
}
