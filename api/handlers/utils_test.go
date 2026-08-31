package handlers

import (
	"testing"
)

// quranTestChapters is a minimal subset of the real Quran chapter metadata,
// enough to cover boundary conditions without importing the full dataset.
var quranTestChapters = map[int]ChapterCache{
	1:   {VerseCount: 7, RevelationOrder: 5},
	2:   {VerseCount: 286, RevelationOrder: 87},
	3:   {VerseCount: 200, RevelationOrder: 89},
	18:  {VerseCount: 110, RevelationOrder: 69},
	114: {VerseCount: 6, RevelationOrder: 21},
}

func testServer() *Server {
	return &Server{
		Chapters:  quranTestChapters,
		Languages: map[string]LanguageCache{},
	}
}

func TestValidateQuranRange(t *testing.T) {
	s := testServer()

	tests := []struct {
		name                     string
		sCh, eCh, sV, eV        int
		wantSCh, wantECh        int
		wantSV, wantEV          int
		wantErr                  bool
	}{
		// --- Happy paths ---
		{
			name:    "single chapter exact verses",
			sCh: 2, eCh: 2, sV: 5, eV: 10,
			wantSCh: 2, wantECh: 2, wantSV: 5, wantEV: 10,
		},
		{
			name:    "full single chapter via default range",
			sCh: 1, eCh: 1, sV: 0, eV: 9999,
			wantSCh: 1, wantECh: 1, wantSV: 0, wantEV: 7,
		},
		{
			name:    "cross-chapter range",
			sCh: 1, eCh: 3, sV: 3, eV: 50,
			wantSCh: 1, wantECh: 3, wantSV: 3, wantEV: 50,
		},
		{
			name:    "last chapter full",
			sCh: 114, eCh: 114, sV: 0, eV: 9999,
			wantSCh: 114, wantECh: 114, wantSV: 0, wantEV: 6,
		},
		{
			name:    "middle chapter full",
			sCh: 18, eCh: 18, sV: 0, eV: 9999,
			wantSCh: 18, wantECh: 18, wantSV: 0, wantEV: 110,
		},

		// --- Verse clamping ---
		{
			name:    "verse start 0 is allowed (Basmallah verse)",
			sCh: 2, eCh: 2, sV: 0, eV: 10,
			wantSCh: 2, wantECh: 2, wantSV: 0, wantEV: 10,
		},
		{
			name:    "verse start negative is clamped to 0 (Basmallah)",
			sCh: 2, eCh: 2, sV: -99, eV: 10,
			wantSCh: 2, wantECh: 2, wantSV: 0, wantEV: 10,
		},
		{
			name:    "verse end 9999 resolves to end-chapter verse count",
			sCh: 1, eCh: 1, sV: 1, eV: 9999,
			wantSCh: 1, wantECh: 1, wantSV: 1, wantEV: 7,
		},
		{
			name:    "verse end beyond chapter size is clamped",
			sCh: 2, eCh: 2, sV: 1, eV: 9998,
			wantSCh: 2, wantECh: 2, wantSV: 1, wantEV: 286,
		},
		{
			name:    "verse start beyond chapter size is clamped",
			sCh: 1, eCh: 1, sV: 100, eV: 9999,
			// sV clamped to 7; eV resolved to 7; no swap needed (equal)
			wantSCh: 1, wantECh: 1, wantSV: 7, wantEV: 7,
		},
		{
			name:    "verse end 0 is clamped to 1 then swapped with start",
			sCh: 2, eCh: 2, sV: 5, eV: 0,
			// eV=0 → eV=1; same chapter and eV(1) < sV(5) → swap → sV=1, eV=5
			wantSCh: 2, wantECh: 2, wantSV: 1, wantEV: 5,
		},

		// --- Inversion fixes ---
		{
			name:    "inverted verse range in same chapter is swapped",
			sCh: 2, eCh: 2, sV: 10, eV: 5,
			wantSCh: 2, wantECh: 2, wantSV: 5, wantEV: 10,
		},
		{
			name:    "inverted chapter range is corrected",
			sCh: 3, eCh: 1, sV: 5, eV: 50,
			// Chapters swap to (1,3); verse clamping uses chapter 1 (startMeta)
			// and chapter 3 (endMeta); eV=50 is within chapter 3's 200 verses.
			wantSCh: 1, wantECh: 3, wantSV: 5, wantEV: 50,
		},
		{
			name:    "inverted chapter range with default eV uses correct end-chapter count",
			// This is the bug that existed before the fix: eV=9999 used to be resolved
			// against the original eCh=1 (7 verses) instead of the corrected eCh=3 (200).
			sCh: 3, eCh: 1, sV: 0, eV: 9999,
			wantSCh: 1, wantECh: 3, wantSV: 0, wantEV: 200,
		},
		{
			name:    "inverted chapter with eV beyond corrected end chapter is clamped",
			sCh: 3, eCh: 1, sV: 1, eV: 9998,
			// After swap: endCh=3, VerseCount=200; eV=9998 → clamped to 200
			wantSCh: 1, wantECh: 3, wantSV: 1, wantEV: 200,
		},

		// --- Invalid inputs ---
		{
			name:    "chapter 0 is invalid",
			sCh: 0, eCh: 2, sV: 1, eV: 10,
			wantErr: true,
		},
		{
			name:    "chapter 115 does not exist",
			sCh: 1, eCh: 115, sV: 1, eV: 10,
			wantErr: true,
		},
		{
			name:    "both chapters invalid",
			sCh: 0, eCh: 115, sV: 1, eV: 10,
			wantErr: true,
		},
		{
			name:    "negative chapter is invalid",
			sCh: -1, eCh: 2, sV: 1, eV: 10,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSCh, gotECh, gotSV, gotEV, err := s.ValidateQuranRange(tt.sCh, tt.eCh, tt.sV, tt.eV)

			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if gotSCh != tt.wantSCh || gotECh != tt.wantECh || gotSV != tt.wantSV || gotEV != tt.wantEV {
				t.Errorf(
					"ValidateQuranRange(%d,%d,%d,%d) = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
					tt.sCh, tt.eCh, tt.sV, tt.eV,
					gotSCh, gotECh, gotSV, gotEV,
					tt.wantSCh, tt.wantECh, tt.wantSV, tt.wantEV,
				)
			}
		})
	}
}

func TestParseVerseRefs(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []VerseSpec
		wantErr bool
	}{
		// --- Valid inputs ---
		{
			name: "single verse",
			raw:  "1:1",
			want: []VerseSpec{{Chapter: 1, Start: 1, End: -1}},
		},
		{
			name: "verse range within chapter",
			raw:  "1:1-3",
			want: []VerseSpec{{Chapter: 1, Start: 1, End: 3}},
		},
		{
			name: "mixed list",
			raw:  "1:2,1:4-5,4:2",
			want: []VerseSpec{
				{Chapter: 1, Start: 2, End: -1},
				{Chapter: 1, Start: 4, End: 5},
				{Chapter: 4, Start: 2, End: -1},
			},
		},
		{
			name: "dot-separated list",
			raw:  "1:2.1:4-5.4:2",
			want: []VerseSpec{
				{Chapter: 1, Start: 2, End: -1},
				{Chapter: 1, Start: 4, End: 5},
				{Chapter: 4, Start: 2, End: -1},
			},
		},
		{
			name: "mixed comma and dot separators",
			raw:  "1:2,1:4-5.4:2",
			want: []VerseSpec{
				{Chapter: 1, Start: 2, End: -1},
				{Chapter: 1, Start: 4, End: 5},
				{Chapter: 4, Start: 2, End: -1},
			},
		},
		{
			name: "negative start verse",
			raw:  "1:-2",
			want: []VerseSpec{{Chapter: 1, Start: -2, End: -1}},
		},
		{
			name: "zero verse",
			raw:  "1:0",
			want: []VerseSpec{{Chapter: 1, Start: 0, End: -1}},
		},
		{
			name: "whitespace around token is trimmed",
			raw:  "1:1 , 2:5",
			want: []VerseSpec{
				{Chapter: 1, Start: 1, End: -1},
				{Chapter: 2, Start: 5, End: -1},
			},
		},

		// --- Invalid inputs ---
		{name: "missing colon", raw: "1", wantErr: true},
		{name: "empty verse part", raw: "1:", wantErr: true},
		{name: "empty chapter part", raw: ":1", wantErr: true},
		{name: "non-numeric chapter", raw: "a:b", wantErr: true},
		{name: "non-numeric verse end", raw: "1:2-a", wantErr: true},
		{name: "empty string", raw: "", wantErr: true},
		{name: "only commas", raw: ",,", wantErr: true},
		{name: "only dots", raw: "..", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseVerseRefs(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseVerseRefs(%q) error = %v, wantErr = %v", tt.raw, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("ParseVerseRefs(%q) returned %d specs, want %d", tt.raw, len(got), len(tt.want))
			}
			for i, w := range tt.want {
				g := got[i]
				if g.Chapter != w.Chapter || g.Start != w.Start || g.End != w.End {
					t.Errorf("spec[%d]: got {%d %d %d}, want {%d %d %d}",
						i, g.Chapter, g.Start, g.End, w.Chapter, w.Start, w.End)
				}
			}
		})
	}
}

func TestExpandVerseRefs(t *testing.T) {
	s := testServer()

	tests := []struct {
		name      string
		raw       string
		maxVerses int
		wantRefs  []VerseRef // nil means only check count
		wantCount int
		wantErr   bool
	}{
		{
			name:      "single verse",
			raw:       "1:1",
			maxVerses: 300,
			wantRefs:  []VerseRef{{Chapter: 1, VerseIndex: 1}},
		},
		{
			name:      "verse range expands to individual refs",
			raw:       "1:1-3",
			maxVerses: 300,
			wantRefs: []VerseRef{
				{Chapter: 1, VerseIndex: 1},
				{Chapter: 1, VerseIndex: 2},
				{Chapter: 1, VerseIndex: 3},
			},
		},
		{
			name:      "negative start clamped to 0 (Basmallah)",
			raw:       "1:-2",
			maxVerses: 300,
			wantRefs:  []VerseRef{{Chapter: 1, VerseIndex: 0}},
		},
		{
			name:      "duplicate verses are deduplicated",
			raw:       "1:1,1:1",
			maxVerses: 300,
			wantRefs:  []VerseRef{{Chapter: 1, VerseIndex: 1}},
		},
		{
			name:      "inverted range is swapped",
			raw:       "1:5-3",
			maxVerses: 300,
			wantRefs: []VerseRef{
				{Chapter: 1, VerseIndex: 3},
				{Chapter: 1, VerseIndex: 4},
				{Chapter: 1, VerseIndex: 5},
			},
		},
		{
			name:      "end verse clamped to chapter verse count",
			raw:       "1:1-999",
			maxVerses: 300,
			wantCount: 7, // chapter 1 has 7 verses
		},
		{
			name:      "unknown chapter returns error",
			raw:       "200:1",
			maxVerses: 300,
			wantErr:   true,
		},
		{
			name:      "exceeding maxVerses returns error",
			raw:       "2:1-286,3:1-15", // 286+15=301 > 300
			maxVerses: 300,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			specs, err := ParseVerseRefs(tt.raw)
			if err != nil {
				if tt.wantErr {
					return
				}
				t.Fatalf("ParseVerseRefs(%q) unexpected error: %v", tt.raw, err)
			}

			refs, err := s.ExpandVerseRefs(specs, tt.maxVerses)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ExpandVerseRefs error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if tt.wantRefs != nil {
				if len(refs) != len(tt.wantRefs) {
					t.Fatalf("got %d refs, want %d", len(refs), len(tt.wantRefs))
				}
				for i, w := range tt.wantRefs {
					if refs[i] != w {
						t.Errorf("ref[%d]: got {%d %d}, want {%d %d}",
							i, refs[i].Chapter, refs[i].VerseIndex, w.Chapter, w.VerseIndex)
					}
				}
			} else if tt.wantCount > 0 && len(refs) != tt.wantCount {
				t.Errorf("got %d refs, want %d", len(refs), tt.wantCount)
			}
		})
	}
}

