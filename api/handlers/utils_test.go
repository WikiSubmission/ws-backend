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
			wantSCh: 1, wantECh: 1, wantSV: 1, wantEV: 7,
		},
		{
			name:    "cross-chapter range",
			sCh: 1, eCh: 3, sV: 3, eV: 50,
			wantSCh: 1, wantECh: 3, wantSV: 3, wantEV: 50,
		},
		{
			name:    "last chapter full",
			sCh: 114, eCh: 114, sV: 0, eV: 9999,
			wantSCh: 114, wantECh: 114, wantSV: 1, wantEV: 6,
		},
		{
			name:    "middle chapter full",
			sCh: 18, eCh: 18, sV: 0, eV: 9999,
			wantSCh: 18, wantECh: 18, wantSV: 1, wantEV: 110,
		},

		// --- Verse clamping ---
		{
			name:    "verse start 0 is clamped to 1",
			sCh: 2, eCh: 2, sV: 0, eV: 10,
			wantSCh: 2, wantECh: 2, wantSV: 1, wantEV: 10,
		},
		{
			name:    "verse start negative is clamped to 1",
			sCh: 2, eCh: 2, sV: -99, eV: 10,
			wantSCh: 2, wantECh: 2, wantSV: 1, wantEV: 10,
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
			wantSCh: 1, wantECh: 3, wantSV: 1, wantEV: 200,
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
