package handlers

import "testing"

func TestBlankHint(t *testing.T) {
	cases := []struct {
		name        string
		answer      string
		firstLetter string
		firstTwo    string
		length      int
	}{
		{"multi-char preserves case", "Praise", "P", "Pr", 6},
		{"single char", "a", "a", "a", 1},
		{"two chars", "of", "o", "of", 2},
		{"unicode by rune count", "café", "c", "ca", 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := blankHint(tc.answer)
			if h.FirstLetter != tc.firstLetter {
				t.Errorf("first_letter = %q, want %q", h.FirstLetter, tc.firstLetter)
			}
			if h.FirstTwo != tc.firstTwo {
				t.Errorf("first_two = %q, want %q", h.FirstTwo, tc.firstTwo)
			}
			if h.Length != tc.length {
				t.Errorf("length = %d, want %d", h.Length, tc.length)
			}
		})
	}
}

// blankHint must never leak more than the first two characters of the answer.
func TestBlankHintDoesNotLeakAnswer(t *testing.T) {
	answer := "guidance"
	h := blankHint(answer)
	if len([]rune(h.FirstTwo)) > 2 {
		t.Fatalf("first_two leaked %q (more than two runes)", h.FirstTwo)
	}
	if h.FirstTwo == answer {
		t.Fatalf("first_two equals the full answer %q", answer)
	}
}
