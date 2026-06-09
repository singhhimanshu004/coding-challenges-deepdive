package shell

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// newTestShell builds a Shell whose Out/Err are captured buffers, so tests can
// assert on what commands print without touching a real terminal.
func newTestShell(stdin string) (*Shell, *bytes.Buffer, *bytes.Buffer) {
	var out, errb bytes.Buffer
	sh := New(strings.NewReader(stdin), &out, &errb)
	return sh, &out, &errb
}

// ---- Variable expansion --------------------------------------------------

func TestExpandVariables(t *testing.T) {
	sh, _, _ := newTestShell("")
	sh.setVar("NAME", "world")
	cases := map[string]string{
		"$NAME":      "world",
		"${NAME}":    "world",
		"hi-$NAME!":  "hi-world!",
		"${NAME}ly":  "worldly",
		"$MISSING":   "",
		"literal":    "literal",
		"a${NAME}b":  "aworldb",
		"price=$NaN": "price=", // $NaN -> var "NaN" (unset) => empty
	}
	for in, want := range cases {
		if got := sh.expandStr(in); got != want {
			t.Errorf("expandStr(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExpandLastStatus(t *testing.T) {
	sh, _, _ := newTestShell("")
	sh.lastStatus = 42
	if got := sh.expandStr("exit=$?"); got != "exit=42" {
		t.Errorf("expandStr($?) = %q, want exit=42", got)
	}
}

func TestExpandSingleQuotesAreLiteral(t *testing.T) {
	// Single-quoted parts are tagged expand=false by the lexer, so $NAME stays
	// literal; double-quoted/unquoted expand.
	sh, _, _ := newTestShell("")
	sh.setVar("NAME", "world")
	toks, _ := tokenize(`'$NAME' "$NAME"`)
	if got := sh.expandWord(toks[0].word); got != "$NAME" {
		t.Errorf("single-quoted expand = %q, want literal $NAME", got)
	}
	if got := sh.expandWord(toks[1].word); got != "world" {
		t.Errorf("double-quoted expand = %q, want world", got)
	}
}

// ---- Builtins ------------------------------------------------------------

func TestBuiltinEcho(t *testing.T) {
	sh, out, _ := newTestShell("")
	sh.RunLine("echo hello world")
	if out.String() != "hello world\n" {
		t.Errorf("echo = %q", out.String())
	}
	out.Reset()
	sh.RunLine("echo -n no-newline")
	if out.String() != "no-newline" {
		t.Errorf("echo -n = %q", out.String())
	}
}

func TestBuiltinPwdAndCd(t *testing.T) {
	sh, out, _ := newTestShell("")
	start, _ := os.Getwd()
	defer os.Chdir(start)

	tmp := t.TempDir()
	if st := sh.RunLine("cd " + tmp); st != 0 {
		t.Fatalf("cd returned %d", st)
	}
	out.Reset()
	sh.RunLine("pwd")
	got := strings.TrimSpace(out.String())
	// macOS /tmp is a symlink to /private/tmp; compare resolved paths.
	wantResolved, _ := os.Getwd()
	if got != wantResolved {
		t.Errorf("pwd after cd = %q, want %q", got, wantResolved)
	}
}

func TestBuiltinExportAndExpansion(t *testing.T) {
	sh, out, _ := newTestShell("")
	defer os.Unsetenv("MYVAR")
	sh.RunLine("export MYVAR=hello")
	out.Reset()
	sh.RunLine("echo $MYVAR")
	if strings.TrimSpace(out.String()) != "hello" {
		t.Errorf("exported var expansion = %q", out.String())
	}
	if os.Getenv("MYVAR") != "hello" {
		t.Errorf("export did not reach process env")
	}
}

func TestBuiltinType(t *testing.T) {
	sh, out, _ := newTestShell("")
	sh.RunLine("type cd")
	if !strings.Contains(out.String(), "cd is a shell builtin") {
		t.Errorf("type cd = %q", out.String())
	}
}

func TestAssignmentSetsShellVar(t *testing.T) {
	sh, out, _ := newTestShell("")
	sh.RunLine("GREETING=hi")
	sh.RunLine("echo $GREETING")
	if strings.TrimSpace(out.String()) != "hi" {
		t.Errorf("assignment+expand = %q", out.String())
	}
	// A plain assignment must NOT leak into the child environment.
	if os.Getenv("GREETING") == "hi" {
		t.Errorf("plain assignment leaked to process env")
	}
}

func TestExitStatusTracking(t *testing.T) {
	sh, out, _ := newTestShell("")
	sh.RunLine("false")
	sh.RunLine("echo $?")
	if strings.TrimSpace(out.String()) != "1" {
		t.Errorf("$? after false = %q, want 1", out.String())
	}
	out.Reset()
	sh.RunLine("true")
	sh.RunLine("echo $?")
	if strings.TrimSpace(out.String()) != "0" {
		t.Errorf("$? after true = %q, want 0", out.String())
	}
}
