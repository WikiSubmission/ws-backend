package handlers

// detectQueryLangs inspects the Unicode script of the query and returns the
// subset of available language codes that should be used for the FTS phase.
//
// Rules:
//   - If the query is predominantly Arabic / Hebrew / Devanagari / CJK etc.,
//     only the matching language codes (filtered to those in available) are
//     returned.
//   - For Latin / ASCII queries — or when no available lang matches the detected
//     script — nil is returned, which the DB layer interprets as "all languages".
func detectQueryLangs(query string, available map[string]bool) []string {
	counts := make(map[string]int)
	total := 0

	for _, r := range query {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			counts["latin"]++
			total++
		case r >= 0x00C0 && r <= 0x024F: // Latin Extended
			counts["latin"]++
			total++
		case r >= 0x0600 && r <= 0x06FF: // Arabic
			counts["arabic"]++
			total++
		case r >= 0x0590 && r <= 0x05FF: // Hebrew
			counts["hebrew"]++
			total++
		case r >= 0x0900 && r <= 0x097F: // Devanagari (Hindi, Marathi…)
			counts["devanagari"]++
			total++
		case r >= 0x0400 && r <= 0x04FF: // Cyrillic
			counts["cyrillic"]++
			total++
		case r >= 0x4E00 && r <= 0x9FFF: // CJK Unified Ideographs
			counts["cjk"]++
			total++
		case r >= 0x3040 && r <= 0x30FF: // Hiragana + Katakana
			counts["japanese"]++
			total++
		case r >= 0xAC00 && r <= 0xD7AF: // Hangul
			counts["korean"]++
			total++
		}
	}

	if total == 0 {
		return nil
	}

	// Find the dominant script.
	var dominant string
	var max int
	for script, n := range counts {
		if n > max {
			max = n
			dominant = script
		}
	}

	// Script → candidate language codes.
	scriptToLangs := map[string][]string{
		"arabic":     {"ar", "fa", "ur", "ku"},
		"hebrew":     {"he"},
		"devanagari": {"hi", "mr", "ne"},
		"cyrillic":   {"ru", "bg", "uk", "sr", "mk"},
		"cjk":        {"zh", "zh-hans", "zh-hant"},
		"japanese":   {"ja"},
		"korean":     {"ko"},
	}

	candidates, ok := scriptToLangs[dominant]
	if !ok {
		// Latin or unrecognised — search across all languages.
		return nil
	}

	// Filter to langs actually in the DB.
	var result []string
	for _, lc := range candidates {
		if available[lc] {
			result = append(result, lc)
		}
	}
	if len(result) == 0 {
		return nil // no match in available; fall back to all
	}
	return result
}
