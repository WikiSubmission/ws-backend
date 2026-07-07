package notify

import "time"

// NextRunAt returns the first occurrence of a schedule strictly after `after`,
// preserving the wall-clock time of startsAt in loc (a 9:00 PM daily reminder
// stays at 9:00 PM across DST transitions; a nonexistent spring-forward time
// normalizes forward one hour).
//
// Frequencies:
//   - once:     startsAt itself (the claim query disables the row afterwards)
//   - daily:    same wall-clock time every day
//   - weekly:   every 7 days from startsAt
//   - biweekly: every 14 days from startsAt (anchored — cadence never drifts)
//   - monthly:  same day-of-month as startsAt, clamped to shorter months; the
//     anchor day is re-derived each month so Jan 31 -> Feb 28 -> Mar 31
//     without permanently drifting to the 28th
//
// The stepping doubles as catch-up policy: a schedule that fell behind (e.g.
// the server was down) advances past all missed occurrences and fires once.
func NextRunAt(frequency string, startsAt, after time.Time, loc *time.Location) time.Time {
	s := startsAt.In(loc)
	hh, mm, ss := s.Clock()

	switch frequency {
	case "once":
		return startsAt

	case "daily", "weekly", "biweekly":
		stepDays := map[string]int{"daily": 1, "weekly": 7, "biweekly": 14}[frequency]
		// Fast-forward close to `after` in whole steps, then walk the rest.
		// Calendar-day addition via time.Date keeps the wall-clock time stable
		// across DST, unlike adding step*24h of absolute duration.
		candidate := s
		if elapsed := after.Sub(candidate); elapsed > 0 {
			steps := int(elapsed.Hours()) / 24 / stepDays
			if steps > 0 {
				candidate = time.Date(s.Year(), s.Month(), s.Day()+steps*stepDays, hh, mm, ss, 0, loc)
			}
		}
		for !candidate.After(after) {
			candidate = time.Date(candidate.Year(), candidate.Month(), candidate.Day()+stepDays, hh, mm, ss, 0, loc)
		}
		return candidate

	case "monthly":
		anchorDay := s.Day()
		year, month := s.Year(), s.Month()
		candidate := monthlyOccurrence(year, month, anchorDay, hh, mm, ss, loc)
		for !candidate.After(after) {
			month++
			if month > 12 {
				month = 1
				year++
			}
			candidate = monthlyOccurrence(year, month, anchorDay, hh, mm, ss, loc)
		}
		return candidate

	default:
		// Unknown frequency (should be prevented by the CHECK constraint):
		// behave like once so the row cannot spin.
		return startsAt
	}
}

// monthlyOccurrence builds the occurrence for a given month, clamping the
// anchor day to the month's length (Jan 31 -> Feb 28/29).
func monthlyOccurrence(year int, month time.Month, anchorDay, hh, mm, ss int, loc *time.Location) time.Time {
	day := anchorDay
	if last := daysInMonth(year, month); day > last {
		day = last
	}
	return time.Date(year, month, day, hh, mm, ss, 0, loc)
}

func daysInMonth(year int, month time.Month) int {
	// Day 0 of the next month is the last day of this month.
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
