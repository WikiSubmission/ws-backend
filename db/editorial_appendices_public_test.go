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
