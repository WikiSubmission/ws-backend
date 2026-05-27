package handlers

import "testing"

func TestParseVerseKey(t *testing.T) {
	cases := []struct {
		name      string
		scripture string
		key       string
		want      []int
		ok        bool
	}{
		{"quran simple", "quran", "2:255", []int{2, 255}, true},
		{"quran first", "quran", "1:1", []int{1, 1}, true},
		{"quran whitespace", "quran", " 3 : 10 ", []int{3, 10}, true},
		{"quran wrong parts", "quran", "2:3:4", nil, false},
		{"quran non-numeric", "quran", "2:abc", nil, false},
		{"bible simple", "bible", "40:5:3", []int{40, 5, 3}, true},
		{"bible wrong parts", "bible", "40:5", nil, false},
		{"unknown scripture", "torah", "1:1", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := parseVerseKey(c.scripture, c.key)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if !ok {
				return
			}
			if len(got) != len(c.want) {
				t.Fatalf("len = %d, want %d", len(got), len(c.want))
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("nums[%d] = %d, want %d", i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestCoveredCount(t *testing.T) {
	cases := []struct {
		name   string
		oldOrd int
		newOrd int
		want   int
	}{
		{"first marker counts all up to new", 0, 10, 10},
		{"forward advance counts the gap", 1, 10, 9},
		{"single verse advance", 7, 8, 1},
		{"no movement counts zero", 12, 12, 0},
		{"backward movement counts zero", 20, 5, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := coveredCount(c.oldOrd, c.newOrd); got != c.want {
				t.Errorf("coveredCount(%d, %d) = %d, want %d", c.oldOrd, c.newOrd, got, c.want)
			}
		})
	}
}
