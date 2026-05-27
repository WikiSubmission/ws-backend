package auth

import "testing"

func TestParseEmailAllowlist(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string // emails expected present
		deny []string // emails expected absent
	}{
		{
			name: "basic comma list with spacing and case",
			raw:  " Editor@Example.com , hesham@wikisubmission.org ",
			want: []string{"editor@example.com", "hesham@wikisubmission.org"},
			deny: []string{"other@example.com"},
		},
		{
			name: "empty string denies everyone",
			raw:  "",
			deny: []string{"editor@example.com", ""},
		},
		{
			name: "trailing commas and blanks ignored",
			raw:  "a@b.com,,, ,",
			want: []string{"a@b.com"},
			deny: []string{""},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			set := ParseEmailAllowlist(tc.raw)
			for _, e := range tc.want {
				if _, ok := set[e]; !ok {
					t.Errorf("expected %q in allowlist", e)
				}
			}
			for _, e := range tc.deny {
				if _, ok := set[e]; ok {
					t.Errorf("did not expect %q in allowlist", e)
				}
			}
		})
	}
}
