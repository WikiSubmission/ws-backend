package db

import "strings"

// arabicToBuckwalter maps each Arabic letter to its Latin Buckwalter form,
// matching the casing convention used by the Word Lab frontend reference
// (mixed case: emphatic letters keep their uppercase; alef → apostrophe).
var arabicToBuckwalter = map[rune]string{
	'ا': "'", // ا alef
	'ب': "b", // ب
	'ت': "t", // ت
	'ث': "v", // ث
	'ج': "j", // ج
	'ح': "H", // ح
	'خ': "x", // خ
	'د': "d", // د
	'ذ': "*", // ذ
	'ر': "r", // ر
	'ز': "z", // ز
	'س': "s", // س
	'ش': "$", // ش
	'ص': "S", // ص
	'ض': "D", // ض
	'ط': "T", // ط
	'ظ': "Z", // ظ
	'ع': "c", // ع
	'غ': "g", // غ
	'ف': "f", // ف
	'ق': "q", // ق
	'ك': "k", // ك
	'ل': "l", // ل
	'م': "m", // م
	'ن': "n", // ن
	'ه': "h", // ه
	'و': "w", // و
	'ي': "y", // ي
	'ء': "'", // ء hamza
	'آ': "|", // آ alef madda
	'أ': ">", // أ alef hamza above
	'إ': "<", // إ alef hamza below
	'ؤ': "&", // ؤ waw hamza
	'ئ': "}", // ئ ya hamza
	'ة': "p", // ة ta marbuta
	'ى': "Y", // ى alef maqsura
}

// TransliterateRoot converts a space-separated Arabic root (e.g. "ك ت ب")
// to its Buckwalter form, joined with hyphens (e.g. "k-t-b").
//
// Letter groups separated by spaces are transliterated independently and
// joined by "-". Unmapped runes are dropped silently. Returns "" for empty
// or all-unmapped input.
func TransliterateRoot(arabic string) string {
	arabic = strings.TrimSpace(arabic)
	if arabic == "" {
		return ""
	}
	groups := strings.Fields(arabic)
	out := make([]string, 0, len(groups))
	for _, g := range groups {
		var sb strings.Builder
		for _, r := range g {
			if mapped, ok := arabicToBuckwalter[r]; ok {
				sb.WriteString(mapped)
			}
		}
		if sb.Len() > 0 {
			out = append(out, sb.String())
		}
	}
	return strings.Join(out, "-")
}
