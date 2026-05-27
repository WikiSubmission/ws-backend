package main

import "testing"

func TestValidateProposals(t *testing.T) {
	cases := []struct {
		name        string
		in          []proposal
		wantKept    int
		wantDropped int
	}{
		{
			name: "happy path",
			in: []proposal{
				{VerseStart: 1, VerseEnd: 7, Label: "Opening"},
				{VerseStart: 8, VerseEnd: 12, Label: "Belief"},
			},
			wantKept: 2,
		},
		{
			name: "too short",
			in: []proposal{
				{VerseStart: 1, VerseEnd: 2, Label: "Tiny"},
			},
			wantDropped: 1,
		},
		{
			name: "too long",
			in: []proposal{
				{VerseStart: 1, VerseEnd: 50, Label: "Huge"},
			},
			wantDropped: 1,
		},
		{
			name: "overlap drops second",
			in: []proposal{
				{VerseStart: 1, VerseEnd: 10, Label: "A"},
				{VerseStart: 5, VerseEnd: 14, Label: "B"},
			},
			wantKept:    1,
			wantDropped: 1,
		},
		{
			name: "empty label",
			in: []proposal{
				{VerseStart: 1, VerseEnd: 5, Label: "  "},
			},
			wantDropped: 1,
		},
		{
			name: "invalid range",
			in: []proposal{
				{VerseStart: 10, VerseEnd: 5, Label: "Reversed"},
			},
			wantDropped: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kept, dropped := validateProposals(tc.in)
			if len(kept) != tc.wantKept {
				t.Errorf("kept = %d, want %d (%+v)", len(kept), tc.wantKept, kept)
			}
			if len(dropped) != tc.wantDropped {
				t.Errorf("dropped = %d, want %d (%+v)", len(dropped), tc.wantDropped, dropped)
			}
		})
	}
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
		got := normalizeDifficulty(tc.in)
		if tc.wantNil && got != nil {
			t.Errorf("normalizeDifficulty(%q) = %q, want nil", tc.in, *got)
			continue
		}
		if !tc.wantNil && (got == nil || *got != tc.wantStr) {
			gv := "<nil>"
			if got != nil {
				gv = *got
			}
			t.Errorf("normalizeDifficulty(%q) = %q, want %q", tc.in, gv, tc.wantStr)
		}
	}
}
