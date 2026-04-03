package handlers

import (
	"testing"
)

func TestValidateBibleRange(t *testing.T) {
	books := map[int]BibleBookCache{
		1:  {Name: "Genesis", Testament: "OT", ChapterCount: 50},
		43: {Name: "John", Testament: "NT", ChapterCount: 21},
		66: {Name: "Revelation", Testament: "NT", ChapterCount: 22},
	}

	tests := []struct {
		name    string
		book    int
		startCh int
		endCh   int
		startV  int
		endV    int
		wantErr string
	}{
		{
			name:    "valid single chapter",
			book:    43, startCh: 3, endCh: 3, startV: 1, endV: 36,
		},
		{
			name:    "valid multi-chapter range",
			book:    1, startCh: 1, endCh: 5, startV: 1, endV: 9999,
		},
		{
			name:    "valid max chapter range (10)",
			book:    1, startCh: 1, endCh: 10, startV: 1, endV: 9999,
		},
		{
			name:    "unknown book",
			book:    99, startCh: 1, endCh: 1, startV: 1, endV: 10,
			wantErr: "unknown book number: 99",
		},
		{
			name:    "chapter_start zero",
			book:    1, startCh: 0, endCh: 1, startV: 1, endV: 10,
			wantErr: "chapter_start must be >= 1",
		},
		{
			name:    "chapter_end before chapter_start",
			book:    43, startCh: 5, endCh: 3, startV: 1, endV: 10,
			wantErr: "chapter_end must be >= chapter_start",
		},
		{
			name:    "chapter_start exceeds book chapter count",
			book:    43, startCh: 25, endCh: 25, startV: 1, endV: 10,
			wantErr: "chapter_start 25 exceeds",
		},
		{
			name:    "chapter range too large (11)",
			book:    1, startCh: 1, endCh: 11, startV: 1, endV: 9999,
			wantErr: "chapter range too large",
		},
		{
			name:    "verse_start zero",
			book:    43, startCh: 3, endCh: 3, startV: 0, endV: 16,
			wantErr: "verse_start must be >= 1",
		},
		{
			name:    "verse_end before verse_start",
			book:    43, startCh: 3, endCh: 3, startV: 10, endV: 5,
			wantErr: "verse_end must be >= verse_start",
		},
		{
			name:    "book with zero chapter count skips chapter_start bound check",
			book:    43, startCh: 3, endCh: 3, startV: 1, endV: 16,
			// ChapterCount is 21 for John — valid
		},
		{
			name:    "first verse of first chapter of last book",
			book:    66, startCh: 1, endCh: 1, startV: 1, endV: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBibleRange(tt.book, tt.startCh, tt.endCh, tt.startV, tt.endV, books)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.wantErr)
				return
			}
			if !containsString(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func containsString(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
