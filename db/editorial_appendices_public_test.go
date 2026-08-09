package db

import (
	"strings"
	"testing"
)

func TestAppendixWherePlaceholders(t *testing.T) {
	cases := []struct {
		name     string
		filter   AppendixFilter
		base     []any
		wantSQL  string
		wantArgs []any
	}{
		{
			name:     "no filter",
			wantSQL:  "a.published IS NOT NULL",
			wantArgs: []any{},
		},
		{
			name:     "language only",
			filter:   AppendixFilter{Language: "en"},
			wantSQL:  "a.published IS NOT NULL AND l.code = $1",
			wantArgs: []any{"en"},
		},
		{
			name:     "language and version",
			filter:   AppendixFilter{Language: "en", VersionSlug: "authorized-translation"},
			wantSQL:  "a.published IS NOT NULL AND l.code = $1 AND v.slug = $2",
			wantArgs: []any{"en", "authorized-translation"},
		},
		{
			name:     "version only",
			filter:   AppendixFilter{VersionSlug: "quran"},
			wantSQL:  "a.published IS NOT NULL AND v.slug = $1",
			wantArgs: []any{"quran"},
		},
		{
			// GetPublishedAppendixByCode seeds args with the code, so the
			// filter placeholders must continue from $2.
			name:     "offset by an existing arg",
			filter:   AppendixFilter{Language: "en"},
			base:     []any{"19"},
			wantSQL:  "a.published IS NOT NULL AND l.code = $2",
			wantArgs: []any{"19", "en"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]any{}, tc.base...)
			got := appendixWhere(tc.filter, &args)
			if got != tc.wantSQL {
				t.Errorf("appendixWhere() = %q, want %q", got, tc.wantSQL)
			}
			if len(args) != len(tc.wantArgs) {
				t.Fatalf("args = %v, want %v", args, tc.wantArgs)
			}
			for i := range args {
				if args[i] != tc.wantArgs[i] {
					t.Errorf("args[%d] = %v, want %v", i, args[i], tc.wantArgs[i])
				}
			}
		})
	}
}

// Drafts must never leak: every public query is gated on `published IS NOT
// NULL` and reads only out of the `published` document.
func TestPublishedAppendixColumnsNeverReadDraft(t *testing.T) {
	for _, full := range []bool{false, true} {
		cols := publishedAppendixColumns(full)
		if strings.Contains(cols, "draft") {
			t.Errorf("publishedAppendixColumns(%v) reads the draft document: %s", full, cols)
		}
	}
	if strings.Contains(publishedAppendixColumns(false), "'body'") {
		t.Error("listing columns must omit the markdown body")
	}
	if !strings.Contains(publishedAppendixColumns(true), "published->>'body'") {
		t.Error("single-appendix columns must include the markdown body")
	}
}

// The video is metadata, not prose, so both views select it: the reader needs
// it beside the body and a listing can flag which appendices have one.
func TestPublishedAppendixColumnsIncludeVideo(t *testing.T) {
	for _, full := range []bool{false, true} {
		cols := publishedAppendixColumns(full)
		for _, key := range []string{"published->>'video_id'", "published->>'video_title'"} {
			if !strings.Contains(cols, key) {
				t.Errorf("publishedAppendixColumns(%v) is missing %s", full, key)
			}
		}
	}
}

// scanPublishedAppendix binds positionally, so the column list and the scan
// targets must stay the same length or every field shifts by one.
func TestPublishedAppendixColumnCountMatchesScan(t *testing.T) {
	// Scan targets in scanPublishedAppendix, in order. Counted by line rather
	// than by comma, because COALESCE(…, '') puts commas inside a column.
	// 14 since body_pt joined body: an appendix carries its prose in both
	// carriers while the Portable Text migration is in flight.
	const scanTargets = 14
	for _, full := range []bool{false, true} {
		got := 0
		for _, line := range strings.Split(publishedAppendixColumns(full), "\n") {
			if strings.TrimSpace(line) != "" {
				got++
			}
		}
		if got != scanTargets {
			t.Errorf("publishedAppendixColumns(%v) selects %d columns, scan binds %d", full, got, scanTargets)
		}
	}
}
