package handlers

import (
	"context"
	"log/slog"
)

// SearchLibrary handles GET /library/search: full-text search over the
// Introduction, Proclamation, and appendix sections (library_documents /
// library_sections; see db/library.go).
func (s *Server) SearchLibrary(ctx context.Context, request SearchLibraryRequestObject) (SearchLibraryResponseObject, error) {
	p := request.Params

	if len(p.Q) < 2 {
		return SearchLibrary400JSONResponse{BadRequestJSONResponse{Message: "q must be at least 2 characters"}}, nil
	}
	if len(p.Q) > 200 {
		return SearchLibrary400JSONResponse{BadRequestJSONResponse{Message: "q must be 200 characters or fewer"}}, nil
	}

	lang := ""
	if p.Lang != nil {
		lang = *p.Lang
	}

	limit := 20
	if p.Limit != nil {
		limit = *p.Limit
	}
	if limit < 1 || limit > 100 {
		return SearchLibrary400JSONResponse{BadRequestJSONResponse{Message: "limit must be between 1 and 100"}}, nil
	}
	offset := 0
	if p.Offset != nil {
		offset = *p.Offset
	}
	if offset < 0 {
		return SearchLibrary400JSONResponse{BadRequestJSONResponse{Message: "offset must be non-negative"}}, nil
	}

	rows, err := s.DB.SearchLibrarySections(ctx, p.Q, lang, limit, offset)
	if err != nil {
		slog.Error("library search failed", "q", p.Q, "err", err)
		return nil, err
	}
	total, err := s.DB.CountSearchLibrarySections(ctx, p.Q, lang)
	if err != nil {
		slog.Error("library search count failed", "q", p.Q, "err", err)
		return nil, err
	}

	results := make([]LibrarySearchResult, 0, len(rows))
	for _, r := range rows {
		rank := r.Score
		results = append(results, LibrarySearchResult{
			DocType:      LibrarySearchResultDocType(r.DocType),
			DocNumber:    r.DocNumber,
			Lang:         r.Lang,
			Title:        r.Title,
			SectionIndex: r.SectionIndex,
			Heading:      r.Heading,
			Snippet:      r.Headline,
			Rank:         &rank,
		})
	}

	return SearchLibrary200JSONResponse(LibrarySearchResponse{
		Q:       p.Q,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
		Results: results,
	}), nil
}
