package sed

import "testing"

func TestConvertReplacement(t *testing.T) {
	cases := []struct{ in, want string }{
		{`\1`, "${1}"},
		{`\2 \1`, "${2} ${1}"},
		{`&`, "${0}"},
		{`[&]`, "[${0}]"},
		{`\&`, "&"},
		{`$5`, "$$5"},
		{`a\nb`, "a\nb"},
		{`\\`, `\`},
	}
	for _, c := range cases {
		if got := convertReplacement(c.in); got != c.want {
			t.Errorf("convertReplacement(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	for _, src := range []string{"s/a", "s/a/b", "2z", "1,d"} {
		if _, err := Parse(src); err == nil {
			t.Errorf("Parse(%q) expected error, got nil", src)
		}
	}
}

func TestSubstituteFirstVsGlobal(t *testing.T) {
	cmds, err := Parse("s/o/0/")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := cmds[0].substitute("foo"); got != "f0o" {
		t.Errorf("first-only substitute = %q, want %q", got, "f0o")
	}

	gcmds, _ := Parse("s/o/0/g")
	if got, _ := gcmds[0].substitute("foo"); got != "f00" {
		t.Errorf("global substitute = %q, want %q", got, "f00")
	}
}

// TestSingleLineRangeViaNumericEnd checks the range state machine closes
// immediately when the numeric end address is not after the start.
func TestSingleLineRangeViaNumericEnd(t *testing.T) {
	cmds, _ := Parse("2,2d")
	c := cmds[0]
	if c.applies(1, "a", false) {
		t.Error("line 1 should not be in range 2,2")
	}
	if !c.applies(2, "b", false) {
		t.Error("line 2 should be in range 2,2")
	}
	if c.applies(3, "c", false) {
		t.Error("line 3 should not be in range 2,2 (range must have closed)")
	}
}

func TestSplitLines(t *testing.T) {
	if got := SplitLines("a\nb\nc\n"); len(got) != 3 {
		t.Errorf("SplitLines trailing newline = %v, want 3 lines", got)
	}
	if got := SplitLines("a\nb"); len(got) != 2 {
		t.Errorf("SplitLines no trailing newline = %v, want 2 lines", got)
	}
	if got := SplitLines(""); got != nil {
		t.Errorf("SplitLines empty = %v, want nil", got)
	}
}
