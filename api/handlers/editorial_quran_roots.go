package handlers

import (
	"context"
	"log/slog"

	"github.com/wikiSubmission/ws-backend/db"
)

// Quran root-meaning editorial handlers (Phase 2). A meaning is stored once per
// (version, root) and rendered into the word table by JOIN, so editing it
// broadcasts to every word sharing the root. Saving a meaning requires version
// write; publishing all pending root meanings requires the approve grant.

const (
	rootsDefaultLimit = 50
	rootsMaxLimit     = 200
)

// ListEditorialQuranRoots returns the paginated root list for a version.
func (s *Server) ListEditorialQuranRoots(ctx context.Context, request ListEditorialQuranRootsRequestObject) (ListEditorialQuranRootsResponseObject, error) {
	versionID := int(request.VersionId)
	perms, err := s.resolveEditorialPermissions(ctx)
	if err != nil {
		slog.Error("ListEditorialQuranRoots: resolve permissions", "error", err)
		return ListEditorialQuranRoots500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !perms.CanReadQuranVersion(versionID) {
		return ListEditorialQuranRoots403JSONResponse{ForbiddenJSONResponse{Message: "quran version access required"}}, nil
	}
	version, err := s.DB.FetchQuranVersionByID(ctx, versionID)
	if err != nil {
		slog.Error("ListEditorialQuranRoots: fetch version", "error", err)
		return ListEditorialQuranRoots500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if version == nil {
		return ListEditorialQuranRoots404JSONResponse{NotFoundJSONResponse{Message: "quran version not found"}}, nil
	}

	limit := rootsDefaultLimit
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
		if limit < 1 {
			limit = rootsDefaultLimit
		}
		if limit > rootsMaxLimit {
			limit = rootsMaxLimit
		}
	}
	offset := 0
	if request.Params.Offset != nil && *request.Params.Offset > 0 {
		offset = *request.Params.Offset
	}
	var q *string
	if request.Params.Q != nil && *request.Params.Q != "" {
		q = request.Params.Q
	}

	rows, total, err := s.DB.ListRootMeanings(ctx, versionID, q, limit, offset)
	if err != nil {
		slog.Error("ListEditorialQuranRoots: list", "error", err)
		return ListEditorialQuranRoots500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	out := make([]QuranRootSummary, 0, len(rows))
	for i := range rows {
		out = append(out, toQuranRootSummary(rows[i]))
	}
	return ListEditorialQuranRoots200JSONResponse(EditorialQuranRootListEnvelope{Data: out, Total: total}), nil
}

// UpsertEditorialQuranRootMeaning saves this version's draft meaning for a root.
func (s *Server) UpsertEditorialQuranRootMeaning(ctx context.Context, request UpsertEditorialQuranRootMeaningRequestObject) (UpsertEditorialQuranRootMeaningResponseObject, error) {
	versionID := int(request.VersionId)
	rootID := int(request.RootId)

	userID, perms, err := s.resolveEditorialActor(ctx)
	if err != nil {
		slog.Error("UpsertEditorialQuranRootMeaning: resolve actor", "error", err)
		return UpsertEditorialQuranRootMeaning500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !perms.CanWriteQuranVersion(versionID) {
		return UpsertEditorialQuranRootMeaning403JSONResponse{ForbiddenJSONResponse{Message: "quran version write access required"}}, nil
	}
	version, err := s.DB.FetchQuranVersionByID(ctx, versionID)
	if err != nil {
		slog.Error("UpsertEditorialQuranRootMeaning: fetch version", "error", err)
		return UpsertEditorialQuranRootMeaning500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if version == nil {
		return UpsertEditorialQuranRootMeaning404JSONResponse{NotFoundJSONResponse{Message: "quran version not found"}}, nil
	}
	// Validate the root id before the upsert so an unknown id is a clean 404
	// rather than a foreign-key 500.
	existing, err := s.DB.FetchRootMeaning(ctx, versionID, rootID)
	if err != nil {
		slog.Error("UpsertEditorialQuranRootMeaning: fetch root", "error", err)
		return UpsertEditorialQuranRootMeaning500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if existing == nil {
		return UpsertEditorialQuranRootMeaning404JSONResponse{NotFoundJSONResponse{Message: "root not found"}}, nil
	}

	var meaning *string
	if request.Body != nil {
		meaning = request.Body.Meaning
	}
	if err := s.DB.UpsertRootMeaningDraft(ctx, versionID, rootID, meaning, userID); err != nil {
		slog.Error("UpsertEditorialQuranRootMeaning: upsert", "error", err)
		return UpsertEditorialQuranRootMeaning500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if err := s.DB.AppendChangelog(ctx, db.ChangelogEntry{
		VersionID: versionID, Action: db.ActionRootMeaningSaved, ActorID: userID,
		Detail: map[string]any{"root_id": rootID, "letters": existing.Letters},
	}); err != nil {
		slog.Error("UpsertEditorialQuranRootMeaning: changelog", "error", err)
	}

	updated, err := s.DB.FetchRootMeaning(ctx, versionID, rootID)
	if err != nil {
		slog.Error("UpsertEditorialQuranRootMeaning: refetch", "error", err)
		return UpsertEditorialQuranRootMeaning500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if updated == nil {
		return UpsertEditorialQuranRootMeaning404JSONResponse{NotFoundJSONResponse{Message: "root not found"}}, nil
	}
	return UpsertEditorialQuranRootMeaning200JSONResponse(EditorialQuranRootMeaningEnvelope{Data: toQuranRootSummary(*updated)}), nil
}

// PublishEditorialQuranRootMeanings syncs all pending root meaning drafts for a
// version into the published table. Requires the approve grant.
func (s *Server) PublishEditorialQuranRootMeanings(ctx context.Context, request PublishEditorialQuranRootMeaningsRequestObject) (PublishEditorialQuranRootMeaningsResponseObject, error) {
	versionID := int(request.VersionId)

	userID, perms, err := s.resolveEditorialActor(ctx)
	if err != nil {
		slog.Error("PublishEditorialQuranRootMeanings: resolve actor", "error", err)
		return PublishEditorialQuranRootMeanings500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !perms.CanApproveQuranVersion(versionID) {
		return PublishEditorialQuranRootMeanings403JSONResponse{ForbiddenJSONResponse{Message: "approve permission required for this version"}}, nil
	}
	version, err := s.DB.FetchQuranVersionByID(ctx, versionID)
	if err != nil {
		slog.Error("PublishEditorialQuranRootMeanings: fetch version", "error", err)
		return PublishEditorialQuranRootMeanings500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if version == nil {
		return PublishEditorialQuranRootMeanings404JSONResponse{NotFoundJSONResponse{Message: "quran version not found"}}, nil
	}

	var note *string
	if request.Body != nil {
		note = request.Body.Note
	}
	count, err := s.DB.PublishRootMeanings(ctx, versionID, userID, note)
	if err != nil {
		slog.Error("PublishEditorialQuranRootMeanings: publish", "error", err)
		return PublishEditorialQuranRootMeanings500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	return PublishEditorialQuranRootMeanings200JSONResponse(EditorialQuranRootPublishEnvelope{Data: QuranRootPublishResult{Published: count}}), nil
}
