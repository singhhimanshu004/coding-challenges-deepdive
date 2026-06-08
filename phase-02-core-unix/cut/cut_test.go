package main

import (
	"bytes"
	"strings"
	"testing"
)

// helper: build a config for field mode from a LIST string.
func fieldCfg(t *testing.T, list, delim string, suppress bool) config {
	t.Helper()
	sel, err := ParseList(list)
	if err != nil {
		t.Fatalf("ParseList(%q) failed: %v", list, err)
	}
	return config{mode: modeFields, sel: sel, delim: delim, suppress: suppress}
}

// helper: build a config for char mode from a LIST string.
func charCfg(t *testing.T, list string) config {
	t.Helper()
	sel, err := ParseList(list)
	if err != nil {
		t.Fatalf("ParseList(%q) failed: %v", list, err)
	}
	return config{mode: modeChars, sel: sel}
}

func runStr(t *testing.T, in string, cfg config) string {
	t.Helper()
	var out bytes.Buffer
	if err := run(strings.NewReader(in), &out, cfg); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	return out.String()
}

func TestParseListSingleAndRanges(t *testing.T) {
	cases := []struct {
		list string
		in   int
		want bool
	}{
		{"1,3", 1, true},
		{"1,3", 2, false},
		{"1,3", 3, true},
		{"2-4", 1, false},
		{"2-4", 2, true},
		{"2-4", 4, true},
		{"2-4", 5, false},
		{"-3", 1, true},
		{"-3", 3, true},
		{"-3", 4, false},
		{"2-", 1, false},
		{"2-", 2, true},
		{"2-", 999, true},
		{"1,4-6,9", 5, true},
		{"1,4-6,9", 7, false},
	}
	for _, c := range cases {
		sel, err := ParseList(c.list)
		if err != nil {
			t.Fatalf("ParseList(%q): %v", c.list, err)
		}
		if got := sel.contains(c.in); got != c.want {
			t.Errorf("ParseList(%q).contains(%d) = %v, want %v", c.list, c.in, got, c.want)
		}
	}
}

func TestParseListErrors(t *testing.T) {
	bad := []string{"", "0", "-", "a", "3-1", "1,,3", "2-b", "1-2-3", "-0"}
	for _, list := range bad {
		if _, err := ParseList(list); err == nil {
			t.Errorf("ParseList(%q) expected error, got nil", list)
		}
	}
}

func TestFieldListTabDefault(t *testing.T) {
	in := "a\tb\tc\td\n"
	got := runStr(t, in, fieldCfg(t, "1,3", "\t", false))
	want := "a\tc\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFieldRange(t *testing.T) {
	in := "1\t2\t3\t4\t5\n"
	got := runStr(t, in, fieldCfg(t, "2-4", "\t", false))
	want := "2\t3\t4\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFieldOpenEndedRange(t *testing.T) {
	in := "1\t2\t3\t4\n"
	got := runStr(t, in, fieldCfg(t, "3-", "\t", false))
	want := "3\t4\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// cut emits fields in input order regardless of how the LIST is written.
func TestFieldOrderIsInputOrder(t *testing.T) {
	in := "a\tb\tc\n"
	got := runStr(t, in, fieldCfg(t, "3,1", "\t", false))
	want := "a\tc\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCustomDelimiter(t *testing.T) {
	in := "a,b,c,d\n"
	got := runStr(t, in, fieldCfg(t, "2,4", ",", false))
	want := "b,d\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCharRanges(t *testing.T) {
	in := "abcdef\n"
	got := runStr(t, in, charCfg(t, "2-4"))
	want := "bcd\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCharListUnicode(t *testing.T) {
	// "héllo": positions are by rune, so 1=h 2=é 3=l ...
	in := "héllo\n"
	got := runStr(t, in, charCfg(t, "1-2"))
	want := "hé\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A line without the delimiter is printed unchanged by default...
func TestMissingDelimiterDefault(t *testing.T) {
	in := "no-delimiter-here\n"
	got := runStr(t, in, fieldCfg(t, "1", "\t", false))
	want := "no-delimiter-here\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ...but suppressed entirely with -s.
func TestMissingDelimiterSuppressed(t *testing.T) {
	in := "no-delim\na\tb\n"
	got := runStr(t, in, fieldCfg(t, "1", "\t", true))
	want := "a\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStdinMultipleLines(t *testing.T) {
	in := "a:b:c\nd:e:f\n"
	got := runStr(t, in, fieldCfg(t, "1,3", ":", false))
	want := "a:c\nd:f\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEmptyInput(t *testing.T) {
	got := runStr(t, "", fieldCfg(t, "1", "\t", false))
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// A line missing high-numbered fields just yields the fields that exist.
func TestFieldBeyondAvailable(t *testing.T) {
	in := "a\tb\n"
	got := runStr(t, in, fieldCfg(t, "2-5", "\t", false))
	want := "b\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseArgsAttachedAndSeparated(t *testing.T) {
	// Attached forms: -f1,3 and -d,
	cfg, files, err := parseArgs([]string{"-f1,3", "-d,", "file.txt"})
	if err != nil {
		t.Fatalf("parseArgs error: %v", err)
	}
	if cfg.mode != modeFields || cfg.delim != "," {
		t.Errorf("unexpected cfg: %+v", cfg)
	}
	if len(files) != 1 || files[0] != "file.txt" {
		t.Errorf("unexpected files: %v", files)
	}

	// Separated forms: -f 1,3 -d ,
	cfg2, _, err := parseArgs([]string{"-f", "1,3", "-d", ","})
	if err != nil {
		t.Fatalf("parseArgs error: %v", err)
	}
	if cfg2.delim != "," {
		t.Errorf("expected delim ',', got %q", cfg2.delim)
	}
}

func TestParseArgsErrors(t *testing.T) {
	bad := [][]string{
		{},                      // no -f/-c
		{"-f", "1", "-c", "1"},  // mutually exclusive
		{"-c", "1", "-d", ","},  // -d only with -f
		{"-c", "1", "-s"},       // -s only with -f
		{"-d", "ab", "-f", "1"}, // multi-char delimiter
		{"-z"},                  // unknown option
		{"-f"},                  // missing value
	}
	for _, args := range bad {
		if _, _, err := parseArgs(args); err == nil {
			t.Errorf("parseArgs(%v) expected error, got nil", args)
		}
	}
}
