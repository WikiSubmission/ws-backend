package handlers

import (
	"slices"
	"testing"
)

// detectQueryLangs is load-bearing for /search, /bible/search and /site/search,
// and had no coverage. nil means "search every available language".
func TestDetectQueryLangs(t *testing.T) {
	all := map[string]bool{
		"en": true, "ar": true, "ckb": true, "de": true, "fr": true, "tr": true, "he": true,
	}

	tests := []struct {
		name      string
		query     string
		available map[string]bool
		want      []string
	}{
		{
			// Latin scripts cannot be attributed to one language, so the FTS
			// phase searches all of them.
			name: "latin falls back to all languages", query: "mercy", available: all, want: nil,
		},
		{
			name: "latin with diacritics still falls back", query: "Barmherzigkeit", available: all, want: nil,
		},
		{
			// The regression this guards: ckb is Sorani Kurdish in Arabic script.
			// Before it was added to the candidate list, an Arabic-script query
			// could never narrow to the Kurdish catalogue.
			name: "arabic script includes sorani kurdish", query: "الزكاة", available: all,
			want: []string{"ar", "ckb"},
		},
		{
			name: "arabic script narrows to what is available",
			query: "الزكاة", available: map[string]bool{"en": true, "ckb": true},
			want: []string{"ckb"},
		},
		{
			name: "hebrew script", query: "בְּרֵאשִׁית", available: all, want: []string{"he"},
		},
		{
			// No available language matches the detected script, so searching
			// all of them beats searching none.
			name: "unmatched script falls back to all", query: "こんにちは", available: all, want: nil,
		},
		{
			name: "empty query falls back to all", query: "", available: all, want: nil,
		},
		{
			name: "digits and punctuation only fall back to all", query: "2:255", available: all, want: nil,
		},
		{
			// Mixed input resolves to the dominant script rather than both.
			name: "dominant script wins over a stray latin character",
			query: "الزكاة x", available: all, want: []string{"ar", "ckb"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := detectQueryLangs(tc.query, tc.available)
			slices.Sort(got)
			want := slices.Clone(tc.want)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("detectQueryLangs(%q) = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

// The Quran and Bible callers must be unaffected by adding ckb to the Arabic
// candidates: a language with no content is filtered out before it is returned.
func TestDetectQueryLangsFiltersLanguagesWithoutContent(t *testing.T) {
	quranLangs := map[string]bool{"en": true, "ar": true}

	got := detectQueryLangs("الزكاة", quranLangs)
	if !slices.Equal(got, []string{"ar"}) {
		t.Errorf("got %v, want [ar] — ckb has no Quran content so it must be dropped", got)
	}
}
