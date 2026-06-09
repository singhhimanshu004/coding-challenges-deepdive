package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// utc is a helper to build a clear, time-zone-stable reference time.
func utc(y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, time.UTC)
}

// ---------------------------------------------------------------------------
// Field parsing: stars, single values, lists, ranges, steps, named values.
// ---------------------------------------------------------------------------

func mustParse(t *testing.T, expr string) *Schedule {
	t.Helper()
	s, err := Parse(expr)
	if err != nil {
		t.Fatalf("Parse(%q) unexpected error: %v", expr, err)
	}
	return s
}

func TestParseStar(t *testing.T) {
	s := mustParse(t, "* * * * *")
	for v := 0; v <= 59; v++ {
		if !s.minute.has(v) {
			t.Fatalf("minute * should match %d", v)
		}
	}
	if !s.minute.isStar || !s.domStar || !s.dowStar {
		t.Fatal("star flags should be set for *")
	}
}

func TestParseSingleAndList(t *testing.T) {
	s := mustParse(t, "1,2,5 * * * *")
	for v := 0; v <= 59; v++ {
		want := v == 1 || v == 2 || v == 5
		if s.minute.has(v) != want {
			t.Fatalf("minute %d: has=%v want=%v", v, s.minute.has(v), want)
		}
	}
}

func TestParseRange(t *testing.T) {
	s := mustParse(t, "* 9-17 * * *")
	for v := 0; v <= 23; v++ {
		want := v >= 9 && v <= 17
		if s.hour.has(v) != want {
			t.Fatalf("hour %d: has=%v want=%v", v, s.hour.has(v), want)
		}
	}
}

func TestParseStarStep(t *testing.T) {
	s := mustParse(t, "*/15 * * * *")
	want := map[int]bool{0: true, 15: true, 30: true, 45: true}
	for v := 0; v <= 59; v++ {
		if s.minute.has(v) != want[v] {
			t.Fatalf("minute %d: has=%v want=%v", v, s.minute.has(v), want[v])
		}
	}
}

func TestParseRangeStep(t *testing.T) {
	// 10-30/5 -> 10,15,20,25,30
	s := mustParse(t, "10-30/5 * * * *")
	want := map[int]bool{10: true, 15: true, 20: true, 25: true, 30: true}
	for v := 0; v <= 59; v++ {
		if s.minute.has(v) != want[v] {
			t.Fatalf("minute %d: has=%v want=%v", v, s.minute.has(v), want[v])
		}
	}
}

func TestParseSingleStep(t *testing.T) {
	// 5/20 -> 5,25,45 (from 5 to 59 step 20)
	s := mustParse(t, "5/20 * * * *")
	want := map[int]bool{5: true, 25: true, 45: true}
	for v := 0; v <= 59; v++ {
		if s.minute.has(v) != want[v] {
			t.Fatalf("minute %d: has=%v want=%v", v, s.minute.has(v), want[v])
		}
	}
}

func TestParseNamedMonths(t *testing.T) {
	s := mustParse(t, "* * * JAN,MAR,DEC *")
	for m := 1; m <= 12; m++ {
		want := m == 1 || m == 3 || m == 12
		if s.month.has(m) != want {
			t.Fatalf("month %d: has=%v want=%v", m, s.month.has(m), want)
		}
	}
}

func TestParseNamedDaysRangeCaseInsensitive(t *testing.T) {
	s := mustParse(t, "* * * * mon-fri")
	for d := 0; d <= 6; d++ {
		want := d >= 1 && d <= 5
		if s.dow.has(d) != want {
			t.Fatalf("dow %d: has=%v want=%v", d, s.dow.has(d), want)
		}
	}
}

func TestParseSundayAlias7(t *testing.T) {
	// 7 must be accepted and normalized to 0 (Sunday).
	s := mustParse(t, "* * * * 7")
	if !s.dow.has(0) {
		t.Fatal("dow 7 should normalize to Sunday(0)")
	}
}

// ---------------------------------------------------------------------------
// Macros.
// ---------------------------------------------------------------------------

func TestMacros(t *testing.T) {
	cases := map[string]string{
		"@hourly":  "0 * * * *",
		"@daily":   "0 0 * * *",
		"@weekly":  "0 0 * * 0",
		"@monthly": "0 0 1 * *",
		"@yearly":  "0 0 1 1 *",
	}
	for macro, equiv := range cases {
		a := mustParse(t, macro)
		b := mustParse(t, equiv)
		if a.minute != b.minute || a.hour != b.hour || a.dom != b.dom ||
			a.month != b.month || a.dow != b.dow {
			t.Fatalf("macro %s should equal %q", macro, equiv)
		}
	}
}

// ---------------------------------------------------------------------------
// Invalid expressions.
// ---------------------------------------------------------------------------

func TestInvalidExpressions(t *testing.T) {
	bad := []string{
		"",             // empty
		"* * * *",      // too few fields
		"* * * * * *",  // too many fields
		"60 * * * *",   // minute out of range
		"* 24 * * *",   // hour out of range
		"* * 0 * *",    // day-of-month below min
		"* * * 13 *",   // month out of range
		"* * * * 8",    // dow above 7
		"*/0 * * * *",  // zero step
		"5-1 * * * *",  // reversed range
		"* * * FOO *",  // bad month name
		"1,,2 * * * *", // empty list term
		"@nonsense",    // unknown macro
		"abc * * * *",  // non-numeric
	}
	for _, expr := range bad {
		if _, err := Parse(expr); err == nil {
			t.Fatalf("Parse(%q) should have failed", expr)
		}
	}
}

// ---------------------------------------------------------------------------
// Next-run computation, including rollovers and the dom/dow OR rule.
// ---------------------------------------------------------------------------

func TestNextSimple(t *testing.T) {
	s := mustParse(t, "*/15 * * * *")
	got, ok := s.Next(utc(2026, 6, 9, 9, 7))
	if !ok {
		t.Fatal("expected a next time")
	}
	if want := utc(2026, 6, 9, 9, 15); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestNextExactBoundaryIsStrictlyAfter(t *testing.T) {
	// At exactly 09:15 the next */15 run must be 09:30, not 09:15 again.
	s := mustParse(t, "*/15 * * * *")
	got, _ := s.Next(utc(2026, 6, 9, 9, 15))
	if want := utc(2026, 6, 9, 9, 30); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestNextBusinessHours(t *testing.T) {
	// */15 9-17 * * 1-5 : Friday 2026-06-12 17:50 -> next is Monday 09:00.
	s := mustParse(t, "*/15 9-17 * * 1-5")
	got, _ := s.Next(utc(2026, 6, 12, 17, 50)) // Friday
	if want := utc(2026, 6, 15, 9, 0); !got.Equal(want) {
		t.Fatalf("got %v want %v (should roll to Monday)", got, want)
	}
}

func TestNextMonthRollover(t *testing.T) {
	// 0 0 1 * * : midnight on the 1st. From mid-June -> 1 July 00:00.
	s := mustParse(t, "0 0 1 * *")
	got, _ := s.Next(utc(2026, 6, 9, 12, 0))
	if want := utc(2026, 7, 1, 0, 0); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestNextYearRollover(t *testing.T) {
	// 0 0 1 1 * : midnight Jan 1st. From mid-2026 -> 1 Jan 2027.
	s := mustParse(t, "@yearly")
	got, _ := s.Next(utc(2026, 6, 9, 0, 0))
	if want := utc(2027, 1, 1, 0, 0); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestNextDomDowOrRule(t *testing.T) {
	// 0 0 13 * 5 : the 13th OR any Friday (both fields restricted => OR).
	s := mustParse(t, "0 0 13 * 5")
	// 2026-06-09 is a Tuesday. Next match should be Friday 2026-06-12 (a Friday),
	// which comes before the 13th.
	got, _ := s.Next(utc(2026, 6, 9, 0, 0))
	if want := utc(2026, 6, 12, 0, 0); !got.Equal(want) {
		t.Fatalf("OR rule: got %v want %v", got, want)
	}
	// From the 12th onward, the 13th should match as the day-of-month branch.
	got2, _ := s.Next(utc(2026, 6, 12, 0, 0))
	if want := utc(2026, 6, 13, 0, 0); !got2.Equal(want) {
		t.Fatalf("OR rule (dom branch): got %v want %v", got2, want)
	}
}

func TestNextDomOnlyRestricted(t *testing.T) {
	// 0 0 15 * * : dow is *, so only the 15th matters (no OR).
	s := mustParse(t, "0 0 15 * *")
	got, _ := s.Next(utc(2026, 6, 1, 0, 0))
	if want := utc(2026, 6, 15, 0, 0); !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestNextImpossibleExpression(t *testing.T) {
	// Feb 30th never exists -> no run time.
	s := mustParse(t, "0 0 30 2 *")
	if _, ok := s.Next(utc(2026, 1, 1, 0, 0)); ok {
		t.Fatal("Feb 30th should never match")
	}
}

func TestNextN(t *testing.T) {
	s := mustParse(t, "*/15 9-17 * * 1-5")
	runs := s.NextN(utc(2026, 6, 9, 8, 0), 5) // Tuesday 08:00
	want := []time.Time{
		utc(2026, 6, 9, 9, 0),
		utc(2026, 6, 9, 9, 15),
		utc(2026, 6, 9, 9, 30),
		utc(2026, 6, 9, 9, 45),
		utc(2026, 6, 9, 10, 0),
	}
	if len(runs) != len(want) {
		t.Fatalf("got %d runs want %d", len(runs), len(want))
	}
	for i := range want {
		if !runs[i].Equal(want[i]) {
			t.Fatalf("run %d: got %v want %v", i, runs[i], want[i])
		}
	}
}

func TestMatches(t *testing.T) {
	s := mustParse(t, "30 9 * * 1")         // 09:30 on Mondays
	if !s.Matches(utc(2026, 6, 8, 9, 30)) { // 2026-06-08 is a Monday
		t.Fatal("should match Monday 09:30")
	}
	if s.Matches(utc(2026, 6, 9, 9, 30)) { // Tuesday
		t.Fatal("should not match Tuesday")
	}
}

// ---------------------------------------------------------------------------
// Explain output.
// ---------------------------------------------------------------------------

func TestExplainMentionsOrRule(t *testing.T) {
	s := mustParse(t, "0 0 13 * 5")
	out := s.Explain()
	if !strings.Contains(out, "OR rule") {
		t.Fatalf("Explain should warn about the OR rule:\n%s", out)
	}
	if !strings.Contains(out, "Friday") {
		t.Fatalf("Explain should name Friday:\n%s", out)
	}
}

func TestExplainRangesCollapse(t *testing.T) {
	s := mustParse(t, "* 9-17 * * *")
	out := s.Explain()
	if !strings.Contains(out, "9-17") {
		t.Fatalf("Explain should collapse 9..17 into 9-17:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// CLI run().
// ---------------------------------------------------------------------------

func TestRunNextN(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"-n", "3", "-from", "2026-06-09T08:00:00Z", "*/15 9-17 * * 1-5"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "Next 3 run(s)") {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}

func TestRunExplain(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"-explain", "@daily"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "Day of week") {
		t.Fatalf("explain output missing field breakdown:\n%s", out.String())
	}
}

func TestRunUnquotedExpression(t *testing.T) {
	// Five separate shell words should still be joined into one expression.
	var out, errBuf bytes.Buffer
	code := run([]string{"-from", "2026-06-09T08:00:00Z", "*/15", "9-17", "*", "*", "1-5"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errBuf.String())
	}
}

func TestRunBadExpression(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"99 * * * *"}, &out, &errBuf)
	if code != 2 {
		t.Fatalf("expected exit 2 for bad expression, got %d", code)
	}
}

func TestRunNoArgs(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run(nil, &out, &errBuf)
	if code != 2 {
		t.Fatalf("expected exit 2 for missing expression, got %d", code)
	}
}

func TestRunHelp(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"-h"}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("help should exit 0, got %d", code)
	}
	if !strings.Contains(out.String(), "USAGE") {
		t.Fatalf("help output missing usage:\n%s", out.String())
	}
}
