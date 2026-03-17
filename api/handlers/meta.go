package handlers

import (
	"context"
	"log/slog"
)

func (s *Server) GetLanguages(ctx context.Context, request GetLanguagesRequestObject) (GetLanguagesResponseObject, error) {
	data, err := s.DB.FetchAllLanguages(ctx)
	if err != nil {
		slog.Error("Failed to fetch languages", "error", err)
		return GetLanguages500JSONResponse{
			InternalServerErrrorJSONResponse{Message: "Internal Server Error"},
		}, nil
	}

	result := make([]Language, len(data))
	for i := range data {
		result[i] = Language{
			Id:        &data[i].ID,
			Code:      &data[i].Code,
			Name:      &data[i].Name,
			Direction: (*LanguageDirection)(&data[i].Direction),
		}
	}

	return GetLanguages200JSONResponse(result), nil
}

func (s *Server) GetChapters(ctx context.Context, request GetChaptersRequestObject) (GetChaptersResponseObject, error) {
	langCode := "en"
	if request.Params.Lang != nil {
		langCode = *request.Params.Lang
	}

	langCache, ok := s.Languages[langCode]
	if !ok {
		return GetChapters400JSONResponse{BadRequestJSONResponse{
			Message: "unsupported language: " + langCode,
		}}, nil
	}

	data, err := s.DB.FetchAllChapters(ctx, langCache.ID)
	if err != nil {
		slog.Error("Failed to fetch chapters", "error", err)
		return GetChapters500JSONResponse{
			InternalServerErrrorJSONResponse{Message: "Internal Server Error"},
		}, nil
	}

	chaptersData := make(ChapterMetadata, len(data))
	for i := range data {
		chaptersData[i] = Chapter{
			ChapterNumber:   &data[i].ChapterNumber,
			VerseCount:      &data[i].VerseCount,
			RevelationOrder: &data[i].RevelationOrder,
			Title:           &data[i].Title,
		}
	}

	return GetChapters200JSONResponse(chaptersData), nil
}

func (s *Server) GetAppendices(ctx context.Context, request GetAppendicesRequestObject) (GetAppendicesResponseObject, error) {
	langCode := "en"
	if request.Params.Lang != nil {
		langCode = *request.Params.Lang
	}

	langCache, ok := s.Languages[langCode]
	if !ok {
		return GetAppendices400JSONResponse{BadRequestJSONResponse{
			Message: "unsupported language: " + langCode,
		}}, nil
	}

	data, err := s.DB.FetchAllAppendices(ctx, langCache.ID)
	if err != nil {
		slog.Error("Failed to fetch appendices", "error", err)
		return GetAppendices500JSONResponse{
			InternalServerErrrorJSONResponse{Message: "Internal Server Error"},
		}, nil
	}

	appendicesData := make(AppendixMetadata, len(data))
	for i := range data {
		appendicesData[i] = Appendix{
			Code:    &data[i].Code,
			Title:   &data[i].Title,
			Snippet: &data[i].Snippet,
		}
	}

	return GetAppendices200JSONResponse(appendicesData), nil
}
