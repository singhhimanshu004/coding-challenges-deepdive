package main

import "time"

// ----------------------------------------------------------------------------
// The schedule / next-time calculator.
//
// Once a cron string is parsed into five bitsets, computing the "next run time"
// is a search: start one minute after the reference time and walk forward until
// every field matches. The trick to doing it fast is to skip in big jumps —
// if the MONTH doesn't match, leap to the first day of the next month rather
// than crawling minute by minute.
// ----------------------------------------------------------------------------

// matchTimeLimit caps the forward search so a pathological (never-matching)
// expression like "0 0 30 2 *" (Feb 30th — impossible) returns instead of
// looping forever. Five years comfortably covers every real schedule.
const matchTimeLimit = 5 * 366 * 24 * time.Hour

// Matches reports whether time t (to the minute) satisfies the schedule.
//
// 🐍➡️🐹 Each `.has(...)` call is just a set-membership test, so this reads like
// `t.minute in minutes and t.hour in hours and ...` would in Python.
func (s *Schedule) Matches(t time.Time) bool {
	return s.minute.has(t.Minute()) &&
		s.hour.has(t.Hour()) &&
		s.month.has(int(t.Month())) &&
		s.dayMatches(t)
}

// dayMatches implements cron's notorious day-of-month vs. day-of-week rule.
//
// THE GOTCHA: when BOTH the day-of-month and day-of-week fields are restricted
// (neither is "*"), a day matches if EITHER field matches — it is an OR, not an
// AND. So "0 0 13 * 5" fires on the 13th of every month AND on every Friday.
//
// When one of the two fields is "*", that field imposes no constraint and the
// other one alone decides — which collapses back to ordinary AND behaviour.
func (s *Schedule) dayMatches(t time.Time) bool {
	domOK := s.dom.has(t.Day())
	dowOK := s.dow.has(int(t.Weekday())) // time.Weekday: Sunday=0 .. Saturday=6

	switch {
	case s.domStar && s.dowStar:
		// "* ... *" — every day qualifies.
		return true
	case s.domStar:
		// Only day-of-week is restricted.
		return dowOK
	case s.dowStar:
		// Only day-of-month is restricted.
		return domOK
	default:
		// Both restricted: the OR rule.
		return domOK || dowOK
	}
}

// Next returns the first time strictly after `after` that matches the schedule.
// The result is always truncated to a whole minute (cron has minute resolution).
// If nothing matches within matchTimeLimit it returns the zero time and false.
func (s *Schedule) Next(after time.Time) (time.Time, bool) {
	// Cron fires at the top of a minute, so drop sub-minute precision and step
	// to the next minute — "strictly after" means we never return `after` itself.
	t := after.Truncate(time.Minute).Add(time.Minute)
	deadline := t.Add(matchTimeLimit)

	for t.Before(deadline) {
		switch {
		case !s.month.has(int(t.Month())):
			// Wrong month: jump to 00:00 on the 1st of the next month.
			t = startOfNextMonth(t)
		case !s.dayMatches(t):
			// Wrong day: jump to 00:00 tomorrow.
			t = startOfNextDay(t)
		case !s.hour.has(t.Hour()):
			// Wrong hour: jump to the start of the next hour.
			t = startOfNextHour(t)
		case !s.minute.has(t.Minute()):
			// Wrong minute: step forward one minute.
			t = t.Add(time.Minute)
		default:
			// Every field matched.
			return t, true
		}
	}
	return time.Time{}, false
}

// NextN returns the next n run times after `after`, in chronological order.
// It feeds each result back in as the new reference, so successive calls to
// Next naturally advance through the schedule.
func (s *Schedule) NextN(after time.Time, n int) []time.Time {
	out := make([]time.Time, 0, n)
	cur := after
	for i := 0; i < n; i++ {
		next, ok := s.Next(cur)
		if !ok {
			break
		}
		out = append(out, next)
		cur = next
	}
	return out
}

// ----------------------------------------------------------------------------
// Small calendar-jump helpers. Each returns a time aligned to the start of the
// next month / day / hour, preserving the original location (time zone). Using
// time.Date keeps day/month arithmetic correct across rollovers automatically —
// e.g. month 12 + 1 becomes January of the next year.
// ----------------------------------------------------------------------------

func startOfNextMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location()).AddDate(0, 1, 0)
}

func startOfNextDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).AddDate(0, 0, 1)
}

func startOfNextHour(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location()).Add(time.Hour)
}
