package db

import "testing"

func TestTransliterateRoot(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
		{"ktb", "ك ت ب", "k-t-b"},
		{"rhm with emphatic H", "ر ح م", "r-H-m"},
		{"alm — c is the convention for ع", "ع ل م", "c-l-m"},
		{"alh — alef → apostrophe", "ا ل ه", "'-l-h"},
		{"qra — terminal alef", "ق ر ا", "q-r-'"},
		{"amn", "ا م ن", "'-m-n"},
		{"trim leading/trailing spaces", "  ك ت ب  ", "k-t-b"},
		{"unmapped runes are dropped", "ك ! ت", "k-t"},
		{"non-arabic input dropped entirely", "abc def", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := TransliterateRoot(tc.in)
			if got != tc.want {
				t.Errorf("TransliterateRoot(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
