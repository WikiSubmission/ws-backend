package builder

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNextDataVersion(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   int
		fails  bool
	}{
		{"bumps past the highest published version", http.StatusOK,
			`{"bundles":[{"data_version":1},{"data_version":3},{"data_version":2}]}`, 4, false},
		{"starts at 1 when nothing is published (404)", http.StatusNotFound, "", 1, false},
		{"starts at 1 when nothing is published (403, S3-style)", http.StatusForbidden, "", 1, false},
		{"starts at 1 for an empty manifest", http.StatusOK, `{"bundles":[]}`, 1, false},
		{"errors on a server failure rather than resetting versions", http.StatusInternalServerError, "", 0, true},
		{"errors on malformed JSON", http.StatusOK, `{"bundles":`, 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			got, err := NextDataVersion(context.Background(), srv.Client(), srv.URL+"/manifest.json")
			if tc.fails {
				if err == nil {
					t.Fatalf("expected error, got version %d", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("NextDataVersion = %d, want %d", got, tc.want)
			}
		})
	}
}
