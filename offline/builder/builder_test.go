package builder

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// fakeCDN serves a manifest plus a fixed set of artifact files, responding to
// HEAD with sizes like CloudFront would.
func fakeCDN(manifest string, files map[string]int64) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest.json" {
			if manifest == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(manifest))
			return
		}
		size, ok := files[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusForbidden) // S3-style missing key
			return
		}
		w.Header().Set("Last-Modified", "Sat, 05 Jul 2026 00:00:00 GMT")
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.WriteHeader(http.StatusOK)
	}))
}

func TestListPublished(t *testing.T) {
	manifest := `{"bundles":[
		{"id":"quran-en","data_version":2},
		{"id":"quran-ar","data_version":2},
		{"id":"quran-words-en","data_version":2}
	]}`
	files := map[string]int64{
		"/quran-en-v1.db":       100,
		"/quran-ar-v1.db":       100,
		"/quran-en-v2.db":       200,
		"/quran-ar-v2.db":       200,
		"/quran-words-en-v2.db": 300,
		"/manifest-v2.json":     10,
		// v3 bundle left behind by a failed rebuild — no manifest references it.
		"/quran-en-v3.db": 400,
	}
	srv := fakeCDN(manifest, files)
	defer srv.Close()

	state, err := ListPublished(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if state.LiveVersion != 2 {
		t.Errorf("LiveVersion = %d, want 2", state.LiveVersion)
	}
	if len(state.Versions) != 3 {
		t.Fatalf("got %d versions, want 3 (v3 orphan, v2 live, v1)", len(state.Versions))
	}

	// Newest first: v3 (orphan), v2 (live), v1.
	if state.Versions[0].DataVersion != 3 || state.Versions[0].Live {
		t.Errorf("versions[0] = %+v, want non-live v3", state.Versions[0])
	}
	if state.Versions[1].DataVersion != 2 || !state.Versions[1].Live {
		t.Errorf("versions[1] = %+v, want live v2", state.Versions[1])
	}
	if got := len(state.Versions[1].Files); got != 4 {
		t.Errorf("v2 has %d files, want 4 (three bundles + manifest snapshot)", got)
	}
	// v1 predates the words bundle and manifest snapshots: only the two text bundles.
	if got := len(state.Versions[2].Files); got != 2 {
		t.Errorf("v1 has %d files, want 2", got)
	}
	for _, f := range state.Versions[2].Files {
		if f.Name != "quran-en-v1.db" && f.Name != "quran-ar-v1.db" {
			t.Errorf("unexpected v1 file %q", f.Name)
		}
	}
}

func TestListPublishedNothingPublished(t *testing.T) {
	srv := fakeCDN("", nil)
	defer srv.Close()

	state, err := ListPublished(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if state.LiveVersion != 0 || len(state.Versions) != 0 {
		t.Errorf("expected empty state, got %+v", state)
	}
}

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
