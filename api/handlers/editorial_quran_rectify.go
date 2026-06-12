package handlers

import (
	"context"
	"log/slog"
)

// Quran rectification handlers (admin-only). The canonical corpus is complete in
// Postgres, so rectification is: a read-only integrity + coverage report, a
// roots-master sync (the one real repair), and optional draft-shell pre-seeding.
// Every endpoint requires admin; editors never reach these.

// GetEditorialQuranRectifyReport returns the integrity + coverage report for a
// version.
func (s *Server) GetEditorialQuranRectifyReport(ctx context.Context, request GetEditorialQuranRectifyReportRequestObject) (GetEditorialQuranRectifyReportResponseObject, error) {
	versionID := int(request.VersionId)
	perms, err := s.resolveEditorialPermissions(ctx)
	if err != nil {
		slog.Error("GetEditorialQuranRectifyReport: resolve permissions", "error", err)
		return GetEditorialQuranRectifyReport500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if perms == nil || !perms.IsAdmin {
		return GetEditorialQuranRectifyReport403JSONResponse{ForbiddenJSONResponse{Message: "admin access required"}}, nil
	}
	version, err := s.DB.FetchQuranVersionByID(ctx, versionID)
	if err != nil {
		slog.Error("GetEditorialQuranRectifyReport: fetch version", "error", err)
		return GetEditorialQuranRectifyReport500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if version == nil {
		return GetEditorialQuranRectifyReport404JSONResponse{NotFoundJSONResponse{Message: "quran version not found"}}, nil
	}
	report, err := s.DB.FetchQuranIntegrityReport(ctx, versionID)
	if err != nil {
		slog.Error("GetEditorialQuranRectifyReport: report", "error", err)
		return GetEditorialQuranRectifyReport500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	return GetEditorialQuranRectifyReport200JSONResponse(EditorialQuranRectifyReportEnvelope{Data: toQuranRectifyReport(report)}), nil
}

// SyncEditorialQuranRoots inserts canonical roots missing from the master so root
// meanings can attach to every word. Version-agnostic; admin-only.
func (s *Server) SyncEditorialQuranRoots(ctx context.Context, _ SyncEditorialQuranRootsRequestObject) (SyncEditorialQuranRootsResponseObject, error) {
	userID, perms, err := s.resolveEditorialActor(ctx)
	if err != nil {
		slog.Error("SyncEditorialQuranRoots: resolve actor", "error", err)
		return SyncEditorialQuranRoots500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if perms == nil || !perms.IsAdmin {
		return SyncEditorialQuranRoots403JSONResponse{ForbiddenJSONResponse{Message: "admin access required"}}, nil
	}
	inserted, err := s.DB.SyncQuranRoots(ctx)
	if err != nil {
		slog.Error("SyncEditorialQuranRoots: sync", "error", err)
		return SyncEditorialQuranRoots500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	// Global repair has no version-scoped changelog (the changelog FK requires a
	// version), so the slog line is the audit trail. Log every call, no-op or not.
	slog.Info("editorial: synced quran roots master", "actor_id", userID, "inserted", inserted)
	return SyncEditorialQuranRoots200JSONResponse(EditorialQuranRootsSyncEnvelope{Data: QuranRootsSyncResult{Inserted: inserted}}), nil
}

// SeedEditorialQuranDraftShells pre-seeds empty draft shells for a version.
func (s *Server) SeedEditorialQuranDraftShells(ctx context.Context, request SeedEditorialQuranDraftShellsRequestObject) (SeedEditorialQuranDraftShellsResponseObject, error) {
	versionID := int(request.VersionId)
	userID, perms, err := s.resolveEditorialActor(ctx)
	if err != nil {
		slog.Error("SeedEditorialQuranDraftShells: resolve actor", "error", err)
		return SeedEditorialQuranDraftShells500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if perms == nil || !perms.IsAdmin {
		return SeedEditorialQuranDraftShells403JSONResponse{ForbiddenJSONResponse{Message: "admin access required"}}, nil
	}
	version, err := s.DB.FetchQuranVersionByID(ctx, versionID)
	if err != nil {
		slog.Error("SeedEditorialQuranDraftShells: fetch version", "error", err)
		return SeedEditorialQuranDraftShells500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if version == nil {
		return SeedEditorialQuranDraftShells404JSONResponse{NotFoundJSONResponse{Message: "quran version not found"}}, nil
	}

	var chapterNumber *int
	seedRoots := false
	if request.Body != nil {
		chapterNumber = request.Body.ChapterNumber
		if request.Body.SeedRoots != nil {
			seedRoots = *request.Body.SeedRoots
		}
	}
	if chapterNumber != nil && (*chapterNumber < 1 || *chapterNumber > 114) {
		return SeedEditorialQuranDraftShells400JSONResponse{BadRequestJSONResponse{Message: "chapter_number out of range"}}, nil
	}

	wordShells, rootShells, err := s.DB.SeedDraftShells(ctx, versionID, int(version.LangID), chapterNumber, seedRoots, userID)
	if err != nil {
		slog.Error("SeedEditorialQuranDraftShells: seed", "error", err)
		return SeedEditorialQuranDraftShells500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	return SeedEditorialQuranDraftShells200JSONResponse(EditorialQuranRectifySeedEnvelope{
		Data: QuranRectifySeedResult{WordShellsCreated: wordShells, RootShellsCreated: rootShells},
	}), nil
}
