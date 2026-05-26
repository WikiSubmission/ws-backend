package handlers

import (
	"testing"
	"time"
)

func TestReadingStatsRangeStart(t *testing.T) {
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name   string
		input  *GetMeReadingStatsParamsRange
		expect time.Time
		zero   bool
	}{
		{name: "nil defaults to 30d", input: nil, expect: now.AddDate(0, 0, -30)},
		{name: "7d", input: rangePtr(N7d), expect: now.AddDate(0, 0, -7)},
		{name: "30d", input: rangePtr(N30d), expect: now.AddDate(0, 0, -30)},
		{name: "90d", input: rangePtr(N90d), expect: now.AddDate(0, 0, -90)},
		{name: "1y", input: rangePtr(N1y), expect: now.AddDate(-1, 0, 0)},
		{name: "all returns zero time", input: rangePtr(All), zero: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := readingStatsRangeStart(now, tc.input)
			if tc.zero {
				if !got.IsZero() {
					t.Fatalf("expected zero time, got %v", got)
				}
				return
			}
			if !got.Equal(tc.expect) {
				t.Fatalf("expected %v, got %v", tc.expect, got)
			}
		})
	}
}

func rangePtr(r GetMeReadingStatsParamsRange) *GetMeReadingStatsParamsRange {
	return &r
}
