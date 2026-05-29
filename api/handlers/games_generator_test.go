package handlers

import (
	"encoding/json"
	"strings"
	"testing"
)

// ── Verse fixtures ──────────────────────────────────────────────────────────

// fixtureVerses produces a deterministic set of generator-input verses with
// enough variety to exercise the scorer.
func fixtureVerses(count int) []generatorVerse {
	templates := []string{
		"Praise be to GOD, Lord of the universe.",
		"Most Gracious, Most Merciful.",
		"Master of the Day of Judgment.",
		"You alone we worship; You alone we ask for help.",
		"Guide us in the right path;",
		"the path of those whom You blessed; not of those who have deserved wrath, nor of the strayers.",
		"This scripture is infallible; a beacon for the righteous;",
		"who believe in the unseen, observe the Contact Prayers, and from our provisions to them, they give.",
		"And they believe in what was revealed to you, and in what was revealed before you, and with regard to the Hereafter, they are absolutely certain.",
		"These are guided by their Lord; these are the winners.",
		"As for those who disbelieve, it is the same for them; whether you warn them, or not warn them, they cannot believe.",
		"GOD seals their minds and their hearing, and their eyes are veiled. They have incurred severe retribution.",
	}
	out := make([]generatorVerse, 0, count)
	for i := 0; i < count; i++ {
		t := templates[i%len(templates)]
		verseKey := "1:" + itoa(i+1)
		out = append(out, generatorVerse{
			VerseKey: verseKey,
			Words:    tokenizeVerse(t),
		})
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

func fixtureInputs(t *testing.T, difficulty, size string, counter int) generatorInputs {
	t.Helper()
	return fixtureInputsWithBand(t, difficulty, size, "medium", counter)
}

func fixtureInputsWithBand(t *testing.T, difficulty, size, skillBand string, counter int) generatorInputs {
	t.Helper()
	verses := fixtureVerses(30)
	freqs := map[string]int{}
	for _, v := range verses {
		for _, w := range v.Words {
			if w.Normalized == "" {
				continue
			}
			freqs[w.Normalized]++
		}
	}
	pool := []string{}
	seen := map[string]struct{}{}
	for w := range freqs {
		if _, dup := seen[w]; dup {
			continue
		}
		seen[w] = struct{}{}
		pool = append(pool, w)
	}
	return generatorInputs{
		PassageID:   42,
		Language:    "en",
		Difficulty:  difficulty,
		Size:        size,
		SkillBand:   skillBand,
		Counter:     counter,
		Verses:      verses,
		Frequencies: freqs,
		ChapterPool: pool,
	}
}

// ── seedFor ─────────────────────────────────────────────────────────────────

func TestSeedFor(t *testing.T) {
	a := seedFor(42, "en", "hard", "medium", "medium", 7)
	b := seedFor(42, "en", "hard", "medium", "medium", 7)
	if a != b {
		t.Fatalf("seedFor not deterministic: %d != %d", a, b)
	}
	if seedFor(42, "en", "hard", "medium", "medium", 8) == a {
		t.Fatalf("seedFor collides on counter")
	}
	if seedFor(42, "en", "easy", "medium", "medium", 7) == a {
		t.Fatalf("seedFor collides on difficulty")
	}
	if seedFor(43, "en", "hard", "medium", "medium", 7) == a {
		t.Fatalf("seedFor collides on passage id")
	}
	if seedFor(42, "en", "hard", "medium", "hard", 7) == a {
		t.Fatalf("seedFor collides on skill band")
	}
}

// ── Tokenization ────────────────────────────────────────────────────────────

func TestTokenizeVerse(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   []string // surfaces
		normEq map[int]string
	}{
		{
			name: "simple",
			in:   "Praise be to GOD.",
			want: []string{"Praise", "be", "to", "GOD"},
			normEq: map[int]string{
				0: "praise",
				3: "god",
			},
		},
		{
			name: "apostrophe stays in word",
			in:   "It's God's mercy.",
			want: []string{"It's", "God's", "mercy"},
		},
		{
			name: "leading and trailing punct",
			in:   "  -- Praise --",
			want: []string{"Praise"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tokenizeVerse(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("len: got %d, want %d (%v)", len(got), len(tc.want), surfaces(got))
			}
			for i, w := range got {
				if w.Surface != tc.want[i] {
					t.Errorf("surface[%d] = %q, want %q", i, w.Surface, tc.want[i])
				}
			}
			for i, n := range tc.normEq {
				if got[i].Normalized != n {
					t.Errorf("normalized[%d] = %q, want %q", i, got[i].Normalized, n)
				}
			}
		})
	}
}

func surfaces(ws []verseWord) []string {
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = w.Surface
	}
	return out
}

// ── Determinism ────────────────────────────────────────────────────────────

func TestGenerateVariantDeterministic(t *testing.T) {
	in := fixtureInputs(t, "hard", "medium", 1)
	v1, err := generateVariant(in)
	if err != nil {
		t.Fatalf("generateVariant: %v", err)
	}
	v2, err := generateVariant(in)
	if err != nil {
		t.Fatalf("generateVariant repeat: %v", err)
	}
	if v1.VariantID != v2.VariantID {
		t.Fatalf("variant id changed: %q vs %q", v1.VariantID, v2.VariantID)
	}
	if string(v1.Blanks) != string(v2.Blanks) {
		t.Fatalf("blanks not deterministic")
	}
	if string(v1.Rendered) != string(v2.Rendered) {
		t.Fatalf("rendered not deterministic")
	}
}

func TestGenerateVariantCounterChangesOutput(t *testing.T) {
	a, err := generateVariant(fixtureInputs(t, "hard", "medium", 1))
	if err != nil {
		t.Fatalf("a: %v", err)
	}
	b, err := generateVariant(fixtureInputs(t, "hard", "medium", 2))
	if err != nil {
		t.Fatalf("b: %v", err)
	}
	if string(a.Blanks) == string(b.Blanks) && string(a.Rendered) == string(b.Rendered) {
		t.Fatalf("incrementing counter produced identical output")
	}
}

// ── Easy tier — options present ────────────────────────────────────────────

func TestGenerateVariantEasyHasOptions(t *testing.T) {
	v, err := generateVariant(fixtureInputs(t, "easy", "short", 1))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	var blanks []blankRecord
	if err := json.Unmarshal(v.Blanks, &blanks); err != nil {
		t.Fatalf("unmarshal blanks: %v", err)
	}
	if len(blanks) == 0 {
		t.Fatalf("no blanks generated")
	}
	for i, b := range blanks {
		if len(b.Options) != 4 {
			t.Errorf("blank[%d] options = %d, want 4 (%v)", i, len(b.Options), b.Options)
			continue
		}
		// Answer must be among the options.
		found := false
		for _, opt := range b.Options {
			if strings.EqualFold(opt, b.Answer) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("blank[%d] answer %q not in options %v", i, b.Answer, b.Options)
		}
		// All options unique.
		seen := map[string]struct{}{}
		for _, opt := range b.Options {
			key := strings.ToLower(opt)
			if _, dup := seen[key]; dup {
				t.Errorf("blank[%d] duplicate option %q", i, opt)
			}
			seen[key] = struct{}{}
		}
	}
}

// ── Non-easy tiers — no options ────────────────────────────────────────────

func TestGenerateVariantHardOmitsOptions(t *testing.T) {
	v, err := generateVariant(fixtureInputs(t, "hard", "medium", 1))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	var blanks []blankRecord
	if err := json.Unmarshal(v.Blanks, &blanks); err != nil {
		t.Fatalf("unmarshal blanks: %v", err)
	}
	for i, b := range blanks {
		if len(b.Options) != 0 {
			t.Errorf("blank[%d] unexpectedly has options: %v", i, b.Options)
		}
	}
}

// ── No consecutive blanks ──────────────────────────────────────────────────

func TestGenerateVariantNoConsecutiveBlanks(t *testing.T) {
	v, err := generateVariant(fixtureInputs(t, "hard", "medium", 3))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	var blanks []blankRecord
	if err := json.Unmarshal(v.Blanks, &blanks); err != nil {
		t.Fatalf("unmarshal blanks: %v", err)
	}
	byVerse := map[string][]int{}
	for _, b := range blanks {
		byVerse[b.VerseKey] = append(byVerse[b.VerseKey], b.WordPosition)
	}
	for vk, positions := range byVerse {
		for i := 1; i < len(positions); i++ {
			if positions[i]-positions[i-1] == 1 {
				t.Errorf("consecutive blanks in verse %s at positions %d, %d", vk, positions[i-1], positions[i])
			}
		}
	}
}

// ── Stopwords avoided for easy/medium ──────────────────────────────────────

func TestGenerateVariantSkipsStopwordsForEasyMedium(t *testing.T) {
	for _, diff := range []string{"easy", "medium"} {
		t.Run(diff, func(t *testing.T) {
			size := "short"
			if diff == "medium" {
				size = "medium"
			}
			v, err := generateVariant(fixtureInputs(t, diff, size, 5))
			if err != nil {
				t.Fatalf("generate: %v", err)
			}
			var blanks []blankRecord
			if err := json.Unmarshal(v.Blanks, &blanks); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			for _, b := range blanks {
				if _, hit := stopwords[strings.ToLower(b.Answer)]; hit {
					t.Errorf("stopword %q chosen as blank at %s:%d", b.Answer, b.VerseKey, b.WordPosition)
				}
			}
		})
	}
}

// ── Blank count within band ────────────────────────────────────────────────

func TestGenerateVariantBlankCountWithinBand(t *testing.T) {
	cases := []struct {
		size       string
		minB, maxB int
	}{
		{"short", 5, 8},
		{"medium", 12, 18},
		{"long", 25, 35},
	}
	for _, tc := range cases {
		t.Run(tc.size, func(t *testing.T) {
			v, err := generateVariant(fixtureInputs(t, "hard", tc.size, 1))
			if err != nil {
				t.Fatalf("generate: %v", err)
			}
			var blanks []blankRecord
			if err := json.Unmarshal(v.Blanks, &blanks); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if len(blanks) > tc.maxB {
				t.Errorf("blanks = %d, max = %d", len(blanks), tc.maxB)
			}
			// Bottom of band is best-effort (depends on available candidates),
			// but with our 30-verse fixture we should hit at least the minimum.
			if len(blanks) < tc.minB {
				t.Errorf("blanks = %d, min = %d", len(blanks), tc.minB)
			}
		})
	}
}

// ── Skill band changes output (different blank selection) ──────────────────

func TestGenerateVariantBandChangesOutput(t *testing.T) {
	easy, err := generateVariant(fixtureInputsWithBand(t, "hard", "medium", "easy", 1))
	if err != nil {
		t.Fatalf("easy band: %v", err)
	}
	hard, err := generateVariant(fixtureInputsWithBand(t, "hard", "medium", "hard", 1))
	if err != nil {
		t.Fatalf("hard band: %v", err)
	}
	if string(easy.Blanks) == string(hard.Blanks) {
		t.Fatalf("easy and hard bands produced identical blank selection")
	}
}

func TestGenerateVariantBandDeterministic(t *testing.T) {
	in := fixtureInputsWithBand(t, "hard", "medium", "hard", 1)
	v1, err := generateVariant(in)
	if err != nil {
		t.Fatalf("generateVariant: %v", err)
	}
	v2, err := generateVariant(in)
	if err != nil {
		t.Fatalf("generateVariant repeat: %v", err)
	}
	if v1.VariantID != v2.VariantID {
		t.Fatalf("variant id changed: %q vs %q", v1.VariantID, v2.VariantID)
	}
	if string(v1.Blanks) != string(v2.Blanks) {
		t.Fatalf("blanks not deterministic across bands")
	}
}

// ── Rendered text contains BLANK tokens at the right positions ─────────────

func TestGenerateVariantRenderedTokens(t *testing.T) {
	v, err := generateVariant(fixtureInputs(t, "hard", "short", 1))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	var blanks []blankRecord
	if err := json.Unmarshal(v.Blanks, &blanks); err != nil {
		t.Fatalf("unmarshal blanks: %v", err)
	}
	var rendered []renderedVerse
	if err := json.Unmarshal(v.Rendered, &rendered); err != nil {
		t.Fatalf("unmarshal rendered: %v", err)
	}
	// Each blank index must appear in exactly one rendered verse, exactly once.
	for _, b := range blanks {
		token := "__BLANK_" + itoa(b.Index) + "__"
		hits := 0
		for _, rv := range rendered {
			hits += strings.Count(rv.Text, token)
		}
		if hits != 1 {
			t.Errorf("token %s appeared %d times, want 1", token, hits)
		}
	}
}
