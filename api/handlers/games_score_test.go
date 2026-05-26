package handlers

import "testing"

func TestNormalizeAnswer(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Praise", "praise"},
		{"GOD!", "god"},
		{"  praise.  ", "praise"},
		{"It's", "its"},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := normalizeAnswer(tc.in); got != tc.want {
				t.Errorf("normalizeAnswer(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestScoreGuess(t *testing.T) {
	blank := &blankRecord{Index: 0, Answer: "Praise", AnswerLemma: "praise"}
	lemmas := map[string]string{
		"praise":  "praise",
		"praised": "praise",
		"worship": "worship",
	}

	cases := []struct {
		name       string
		guess      string
		difficulty string
		lemmas     map[string]string
		want       bool
	}{
		{"exact match easy", "Praise", "easy", nil, true},
		{"case insensitive medium", "PRAISE", "medium", nil, true},
		{"trailing punct professional", "praise.", "professional", nil, true},
		{"wrong word professional", "thanks", "professional", nil, false},
		{"lemma match hard", "praised", "hard", lemmas, true},
		{"lemma miss hard", "worship", "hard", lemmas, false},
		{"empty guess never", "", "easy", nil, false},
		{"hard falls back to exact when no lemma map", "Praise", "hard", nil, true},
		{"hard rejects close word when no lemma map", "praised", "hard", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scoreGuess(tc.guess, blank, tc.difficulty, tc.lemmas)
			if got != tc.want {
				t.Errorf("scoreGuess(%q, diff=%q) = %v, want %v", tc.guess, tc.difficulty, got, tc.want)
			}
		})
	}
}

// Tests the score formula end-to-end: base * mult - penalty.
func TestScoreFormula(t *testing.T) {
	cases := []struct {
		name       string
		correct    int
		total      int
		difficulty string
		hints      int
		want       int
	}{
		{"easy perfect", 8, 8, "easy", 0, 1000},
		{"medium one wrong two hints", 14, 15, "medium", 2, 1350},
		{"hard one wrong", 7, 8, "hard", 0, 1750},
		{"professional perfect", 25, 25, "professional", 0, 3000},
		{"floor at zero", 0, 10, "easy", 100, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := float64(tc.correct) / float64(tc.total) * 1000.0
			mult := difficultyMultipliers[tc.difficulty]
			penalty := tc.hints * hintPenaltyPerHint
			got := int(base*mult) - penalty
			if got < 0 {
				got = 0
			}
			if got != tc.want {
				t.Errorf("score = %d, want %d", got, tc.want)
			}
		})
	}
}
