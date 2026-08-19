package handlers

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"github.com/wikiSubmission/ws-backend/db"
)

// The Quran's printed topical index (topic_index_entries; see db/topic_index.go
// and migrations/031_quran_topic_index.sql).
//
// This is a separate corpus from /search, which covers verse text, subtitles and
// footnotes. A client wanting both runs the two in parallel: the topical index
// answers "what does the book say about X", verse search answers "where does this
// wording appear". Keeping them apart leaves /search's ranking untouched.

// defaultTopicIndexLang is used when the caller does not name an edition. Only
// the English index exists today; when a second one lands this should follow the
// query's UI locale the way /site/search does.
const defaultTopicIndexLang = "en"

// resolveTopicIndexLang validates a caller-supplied edition. An empty
// TopicIndexLangs cache means the index has not been seeded, in which case
// rejecting every lang would be misleading — the query simply returns nothing.
func (s *Server) resolveTopicIndexLang(lang *string) (string, bool) {
	if lang == nil || *lang == "" {
		return defaultTopicIndexLang, true
	}
	if len(s.TopicIndexLangs) > 0 && !s.TopicIndexLangs[*lang] {
		return "", false
	}
	return *lang, true
}

// GetTopicIndex handles GET /topic-index: one alphabetical section of the index.
func (s *Server) GetTopicIndex(ctx context.Context, request GetTopicIndexRequestObject) (GetTopicIndexResponseObject, error) {
	p := request.Params

	letter := strings.ToUpper(strings.TrimSpace(p.Letter))
	if len(letter) != 1 || letter[0] < 'A' || letter[0] > 'Z' {
		return GetTopicIndex400JSONResponse{BadRequestJSONResponse{
			Message: "letter must be a single letter A-Z",
		}}, nil
	}

	lang, ok := s.resolveTopicIndexLang(p.Lang)
	if !ok {
		return GetTopicIndex400JSONResponse{BadRequestJSONResponse{
			Message: "unknown lang: " + *p.Lang,
		}}, nil
	}

	// 500 covers the largest letter (S, 186 entries) in one request, so the page
	// never has to fetch twice for a normal letter.
	limit := 500
	if p.Limit != nil {
		limit = *p.Limit
	}
	if limit < 1 || limit > 500 {
		return GetTopicIndex400JSONResponse{BadRequestJSONResponse{
			Message: "limit must be between 1 and 500",
		}}, nil
	}
	offset := 0
	if p.Offset != nil {
		offset = *p.Offset
	}
	if offset < 0 {
		return GetTopicIndex400JSONResponse{BadRequestJSONResponse{
			Message: "offset must be non-negative",
		}}, nil
	}

	entries, total, err := s.DB.TopicIndexByLetter(ctx, lang, letter, limit, offset)
	if err != nil {
		slog.Error("topic index letter fetch failed", "letter", letter, "lang", lang, "err", err)
		return nil, err
	}

	return GetTopicIndex200JSONResponse(TopicIndexResponse{
		Items: topicEntriesToAPI(entries),
		Total: total,
	}), nil
}

// GetTopicIndexLetters handles GET /topic-index/letters: the A-Z rail. The
// printed index has no X, so a client must not assume all 26 letters exist.
func (s *Server) GetTopicIndexLetters(ctx context.Context, request GetTopicIndexLettersRequestObject) (GetTopicIndexLettersResponseObject, error) {
	lang, ok := s.resolveTopicIndexLang(request.Params.Lang)
	if !ok {
		return GetTopicIndexLetters400JSONResponse{BadRequestJSONResponse{
			Message: "unknown lang: " + *request.Params.Lang,
		}}, nil
	}

	counts, err := s.DB.TopicIndexLetterCounts(ctx, lang)
	if err != nil {
		slog.Error("topic index letter counts failed", "lang", lang, "err", err)
		return nil, err
	}

	out := make([]TopicIndexLetter, len(counts))
	for i, c := range counts {
		out[i] = TopicIndexLetter{Letter: c.Letter, Count: c.Count}
	}
	return GetTopicIndexLetters200JSONResponse(out), nil
}

// SearchTopicIndex handles GET /topic-index/search.
func (s *Server) SearchTopicIndex(ctx context.Context, request SearchTopicIndexRequestObject) (SearchTopicIndexResponseObject, error) {
	p := request.Params

	query := strings.TrimSpace(p.Q)
	if len(query) < 2 {
		return SearchTopicIndex400JSONResponse{BadRequestJSONResponse{
			Message: "q must be at least 2 characters",
		}}, nil
	}
	if len(query) > 200 {
		return SearchTopicIndex400JSONResponse{BadRequestJSONResponse{
			Message: "q must be 200 characters or fewer",
		}}, nil
	}

	// An explicit lang restricts the search; omitting it searches every edition,
	// which the DB layer expresses as an empty string.
	lang := ""
	if p.Lang != nil && *p.Lang != "" {
		resolved, ok := s.resolveTopicIndexLang(p.Lang)
		if !ok {
			return SearchTopicIndex400JSONResponse{BadRequestJSONResponse{
				Message: "unknown lang: " + *p.Lang,
			}}, nil
		}
		lang = resolved
	}

	limit := 10
	if p.Limit != nil {
		limit = *p.Limit
	}
	if limit < 1 || limit > 100 {
		return SearchTopicIndex400JSONResponse{BadRequestJSONResponse{
			Message: "limit must be between 1 and 100",
		}}, nil
	}
	offset := 0
	if p.Offset != nil {
		offset = *p.Offset
	}
	if offset < 0 {
		return SearchTopicIndex400JSONResponse{BadRequestJSONResponse{
			Message: "offset must be non-negative",
		}}, nil
	}

	// The count runs alongside the search, the same shape api/handlers/search.go
	// uses for its own total.
	type countResult struct {
		total int
		err   error
	}
	countCh := make(chan countResult, 1)
	go func() {
		total, err := s.DB.CountSearchTopicIndex(ctx, query, lang)
		countCh <- countResult{total: total, err: err}
	}()

	entries, err := s.DB.SearchTopicIndex(ctx, query, lang, limit, offset)
	if err != nil {
		slog.Error("topic index search failed", "q", query, "err", err)
		return nil, err
	}

	total := 0
	if c := <-countCh; c.err != nil {
		// A failed count must not fail the search: fall back to what this page
		// holds, clamped up the way /search does.
		slog.Warn("topic index count failed", "q", query, "err", c.err)
		total = offset + len(entries)
	} else {
		total = c.total
		if total < offset+len(entries) {
			total = offset + len(entries)
		}
	}

	return SearchTopicIndex200JSONResponse(TopicIndexSearchResponse{
		Q:       query,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
		Results: topicEntriesToAPI(entries),
	}), nil
}

// GetTopicIndexForVerse handles GET /topic-index/verse/{verseKey}: the reverse
// lookup, so a verse can show which topics cite it.
func (s *Server) GetTopicIndexForVerse(ctx context.Context, request GetTopicIndexForVerseRequestObject) (GetTopicIndexForVerseResponseObject, error) {
	chapter, verse, ok := parseTopicVerseKey(request.VerseKey)
	if !ok {
		return GetTopicIndexForVerse400JSONResponse{BadRequestJSONResponse{
			Message: "verseKey must be chapter:verse, e.g. 2:255",
		}}, nil
	}
	if chapter < 1 || chapter > 114 {
		return GetTopicIndexForVerse400JSONResponse{BadRequestJSONResponse{
			Message: "chapter must be between 1 and 114",
		}}, nil
	}

	// Verse 0 is the Basmalah, which every chapter except 1 and 9 carries.
	if ch, cached := s.Chapters[chapter]; cached && verse > ch.VerseCount {
		return GetTopicIndexForVerse404JSONResponse{NotFoundJSONResponse{
			Message: "chapter " + strconv.Itoa(chapter) + " has no verse " + strconv.Itoa(verse),
		}}, nil
	}

	lang := ""
	if request.Params.Lang != nil && *request.Params.Lang != "" {
		resolved, ok := s.resolveTopicIndexLang(request.Params.Lang)
		if !ok {
			return GetTopicIndexForVerse400JSONResponse{BadRequestJSONResponse{
				Message: "unknown lang: " + *request.Params.Lang,
			}}, nil
		}
		lang = resolved
	}

	entries, err := s.DB.TopicIndexForVerse(ctx, lang, chapter, verse)
	if err != nil {
		slog.Error("topic index verse lookup failed", "verse_key", request.VerseKey, "err", err)
		return nil, err
	}

	items := topicEntriesToAPI(entries)
	return GetTopicIndexForVerse200JSONResponse(TopicIndexResponse{
		Items: items,
		Total: len(items),
	}), nil
}

// parseTopicVerseKey splits "2:255" into its parts. Verse numbers here are
// 1-based, matching the printed index and the frontend's ?verse=N.
func parseTopicVerseKey(key string) (chapter, verse int, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(key), ":", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	chapter, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	verse, err = strconv.Atoi(parts[1])
	if err != nil || verse < 0 {
		return 0, 0, false
	}
	return chapter, verse, true
}

// topicEntriesToAPI maps the db tree onto the generated response types. Slices
// are always non-nil so the JSON carries `[]` rather than `null` — a client
// should not have to null-check an empty reference list.
func topicEntriesToAPI(entries []db.TopicIndexEntry) []TopicIndexEntry {
	out := make([]TopicIndexEntry, len(entries))
	for i, e := range entries {
		lang := e.Lang
		item := TopicIndexEntry{
			Title:      e.Title,
			Letter:     e.Letter,
			Slug:       e.Slug,
			Lang:       &lang,
			CrossRefs:  e.CrossRefs,
			Refs:       topicRefsToAPI(e.Refs),
			Subentries: make([]TopicIndexSubentry, len(e.Subentries)),
		}
		if item.CrossRefs == nil {
			item.CrossRefs = []string{}
		}
		for j, sub := range e.Subentries {
			item.Subentries[j] = TopicIndexSubentry{
				Label: sub.Label,
				Refs:  topicRefsToAPI(sub.Refs),
			}
		}
		// Score and headline exist only on search results; leave them unset
		// elsewhere so the keys are omitted rather than sent as zeroes.
		if e.Headline != "" {
			headline := e.Headline
			item.Headline = &headline
		}
		if e.Score != 0 {
			score := e.Score
			item.Score = &score
		}
		out[i] = item
	}
	return out
}

func topicRefsToAPI(refs []db.TopicRefRow) []TopicIndexRef {
	out := make([]TopicIndexRef, len(refs))
	for i, r := range refs {
		out[i] = TopicIndexRef{
			ChapterNumber: r.ChapterNumber,
			VerseStart:    r.VerseStart,
			VerseEnd:      r.VerseEnd,
			Display:       r.Display,
		}
	}
	return out
}
