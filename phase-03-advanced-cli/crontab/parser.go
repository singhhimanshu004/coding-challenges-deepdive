package main

import (
	"fmt"
	"strings"
)

// ----------------------------------------------------------------------------
// The cron expression parser.
//
// A standard cron expression has FIVE space-separated fields:
//
//	┌──────────── minute        (0-59)
//	│ ┌────────── hour          (0-23)
//	│ │ ┌──────── day-of-month  (1-31)
//	│ │ │ ┌────── month         (1-12 or JAN-DEC)
//	│ │ │ │ ┌──── day-of-week   (0-6  or SUN-SAT; 0 and 7 both mean Sunday)
//	│ │ │ │ │
//	* * * * *
//
// Each field is parsed into a `field` — a bitset where "value v is allowed"
// is stored as bit v. This is the heart of the whole design: once a field is
// a bitset, asking "does the time T match?" is a single bit test.
//
// 🐍➡️🐹 Python analogy: a `field` is basically a `set[int]` of allowed values,
// but packed into a single 64-bit integer for speed. `f.has(5)` is the Go
// equivalent of `5 in allowed_minutes`.
// ----------------------------------------------------------------------------

// field is a bitset of the integer values a single cron field permits.
// bit i (counting from the least-significant bit) is set when value i matches.
// A uint64 is wide enough: the largest value we ever store is 59 (minutes).
type field struct {
	bits   uint64 // allowed values, one bit per value
	isStar bool   // true when the original token was a bare "*" (unrestricted)
}

// has reports whether value v is permitted by this field.
func (f field) has(v int) bool {
	if v < 0 || v > 63 {
		return false
	}
	return f.bits&(1<<uint(v)) != 0
}

// set marks value v as allowed.
func (f *field) set(v int) {
	f.bits |= 1 << uint(v)
}

// bounds describes the legal numeric range of one cron field, plus an optional
// table of three-letter names (months, weekdays) the field accepts.
type bounds struct {
	min, max int
	names    map[string]int // e.g. "JAN" -> 1; nil when the field has no names
}

var (
	minuteBounds = bounds{0, 59, nil}
	hourBounds   = bounds{0, 23, nil}
	domBounds    = bounds{1, 31, nil}
	monthBounds  = bounds{1, 12, monthNames}
	dowBounds    = bounds{0, 7, dayNames} // 7 is accepted as an alias for Sunday(0)

	monthNames = map[string]int{
		"JAN": 1, "FEB": 2, "MAR": 3, "APR": 4, "MAY": 5, "JUN": 6,
		"JUL": 7, "AUG": 8, "SEP": 9, "OCT": 10, "NOV": 11, "DEC": 12,
	}
	dayNames = map[string]int{
		"SUN": 0, "MON": 1, "TUE": 2, "WED": 3, "THU": 4, "FRI": 5, "SAT": 6,
	}
)

// macros maps the convenience shorthands (e.g. @daily) to their equivalent
// five-field cron expression. These are expanded before normal parsing.
var macros = map[string]string{
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
	"@monthly":  "0 0 1 * *",
	"@weekly":   "0 0 * * 0",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@hourly":   "0 * * * *",
}

// Schedule is a fully parsed cron expression: one bitset per field.
//
// The two boolean flags `domStar` / `dowStar` record whether the day-of-month
// and day-of-week fields were the bare "*". They exist solely to implement the
// famous OR rule — see dayMatches in schedule.go.
type Schedule struct {
	minute, hour, dom, month, dow field
	domStar, dowStar              bool
	expr                          string // the original (post-macro) expression, for display
}

// Parse turns a cron string into a Schedule, or returns a descriptive error.
//
// It accepts either a macro ("@daily") or a five-field expression
// ("*/15 9-17 * * 1-5").
func Parse(expr string) (*Schedule, error) {
	trimmed := strings.TrimSpace(expr)
	if trimmed == "" {
		return nil, fmt.Errorf("empty cron expression")
	}

	// Expand a leading macro (e.g. "@weekly") into its five-field form.
	if strings.HasPrefix(trimmed, "@") {
		expanded, ok := macros[strings.ToLower(trimmed)]
		if !ok {
			return nil, fmt.Errorf("unknown macro %q (try @hourly, @daily, @weekly, @monthly, @yearly)", trimmed)
		}
		trimmed = expanded
	}

	// strings.Fields splits on ANY run of whitespace and drops empties, so
	// "0   0 * * *" (extra spaces) parses just fine.
	parts := strings.Fields(trimmed)
	if len(parts) != 5 {
		return nil, fmt.Errorf("expected 5 fields, got %d in %q", len(parts), expr)
	}

	s := &Schedule{expr: trimmed}
	var err error

	if s.minute, err = parseField(parts[0], minuteBounds); err != nil {
		return nil, fmt.Errorf("minute: %w", err)
	}
	if s.hour, err = parseField(parts[1], hourBounds); err != nil {
		return nil, fmt.Errorf("hour: %w", err)
	}
	if s.dom, err = parseField(parts[2], domBounds); err != nil {
		return nil, fmt.Errorf("day-of-month: %w", err)
	}
	if s.month, err = parseField(parts[3], monthBounds); err != nil {
		return nil, fmt.Errorf("month: %w", err)
	}
	if s.dow, err = parseField(parts[4], dowBounds); err != nil {
		return nil, fmt.Errorf("day-of-week: %w", err)
	}

	s.domStar = parts[2] == "*"
	s.dowStar = parts[4] == "*"
	return s, nil
}

// parseField parses one whole field, which may be a comma-separated LIST of
// terms (e.g. "1,2,5-7,*/10"). Each term is parsed independently and the
// resulting bitsets are OR'd together.
func parseField(token string, b bounds) (field, error) {
	var f field
	if token == "*" {
		f.isStar = true
	}
	for _, term := range strings.Split(token, ",") {
		if term == "" {
			return field{}, fmt.Errorf("empty term in %q", token)
		}
		if err := parseTerm(term, b, &f); err != nil {
			return field{}, err
		}
	}
	return f, nil
}

// parseTerm parses a single comma-free term and ORs its values into f.
//
// Supported shapes:
//
//   - every value in range
//     */step     every step-th value across the whole range
//     a          a single value
//     a-b        an inclusive range
//     a-b/step   every step-th value within the range
//     a/step     shorthand for a-max/step (from a to the field's maximum)
//
// Values may be numeric or, for month/day-of-week fields, three-letter names.
func parseTerm(term string, b bounds, f *field) error {
	// Split off an optional "/step" suffix.
	rangePart := term
	step := 1
	if slash := strings.IndexByte(term, '/'); slash >= 0 {
		rangePart = term[:slash]
		stepStr := term[slash+1:]
		n, err := parseValue(stepStr, b)
		// A step is a plain positive integer, never a name, and must be >= 1.
		if err != nil || n < 1 || stepStr == "" {
			return fmt.Errorf("invalid step %q in %q", stepStr, term)
		}
		step = n
	}

	// Work out the low/high bounds the step iterates over.
	var lo, hi int
	switch {
	case rangePart == "*":
		// "*" or "*/step": the whole legal range.
		lo, hi = b.min, b.max
	case strings.IndexByte(rangePart, '-') > 0:
		// "a-b" or "a-b/step": an explicit inclusive range.
		dash := strings.IndexByte(rangePart, '-')
		var err error
		if lo, err = parseValue(rangePart[:dash], b); err != nil {
			return err
		}
		if hi, err = parseValue(rangePart[dash+1:], b); err != nil {
			return err
		}
		if lo > hi {
			return fmt.Errorf("range start %d is after end %d in %q", lo, hi, term)
		}
	default:
		// A bare single value, e.g. "5" or "MON".
		v, err := parseValue(rangePart, b)
		if err != nil {
			return err
		}
		if step == 1 {
			f.set(normalize(v, b))
			return nil
		}
		// "a/step" means "from a to the maximum, every step".
		lo, hi = v, b.max
	}

	// Stamp every step-th value in [lo, hi] into the bitset.
	for v := lo; v <= hi; v += step {
		f.set(normalize(v, b))
	}
	return nil
}

// parseValue parses a single numeric value or named value, validating it lies
// within the field's bounds.
func parseValue(s string, b bounds) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty value")
	}
	// Try a name first (months / weekdays are case-insensitive).
	if b.names != nil {
		if v, ok := b.names[strings.ToUpper(s)]; ok {
			return v, nil
		}
	}
	// Otherwise it must be a non-negative integer.
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid value %q", s)
		}
		n = n*10 + int(r-'0')
	}
	if n < b.min || n > b.max {
		return 0, fmt.Errorf("value %d out of range %d-%d", n, b.min, b.max)
	}
	return n, nil
}

// normalize collapses the day-of-week alias 7 (Sunday) down to 0, so the bitset
// only ever stores 0-6. Go's time.Weekday() also uses Sunday=0, which lines the
// two up perfectly at match time. All other fields pass through unchanged.
func normalize(v int, b bounds) int {
	if b.max == 7 && v == 7 { // the day-of-week field
		return 0
	}
	return v
}
