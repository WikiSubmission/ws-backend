package handlers

import (
	"context"
	"testing"

	"github.com/wikiSubmission/ws-backend/db"
)

// Pure-logic tests for the topical index handlers: parameter validation and the
// db-tree-to-API mapping. The query behaviour itself is covered by
// db/topic_index_test.go against a live database.

func TestParseTopicVerseKey(t *testing.T) {
	cases := []struct {
		in      string
		chapter int
		verse   int
		ok      bool
	}{
		{"2:255", 2, 255, true},
		{"114:6", 114, 6, true},
		{" 2:255 ", 2, 255, true},
		// Verse 0 is the Basmalah, which every chapter except 1 and 9 carries.
		{"2:0", 2, 0, true},
		{"2", 0, 0, false},
		{"2:", 0, 0, false},
		{":255", 0, 0, false},
		{"a:b", 0, 0, false},
		{"2:-1", 0, 0, false},
		{"", 0, 0, false},
	}

	for _, c := range cases {
		chapter, verse, ok := parseTopicVerseKey(c.in)
		if ok != c.ok {
			t.Errorf("parseTopicVerseKey(%q) ok = %v, want %v", c.in, ok, c.ok)
			continue
		}
		if ok && (chapter != c.chapter || verse != c.verse) {
			t.Errorf("parseTopicVerseKey(%q) = %d:%d, want %d:%d", c.in, chapter, verse, c.chapter, c.verse)
		}
	}
}

func TestResolveTopicIndexLang(t *testing.T) {
	seeded := &Server{TopicIndexLangs: map[string]bool{"en": true}}

	if lang, ok := seeded.resolveTopicIndexLang(nil); !ok || lang != "en" {
		t.Errorf("nil lang resolved to %q/%v, want en/true", lang, ok)
	}
	if lang, ok := seeded.resolveTopicIndexLang(ptr("")); !ok || lang != "en" {
		t.Errorf("empty lang resolved to %q/%v, want en/true", lang, ok)
	}
	if lang, ok := seeded.resolveTopicIndexLang(ptr("en")); !ok || lang != "en" {
		t.Errorf("en resolved to %q/%v, want en/true", lang, ok)
	}
	if _, ok := seeded.resolveTopicIndexLang(ptr("zz")); ok {
		t.Error("an unseeded edition should be rejected")
	}

	// An empty cache means the index has not been seeded. Rejecting every lang
	// then would be misleading — the query simply returns nothing.
	unseeded := &Server{TopicIndexLangs: map[string]bool{}}
	if _, ok := unseeded.resolveTopicIndexLang(ptr("zz")); !ok {
		t.Error("with an unseeded index every lang should be accepted")
	}
}

func TestGetTopicIndexRejectsBadLetter(t *testing.T) {
	s := &Server{TopicIndexLangs: map[string]bool{"en": true}}
	ctx := context.Background()

	for _, letter := range []string{"", "AB", "1", "-", "ä"} {
		resp, err := s.GetTopicIndex(ctx, GetTopicIndexRequestObject{
			Params: GetTopicIndexParams{Letter: letter},
		})
		if err != nil {
			t.Fatalf("letter %q returned an error instead of a 400: %v", letter, err)
		}
		if _, isBadRequest := resp.(GetTopicIndex400JSONResponse); !isBadRequest {
			t.Errorf("letter %q was accepted, want 400", letter)
		}
	}
}

func TestGetTopicIndexRejectsBadPaging(t *testing.T) {
	s := &Server{TopicIndexLangs: map[string]bool{"en": true}}
	ctx := context.Background()

	for _, limit := range []int{0, -1, 501} {
		resp, err := s.GetTopicIndex(ctx, GetTopicIndexRequestObject{
			Params: GetTopicIndexParams{Letter: "A", Limit: ptr(limit)},
		})
		if err != nil {
			t.Fatalf("limit %d returned an error instead of a 400: %v", limit, err)
		}
		if _, isBadRequest := resp.(GetTopicIndex400JSONResponse); !isBadRequest {
			t.Errorf("limit %d was accepted, want 400", limit)
		}
	}

	resp, err := s.GetTopicIndex(ctx, GetTopicIndexRequestObject{
		Params: GetTopicIndexParams{Letter: "A", Offset: ptr(-1)},
	})
	if err != nil {
		t.Fatalf("negative offset returned an error instead of a 400: %v", err)
	}
	if _, isBadRequest := resp.(GetTopicIndex400JSONResponse); !isBadRequest {
		t.Error("a negative offset was accepted, want 400")
	}
}

func TestSearchTopicIndexRejectsBadQuery(t *testing.T) {
	s := &Server{TopicIndexLangs: map[string]bool{"en": true}}
	ctx := context.Background()

	tooLong := make([]byte, 201)
	for i := range tooLong {
		tooLong[i] = 'a'
	}

	for _, q := range []string{"", "a", "  ", " a ", string(tooLong)} {
		resp, err := s.SearchTopicIndex(ctx, SearchTopicIndexRequestObject{
			Params: SearchTopicIndexParams{Q: q},
		})
		if err != nil {
			t.Fatalf("q %q returned an error instead of a 400: %v", q, err)
		}
		if _, isBadRequest := resp.(SearchTopicIndex400JSONResponse); !isBadRequest {
			t.Errorf("q %q was accepted, want 400", q)
		}
	}
}

func TestGetTopicIndexForVerseRejectsBadKey(t *testing.T) {
	s := &Server{TopicIndexLangs: map[string]bool{"en": true}}
	ctx := context.Background()

	for _, key := range []string{"2", "abc", "0:1", "115:1"} {
		resp, err := s.GetTopicIndexForVerse(ctx, GetTopicIndexForVerseRequestObject{VerseKey: key})
		if err != nil {
			t.Fatalf("verseKey %q returned an error instead of a 400: %v", key, err)
		}
		if _, isBadRequest := resp.(GetTopicIndexForVerse400JSONResponse); !isBadRequest {
			t.Errorf("verseKey %q was accepted, want 400", key)
		}
	}
}

func TestGetTopicIndexForVerse404sPastEndOfChapter(t *testing.T) {
	// Chapter 114 has 6 verses; the cache is what the handler checks against.
	s := &Server{
		TopicIndexLangs: map[string]bool{"en": true},
		Chapters:        map[int]ChapterCache{114: {VerseCount: 6}},
	}

	resp, err := s.GetTopicIndexForVerse(context.Background(),
		GetTopicIndexForVerseRequestObject{VerseKey: "114:99"})
	if err != nil {
		t.Fatalf("returned an error instead of a 404: %v", err)
	}
	if _, isNotFound := resp.(GetTopicIndexForVerse404JSONResponse); !isNotFound {
		t.Error("a verse past the end of the chapter was accepted, want 404")
	}
}

func TestTopicEntriesToAPIMapsTheTree(t *testing.T) {
	entries := []db.TopicIndexEntry{{
		TopicEntryRow: db.TopicEntryRow{
			ID: 1, Lang: "en", Letter: "A", Title: "Abraham", Slug: "abraham",
			Position: 0, CrossRefs: []string{"Hood"},
		},
		Refs: []db.TopicRefRow{
			{ChapterNumber: 2, VerseStart: 124, VerseEnd: 124, Display: "2:124"},
		},
		Subentries: []db.TopicIndexSubentry{{
			TopicSubentryRow: db.TopicSubentryRow{ID: 9, EntryID: 1, Label: "monotheist", Position: 0},
			Refs: []db.TopicRefRow{
				{ChapterNumber: 7, VerseStart: 65, VerseEnd: 72, Display: "7:65-72"},
			},
		}},
		Score:    1.5,
		Headline: "<b>Abraham</b>",
	}}

	got := topicEntriesToAPI(entries)
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	e := got[0]

	if e.Title != "Abraham" || e.Letter != "A" || e.Slug != "abraham" {
		t.Errorf("entry fields did not carry over: %+v", e)
	}
	if e.Lang == nil || *e.Lang != "en" {
		t.Error("lang did not carry over")
	}
	if len(e.CrossRefs) != 1 || e.CrossRefs[0] != "Hood" {
		t.Errorf("cross_refs = %v, want [Hood]", e.CrossRefs)
	}
	if len(e.Refs) != 1 || e.Refs[0].Display != "2:124" || e.Refs[0].VerseEnd != 124 {
		t.Errorf("entry-level refs did not carry over: %+v", e.Refs)
	}
	if len(e.Subentries) != 1 || e.Subentries[0].Label != "monotheist" {
		t.Fatalf("sub-entries did not carry over: %+v", e.Subentries)
	}
	if r := e.Subentries[0].Refs; len(r) != 1 || r[0].VerseStart != 65 || r[0].VerseEnd != 72 {
		t.Errorf("sub-entry refs did not carry over: %+v", r)
	}
	if e.Score == nil || *e.Score != 1.5 {
		t.Error("score did not carry over")
	}
	if e.Headline == nil || *e.Headline != "<b>Abraham</b>" {
		t.Error("headline did not carry over")
	}
}

// Score and headline exist only on search results; elsewhere the keys must be
// omitted rather than sent as zeroes.
func TestTopicEntriesToAPIOmitsSearchOnlyFields(t *testing.T) {
	got := topicEntriesToAPI([]db.TopicIndexEntry{{
		TopicEntryRow: db.TopicEntryRow{Lang: "en", Letter: "A", Title: "Alimony", Slug: "alimony"},
		Refs:          []db.TopicRefRow{},
		Subentries:    []db.TopicIndexSubentry{},
	}})

	if got[0].Score != nil {
		t.Error("score should be omitted outside search results")
	}
	if got[0].Headline != nil {
		t.Error("headline should be omitted outside search results")
	}
}

// The API layer promises [] rather than null for every list, so a client never
// has to null-check an empty reference list.
func TestTopicEntriesToAPINeverEmitsNilSlices(t *testing.T) {
	got := topicEntriesToAPI([]db.TopicIndexEntry{{
		TopicEntryRow: db.TopicEntryRow{Lang: "en", Letter: "S", Title: "Spices", Slug: "spices"},
	}})

	if got[0].CrossRefs == nil {
		t.Error("cross_refs is nil, want an empty slice")
	}
	if got[0].Refs == nil {
		t.Error("refs is nil, want an empty slice")
	}
	if got[0].Subentries == nil {
		t.Error("subentries is nil, want an empty slice")
	}
}
