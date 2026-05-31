package handlers

// CheckBlank itself is pure glue: parse body -> DB fetch variant -> unmarshal
// blanks -> find blank by index -> (optional) DB fetch lemmas -> scoreGuess ->
// (optional) DB increment wrong count -> JSON response. There are no extractable
// logic helpers unique to games_check.go beyond what is already covered by
// TestScoreGuess in games_score_test.go.
//
// This file therefore focuses on:
//   1. normalizeForLang across every supported language code -- the new
//      multi-language addition that had no dedicated coverage.
//   2. maxAttemptsForDifficulty -- controls the attempt-tracking branch in
//      CheckBlank (0 = no tracking, 3 = tracked).
//   3. wrongPenaltyForBlank -- the progressive wrong-attempt penalty formula.
//   4. scoreGuess with language-aware normalizeForLang integration, exercising
//      the Arabic and accented-Latin script paths that TestScoreGuess did not
//      reach (it only used lang="en").

import "testing"

// ── normalizeForLang ─────────────────────────────────────────────────────────

func TestNormalizeForLang(t *testing.T) {
	cases := []struct {
		name string
		in   string
		lang string
		want string
	}{
		// English: basic lowercase + strip punctuation, no NFKD applied.
		{"en basic", "Praise", "en", "praise"},
		{"en punct", "GOD!", "en", "god"},
		{"en apostrophe stripped", "It's", "en", "its"},
		{"en empty", "", "en", ""},
		{"en digits kept", "Al-Fatihah1", "en", "alfatihah1"},

		// Arabic (ar): diacritics (harakat, Mn category) stripped, tatweel
		// (U+0640, a letter) removed by the explicit guard in the function.
		{"ar plain", "الله", "ar", "الله"},
		{"ar tatweel removed", "اللـه", "ar", "الله"},
		{"ar diacritics stripped", "الرَّحْمَٰنِ", "ar", "الرحمن"},
		{"ar empty", "", "ar", ""},

		// Quranic Arabic (ac): same rules as ar.
		{"ac diacritics stripped", "بِسْمِ", "ac", "بسم"},
		{"ac tatweel removed", "اللـه", "ac", "الله"},

		// French: NFKD decomposes precomposed accented letters; the combining
		// marks (Mn) are dropped, leaving only the base ASCII letter.
		{"fr accent e acute", "épreuve", "fr", "epreuve"},
		{"fr accent e grave", "père", "fr", "pere"},
		{"fr cedilla", "leçon", "fr", "lecon"},
		{"fr uppercase accent", "Étoile", "fr", "etoile"},
		{"fr no accent", "grace", "fr", "grace"},

		// German: umlauts decompose under NFKD to base letter + combining mark.
		{"de umlaut ü", "über", "de", "uber"},
		{"de umlaut ö", "schön", "de", "schon"},
		{"de umlaut ä", "Mädchen", "de", "madchen"},

		// Spanish: tilde and acute accent decompose under NFKD.
		{"es tilde n", "mañana", "es", "manana"},
		{"es accent", "corazón", "es", "corazon"},
		{"es uppercase", "Ángel", "es", "angel"},

		// Swedish: ring above (å) and umlauts decompose under NFKD.
		{"sv ring a", "åska", "sv", "aska"},
		{"sv a-umlaut", "ärlig", "sv", "arlig"},
		{"sv o-umlaut", "öppen", "sv", "oppen"},

		// Turkish: NFKD is applied. ş (U+015F) decomposes to s + combining
		// cedilla (Mn), so the cedilla is dropped and s remains. The dotless
		// i (U+0131) is an IsLetter rune whose NFKD form is itself; it is
		// lowercased but stays as ı, not ASCII i.
		{"tr s-cedilla", "şeker", "tr", "seker"},
		{"tr dotless i stays", "kış", "tr", "kıs"},

		// Filipino (tl): accented vowels decompose under NFKD.
		{"tl accent", "káin", "tl", "kain"},

		// Languages that do not use NFKD: only IsLetter/IsDigit filter +
		// lowercase. Combining marks that are not IsLetter are dropped.
		// Bengali virama (U+09CD, Mn) and vowel sign ā (U+09BE, Mc) are not
		// IsLetter, so they are stripped.
		{"id no accent", "Allah", "id", "allah"},
		{"ru cyrillic", "Бог", "ru", "бог"},
		{"bn virama stripped", "আল্লাহ", "bn", "আললহ"},
		// Tamil vowel sign ை (U+0BC8, Mc) and virama ் (U+0BCD, Mn) are not
		// IsLetter; only the base consonant letters survive.
		{"ta non-letter signs stripped", "இறைவன்", "ta", "இறவன"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeForLang(tc.in, tc.lang)
			if got != tc.want {
				t.Errorf("normalizeForLang(%q, %q) = %q, want %q", tc.in, tc.lang, got, tc.want)
			}
		})
	}
}

// Idempotency: applying normalizeForLang twice must produce the same result as
// applying it once. This guards against any normalization step that could
// interact with itself.
func TestNormalizeForLangIdempotent(t *testing.T) {
	cases := []struct {
		in, lang string
	}{
		{"Épreuve", "fr"},
		{"الرَّحْمَٰنِ", "ar"},
		{"über", "de"},
		{"mañana", "es"},
		{"åska", "sv"},
		{"praise", "en"},
	}
	for _, tc := range cases {
		t.Run(tc.lang+"/"+tc.in, func(t *testing.T) {
			once := normalizeForLang(tc.in, tc.lang)
			twice := normalizeForLang(once, tc.lang)
			if once != twice {
				t.Errorf("not idempotent: first=%q second=%q", once, twice)
			}
		})
	}
}

// ── scoreGuess with language-aware normalizeForLang ──────────────────────────

// TestScoreGuessMultiLang verifies that scoreGuess calls normalizeForLang with
// the correct language and therefore accepts language-appropriate inputs. It
// exercises script paths not covered by the English-only TestScoreGuess.
func TestScoreGuessMultiLang(t *testing.T) {
	cases := []struct {
		name       string
		answer     string
		guess      string
		lang       string
		difficulty string
		want       bool
	}{
		// Arabic: diacritics on answer and guess both normalize away, so they
		// compare equal after normalizeForLang is applied to each.
		{
			name:       "ar diacritics on both sides",
			answer:     "الرَّحْمَٰنِ",
			guess:      "الرحمن",
			lang:       "ar",
			difficulty: "medium",
			want:       true,
		},
		{
			name:       "ar tatweel in guess",
			answer:     "الله",
			guess:      "اللـه",
			lang:       "ar",
			difficulty: "easy",
			want:       true,
		},
		{
			name:       "ar wrong word",
			answer:     "الله",
			guess:      "رحمن",
			lang:       "ar",
			difficulty: "medium",
			want:       false,
		},
		// French: accented guess matches unaccented stored answer (and vice
		// versa) because both sides are normalized before comparison.
		{
			name:       "fr accent on guess",
			answer:     "grace",
			guess:      "grâce",
			lang:       "fr",
			difficulty: "medium",
			want:       true,
		},
		{
			name:       "fr accent on answer",
			answer:     "grâce",
			guess:      "grace",
			lang:       "fr",
			difficulty: "medium",
			want:       true,
		},
		// German: umlaut equivalence across both sides.
		{
			name:       "de umlaut folded",
			answer:     "Über",
			guess:      "uber",
			lang:       "de",
			difficulty: "medium",
			want:       true,
		},
		// Spanish: tilde folded.
		{
			name:       "es tilde folded",
			answer:     "Mañana",
			guess:      "manana",
			lang:       "es",
			difficulty: "medium",
			want:       true,
		},
		// Languages without NFKD still apply case folding.
		{
			name:       "ru case fold",
			answer:     "Бог",
			guess:      "бог",
			lang:       "ru",
			difficulty: "easy",
			want:       true,
		},
		{
			name:       "id case fold",
			answer:     "Allah",
			guess:      "allah",
			lang:       "id",
			difficulty: "easy",
			want:       true,
		},
		// Empty guess is always rejected regardless of language.
		{
			name:       "empty guess ar",
			answer:     "الله",
			guess:      "",
			lang:       "ar",
			difficulty: "easy",
			want:       false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			blank := &blankRecord{Index: 0, Answer: tc.answer}
			got := scoreGuess(tc.guess, blank, tc.difficulty, tc.lang, nil)
			if got != tc.want {
				t.Errorf("scoreGuess(%q, answer=%q, lang=%q, diff=%q) = %v, want %v",
					tc.guess, tc.answer, tc.lang, tc.difficulty, got, tc.want)
			}
		})
	}
}

// ── maxAttemptsForDifficulty ─────────────────────────────────────────────────

// maxAttemptsForDifficulty determines whether CheckBlank enables attempt
// tracking. Easy returns 0 (no tracking, multiple-choice blanks); all other
// tiers return 3.
func TestMaxAttemptsForDifficulty(t *testing.T) {
	cases := []struct {
		difficulty string
		want       int
	}{
		{"easy", 0},
		{"medium", 3},
		{"hard", 3},
		{"professional", 3},
	}
	for _, tc := range cases {
		t.Run(tc.difficulty, func(t *testing.T) {
			got := maxAttemptsForDifficulty(tc.difficulty)
			if got != tc.want {
				t.Errorf("maxAttemptsForDifficulty(%q) = %d, want %d", tc.difficulty, got, tc.want)
			}
		})
	}
}

// ── wrongPenaltyForBlank ─────────────────────────────────────────────────────

// wrongPenaltyForBlank uses a triangular formula: N wrong attempts on a single
// blank costs 10 * N*(N+1)/2 points (1 wrong=10, 2 wrong=30, 3 wrong=60).
func TestWrongPenaltyForBlank(t *testing.T) {
	cases := []struct {
		wrongCount int
		want       int
	}{
		{0, 0},
		{1, 10},
		{2, 30},
		{3, 60},
		{4, 100},
	}
	for _, tc := range cases {
		t.Run(itoa(tc.wrongCount), func(t *testing.T) {
			got := wrongPenaltyForBlank(tc.wrongCount)
			if got != tc.want {
				t.Errorf("wrongPenaltyForBlank(%d) = %d, want %d", tc.wrongCount, got, tc.want)
			}
		})
	}
}
