package notify

import (
	"testing"
	"time"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("LoadLocation(%s): %v", name, err)
	}
	return loc
}

func at(t *testing.T, loc *time.Location, value string) time.Time {
	t.Helper()
	ts, err := time.ParseInLocation("2006-01-02 15:04", value, loc)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return ts
}

func TestNextRunAtDaily(t *testing.T) {
	loc := mustLoc(t, "America/New_York")
	starts := at(t, loc, "2026-01-10 21:00")

	// Plain next day.
	got := NextRunAt("daily", starts, at(t, loc, "2026-01-10 21:00"), loc)
	if want := at(t, loc, "2026-01-11 21:00"); !got.Equal(want) {
		t.Errorf("daily: got %v, want %v", got, want)
	}

	// starts_at still in the future: first occurrence is starts_at itself.
	got = NextRunAt("daily", starts, at(t, loc, "2026-01-01 09:00"), loc)
	if !got.Equal(starts) {
		t.Errorf("daily future start: got %v, want %v", got, starts)
	}

	// DST spring forward (2026-03-08 in America/New_York): wall clock holds.
	got = NextRunAt("daily", starts, at(t, loc, "2026-03-08 12:00"), loc)
	if want := at(t, loc, "2026-03-08 21:00"); !got.Equal(want) {
		t.Errorf("daily across spring-forward: got %v, want %v", got, want)
	}
	if h, m, _ := got.In(loc).Clock(); h != 21 || m != 0 {
		t.Errorf("daily DST wall clock drifted: %02d:%02d", h, m)
	}

	// DST fall back (2026-11-01): wall clock still holds.
	got = NextRunAt("daily", starts, at(t, loc, "2026-11-01 22:00"), loc)
	if want := at(t, loc, "2026-11-02 21:00"); !got.Equal(want) {
		t.Errorf("daily across fall-back: got %v, want %v", got, want)
	}
}

func TestNextRunAtWeeklyBiweeklyAnchoring(t *testing.T) {
	loc := mustLoc(t, "America/New_York")
	starts := at(t, loc, "2026-01-05 08:30") // a Monday

	// Weekly: always a Monday 08:30, even asked from mid-week months later.
	got := NextRunAt("weekly", starts, at(t, loc, "2026-06-11 12:00"), loc) // a Thursday
	if want := at(t, loc, "2026-06-15 08:30"); !got.Equal(want) {
		t.Errorf("weekly: got %v, want %v", got, want)
	}
	if got.Weekday() != time.Monday {
		t.Errorf("weekly landed on %v, want Monday", got.Weekday())
	}

	// Biweekly: cadence anchored on starts_at, so only every other Monday
	// qualifies. 2026-06-15 is 23 weeks + 4 days after the anchor; the next
	// on-cadence Monday is 2026-06-22 (24 weeks).
	got = NextRunAt("biweekly", starts, at(t, loc, "2026-06-11 12:00"), loc)
	if want := at(t, loc, "2026-06-22 08:30"); !got.Equal(want) {
		t.Errorf("biweekly anchoring: got %v, want %v", got, want)
	}
}

func TestNextRunAtMonthlyClamping(t *testing.T) {
	loc := mustLoc(t, "UTC")
	starts := at(t, loc, "2026-01-31 09:00")

	// Jan 31 -> Feb 28 (2026 is not a leap year).
	got := NextRunAt("monthly", starts, starts, loc)
	if want := at(t, loc, "2026-02-28 09:00"); !got.Equal(want) {
		t.Errorf("monthly Jan->Feb: got %v, want %v", got, want)
	}

	// Feb 28 -> Mar 31: the anchor day re-expands, no permanent drift to 28.
	got = NextRunAt("monthly", starts, got, loc)
	if want := at(t, loc, "2026-03-31 09:00"); !got.Equal(want) {
		t.Errorf("monthly Feb->Mar re-expansion: got %v, want %v", got, want)
	}

	// Leap year: Jan 31 2028 -> Feb 29 2028.
	starts28 := at(t, loc, "2028-01-31 09:00")
	got = NextRunAt("monthly", starts28, starts28, loc)
	if want := at(t, loc, "2028-02-29 09:00"); !got.Equal(want) {
		t.Errorf("monthly leap Feb: got %v, want %v", got, want)
	}

	// Year rollover: Dec -> Jan.
	startsDec := at(t, loc, "2026-12-15 07:00")
	got = NextRunAt("monthly", startsDec, startsDec, loc)
	if want := at(t, loc, "2027-01-15 07:00"); !got.Equal(want) {
		t.Errorf("monthly year rollover: got %v, want %v", got, want)
	}
}

func TestNextRunAtCatchUpSkipsMissedOccurrences(t *testing.T) {
	loc := mustLoc(t, "UTC")
	starts := at(t, loc, "2026-01-01 10:00")

	// Server was down for a week: the schedule advances past all missed daily
	// occurrences and lands on the next one after `after`, not seven of them.
	after := at(t, loc, "2026-01-08 15:00")
	got := NextRunAt("daily", starts, after, loc)
	if want := at(t, loc, "2026-01-09 10:00"); !got.Equal(want) {
		t.Errorf("catch-up: got %v, want %v", got, want)
	}
}

func TestNextRunAtOnce(t *testing.T) {
	loc := mustLoc(t, "UTC")
	starts := at(t, loc, "2026-05-01 12:00")
	got := NextRunAt("once", starts, at(t, loc, "2026-06-01 12:00"), loc)
	if !got.Equal(starts) {
		t.Errorf("once: got %v, want %v", got, starts)
	}
}
