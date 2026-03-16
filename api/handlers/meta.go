package handlers

import (
	"context"
	"log/slog"
)

// getChapters
func (s *Server) GetChapters(ctx context.Context, request GetChaptersRequestObject) (GetChaptersResponseObject, error) {
	p := request.Params
	// Using a default language if p.Lang is nil might be safer depending on your API spec
	lang := "en"
	if p.Lang != nil {
		lang = *p.Lang
	}

	data, err := s.DB.FetchAllChapters(ctx, lang)
	if err != nil {
		slog.Error("Failed to fetch chapters", "error", err)
		return GetChapters500JSONResponse{
			InternalServerErrrorJSONResponse{Message: "Internal Server Error"},
		}, nil
	}

	// PRE-ALLOCATE: This prevents the slice from resizing multiple times in memory.
	chaptersData := make(ChapterMetadata, len(data))

	for i := range data {
		// We take the address of the fields.
		// Note: In Go 1.22+, taking &row.Field is safe.
		// In older versions, you'd need a local copy: r := row
		chaptersData[i] = Chapter{
			ChapterNumber:   &data[i].ChapterNumber,
			VerseCount:      &data[i].VerseCount,
			RevelationOrder: &data[i].RevelationOrder,
			Title:           &data[i].Title,
		}
	}

	return GetChapters200JSONResponse(chaptersData), nil
}

// getAppendices
// getAppendices
func (s *Server) GetAppendices(ctx context.Context, request GetAppendicesRequestObject) (GetAppendicesResponseObject, error) {
	p := request.Params
	lang := "en" // Default or handle as error
	if p.Lang != nil {
		lang = *p.Lang
	}

	data, err := s.DB.FetchAllAppendices(ctx, lang)
	if err != nil {
		slog.Error("Failed to fetch appendices", "error", err)
		return GetAppendices500JSONResponse{
			InternalServerErrrorJSONResponse{
				Message: "Internal Server Error",
			},
		}, nil
	}

	// Pre-allocate for efficiency
	appendicesData := make(AppendixMetadata, len(data))

	for i := range data {
		// Map db.AppendixRow fields to API Appendix pointers
		appendicesData[i] = Appendix{
			Code:  &data[i].Code,
			Title: &data[i].Title,
			// If your Appendix model has a Snippet field:
			// Snippet: &data[i].Snippet,
		}
	}

	// Return the successful response cast to the generated type
	return GetAppendices200JSONResponse(appendicesData), nil
}
