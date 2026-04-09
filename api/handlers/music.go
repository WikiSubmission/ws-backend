package handlers

import (
	"context"
	"log/slog"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/wikiSubmission/ws-backend/db"
)

// parseDate converts a "YYYY-MM-DD" string from the DB into openapi_types.Date.
func parseDate(s string) openapi_types.Date {
	t, _ := time.Parse("2006-01-02", s)
	return openapi_types.Date{Time: t}
}

func (s *Server) GetMusicArtists(ctx context.Context, request GetMusicArtistsRequestObject) (GetMusicArtistsResponseObject, error) {
	rows, err := s.DB.FetchMusicArtists(ctx)
	if err != nil {
		slog.Error("FetchMusicArtists failed", slog.Any("error", err))
		return GetMusicArtists500JSONResponse{InternalServerErrrorJSONResponse{Message: "failed to fetch artists"}}, nil
	}
	result := make([]MusicArtist, 0, len(rows))
	for _, r := range rows {
		result = append(result, musicArtistFromRow(r))
	}
	return GetMusicArtists200JSONResponse(result), nil
}

func (s *Server) GetMusicCategories(ctx context.Context, request GetMusicCategoriesRequestObject) (GetMusicCategoriesResponseObject, error) {
	rows, err := s.DB.FetchMusicCategories(ctx)
	if err != nil {
		slog.Error("FetchMusicCategories failed", slog.Any("error", err))
		return GetMusicCategories500JSONResponse{InternalServerErrrorJSONResponse{Message: "failed to fetch categories"}}, nil
	}
	result := make([]MusicCategory, 0, len(rows))
	for _, r := range rows {
		result = append(result, MusicCategory{
			Id:              r.ID,
			Name:            r.Name,
			Description:     r.Description,
			DisplayPriority: r.DisplayPriority,
		})
	}
	return GetMusicCategories200JSONResponse(result), nil
}

func (s *Server) GetMusicAlbums(ctx context.Context, request GetMusicAlbumsRequestObject) (GetMusicAlbumsResponseObject, error) {
	rows, err := s.DB.FetchMusicAlbums(ctx, request.Params.ArtistId)
	if err != nil {
		slog.Error("FetchMusicAlbums failed", slog.Any("error", err))
		return GetMusicAlbums500JSONResponse{InternalServerErrrorJSONResponse{Message: "failed to fetch albums"}}, nil
	}
	result := make([]MusicAlbum, 0, len(rows))
	for _, r := range rows {
		result = append(result, MusicAlbum{
			Id:          r.ID,
			Name:        r.Name,
			ArtistId:    r.ArtistID,
			ReleaseDate: parseDate(r.ReleaseDate),
			Description: r.Description,
		})
	}
	return GetMusicAlbums200JSONResponse(result), nil
}

func (s *Server) GetMusicTracks(ctx context.Context, request GetMusicTracksRequestObject) (GetMusicTracksResponseObject, error) {
	p := request.Params
	rows, err := s.DB.FetchMusicTracks(ctx, p.ArtistId, p.CategoryId, p.AlbumId, p.Featured)
	if err != nil {
		slog.Error("FetchMusicTracks failed", slog.Any("error", err))
		return GetMusicTracks500JSONResponse{InternalServerErrrorJSONResponse{Message: "failed to fetch tracks"}}, nil
	}
	result := make([]MusicTrack, 0, len(rows))
	for _, r := range rows {
		result = append(result, musicTrackFromRow(r))
	}
	return GetMusicTracks200JSONResponse(result), nil
}

// ---------------------------------------------------------------------------
// Row → schema helpers
// ---------------------------------------------------------------------------

func musicArtistFromRow(r db.MusicArtistRow) MusicArtist {
	return MusicArtist{
		Id:              r.ID,
		Name:            r.Name,
		Description:     r.Description,
		ImageUrl:        r.ImageURL,
		DisplayPriority: r.DisplayPriority,
	}
}

func musicTrackFromRow(r db.MusicTrackRow) MusicTrack {
	return MusicTrack{
		Id:          r.ID,
		Name:        r.Name,
		ReleaseDate: parseDate(r.ReleaseDate),
		Url:         r.URL,
		ArtistId:    r.ArtistID,
		CategoryId:  r.CategoryID,
		AlbumId:     r.AlbumID,
		Featured:    r.Featured,
		Lyrics:      r.Lyrics,
	}
}
