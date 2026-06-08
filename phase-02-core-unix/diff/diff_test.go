package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// LCS core
// ---------------------------------------------------------------------------

func TestLCSLength(t *testing.T) {
	cases := []struct {
		a, b []string
		want int
	}{
		{nil, nil, 0},
		{[]string{"a", "b", "c"}, []string{"a", "b", "c"}, 3},
		{[]string{"a", "b", "c"}, nil, 0},
		{[]string{"a", "b", "c", "d"}, []string{"a", "c", "d"}, 3}, // drop b
		{[]string{"x", "a", "y"}, []string{"a"}, 1},
		{[]string{"a", "b", "c"}, []string{"c", "b", "a"}, 1},
	}
	for _, c := range cases {
		tbl := lcsTable(c.a, c.b)
		got := tbl[len(c.a)][len(c.b)]
		if got != c.want {
			t.Errorf("lcs(%v,%v)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Edit script
// ---------------------------------------------------------------------------

func TestIdenticalNoChanges(t *testing.T) {
	a := []string{"one", "two", "three"}
	edits := buildEditScript(a, a)
	if hasChanges(edits) {
		t.Fatalf("identical files reported changes: %+v", edits)
	}
	for _, e := range edits {
		if e.kind != opEqual {
			t.Errorf("expected only opEqual, got %v", e)
		}
	}
}

func TestPureInsertion(t *testing.T) {
	a := []string{"one", "two"}
	b := []string{"one", "inserted", "two"}
	edits := buildEditScript(a, b)
	if !hasChanges(edits) {
		t.Fatal("expected changes")
	}
	var inserts, deletes int
	for _, e := range edits {
		switch e.kind {
		case opInsert:
			inserts++
			if e.text != "inserted" {
				t.Errorf("unexpected insert text %q", e.text)
			}
		case opDelete:
			deletes++
		}
	}
	if inserts != 1 || deletes != 0 {
		t.Errorf("inserts=%d deletes=%d, want 1 and 0", inserts, deletes)
	}
}

func TestPureDeletion(t *testing.T) {
	a := []string{"one", "gone", "two"}
	b := []string{"one", "two"}
	edits := buildEditScript(a, b)
	var inserts, deletes int
	for _, e := range edits {
		switch e.kind {
		case opInsert:
			inserts++
		case opDelete:
			deletes++
			if e.text != "gone" {
				t.Errorf("unexpected delete text %q", e.text)
			}
		}
	}
	if inserts != 0 || deletes != 1 {
		t.Errorf("inserts=%d deletes=%d, want 0 and 1", inserts, deletes)
	}
}

func TestMixedChange(t *testing.T) {
	a := []string{"alpha", "beta", "gamma"}
	b := []string{"alpha", "BETA", "gamma"}
	out := normalDiff(buildEditScript(a, b))
	want := "2c2\n< beta\n---\n> BETA\n"
	if out != want {
		t.Errorf("normal diff mismatch:\n got: %q\nwant: %q", out, want)
	}
}

func TestEmptyVsNonEmpty(t *testing.T) {
	var a []string
	b := []string{"x", "y"}
	edits := buildEditScript(a, b)
	if !hasChanges(edits) {
		t.Fatal("expected changes between empty and non-empty")
	}
	out := normalDiff(edits)
	want := "0a1,2\n> x\n> y\n"
	if out != want {
		t.Errorf("empty-vs-nonempty normal diff:\n got: %q\nwant: %q", out, want)
	}
}

// ---------------------------------------------------------------------------
// Normal format edge cases
// ---------------------------------------------------------------------------

func TestNormalPureDeletionFormat(t *testing.T) {
	a := []string{"one", "two", "three"}
	b := []string{"one", "three"}
	out := normalDiff(buildEditScript(a, b))
	want := "2d1\n< two\n"
	if out != want {
		t.Errorf("normal deletion:\n got: %q\nwant: %q", out, want)
	}
}

// ---------------------------------------------------------------------------
// Unified format
// ---------------------------------------------------------------------------

func TestUnifiedHunkHeader(t *testing.T) {
	a := []string{"1", "2", "3", "4", "5", "6", "7", "8"}
	b := []string{"1", "2", "3", "X", "5", "6", "7", "8"}
	out := unifiedDiff(buildEditScript(a, b), "a", "b", 3)

	if !strings.HasPrefix(out, "--- a\n+++ b\n") {
		t.Errorf("missing file headers:\n%s", out)
	}
	if !strings.Contains(out, "@@ -1,7 +1,7 @@") {
		t.Errorf("unexpected hunk header:\n%s", out)
	}
	if !strings.Contains(out, "-4\n") || !strings.Contains(out, "+X\n") {
		t.Errorf("missing change lines:\n%s", out)
	}
	// Context lines should carry a single leading space.
	if !strings.Contains(out, " 3\n") || !strings.Contains(out, " 5\n") {
		t.Errorf("missing context lines:\n%s", out)
	}
}

func TestUnifiedPureInsertionHeader(t *testing.T) {
	a := []string{"a", "b"}
	b := []string{"a", "new", "b"}
	out := unifiedDiff(buildEditScript(a, b), "a", "b", 3)
	if !strings.Contains(out, "@@ -1,2 +1,3 @@") {
		t.Errorf("expected -1,2 +1,3 header:\n%s", out)
	}
	if !strings.Contains(out, "+new\n") {
		t.Errorf("expected +new line:\n%s", out)
	}
}

func TestUnifiedSeparateHunks(t *testing.T) {
	// Two changes far apart should yield two hunks.
	a := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14"}
	b := []string{"X", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "Y"}
	out := unifiedDiff(buildEditScript(a, b), "a", "b", 3)
	if n := strings.Count(out, "@@ "); n != 2 {
		t.Errorf("expected 2 hunks, got %d:\n%s", n, out)
	}
}

func TestUnifiedZeroContext(t *testing.T) {
	a := []string{"1", "2", "3"}
	b := []string{"1", "X", "3"}
	out := unifiedDiff(buildEditScript(a, b), "a", "b", 0)
	if !strings.Contains(out, "@@ -2 +2 @@") {
		t.Errorf("expected single-line hunk header @@ -2 +2 @@:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// CLI end to end
// ---------------------------------------------------------------------------

func runCLI(t *testing.T, args []string, stdin string) (int, string, string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code := cli(args, strings.NewReader(stdin), &out, &errBuf)
	return code, out.String(), errBuf.String()
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := dir + "/" + name
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func TestCLIIdenticalExit0(t *testing.T) {
	f1 := writeTemp(t, "a.txt", "x\ny\nz\n")
	f2 := writeTemp(t, "b.txt", "x\ny\nz\n")
	code, out, _ := runCLI(t, []string{f1, f2}, "")
	if code != 0 {
		t.Errorf("identical files: exit=%d want 0", code)
	}
	if out != "" {
		t.Errorf("identical files produced output: %q", out)
	}
}

func TestCLIDifferExit1(t *testing.T) {
	f1 := writeTemp(t, "a.txt", "x\ny\n")
	f2 := writeTemp(t, "b.txt", "x\nY\n")
	code, out, _ := runCLI(t, []string{"-u", f1, f2}, "")
	if code != 1 {
		t.Errorf("differing files: exit=%d want 1", code)
	}
	if !strings.Contains(out, "-y") || !strings.Contains(out, "+Y") {
		t.Errorf("unexpected unified output:\n%s", out)
	}
}

func TestCLIMissingFileExit2(t *testing.T) {
	f1 := writeTemp(t, "a.txt", "x\n")
	code, _, errOut := runCLI(t, []string{f1, "/no/such/file"}, "")
	if code != 2 {
		t.Errorf("missing file: exit=%d want 2", code)
	}
	if !strings.Contains(errOut, "diff:") {
		t.Errorf("expected error message, got %q", errOut)
	}
}

func TestCLIBadFlagExit2(t *testing.T) {
	code, _, _ := runCLI(t, []string{"-z", "a", "b"}, "")
	if code != 2 {
		t.Errorf("bad flag: exit=%d want 2", code)
	}
}

func TestCLIStdin(t *testing.T) {
	f2 := writeTemp(t, "b.txt", "hello\nworld\n")
	code, out, _ := runCLI(t, []string{"-u", "-", f2}, "hello\nplanet\n")
	if code != 1 {
		t.Errorf("stdin diff: exit=%d want 1", code)
	}
	if !strings.Contains(out, "@@ ") {
		t.Errorf("expected a hunk header:\n%s", out)
	}
	if !strings.Contains(out, "+world") || !strings.Contains(out, "-planet") {
		t.Errorf("unexpected stdin diff output:\n%s", out)
	}
}
