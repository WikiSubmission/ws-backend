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
			got := scoreGuess(tc.guess, blank, tc.difficulty, "en", tc.lemmas)
			if got != tc.want {
				t.Errorf("scoreGuess(%q, diff=%q) = %v, want %v", tc.guess, tc.difficulty, got, tc.want)
			}
		})
	}
}

// Tests the score formula end-to-end: base * mult * bandBonus - penalty.
func TestScoreFormula(t *testing.T) {
	cases := []struct {
		name       string
		correct    int
		total      int
		difficulty string
		skillBand  string
		hints      int
		want       int
	}{
		{"easy perfect medium band", 8, 8, "easy", "medium", 0, 1000},
		{"medium one wrong two hints", 14, 15, "medium", "medium", 2, 1350},
		{"hard one wrong medium band", 7, 8, "hard", "medium", 0, 1750},
		{"professional perfect medium band", 25, 25, "professional", "medium", 0, 3000},
		{"floor at zero", 0, 10, "easy", "medium", 100, 0},
		{"hard tier hard band bonus", 8, 8, "hard", "hard", 0, int(1000.0 * 2.0 * 1.05)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := float64(tc.correct) / float64(tc.total) * 1000.0
			mult := difficultyMultipliers[tc.difficulty]
			bonus, ok := bandBonuses[tc.skillBand]
			if !ok {
				bonus = 1.0
			}
			penalty := tc.hints * hintPenaltyPerHint
			got := int(base*mult*bonus) - penalty
			if got < 0 {
				got = 0
			}
			if got != tc.want {
				t.Errorf("score = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestComputeSkillBand(t *testing.T) {
	cases := []struct {
		name     string
		count    int
		accuracy float64
		want     string
	}{
		{"cold start zero", 0, 0.0, "medium"},
		{"cold start one", 1, 1.0, "medium"},
		{"cold start two", 2, 1.0, "medium"},
		{"three attempts low accuracy", 3, 0.45, "easy"},
		{"three attempts at threshold easy", 3, 0.50, "medium"},
		{"ten attempts middle", 10, 0.70, "medium"},
		{"ten attempts just below hard", 10, 0.84, "medium"},
		{"ten attempts at threshold hard", 10, 0.85, "hard"},
		{"ten attempts perfect", 10, 1.0, "hard"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := computeSkillBand(tc.count, tc.accuracy)
			if got != tc.want {
				t.Errorf("computeSkillBand(%d, %.2f) = %q, want %q", tc.count, tc.accuracy, got, tc.want)
			}
		})
	}
}
