package curate

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidateProposals(t *testing.T) {
	cases := []struct {
		name        string
		in          []Proposal
		wantKept    int
		wantDropped int
	}{
		{
			name: "happy path",
			in: []Proposal{
				{VerseStart: 1, VerseEnd: 7, Label: "Opening"},
				{VerseStart: 8, VerseEnd: 12, Label: "Belief"},
			},
			wantKept: 2,
		},
		{
			name: "too short",
			in: []Proposal{
				{VerseStart: 1, VerseEnd: 2, Label: "Tiny"},
			},
			wantDropped: 1,
		},
		{
			name: "too long",
			in: []Proposal{
				{VerseStart: 1, VerseEnd: 50, Label: "Huge"},
			},
			wantDropped: 1,
		},
		{
			name: "overlap drops second",
			in: []Proposal{
				{VerseStart: 1, VerseEnd: 10, Label: "A"},
				{VerseStart: 5, VerseEnd: 14, Label: "B"},
			},
			wantKept:    1,
			wantDropped: 1,
		},
		{
			name: "empty label",
			in: []Proposal{
				{VerseStart: 1, VerseEnd: 5, Label: "  "},
			},
			wantDropped: 1,
		},
		{
			name: "invalid range",
			in: []Proposal{
				{VerseStart: 10, VerseEnd: 5, Label: "Reversed"},
			},
			wantDropped: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kept, dropped := ValidateProposals(tc.in)
			if len(kept) != tc.wantKept {
				t.Errorf("kept = %d, want %d (%+v)", len(kept), tc.wantKept, kept)
			}
			if len(dropped) != tc.wantDropped {
				t.Errorf("dropped = %d, want %d (%+v)", len(dropped), tc.wantDropped, dropped)
			}
		})
	}
}

func TestBuildWindows(t *testing.T) {
	// A realistic verse length (~240 chars ≈ 60 tokens) so a long chapter
	// exceeds maxVerseTokensPerWindow and must split.
	verse := func(n int) []string {
		line := strings.Repeat("word ", 48) // ~240 chars
		out := make([]string, n)
		for i := range out {
			out[i] = line
		}
		return out
	}

	t.Run("short chapter is one window", func(t *testing.T) {
		ws := buildWindows(verse(7), map[int]bool{})
		if len(ws) != 1 || ws[0].start != 1 || ws[0].end != 7 {
			t.Fatalf("got %+v, want one window 1-7", ws)
		}
	})

	t.Run("long chapter splits into multiple windows", func(t *testing.T) {
		ws := buildWindows(verse(286), map[int]bool{})
		if len(ws) < 2 {
			t.Fatalf("expected multiple windows for 286 verses, got %d", len(ws))
		}
		// Windows must be contiguous, ordered, and cover every verse exactly once.
		want := 1
		for _, w := range ws {
			if w.start != want {
				t.Fatalf("window starts at %d, want %d (%+v)", w.start, want, ws)
			}
			if w.end < w.start {
				t.Fatalf("window end %d before start %d", w.end, w.start)
			}
			want = w.end + 1
		}
		if want != 287 {
			t.Fatalf("windows cover up to %d, want 286", want-1)
		}
	})

	t.Run("covered verses are skipped", func(t *testing.T) {
		covered := map[int]bool{}
		for v := 1; v <= 5; v++ {
			covered[v] = true
		}
		ws := buildWindows(verse(12), covered)
		if len(ws) != 1 || ws[0].start != 6 || ws[0].end != 12 {
			t.Fatalf("got %+v, want one window 6-12", ws)
		}
	})

	t.Run("runs shorter than the minimum are dropped", func(t *testing.T) {
		covered := map[int]bool{3: true, 4: true, 5: true, 6: true, 7: true}
		// Uncovered runs: 1-2 (too short) and 8-10 (kept).
		ws := buildWindows(verse(10), covered)
		if len(ws) != 1 || ws[0].start != 8 || ws[0].end != 10 {
			t.Fatalf("got %+v, want one window 8-10", ws)
		}
	})
}

func TestNormalizeDifficulty(t *testing.T) {
	cases := []struct {
		in      string
		wantNil bool
		wantStr string
	}{
		{"easy", false, "easy"},
		{"  Moderate ", false, "moderate"},
		{"CHALLENGING", false, "challenging"},
		{"hard", true, ""},
		{"", true, ""},
	}
	for _, tc := range cases {
		got := NormalizeDifficulty(tc.in)
		if tc.wantNil && got != nil {
			t.Errorf("NormalizeDifficulty(%q) = %q, want nil", tc.in, *got)
			continue
		}
		if !tc.wantNil && (got == nil || *got != tc.wantStr) {
			gv := "<nil>"
			if got != nil {
				gv = *got
			}
			t.Errorf("NormalizeDifficulty(%q) = %q, want %q", tc.in, gv, tc.wantStr)
		}
	}
}

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		name   string
		header string
		body   string
		want   time.Duration
	}{
		{"header seconds", "30", "", 30 * time.Second},
		{"header fractional seconds", "12.5", "", 12500 * time.Millisecond},
		{"header empty body parses milliseconds", "", `{"error":"please try again in 250ms"}`, 250 * time.Millisecond},
		{"header empty body parses seconds", "", `try again in 47.123s`, 47123 * time.Millisecond},
		{"header empty body parses minutes", "", `try again in 2m`, 2 * time.Minute},
		{"header wins over body", "5", `try again in 60s`, 5 * time.Second},
		{"nothing", "", `{"error":"something else"}`, 0},
		{"negative ignored", "-5", "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRetryAfter(tc.header, []byte(tc.body))
			if got != tc.want {
				t.Errorf("parseRetryAfter(%q, %q) = %s, want %s", tc.header, tc.body, got, tc.want)
			}
		})
	}
}

func TestRateLimitErrorIs(t *testing.T) {
	e := &RateLimitError{RetryAfter: 5 * time.Second, Status: 429, Body: "rate_limit_exceeded"}
	if !errors.Is(e, ErrRateLimited) {
		t.Fatal("errors.Is(*RateLimitError, ErrRateLimited) = false, want true")
	}
	var target *RateLimitError
	if !errors.As(e, &target) || target.RetryAfter != 5*time.Second {
		t.Fatalf("errors.As did not extract RetryAfter (got %+v)", target)
	}
}
