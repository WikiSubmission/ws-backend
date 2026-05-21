package handlers

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/wikiSubmission/ws-backend/db"
)

// GetRoots returns a paginated index of Quranic Arabic roots aggregated from
// the materialized view. Sort modes: frequency (default), abjadi, reverse.
func (s *Server) GetRoots(ctx context.Context, request GetRootsRequestObject) (GetRootsResponseObject, error) {
	p := request.Params

	sort := "frequency"
	if p.Sort != nil {
		sort = string(*p.Sort)
		if sort != "frequency" && sort != "abjadi" && sort != "reverse" {
			return GetRoots400JSONResponse{BadRequestJSONResponse{
				Message: "sort must be one of: frequency, abjadi, reverse",
			}}, nil
		}
	}

	limit := 50
	if p.Limit != nil {
		if *p.Limit < 1 {
			return GetRoots400JSONResponse{BadRequestJSONResponse{
				Message: "limit must be >= 1",
			}}, nil
		}
		limit = *p.Limit
	}
	offset := 0
	if p.Offset != nil {
		if *p.Offset < 0 {
			return GetRoots400JSONResponse{BadRequestJSONResponse{
				Message: "offset must be >= 0",
			}}, nil
		}
		offset = *p.Offset
	}

	var qPtr *string
	if p.Q != nil {
		trimmed := strings.TrimSpace(*p.Q)
		if trimmed != "" {
			qPtr = &trimmed
		}
	}

	rows, total, err := s.DB.RootsIndex(ctx, qPtr, sort, limit, offset)
	if err != nil {
		slog.Error("roots index query failed", "error", err)
		return GetRoots500JSONResponse{InternalServerErrrorJSONResponse{
			Message: "roots index failed",
		}}, nil
	}

	items := make([]RootSummary, 0, len(rows))
	for _, r := range rows {
		items = append(items, RootSummary{
			R:  r.Letters,
			Tr: db.TransliterateRoot(r.Letters),
			C:  r.Occurrences,
		})
	}
	return GetRoots200JSONResponse{
		Items: items,
		Total: total,
	}, nil
}

// GetRootDetail returns full metadata for a single root: summary plus
// derived surface forms and the first 20 occurrences.
func (s *Server) GetRootDetail(ctx context.Context, request GetRootDetailRequestObject) (GetRootDetailResponseObject, error) {
	letters := strings.TrimSpace(request.Letters)
	if letters == "" {
		return GetRootDetail400JSONResponse{BadRequestJSONResponse{
			Message: "letters path parameter must not be empty",
		}}, nil
	}

	summary, derivRows, occRows, meaning, err := s.DB.RootDetail(ctx, letters)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GetRootDetail404JSONResponse{NotFoundJSONResponse{
				Message: "root not found: " + letters,
			}}, nil
		}
		slog.Error("root detail query failed", "letters", letters, "error", err)
		return GetRootDetail500JSONResponse{InternalServerErrrorJSONResponse{
			Message: "root detail failed",
		}}, nil
	}

	derivs := make([]Derivative, 0, len(derivRows))
	for _, d := range derivRows {
		derivs = append(derivs, Derivative{
			Ar: d.Ar,
			Tr: d.Tr,
			En: d.En,
			C:  d.Count,
		})
	}
	occs := make([]Occurrence, 0, len(occRows))
	for _, o := range occRows {
		hl := o.Hl
		occs = append(occs, Occurrence{
			Vk: o.VerseKey(),
			Ar: o.Ar,
			En: o.En,
			Hl: &hl,
			Tl: o.Tl,
			Wi: o.Wi,
		})
	}

	return GetRootDetail200JSONResponse{
		R:  summary.Letters,
		Tr: db.TransliterateRoot(summary.Letters),
		C:  summary.Occurrences,
		Dv: &derivs,
		Oc: &occs,
		M:  meaning,
	}, nil
}

// GetRootOccurrences returns paginated occurrences for a single root.
func (s *Server) GetRootOccurrences(ctx context.Context, request GetRootOccurrencesRequestObject) (GetRootOccurrencesResponseObject, error) {
	letters := strings.TrimSpace(request.Letters)
	if letters == "" {
		return GetRootOccurrences400JSONResponse{BadRequestJSONResponse{
			Message: "letters path parameter must not be empty",
		}}, nil
	}

	p := request.Params
	limit := 50
	if p.Limit != nil {
		if *p.Limit < 1 {
			return GetRootOccurrences400JSONResponse{BadRequestJSONResponse{
				Message: "limit must be >= 1",
			}}, nil
		}
		limit = *p.Limit
	}
	offset := 0
	if p.Offset != nil {
		if *p.Offset < 0 {
			return GetRootOccurrences400JSONResponse{BadRequestJSONResponse{
				Message: "offset must be >= 0",
			}}, nil
		}
		offset = *p.Offset
	}

	surface := ""
	if p.Surface != nil {
		surface = strings.TrimSpace(*p.Surface)
	}

	rows, total, err := s.DB.RootOccurrences(ctx, letters, surface, limit, offset)
	if err != nil {
		slog.Error("root occurrences query failed", "letters", letters, "error", err)
		return GetRootOccurrences500JSONResponse{InternalServerErrrorJSONResponse{
			Message: "root occurrences failed",
		}}, nil
	}

	items := make([]Occurrence, 0, len(rows))
	for _, o := range rows {
		hl := o.Hl
		items = append(items, Occurrence{
			Vk: o.VerseKey(),
			Ar: o.Ar,
			En: o.En,
			Hl: &hl,
			Tl: o.Tl,
			Wi: o.Wi,
		})
	}
	return GetRootOccurrences200JSONResponse{
		Items: items,
		Total: total,
	}, nil
}
