package main

import "testing"

func TestResolveStatusChange(t *testing.T) {
	cases := []struct {
		name                         string
		approve, reject, needsRefine bool
		wantStatus                   string
		wantChanging                 bool
		wantErr                      bool
	}{
		{"none", false, false, false, "", false, false},
		{"approve", true, false, false, "approved", true, false},
		{"reject", false, true, false, "rejected", true, false},
		{"needs refinement", false, false, true, "needs_refinement", true, false},
		{"conflict", true, true, false, "", false, true},
		{"triple conflict", true, true, true, "", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, changing, err := resolveStatusChange(tc.approve, tc.reject, tc.needsRefine)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if status != tc.wantStatus {
				t.Errorf("status = %q, want %q", status, tc.wantStatus)
			}
			if changing != tc.wantChanging {
				t.Errorf("changing = %v, want %v", changing, tc.wantChanging)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"short", 40, "short"},
		{"exactly-ten", 11, "exactly-ten"},
		{"this is a very long label indeed", 10, "this is a…"},
	}
	for _, tc := range cases {
		if got := truncate(tc.in, tc.n); got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
		}
	}
}
