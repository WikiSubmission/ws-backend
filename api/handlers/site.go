package handlers

import (
	"context"
	"log/slog"

	"github.com/wikiSubmission/ws-backend/db"
)

// SearchSite handles GET /site/search: full-text search over the site's own
// pages (site_documents; see db/site.go) unioned with the library corpus.
//
// The search language is decided by the query's script, independently of the
// language results are displayed in — the same two-phase split /search uses. An
// explicit `lang` always wins over detection.
func (s *Server) SearchSite(ctx context.Context, request SearchSiteRequestObject) (SearchSiteResponseObject, error) {
	p := request.Params

	if len(p.Q) < 2 {
		return SearchSite400JSONResponse{BadRequestJSONResponse{Message: "q must be at least 2 characters"}}, nil
	}
	if len(p.Q) > 200 {
		return SearchSite400JSONResponse{BadRequestJSONResponse{Message: "q must be 200 characters or fewer"}}, nil
	}

	limit := 10
	if p.Limit != nil {
		limit = *p.Limit
	}
	if limit < 1 || limit > 50 {
		return SearchSite400JSONResponse{BadRequestJSONResponse{Message: "limit must be between 1 and 50"}}, nil
	}
	offset := 0
	if p.Offset != nil {
		offset = *p.Offset
	}
	if offset < 0 {
		return SearchSite400JSONResponse{BadRequestJSONResponse{Message: "offset must be non-negative"}}, nil
	}

	// Script detection narrows the languages searched; an explicit lang overrides
	// it. nil means "every indexed locale" in the DB layer.
	searchLangs := detectQueryLangs(p.Q, s.SiteLangs)
	if p.Lang != nil && *p.Lang != "" {
		// An empty SiteLangs means the catalogue has not been seeded, in which
		// case rejecting every lang would be misleading — the query simply
		// returns nothing.
		if len(s.SiteLangs) > 0 && !s.SiteLangs[*p.Lang] {
			return SearchSite400JSONResponse{BadRequestJSONResponse{Message: "unknown lang: " + *p.Lang}}, nil
		}
		searchLangs = []string{*p.Lang}
	}

	var kinds []string
	if p.Kind != nil && len(*p.Kind) > 0 {
		kinds = make([]string, 0, len(*p.Kind))
		for _, k := range *p.Kind {
			kinds = append(kinds, string(k))
		}
	}

	// Over-fetch so the cross-locale dedupe below cannot return a short page.
	rows, err := s.DB.SearchSiteDocuments(ctx, p.Q, searchLangs, kinds, limit*2, offset)
	if err != nil {
		slog.Error("site search failed", "q", p.Q, "err", err)
		return nil, err
	}

	preferred := ""
	if p.Lang != nil {
		preferred = *p.Lang
	}
	results := dedupeSiteHits(rows, preferred, limit)

	langs := searchLangs
	if langs == nil {
		langs = []string{}
	}

	return SearchSite200JSONResponse(SiteSearchResponse{
		Q:       p.Q,
		Limit:   limit,
		Offset:  offset,
		Langs:   &langs,
		Results: results,
	}), nil
}

// dedupeSiteHits collapses rows that point at the same place.
//
// The site has no locale path segment, so one route is one URL across all seven
// locales; a query whose script matches several of them can match the same page
// more than once. Keeps the highest-scoring row per (route, anchor), preferring
// the caller's own language when scores tie.
func dedupeSiteHits(rows []db.SiteHitRow, preferredLang string, limit int) []SiteSearchResult {
	type slot struct {
		index int
		score float32
		lang  string
	}
	seen := make(map[string]slot, len(rows))
	results := make([]SiteSearchResult, 0, len(rows))

	for _, r := range rows {
		anchor := ""
		if r.Anchor != nil {
			anchor = *r.Anchor
		}
		key := r.Route + "\x00" + anchor

		rank := r.Score
		candidate := SiteSearchResult{
			Route:       r.Route,
			Anchor:      r.Anchor,
			Lang:        r.Lang,
			Kind:        SiteSearchResultKind(r.Kind),
			Title:       r.Title,
			Heading:     r.Heading,
			Description: r.Description,
			Snippet:     r.Headline,
			Rank:        &rank,
		}

		existing, ok := seen[key]
		if !ok {
			seen[key] = slot{index: len(results), score: r.Score, lang: r.Lang}
			results = append(results, candidate)
			continue
		}

		better := r.Score > existing.score
		// On a tie, the caller's own language is the more useful row to show.
		if !better && r.Score == existing.score && preferredLang != "" {
			better = r.Lang == preferredLang && existing.lang != preferredLang
		}
		if better {
			results[existing.index] = candidate
			seen[key] = slot{index: existing.index, score: r.Score, lang: r.Lang}
		}
	}

	if len(results) > limit {
		results = results[:limit]
	}
	return results
}
