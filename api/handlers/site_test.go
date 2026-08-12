package handlers

import (
	"testing"

	"github.com/wikiSubmission/ws-backend/db"
)

func ptr[T any](v T) *T { return &v }

// dedupeSiteHits collapses rows pointing at the same place. The site has no
// locale path segment, so one route is one URL across all seven locales and a
// script-matched query can return the same page more than once.
func TestDedupeSiteHitsCollapsesByRouteAndAnchor(t *testing.T) {
	rows := []db.SiteHitRow{
		{Route: "/practices/zakat", Lang: "ar", Kind: "practice", Title: "الزكاة", Score: 1.2},
		{Route: "/practices/zakat", Lang: "en", Kind: "practice", Title: "Zakat", Score: 1.9},
		{Route: "/downloads", Lang: "en", Kind: "page", Title: "Downloads", Score: 0.5},
	}

	got := dedupeSiteHits(rows, "", 10)

	if len(got) != 2 {
		t.Fatalf("got %d results, want 2 (zakat collapsed)", len(got))
	}
	if got[0].Route != "/practices/zakat" || got[0].Lang != "en" {
		t.Errorf("kept the wrong row: %s/%s, want /practices/zakat/en (higher score)",
			got[0].Route, got[0].Lang)
	}
}

// An anchor makes a row a distinct destination, so a section hit must survive
// alongside its page.
func TestDedupeSiteHitsKeepsDistinctAnchors(t *testing.T) {
	rows := []db.SiteHitRow{
		{Route: "/practices/zakat", Lang: "en", Kind: "practice", Title: "Zakat", Score: 1.9},
		{Route: "/practices/zakat", Anchor: ptr("#calculating"), Lang: "en", Kind: "section",
			Title: "Zakat", Score: 1.5},
	}

	if got := dedupeSiteHits(rows, "", 10); len(got) != 2 {
		t.Fatalf("got %d results, want 2 (page and section are different destinations)", len(got))
	}
}

// On an equal score the caller's own language is the more useful row to show.
func TestDedupeSiteHitsPrefersCallerLangOnTie(t *testing.T) {
	rows := []db.SiteHitRow{
		{Route: "/practices/zakat", Lang: "ar", Kind: "practice", Title: "الزكاة", Score: 1.5},
		{Route: "/practices/zakat", Lang: "de", Kind: "practice", Title: "Zakat", Score: 1.5},
	}

	got := dedupeSiteHits(rows, "de", 10)
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	if got[0].Lang != "de" {
		t.Errorf("lang = %q, want de (caller's language wins a tie)", got[0].Lang)
	}
}

// A tie with no caller preference must not reorder: first row seen wins, so
// results are stable rather than dependent on map iteration.
func TestDedupeSiteHitsStableOnTieWithoutPreference(t *testing.T) {
	rows := []db.SiteHitRow{
		{Route: "/x", Lang: "ar", Title: "first", Score: 1.0},
		{Route: "/x", Lang: "de", Title: "second", Score: 1.0},
	}

	for range 20 {
		got := dedupeSiteHits(rows, "", 10)
		if len(got) != 1 || got[0].Lang != "ar" {
			t.Fatalf("unstable tie-break: got %+v", got)
		}
	}
}

func TestDedupeSiteHitsAppliesLimit(t *testing.T) {
	rows := []db.SiteHitRow{
		{Route: "/a", Lang: "en", Title: "a", Score: 3},
		{Route: "/b", Lang: "en", Title: "b", Score: 2},
		{Route: "/c", Lang: "en", Title: "c", Score: 1},
	}

	if got := dedupeSiteHits(rows, "", 2); len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
}

func TestDedupeSiteHitsHandlesEmptyInput(t *testing.T) {
	got := dedupeSiteHits(nil, "en", 10)
	if got == nil {
		t.Error("returned nil; an empty slice keeps the JSON an array rather than null")
	}
	if len(got) != 0 {
		t.Errorf("got %d results, want 0", len(got))
	}
}

// The headline carries the <b> markup the client's highlight splitter expects,
// and it must survive the mapping into the response type unchanged.
func TestDedupeSiteHitsPreservesSnippetAndRank(t *testing.T) {
	rows := []db.SiteHitRow{{
		Route: "/practices/zakat", Lang: "en", Kind: "practice", Title: "Zakat",
		Heading: ptr("Calculating zakat"), Description: ptr("Obligatory Charity"),
		Headline: "the <b>zakat</b> due", Score: 1.925,
	}}

	got := dedupeSiteHits(rows, "", 10)
	if got[0].Snippet != "the <b>zakat</b> due" {
		t.Errorf("snippet = %q, want the <b> markup preserved", got[0].Snippet)
	}
	if got[0].Rank == nil || *got[0].Rank != 1.925 {
		t.Errorf("rank not carried through: %v", got[0].Rank)
	}
	if got[0].Heading == nil || *got[0].Heading != "Calculating zakat" {
		t.Errorf("heading not carried through: %v", got[0].Heading)
	}
	if got[0].Kind != SiteSearchResultKind("practice") {
		t.Errorf("kind = %q, want practice", got[0].Kind)
	}
}
