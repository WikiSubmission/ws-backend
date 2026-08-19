package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"

	"github.com/wikiSubmission/ws-backend/db"
)

// Editorial endpoints back the first-party Next.js editor (Sanity Studio port).
// They are part of the OpenAPI contract (operationIds getEditorialSession,
// listEditorialQuranVersions, listEditorialBibleVersions). The validator's
// AuthenticationFunc verifies the bearer JWT and propagates the auth id; these
// handlers perform authorization in-process and remain the authority — the
// session response only shapes the UI, every mutation is re-checked server-side.

// resolveEditorialPermissions builds the caller's permission snapshot from the
// auth id on the context. It returns (nil, nil) when the caller is
// unauthenticated, unknown, or deactivated so callers deny by default; a
// non-nil error indicates an infrastructure failure (HTTP 500).
func (s *Server) resolveEditorialPermissions(ctx context.Context) (*db.EditorPermissions, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return nil, nil
	}
	user, err := s.DB.FetchUserByAuthID(ctx, authID)
	if err != nil {
		return nil, err
	}
	if user == nil || !user.IsActive {
		return nil, nil
	}
	perms, err := s.DB.FetchEditorPermissions(ctx, user.ID, user.HasRole("admin"))
	if err != nil {
		return nil, err
	}
	applyUserRoles(perms, user)
	return perms, nil
}

// resolveEditorialActor is like resolveEditorialPermissions but also returns the
// acting user's id (for author_id / requested_by / reviewed_by attribution).
// userID is 0 and perms nil when the caller is unauthenticated, unknown, or
// deactivated, so callers deny by default.
func (s *Server) resolveEditorialActor(ctx context.Context) (int, *db.EditorPermissions, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return 0, nil, nil
	}
	user, err := s.DB.FetchUserByAuthID(ctx, authID)
	if err != nil {
		return 0, nil, err
	}
	if user == nil || !user.IsActive {
		return 0, nil, nil
	}
	perms, err := s.DB.FetchEditorPermissions(ctx, user.ID, user.HasRole("admin"))
	if err != nil {
		return 0, nil, err
	}
	applyUserRoles(perms, user)
	return user.ID, perms, nil
}

// applyUserRoles makes roles the coarse gate and grants the granular gate.
// Legacy author/category grants remain stored for rollback but no longer grant
// standalone surfaces: authors are admin-only and categories inherit Articles.
func applyUserRoles(perms *db.EditorPermissions, user *db.UserRow) {
	if perms == nil || user == nil || perms.IsAdmin {
		return
	}
	if !user.HasRole("editor") {
		for module := range perms.Modules {
			if module != db.ModuleGame {
				delete(perms.Modules, module)
			}
		}
		perms.QuranVersions = map[int]bool{}
		perms.QuranApproverVersions = map[int]bool{}
		perms.BibleVersions = map[int]bool{}
	}
	delete(perms.Modules, db.ModuleAuthor)
	delete(perms.Modules, db.ModuleCategory)

	if !user.HasRole("game_editor") {
		delete(perms.Modules, db.ModuleGame)
		perms.Games = map[string]bool{}
	}
}

// GetEditorialSession returns the resolved editorial permissions for the caller.
func (s *Server) GetEditorialSession(ctx context.Context, _ GetEditorialSessionRequestObject) (GetEditorialSessionResponseObject, error) {
	perms, err := s.resolveEditorialPermissions(ctx)
	if err != nil {
		slog.Error("GetEditorialSession: resolve permissions", "error", err)
		return GetEditorialSession500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	// Games-only editors get a snapshot too: the games studio reads this endpoint
	// to resolve per-game access on every request rather than trusting a stale
	// flag baked into the caller's session token.
	if !perms.HasAnyEditorialAccess() && !perms.HasAnyGameAccess() {
		return GetEditorialSession403JSONResponse{ForbiddenJSONResponse{Message: "editorial access required"}}, nil
	}
	return GetEditorialSession200JSONResponse(EditorialSessionEnvelope{Data: editorialSessionFromPerms(perms)}), nil
}

// ListEditorialQuranVersions returns the Quran version registry. Requires read
// access to the quran module; the editor filters to writable versions in the UI
// while the backend re-checks per-version write on every mutation.
func (s *Server) ListEditorialQuranVersions(ctx context.Context, _ ListEditorialQuranVersionsRequestObject) (ListEditorialQuranVersionsResponseObject, error) {
	perms, err := s.resolveEditorialPermissions(ctx)
	if err != nil {
		slog.Error("ListEditorialQuranVersions: resolve permissions", "error", err)
		return ListEditorialQuranVersions500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !perms.CanReadModule(db.ModuleQuran) {
		return ListEditorialQuranVersions403JSONResponse{ForbiddenJSONResponse{Message: "quran editorial access required"}}, nil
	}
	rows, err := s.DB.FetchQuranVersions(ctx)
	if err != nil {
		slog.Error("ListEditorialQuranVersions: DB error", "error", err)
		return ListEditorialQuranVersions500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	out := make([]QuranVersion, len(rows))
	for i := range rows {
		out[i] = quranVersionFromRow(&rows[i])
	}
	return ListEditorialQuranVersions200JSONResponse(EditorialQuranVersionListEnvelope{Data: out}), nil
}

// UpdateEditorialQuranReferenceVersion lets an editor choose their own
// word-by-word reference from the Quran versions they can read.
func (s *Server) UpdateEditorialQuranReferenceVersion(ctx context.Context, request UpdateEditorialQuranReferenceVersionRequestObject) (UpdateEditorialQuranReferenceVersionResponseObject, error) {
	if request.Body == nil {
		return UpdateEditorialQuranReferenceVersion400JSONResponse{BadRequestJSONResponse{Message: "request body is required"}}, nil
	}
	userID, perms, err := s.resolveEditorialActor(ctx)
	if err != nil {
		slog.Error("UpdateEditorialQuranReferenceVersion: resolve actor", "error", err)
		return UpdateEditorialQuranReferenceVersion500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if userID == 0 || perms == nil || !perms.CanReadModule(db.ModuleQuran) {
		return UpdateEditorialQuranReferenceVersion403JSONResponse{ForbiddenJSONResponse{Message: "quran editorial access required"}}, nil
	}

	var versionID *int
	if request.Body.ReferenceVersionId != nil {
		id := int(*request.Body.ReferenceVersionId)
		if id <= 0 {
			return UpdateEditorialQuranReferenceVersion400JSONResponse{BadRequestJSONResponse{Message: "invalid reference version"}}, nil
		}
		if !perms.CanReadQuranVersion(id) {
			return UpdateEditorialQuranReferenceVersion403JSONResponse{ForbiddenJSONResponse{Message: "reference version access required"}}, nil
		}
		versionID = &id
	}
	if err := s.DB.UpdateEditorQuranReferencePreference(ctx, userID, versionID); err != nil {
		if errors.Is(err, db.ErrContentBadReference) {
			return UpdateEditorialQuranReferenceVersion400JSONResponse{BadRequestJSONResponse{Message: "reference version does not exist"}}, nil
		}
		slog.Error("UpdateEditorialQuranReferenceVersion: DB error", "error", err)
		return UpdateEditorialQuranReferenceVersion500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	perms.QuranReferenceVersionID = versionID
	return UpdateEditorialQuranReferenceVersion200JSONResponse(EditorialSessionEnvelope{Data: editorialSessionFromPerms(perms)}), nil
}

// ListEditorialBibleVersions returns the Bible version registry. Requires read
// access to the bible module.
func (s *Server) ListEditorialBibleVersions(ctx context.Context, _ ListEditorialBibleVersionsRequestObject) (ListEditorialBibleVersionsResponseObject, error) {
	perms, err := s.resolveEditorialPermissions(ctx)
	if err != nil {
		slog.Error("ListEditorialBibleVersions: resolve permissions", "error", err)
		return ListEditorialBibleVersions500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !perms.CanReadModule(db.ModuleBible) {
		return ListEditorialBibleVersions403JSONResponse{ForbiddenJSONResponse{Message: "bible editorial access required"}}, nil
	}
	rows, err := s.DB.FetchBibleVersions(ctx)
	if err != nil {
		slog.Error("ListEditorialBibleVersions: DB error", "error", err)
		return ListEditorialBibleVersions500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	out := make([]BibleVersion, len(rows))
	for i := range rows {
		out[i] = bibleVersionFromRow(&rows[i])
	}
	return ListEditorialBibleVersions200JSONResponse(EditorialBibleVersionListEnvelope{Data: out}), nil
}

// ListEditorialGames returns the game registry so the access console can offer a
// per-game grant row for each. Admin-only: it exists to author grants, and only
// admins can do that (see ReplaceEditorialEditorGrants).
func (s *Server) ListEditorialGames(ctx context.Context, _ ListEditorialGamesRequestObject) (ListEditorialGamesResponseObject, error) {
	isAdmin, err := s.requireEditorialAdmin(ctx)
	if err != nil {
		slog.Error("ListEditorialGames: resolve permissions", "error", err)
		return ListEditorialGames500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !isAdmin {
		return ListEditorialGames403JSONResponse{ForbiddenJSONResponse{Message: "admin access required"}}, nil
	}
	rows, err := s.DB.FetchEditorGames(ctx)
	if err != nil {
		slog.Error("ListEditorialGames: DB error", "error", err)
		return ListEditorialGames500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	out := make([]EditorGame, len(rows))
	for i, r := range rows {
		sortOrder := r.SortOrder
		out[i] = EditorGame{Key: r.Key, Name: r.Name, SortOrder: &sortOrder}
	}
	return ListEditorialGames200JSONResponse(EditorGameListEnvelope{Data: out}), nil
}

// editorialSessionFromPerms shapes the resolved permissions into the wire model.
// Integer version-id keys become string map keys per JSON object semantics.
func editorialSessionFromPerms(p *db.EditorPermissions) EditorialSession {
	modules := make(map[string]bool, len(p.Modules))
	for m, canWrite := range p.Modules {
		modules[string(m)] = canWrite
	}
	quran := make(map[string]bool, len(p.QuranVersions))
	for id, canWrite := range p.QuranVersions {
		quran[strconv.Itoa(id)] = canWrite
	}
	bible := make(map[string]bool, len(p.BibleVersions))
	for id, canWrite := range p.BibleVersions {
		bible[strconv.Itoa(id)] = canWrite
	}
	approver := make(map[string]bool, len(p.QuranApproverVersions))
	for id, canApprove := range p.QuranApproverVersions {
		approver[strconv.Itoa(id)] = canApprove
	}
	// Per-game grants only. The global tier already rides along in modules under
	// the "game" key, and admins hold no grant rows at all — clients resolve both
	// via is_admin / modules.game.
	games := make(map[string]bool, len(p.Games))
	for key, canWrite := range p.Games {
		games[key] = canWrite
	}
	var ref *int64
	if p.QuranReferenceVersionID != nil {
		v := int64(*p.QuranReferenceVersionID)
		ref = &v
	}
	return EditorialSession{
		IsAdmin:                 p.IsAdmin,
		Modules:                 modules,
		QuranVersions:           quran,
		QuranApproverVersions:   approver,
		BibleVersions:           bible,
		Games:                   games,
		QuranReferenceVersionId: ref,
	}
}

func quranVersionFromRow(r *db.QuranVersionRow) QuranVersion {
	v := QuranVersion{
		Id:                 int64(r.ID),
		Slug:               r.Slug,
		Name:               r.Name,
		LangId:             int(r.LangID),
		Direction:          r.Direction,
		IsCanonicalEnglish: r.IsCanonicalEnglish,
		CreatedAt:          r.CreatedAt,
		UpdatedAt:          r.UpdatedAt,
	}
	if r.ReferenceVersionID != nil {
		id := int64(*r.ReferenceVersionID)
		v.ReferenceVersionId = &id
	}
	if authors := parseVersionAuthors(r.Authors); len(authors) > 0 {
		v.Authors = &authors
	}
	return v
}

func bibleVersionFromRow(r *db.BibleVersionRow) BibleVersion {
	v := BibleVersion{
		Id:        int64(r.ID),
		Slug:      r.Slug,
		Name:      r.Name,
		LangId:    int(r.LangID),
		Direction: r.Direction,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
	if authors := parseVersionAuthors(r.Authors); len(authors) > 0 {
		v.Authors = &authors
	}
	return v
}

// parseVersionAuthors decodes the authors jsonb blob. Malformed JSON yields no
// authors rather than failing the whole request.
func parseVersionAuthors(raw json.RawMessage) []VersionAuthor {
	if len(raw) == 0 {
		return nil
	}
	var authors []VersionAuthor
	if err := json.Unmarshal(raw, &authors); err != nil {
		slog.Warn("parseVersionAuthors: malformed authors json", "error", err)
		return nil
	}
	return authors
}
