package main

import (
	"fmt"
	"strings"
)

// ----------------------------------------------------------------------------
// Human-readable explanation of a parsed schedule.
//
// Rather than parroting the raw expression back, we describe each field in plain
// English. We reconstruct the description from the BITSET, not the original
// text, which proves the parser understood the input.
// ----------------------------------------------------------------------------

var monthLabels = []string{
	"", "January", "February", "March", "April", "May", "June",
	"July", "August", "September", "October", "November", "December",
}

var dayLabels = []string{
	"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday",
}

// Explain returns a multi-line, human-friendly description of the schedule.
func (s *Schedule) Explain() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Expression: %s\n", s.expr)
	fmt.Fprintf(&b, "  Minute:       %s\n", describeField(s.minute, minuteBounds, numberLabel))
	fmt.Fprintf(&b, "  Hour:         %s\n", describeField(s.hour, hourBounds, numberLabel))
	fmt.Fprintf(&b, "  Day of month: %s\n", describeField(s.dom, domBounds, numberLabel))
	fmt.Fprintf(&b, "  Month:        %s\n", describeField(s.month, monthBounds, monthLabel))
	fmt.Fprintf(&b, "  Day of week:  %s\n", describeField(s.dow, dowBounds, dayLabel))
	if !s.domStar && !s.dowStar {
		b.WriteString("  Note: day-of-month and day-of-week are BOTH set, so a day matches when EITHER does (OR rule).\n")
	}
	return b.String()
}

// labeler turns a numeric field value into a display string.
type labeler func(int) string

func numberLabel(v int) string { return fmt.Sprintf("%d", v) }
func monthLabel(v int) string  { return monthLabels[v] }
func dayLabel(v int) string    { return dayLabels[v] }

// describeField renders one field's allowed values, collapsing consecutive runs
// into ranges (e.g. 9,10,11,12 -> "9-12") for readability.
func describeField(f field, b bounds, label labeler) string {
	if f.isStar {
		return "every value (*)"
	}

	// Collect the allowed values in ascending order. For day-of-week we only
	// ever store 0-6 (7 was normalized to 0 at parse time).
	var vals []int
	for v := b.min; v <= b.max; v++ {
		if f.has(v) {
			vals = append(vals, v)
		}
	}
	if len(vals) == 0 {
		return "(none)"
	}

	// Group consecutive integers into [start,end] runs.
	var parts []string
	start := vals[0]
	prev := vals[0]
	flush := func(lo, hi int) {
		if lo == hi {
			parts = append(parts, label(lo))
		} else {
			parts = append(parts, label(lo)+"-"+label(hi))
		}
	}
	for _, v := range vals[1:] {
		if v == prev+1 {
			prev = v
			continue
		}
		flush(start, prev)
		start, prev = v, v
	}
	flush(start, prev)

	return strings.Join(parts, ", ")
}
