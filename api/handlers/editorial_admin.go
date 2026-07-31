package handlers

import (
	"context"
	"errors"
	"log/slog"

	"github.com/wikiSubmission/ws-backend/db"
)

// Admin-only editorial grant management (the /editor/admin surface). Lists
// users with their editorial grants and replaces one user's grants atomically.
// Only admins pass; module/version grants never grant access to this surface.

// validEditorModules mirrors the editor_module DB enum for input validation.
//
// "game" is accepted here — it is the global games tier — but is deliberately
// absent from the EditorialContentModule enum, since games have no content
// documents and must never route into the /editorial/content endpoints.
var validEditorModules = map[string]bool{
	"article":   true,
	"quran":     true,
	"bible":     true,
	"community": true,
	"author":    true,
	"category":  true,
	"appendix":  true,
	"game":      true,
}

// gameGrantRows validates the requested per-game grants against the registry
// and converts them to DB rows. A non-empty badKey names the first unknown game
// key; games is only meaningful when badKey is empty and err is nil.
func (s *Server) gameGrantRows(
	ctx context.Context,
	grants *[]EditorGameGrant,
) (games []db.EditorGameGrantRow, badKey string, err error) {
	if grants == nil || len(*grants) == 0 {
		return nil, "", nil
	}
	known, err := s.DB.FetchEditorGameKeys(ctx)
	if err != nil {
		return nil, "", err
	}
	games = make([]db.EditorGameGrantRow, 0, len(*grants))
	for _, g := range *grants {
		if !known[g.GameKey] {
			return nil, g.GameKey, nil
		}
		games = append(games, db.EditorGameGrantRow{GameKey: g.GameKey, CanWrite: g.CanWrite})
	}
	return games, "", nil
}

// requireEditorialAdmin resolves the caller and returns true only for admins.
func (s *Server) requireEditorialAdmin(ctx context.Context) (bool, error) {
	perms, err := s.resolveEditorialPermissions(ctx)
	if err != nil {
		return false, err
	}
	return perms != nil && perms.IsAdmin, nil
}

func editorFromGrantsRow(r *db.EditorGrantsRow) EditorialEditor {
	modules := make(map[string]bool, len(r.Modules))
	for m, canWrite := range r.Modules {
		modules[m] = canWrite
	}
	quran := make([]EditorVersionGrant, len(r.QuranVersions))
	for i, g := range r.QuranVersions {
		approve := g.CanApprove
		quran[i] = EditorVersionGrant{VersionId: int64(g.VersionID), CanWrite: g.CanWrite, CanApprove: &approve}
	}
	bible := make([]EditorVersionGrant, len(r.BibleVersions))
	for i, g := range r.BibleVersions {
		bible[i] = EditorVersionGrant{VersionId: int64(g.VersionID), CanWrite: g.CanWrite}
	}
	games := make([]EditorGameGrant, len(r.Games))
	for i, g := range r.Games {
		games[i] = EditorGameGrant{GameKey: g.GameKey, CanWrite: g.CanWrite}
	}
	var ref *int64
	if r.QuranReferenceVersionID != nil {
		v := int64(*r.QuranReferenceVersionID)
		ref = &v
	}
	return EditorialEditor{
		UserId:                  int64(r.UserID),
		Email:                   r.Email,
		DisplayName:             r.DisplayName,
		Role:                    r.Role,
		IsActive:                r.IsActive,
		Modules:                 modules,
		QuranVersions:           quran,
		BibleVersions:           bible,
		Games:                   games,
		QuranReferenceVersionId: ref,
	}
}

// ListEditorialEditors returns users and their editorial grants (admin only).
func (s *Server) ListEditorialEditors(ctx context.Context, request ListEditorialEditorsRequestObject) (ListEditorialEditorsResponseObject, error) {
	isAdmin, err := s.requireEditorialAdmin(ctx)
	if err != nil {
		slog.Error("ListEditorialEditors: resolve permissions", "error", err)
		return ListEditorialEditors500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !isAdmin {
		return ListEditorialEditors403JSONResponse{ForbiddenJSONResponse{Message: "admin access required"}}, nil
	}

	limit := 200
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	offset := 0
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	rows, total, err := s.DB.ListEditorGrants(ctx, limit, offset)
	if err != nil {
		slog.Error("ListEditorialEditors: DB error", "error", err)
		return ListEditorialEditors500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	out := make([]EditorialEditor, len(rows))
	for i := range rows {
		out[i] = editorFromGrantsRow(&rows[i])
	}
	return ListEditorialEditors200JSONResponse(EditorialEditorListEnvelope{Data: out, Total: total}), nil
}

// ReplaceEditorialEditorGrants swaps one user's editorial grants (admin only).
func (s *Server) ReplaceEditorialEditorGrants(ctx context.Context, request ReplaceEditorialEditorGrantsRequestObject) (ReplaceEditorialEditorGrantsResponseObject, error) {
	isAdmin, err := s.requireEditorialAdmin(ctx)
	if err != nil {
		slog.Error("ReplaceEditorialEditorGrants: resolve permissions", "error", err)
		return ReplaceEditorialEditorGrants500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !isAdmin {
		return ReplaceEditorialEditorGrants403JSONResponse{ForbiddenJSONResponse{Message: "admin access required"}}, nil
	}
	if request.Body == nil {
		return ReplaceEditorialEditorGrants400JSONResponse{BadRequestJSONResponse{Message: "request body is required"}}, nil
	}

	modules := make(map[db.EditorModule]bool, len(request.Body.Modules))
	for key, canWrite := range request.Body.Modules {
		if !validEditorModules[key] {
			return ReplaceEditorialEditorGrants400JSONResponse{BadRequestJSONResponse{Message: "unknown module " + key}}, nil
		}
		modules[db.EditorModule(key)] = canWrite
	}

	toGrantRows := func(grants []EditorVersionGrant) []db.EditorVersionGrantRow {
		out := make([]db.EditorVersionGrantRow, len(grants))
		for i, g := range grants {
			row := db.EditorVersionGrantRow{VersionID: int(g.VersionId), CanWrite: g.CanWrite}
			if g.CanApprove != nil {
				row.CanApprove = *g.CanApprove
			}
			out[i] = row
		}
		return out
	}

	// Validate game keys against the registry rather than a hard-coded list, so
	// adding a game is a data change and a typo is a 400 instead of a silent
	// grant nobody holds.
	games, badKey, err := s.gameGrantRows(ctx, request.Body.Games)
	if err != nil {
		slog.Error("ReplaceEditorialEditorGrants: fetch game registry", "error", err)
		return ReplaceEditorialEditorGrants500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if badKey != "" {
		return ReplaceEditorialEditorGrants400JSONResponse{BadRequestJSONResponse{Message: "unknown game " + badKey}}, nil
	}

	var ref *int
	if request.Body.QuranReferenceVersionId != nil {
		v := int(*request.Body.QuranReferenceVersionId)
		ref = &v
	}

	row, err := s.DB.ReplaceEditorGrants(
		ctx,
		int(request.UserId),
		modules,
		toGrantRows(request.Body.QuranVersions),
		toGrantRows(request.Body.BibleVersions),
		games,
		ref,
	)
	switch {
	case errors.Is(err, db.ErrUserNotFound):
		return ReplaceEditorialEditorGrants404JSONResponse{NotFoundJSONResponse{Message: "user not found"}}, nil
	case errors.Is(err, db.ErrContentBadReference):
		return ReplaceEditorialEditorGrants400JSONResponse{BadRequestJSONResponse{Message: "a referenced version does not exist"}}, nil
	case errors.Is(err, db.ErrContentConflict):
		return ReplaceEditorialEditorGrants400JSONResponse{BadRequestJSONResponse{Message: "duplicate grant in request"}}, nil
	case err != nil:
		slog.Error("ReplaceEditorialEditorGrants: DB error", "error", err)
		return ReplaceEditorialEditorGrants500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	return ReplaceEditorialEditorGrants200JSONResponse(EditorialEditorEnvelope{Data: editorFromGrantsRow(row)}), nil
}
