package main

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

// run drives the full CLI with the given args and stdin, returning stdout and
// the exit status. This is the same surface a shell user touches.
func run(t *testing.T, args []string, stdin string) (string, int) {
	t.Helper()
	var out, errOut bytes.Buffer
	status := cli(args, strings.NewReader(stdin), &out, &errOut)
	return out.String(), status
}

func TestLexicographicDefault(t *testing.T) {
	in := "banana\napple\ncherry\n"
	got, status := run(t, nil, in)
	want := "apple\nbanana\ncherry\n"
	if status != 0 {
		t.Fatalf("status = %d, want 0", status)
	}
	if got != want {
		t.Errorf("default sort = %q, want %q", got, want)
	}
}

func TestReverse(t *testing.T) {
	in := "apple\nbanana\ncherry\n"
	got, _ := run(t, []string{"-r"}, in)
	want := "cherry\nbanana\napple\n"
	if got != want {
		t.Errorf("-r sort = %q, want %q", got, want)
	}
}

func TestNumeric(t *testing.T) {
	in := "10\n2\n1\n100\n21\n"
	// Lexicographically "10" < "100" < "2"; numerically the order is different.
	got, _ := run(t, []string{"-n"}, in)
	want := "1\n2\n10\n21\n100\n"
	if got != want {
		t.Errorf("-n sort = %q, want %q", got, want)
	}
}

func TestNumericReverse(t *testing.T) {
	in := "10\n2\n1\n100\n21\n"
	got, _ := run(t, []string{"-n", "-r"}, in)
	want := "100\n21\n10\n2\n1\n"
	if got != want {
		t.Errorf("-nr sort = %q, want %q", got, want)
	}
}

func TestUnique(t *testing.T) {
	in := "b\na\nb\nc\na\na\n"
	got, _ := run(t, []string{"-u"}, in)
	want := "a\nb\nc\n"
	if got != want {
		t.Errorf("-u sort = %q, want %q", got, want)
	}
}

func TestFoldCase(t *testing.T) {
	in := "Banana\napple\nCherry\nBANANA\n"
	got, _ := run(t, []string{"-f"}, in)
	// With case folding the order is apple, Banana/BANANA, Cherry. The two
	// "banana"s are equal under -f; stable sort keeps input order (Banana first).
	want := "apple\nBanana\nBANANA\nCherry\n"
	if got != want {
		t.Errorf("-f sort = %q, want %q", got, want)
	}
}

func TestFoldUnique(t *testing.T) {
	in := "Foo\nfoo\nbar\nBAR\n"
	got, _ := run(t, []string{"-f", "-u"}, in)
	want := "bar\nFoo\n"
	if got != want {
		t.Errorf("-fu sort = %q, want %q", got, want)
	}
}

func TestKeyFieldWhitespace(t *testing.T) {
	// Sort on the second whitespace-delimited field.
	in := "alice 30\nbob 25\ncarol 40\n"
	got, _ := run(t, []string{"-k", "2", "-n"}, in)
	want := "bob 25\nalice 30\ncarol 40\n"
	if got != want {
		t.Errorf("-k2 -n sort = %q, want %q", got, want)
	}
}

func TestKeyFieldDelimiter(t *testing.T) {
	// CSV-style input sorted on field 1 with a comma separator.
	in := "charlie,3\nalpha,1\nbravo,2\n"
	got, _ := run(t, []string{"-t", ",", "-k", "1"}, in)
	want := "alpha,1\nbravo,2\ncharlie,3\n"
	if got != want {
		t.Errorf("-t, -k1 sort = %q, want %q", got, want)
	}
}

func TestStdinDefault(t *testing.T) {
	// No file operand -> read stdin.
	in := "3\n1\n2\n"
	got, status := run(t, []string{"-n"}, in)
	if status != 0 {
		t.Fatalf("status = %d, want 0", status)
	}
	if got != "1\n2\n3\n" {
		t.Errorf("stdin sort = %q", got)
	}
}

func TestEmptyInput(t *testing.T) {
	got, status := run(t, nil, "")
	if status != 0 {
		t.Fatalf("status = %d, want 0", status)
	}
	if got != "" {
		t.Errorf("empty input sort = %q, want empty", got)
	}
}

func TestFileOperand(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/data.txt"
	if err := os.WriteFile(path, []byte("gamma\nalpha\nbeta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, status := run(t, []string{path}, "")
	if status != 0 {
		t.Fatalf("status = %d, want 0", status)
	}
	if got != "alpha\nbeta\ngamma\n" {
		t.Errorf("file sort = %q", got)
	}
}

func TestMissingFile(t *testing.T) {
	_, status := run(t, []string{"does-not-exist.txt"}, "")
	if status != 1 {
		t.Errorf("missing file status = %d, want 1", status)
	}
}

func TestBadFlag(t *testing.T) {
	_, status := run(t, []string{"-z"}, "")
	if status != 2 {
		t.Errorf("bad flag status = %d, want 2", status)
	}
}

// TestExternalSortForced forces the on-disk path with a tiny chunk size so that
// many runs are produced and merged. The result must match a normal sort.
func TestExternalSortForced(t *testing.T) {
	// 50 shuffled numbers; chunk-lines=4 forces ~13 runs through the k-way merge.
	var lines []string
	for _, n := range []int{
		37, 4, 19, 50, 1, 23, 8, 42, 15, 30,
		2, 48, 11, 26, 39, 6, 17, 44, 33, 21,
		9, 28, 49, 13, 36, 3, 24, 46, 18, 31,
		7, 40, 12, 27, 5, 47, 16, 35, 22, 10,
		29, 45, 14, 38, 20, 34, 25, 43, 32, 41,
	} {
		lines = append(lines, fmt.Sprintf("%d", n))
	}
	in := strings.Join(lines, "\n") + "\n"

	got, status := run(t, []string{"--external", "--chunk-lines", "4", "-n"}, in)
	if status != 0 {
		t.Fatalf("status = %d, want 0", status)
	}

	// Build the expected numeric order independently.
	nums := make([]int, len(lines))
	for i := range lines {
		fmt.Sscanf(lines[i], "%d", &nums[i])
	}
	sort.Ints(nums)
	var wantB strings.Builder
	for _, n := range nums {
		fmt.Fprintf(&wantB, "%d\n", n)
	}
	if got != wantB.String() {
		t.Errorf("external sort mismatch\n got: %q\nwant: %q", got, wantB.String())
	}
}

// TestExternalMatchesInMemory checks the two code paths agree for the same input
// across several flag combinations — the strongest guarantee that external sort
// is correct is that it equals the simple in-memory sort.
func TestExternalMatchesInMemory(t *testing.T) {
	in := "Banana\napple\n10\n2\ncherry\nApple\n100\n2\nbanana\n1\n"
	cases := [][]string{
		{},
		{"-r"},
		{"-n"},
		{"-u"},
		{"-f"},
		{"-f", "-u"},
	}
	for _, base := range cases {
		mem, _ := run(t, base, in)
		ext, _ := run(t, append([]string{"--external", "--chunk-lines", "3"}, base...), in)
		if mem != ext {
			t.Errorf("paths disagree for flags %v\n mem: %q\n ext: %q", base, mem, ext)
		}
	}
}

func TestExternalUnique(t *testing.T) {
	// Duplicates spread across multiple runs must still be collapsed by -u.
	in := "c\na\nb\na\nc\nb\na\nd\n"
	got, _ := run(t, []string{"--external", "--chunk-lines", "2", "-u"}, in)
	want := "a\nb\nc\nd\n"
	if got != want {
		t.Errorf("external -u = %q, want %q", got, want)
	}
}

func TestLeadingNumberParsing(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"42", 42},
		{"  -7", -7},
		{"3.5kg", 3.5},
		{"abc", 0},
		{"", 0},
		{"+12", 12},
		{"10abc20", 10},
	}
	for _, c := range cases {
		if got := leadingNumber(c.in); got != c.want {
			t.Errorf("leadingNumber(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
