package textnorm

import (
	"encoding/json"
	"os"
	"testing"
)

// vectorFile mirrors the shared vectors.json shape. The file is a verbatim copy
// of org/packages/shared/lib/text-normalization/vectors.json; the Go and TS
// implementations must agree on every case.
type vectorFile struct {
	Version int `json:"version"`
	Cases   []struct {
		Name string `json:"name"`
		In   string `json:"in"`
		Out  string `json:"out"`
	} `json:"cases"`
}

func loadVectors(t *testing.T) vectorFile {
	t.Helper()
	data, err := os.ReadFile("testdata/normalization_vectors.json")
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	var vf vectorFile
	if err := json.Unmarshal(data, &vf); err != nil {
		t.Fatalf("parse vectors: %v", err)
	}
	if len(vf.Cases) == 0 {
		t.Fatal("vectors file has no cases")
	}
	return vf
}

func TestForSearchMatchesSharedVectors(t *testing.T) {
	vf := loadVectors(t)
	if vf.Version != Version {
		t.Fatalf("vector version %d != textnorm.Version %d (keep TS, Go, and vectors in lockstep)", vf.Version, Version)
	}
	for _, c := range vf.Cases {
		if got := ForSearch(c.In); got != c.Out {
			t.Errorf("%s: ForSearch(%q) = %q, want %q", c.Name, c.In, got, c.Out)
		}
	}
}

func TestForSearchIdempotent(t *testing.T) {
	vf := loadVectors(t)
	for _, c := range vf.Cases {
		once := ForSearch(c.In)
		if twice := ForSearch(once); twice != once {
			t.Errorf("%s: not idempotent: %q -> %q", c.Name, once, twice)
		}
	}
}
