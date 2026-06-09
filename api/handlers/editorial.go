package handlers

import (
	"context"
	"encoding/json"
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
	return s.DB.FetchEditorPermissions(ctx, user.ID, user.Role == "admin")
}

// GetEditorialSession returns the resolved editorial permissions for the caller.
func (s *Server) GetEditorialSession(ctx context.Context, _ GetEditorialSessionRequestObject) (GetEditorialSessionResponseObject, error) {
	perms, err := s.resolveEditorialPermissions(ctx)
	if err != nil {
		slog.Error("GetEditorialSession: resolve permissions", "error", err)
		return GetEditorialSession500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !perms.HasAnyEditorialAccess() {
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
	var ref *int64
	if p.QuranReferenceVersionID != nil {
		v := int64(*p.QuranReferenceVersionID)
		ref = &v
	}
	return EditorialSession{
		IsAdmin:                 p.IsAdmin,
		Modules:                 modules,
		QuranVersions:           quran,
		BibleVersions:           bible,
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
