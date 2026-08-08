package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/wikiSubmission/ws-backend/db"
)

// Editorial content endpoints for the first-party document store (articles,
// authors, categories, communities, appendices). This surface replaces the
// retired Sanity Studio: documents live in Postgres as draft/published pairs
// (db/editorial_content.go), and payloads are validated here per module before
// anything is written. Module read/write is authorized on every call.

var (
	slugPattern         = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	appendixCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	uuidPattern         = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	// A bare YouTube video id, not a URL: the reader builds the embed src from
	// it, so anything URL-shaped would produce a broken iframe.
	youTubeIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)
)

// contentModuleFromParam maps the wire enum onto the db module key. The two
// share values by construction; the switch keeps the mapping explicit.
func contentModuleFromParam(m EditorialContentModule) (db.EditorModule, bool) {
	switch m {
	case EditorialContentModuleArticle:
		return db.ModuleArticle, true
	case EditorialContentModuleAuthor:
		return db.ModuleAuthor, true
	case EditorialContentModuleCategory:
		return db.ModuleCategory, true
	case EditorialContentModuleCommunity:
		return db.ModuleCommunity, true
	case EditorialContentModuleAppendix:
		return db.ModuleAppendix, true
	default:
		return "", false
	}
}

// ── payload validation ─────────────────────────────────────────────────────────

type fieldError struct{ msg string }

func (e *fieldError) Error() string { return e.msg }

func fieldErrf(format string, args ...any) error {
	return &fieldError{msg: fmt.Sprintf(format, args...)}
}

func optionalString(fields map[string]any, key string) (*string, error) {
	v, ok := fields[key]
	if !ok || v == nil {
		return nil, nil
	}
	s, ok := v.(string)
	if !ok {
		return nil, fieldErrf("%s must be a string", key)
	}
	return &s, nil
}

func requiredString(fields map[string]any, key string) (string, error) {
	s, err := optionalString(fields, key)
	if err != nil {
		return "", err
	}
	if s == nil || strings.TrimSpace(*s) == "" {
		return "", fieldErrf("%s is required", key)
	}
	return *s, nil
}

func optionalInt(fields map[string]any, key string) (*int, error) {
	v, ok := fields[key]
	if !ok || v == nil {
		return nil, nil
	}
	f, ok := v.(float64)
	if !ok || f != float64(int(f)) {
		return nil, fieldErrf("%s must be an integer", key)
	}
	i := int(f)
	return &i, nil
}

func optionalBool(fields map[string]any, key string) error {
	v, ok := fields[key]
	if !ok || v == nil {
		return nil
	}
	if _, ok := v.(bool); !ok {
		return fieldErrf("%s must be a boolean", key)
	}
	return nil
}

func optionalArray(fields map[string]any, key string) error {
	v, ok := fields[key]
	if !ok || v == nil {
		return nil
	}
	if _, ok := v.([]any); !ok {
		return fieldErrf("%s must be an array", key)
	}
	return nil
}

func rejectUnknownKeys(fields map[string]any, allowed ...string) error {
	set := make(map[string]bool, len(allowed))
	for _, k := range allowed {
		set[k] = true
	}
	for k := range fields {
		if !set[k] {
			return fieldErrf("unknown field %q", k)
		}
	}
	return nil
}

func validSlug(s string) bool { return len(s) <= 200 && slugPattern.MatchString(s) }

// validateContentFields checks the payload for one module and extracts the
// column-mirrored scalars. Returns a *fieldError for caller mistakes (400).
func (s *Server) validateContentFields(
	module db.EditorModule,
	fields map[string]any,
) (db.ContentScalars, error) {
	var scalars db.ContentScalars

	switch module {
	case db.ModuleArticle:
		if err := rejectUnknownKeys(fields,
			"title", "slug", "language", "author_id", "categories",
			"excerpt", "thumbnail_url", "thumbnail_text", "body",
			"enable_scripture_refs",
		); err != nil {
			return scalars, err
		}
		if _, err := requiredString(fields, "title"); err != nil {
			return scalars, err
		}
		slug, err := requiredString(fields, "slug")
		if err != nil {
			return scalars, err
		}
		if !validSlug(slug) {
			return scalars, fieldErrf("slug must match %s", slugPattern.String())
		}
		lang, err := requiredString(fields, "language")
		if err != nil {
			return scalars, err
		}
		langCache, ok := s.Languages[lang]
		if !ok {
			return scalars, fieldErrf("unknown language %q", lang)
		}
		authorID, err := optionalInt(fields, "author_id")
		if err != nil {
			return scalars, err
		}
		if err := optionalArray(fields, "categories"); err != nil {
			return scalars, err
		}
		for _, ostr := range []string{"excerpt", "thumbnail_url"} {
			if _, err := optionalString(fields, ostr); err != nil {
				return scalars, err
			}
		}
		for _, arr := range []string{"thumbnail_text", "body"} {
			if err := optionalArray(fields, arr); err != nil {
				return scalars, err
			}
		}
		if err := optionalBool(fields, "enable_scripture_refs"); err != nil {
			return scalars, err
		}
		langID := langCache.ID
		scalars.Slug = &slug
		scalars.LangID = &langID
		scalars.AuthorID = authorID

	case db.ModuleAuthor:
		if err := rejectUnknownKeys(fields,
			"first_name", "last_name", "slug", "photo_url", "bio", "user_id",
		); err != nil {
			return scalars, err
		}
		if _, err := requiredString(fields, "first_name"); err != nil {
			return scalars, err
		}
		slug, err := requiredString(fields, "slug")
		if err != nil {
			return scalars, err
		}
		if !validSlug(slug) {
			return scalars, fieldErrf("slug must match %s", slugPattern.String())
		}
		// last_name is optional so single-name (mononym) authors are valid.
		for _, ostr := range []string{"last_name", "photo_url", "bio"} {
			if _, err := optionalString(fields, ostr); err != nil {
				return scalars, err
			}
		}
		userID, err := optionalInt(fields, "user_id")
		if err != nil {
			return scalars, err
		}
		scalars.Slug = &slug
		scalars.UserID = userID

	case db.ModuleCategory:
		if err := rejectUnknownKeys(fields, "name", "slug", "description"); err != nil {
			return scalars, err
		}
		if _, err := requiredString(fields, "name"); err != nil {
			return scalars, err
		}
		slug, err := requiredString(fields, "slug")
		if err != nil {
			return scalars, err
		}
		if !validSlug(slug) {
			return scalars, fieldErrf("slug must match %s", slugPattern.String())
		}
		if _, err := optionalString(fields, "description"); err != nil {
			return scalars, err
		}
		scalars.Slug = &slug

	case db.ModuleCommunity:
		if err := rejectUnknownKeys(fields,
			"kind", "name", "slug", "language", "description", "image_url", "image_alt",
			"tags", "is_active", "platform", "url", "invite_code", "member_count",
			"address", "city", "country", "geo", "contact_email", "contact_phone",
			"meeting_schedule",
		); err != nil {
			return scalars, err
		}
		kind, err := requiredString(fields, "kind")
		if err != nil {
			return scalars, err
		}
		if kind != "online" && kind != "physical" {
			return scalars, fieldErrf("kind must be online or physical")
		}
		if _, err := requiredString(fields, "name"); err != nil {
			return scalars, err
		}
		slug, err := requiredString(fields, "slug")
		if err != nil {
			return scalars, err
		}
		if !validSlug(slug) {
			return scalars, fieldErrf("slug must match %s", slugPattern.String())
		}
		for _, ostr := range []string{
			"language", "description", "image_url", "image_alt", "platform", "url",
			"invite_code", "address", "city", "country", "contact_email",
			"contact_phone", "meeting_schedule",
		} {
			if _, err := optionalString(fields, ostr); err != nil {
				return scalars, err
			}
		}
		if err := optionalArray(fields, "tags"); err != nil {
			return scalars, err
		}
		if err := optionalBool(fields, "is_active"); err != nil {
			return scalars, err
		}
		if _, err := optionalInt(fields, "member_count"); err != nil {
			return scalars, err
		}
		scalars.Slug = &slug

	case db.ModuleAppendix:
		if err := rejectUnknownKeys(
			fields, "version_id", "code", "title", "snippet", "body",
			"video_id", "video_title",
		); err != nil {
			return scalars, err
		}
		versionID, err := optionalInt(fields, "version_id")
		if err != nil {
			return scalars, err
		}
		if versionID == nil {
			return scalars, fieldErrf("version_id is required")
		}
		code, err := requiredString(fields, "code")
		if err != nil {
			return scalars, err
		}
		if len(code) > 50 || !appendixCodePattern.MatchString(code) {
			return scalars, fieldErrf("code must match %s", appendixCodePattern.String())
		}
		if _, err := requiredString(fields, "title"); err != nil {
			return scalars, err
		}
		// body is markdown text (matching the appendix model), not Portable Text.
		// video_id / video_title hold the single trailing YouTube embed as
		// metadata, so the body stays plain markdown with no embed syntax.
		for _, ostr := range []string{"snippet", "body", "video_id", "video_title"} {
			if _, err := optionalString(fields, ostr); err != nil {
				return scalars, err
			}
		}
		videoID, err := optionalString(fields, "video_id")
		if err != nil {
			return scalars, err
		}
		if videoID != nil && *videoID != "" && !youTubeIDPattern.MatchString(*videoID) {
			return scalars, fieldErrf("video_id must be a bare YouTube id matching %s", youTubeIDPattern.String())
		}
		scalars.VersionID = versionID
		scalars.Code = &code

	default:
		return scalars, fmt.Errorf("unhandled content module %q", module)
	}

	return scalars, nil
}

// ── shared plumbing ────────────────────────────────────────────────────────────

// companionReadModules lets holders of one module's read grant list another
// module's documents when the editing surface needs them as picker options:
// the article editor references authors and categories.
var companionReadModules = map[db.EditorModule][]db.EditorModule{
	db.ModuleAuthor:   {db.ModuleArticle},
	db.ModuleCategory: {db.ModuleArticle},
}

// resolveContentActor authorizes the caller for one module. write=false checks
// read access (including companion grants). Returns userID 0 when the caller
// must be denied (403).
func (s *Server) resolveContentActor(
	ctx context.Context,
	module db.EditorModule,
	write bool,
) (int, error) {
	userID, perms, err := s.resolveEditorialActor(ctx)
	if err != nil {
		return 0, err
	}
	if perms == nil {
		return 0, nil
	}
	allowed := perms.CanReadModule(module)
	if !allowed && !write {
		for _, companion := range companionReadModules[module] {
			if perms.CanReadModule(companion) {
				allowed = true
				break
			}
		}
	}
	if write {
		allowed = perms.CanWriteModule(module)
	}
	if !allowed {
		return 0, nil
	}
	return userID, nil
}

func contentDocFromRow(module EditorialContentModule, r *db.ContentDocRow) EditorialContentDoc {
	fields := map[string]any{}
	if len(r.Draft) > 0 {
		if err := json.Unmarshal(r.Draft, &fields); err != nil {
			slog.Warn("contentDocFromRow: malformed draft payload", "module", module, "id", r.ID, "error", err)
		}
	}
	return EditorialContentDoc{
		Id:               int64(r.ID),
		Module:           module,
		Status:           EditorialContentStatus(r.Status),
		Fields:           fields,
		TranslationGroup: r.TranslationGroup,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
		PublishedAt:      r.PublishedAt,
		UpdatedByEmail:   r.UpdatedByEmail,
	}
}

// prepareContentWrite validates and re-serializes the input payload. The
// returned draft is the canonical stored form (marshalled from the validated
// field map, so key order is normalized).
func (s *Server) prepareContentWrite(
	module db.EditorModule,
	input *EditorialContentInput,
) (json.RawMessage, db.ContentScalars, *string, error) {
	if input == nil || input.Fields == nil {
		return nil, db.ContentScalars{}, nil, fieldErrf("fields is required")
	}
	scalars, err := s.validateContentFields(module, input.Fields)
	if err != nil {
		return nil, db.ContentScalars{}, nil, err
	}
	var group *string
	if input.TranslationGroup != nil && *input.TranslationGroup != "" {
		if module != db.ModuleArticle {
			return nil, db.ContentScalars{}, nil, fieldErrf("translation_group only applies to articles")
		}
		if !uuidPattern.MatchString(*input.TranslationGroup) {
			return nil, db.ContentScalars{}, nil, fieldErrf("translation_group must be a UUID")
		}
		group = input.TranslationGroup
	}
	draft, err := json.Marshal(input.Fields)
	if err != nil {
		return nil, db.ContentScalars{}, nil, err
	}
	return draft, scalars, group, nil
}

// ── handlers ───────────────────────────────────────────────────────────────────

// ListEditorialContent lists one module's documents. Requires module read.
func (s *Server) ListEditorialContent(ctx context.Context, request ListEditorialContentRequestObject) (ListEditorialContentResponseObject, error) {
	module, ok := contentModuleFromParam(request.Module)
	if !ok {
		return ListEditorialContent400JSONResponse{BadRequestJSONResponse{Message: "unknown content module"}}, nil
	}
	userID, err := s.resolveContentActor(ctx, module, false)
	if err != nil {
		slog.Error("ListEditorialContent: resolve actor", "error", err)
		return ListEditorialContent500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if userID == 0 {
		return ListEditorialContent403JSONResponse{ForbiddenJSONResponse{Message: "module access required"}}, nil
	}

	q := ""
	if request.Params.Q != nil {
		q = strings.TrimSpace(*request.Params.Q)
	}
	status := db.ContentStatus("")
	if request.Params.Status != nil {
		status = db.ContentStatus(*request.Params.Status)
	}
	limit := 100
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	offset := 0
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	rows, total, err := s.DB.ListContentDocs(ctx, module, q, status, limit, offset)
	if err != nil {
		slog.Error("ListEditorialContent: DB error", "module", module, "error", err)
		return ListEditorialContent500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	out := make([]EditorialContentDoc, len(rows))
	for i := range rows {
		out[i] = contentDocFromRow(request.Module, &rows[i])
	}
	return ListEditorialContent200JSONResponse(EditorialContentListEnvelope{Data: out, Total: total}), nil
}

// GetEditorialContent fetches one document. Requires module read.
func (s *Server) GetEditorialContent(ctx context.Context, request GetEditorialContentRequestObject) (GetEditorialContentResponseObject, error) {
	module, ok := contentModuleFromParam(request.Module)
	if !ok {
		return GetEditorialContent404JSONResponse{NotFoundJSONResponse{Message: "unknown content module"}}, nil
	}
	userID, err := s.resolveContentActor(ctx, module, false)
	if err != nil {
		slog.Error("GetEditorialContent: resolve actor", "error", err)
		return GetEditorialContent500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if userID == 0 {
		return GetEditorialContent403JSONResponse{ForbiddenJSONResponse{Message: "module access required"}}, nil
	}

	row, err := s.DB.GetContentDoc(ctx, module, int(request.DocId))
	if errors.Is(err, db.ErrContentNotFound) {
		return GetEditorialContent404JSONResponse{NotFoundJSONResponse{Message: "document not found"}}, nil
	}
	if err != nil {
		slog.Error("GetEditorialContent: DB error", "module", module, "error", err)
		return GetEditorialContent500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	return GetEditorialContent200JSONResponse(EditorialContentEnvelope{Data: contentDocFromRow(request.Module, row)}), nil
}

// CreateEditorialContent creates a draft document. Requires module write.
func (s *Server) CreateEditorialContent(ctx context.Context, request CreateEditorialContentRequestObject) (CreateEditorialContentResponseObject, error) {
	module, ok := contentModuleFromParam(request.Module)
	if !ok {
		return CreateEditorialContent400JSONResponse{BadRequestJSONResponse{Message: "unknown content module"}}, nil
	}
	userID, err := s.resolveContentActor(ctx, module, true)
	if err != nil {
		slog.Error("CreateEditorialContent: resolve actor", "error", err)
		return CreateEditorialContent500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if userID == 0 {
		return CreateEditorialContent403JSONResponse{ForbiddenJSONResponse{Message: "module write access required"}}, nil
	}

	draft, scalars, group, err := s.prepareContentWrite(module, request.Body)
	if err != nil {
		var fe *fieldError
		if errors.As(err, &fe) {
			return CreateEditorialContent400JSONResponse{BadRequestJSONResponse{Message: fe.Error()}}, nil
		}
		slog.Error("CreateEditorialContent: prepare", "module", module, "error", err)
		return CreateEditorialContent500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	row, err := s.DB.CreateContentDoc(ctx, module, draft, scalars, group, userID)
	switch {
	case errors.Is(err, db.ErrContentConflict):
		return CreateEditorialContent409JSONResponse{ConflictJSONResponse{Message: "a document with this slug already exists"}}, nil
	case errors.Is(err, db.ErrContentBadReference):
		return CreateEditorialContent400JSONResponse{BadRequestJSONResponse{Message: "a referenced row does not exist"}}, nil
	case err != nil:
		slog.Error("CreateEditorialContent: DB error", "module", module, "error", err)
		return CreateEditorialContent500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	return CreateEditorialContent201JSONResponse(EditorialContentEnvelope{Data: contentDocFromRow(request.Module, row)}), nil
}

// UpdateEditorialContent replaces a document's draft. Requires module write.
func (s *Server) UpdateEditorialContent(ctx context.Context, request UpdateEditorialContentRequestObject) (UpdateEditorialContentResponseObject, error) {
	module, ok := contentModuleFromParam(request.Module)
	if !ok {
		return UpdateEditorialContent404JSONResponse{NotFoundJSONResponse{Message: "unknown content module"}}, nil
	}
	userID, err := s.resolveContentActor(ctx, module, true)
	if err != nil {
		slog.Error("UpdateEditorialContent: resolve actor", "error", err)
		return UpdateEditorialContent500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if userID == 0 {
		return UpdateEditorialContent403JSONResponse{ForbiddenJSONResponse{Message: "module write access required"}}, nil
	}

	draft, scalars, group, err := s.prepareContentWrite(module, request.Body)
	if err != nil {
		var fe *fieldError
		if errors.As(err, &fe) {
			return UpdateEditorialContent400JSONResponse{BadRequestJSONResponse{Message: fe.Error()}}, nil
		}
		slog.Error("UpdateEditorialContent: prepare", "module", module, "error", err)
		return UpdateEditorialContent500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if group != nil {
		return UpdateEditorialContent400JSONResponse{BadRequestJSONResponse{Message: "translation_group cannot be changed after creation"}}, nil
	}

	row, err := s.DB.UpdateContentDoc(ctx, module, int(request.DocId), draft, scalars, userID)
	switch {
	case errors.Is(err, db.ErrContentNotFound):
		return UpdateEditorialContent404JSONResponse{NotFoundJSONResponse{Message: "document not found"}}, nil
	case errors.Is(err, db.ErrContentConflict):
		return UpdateEditorialContent409JSONResponse{ConflictJSONResponse{Message: "a document with this slug already exists"}}, nil
	case errors.Is(err, db.ErrContentBadReference):
		return UpdateEditorialContent400JSONResponse{BadRequestJSONResponse{Message: "a referenced row does not exist"}}, nil
	case err != nil:
		slog.Error("UpdateEditorialContent: DB error", "module", module, "error", err)
		return UpdateEditorialContent500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	return UpdateEditorialContent200JSONResponse(EditorialContentEnvelope{Data: contentDocFromRow(request.Module, row)}), nil
}

// DeleteEditorialContent removes a document. Requires module write.
func (s *Server) DeleteEditorialContent(ctx context.Context, request DeleteEditorialContentRequestObject) (DeleteEditorialContentResponseObject, error) {
	module, ok := contentModuleFromParam(request.Module)
	if !ok {
		return DeleteEditorialContent404JSONResponse{NotFoundJSONResponse{Message: "unknown content module"}}, nil
	}
	userID, err := s.resolveContentActor(ctx, module, true)
	if err != nil {
		slog.Error("DeleteEditorialContent: resolve actor", "error", err)
		return DeleteEditorialContent500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if userID == 0 {
		return DeleteEditorialContent403JSONResponse{ForbiddenJSONResponse{Message: "module write access required"}}, nil
	}

	err = s.DB.DeleteContentDoc(ctx, module, int(request.DocId))
	switch {
	case errors.Is(err, db.ErrContentNotFound):
		return DeleteEditorialContent404JSONResponse{NotFoundJSONResponse{Message: "document not found"}}, nil
	case errors.Is(err, db.ErrContentBadReference):
		// Restricted FK from another table (none today, but future-proof).
		return DeleteEditorialContent404JSONResponse{NotFoundJSONResponse{Message: "document is referenced elsewhere"}}, nil
	case err != nil:
		slog.Error("DeleteEditorialContent: DB error", "module", module, "error", err)
		return DeleteEditorialContent500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	return DeleteEditorialContent204Response{}, nil
}

// PublishEditorialContent copies the draft into the published snapshot.
func (s *Server) PublishEditorialContent(ctx context.Context, request PublishEditorialContentRequestObject) (PublishEditorialContentResponseObject, error) {
	module, ok := contentModuleFromParam(request.Module)
	if !ok {
		return PublishEditorialContent404JSONResponse{NotFoundJSONResponse{Message: "unknown content module"}}, nil
	}
	userID, err := s.resolveContentActor(ctx, module, true)
	if err != nil {
		slog.Error("PublishEditorialContent: resolve actor", "error", err)
		return PublishEditorialContent500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if userID == 0 {
		return PublishEditorialContent403JSONResponse{ForbiddenJSONResponse{Message: "module write access required"}}, nil
	}

	row, err := s.DB.PublishContentDoc(ctx, module, int(request.DocId), userID)
	switch {
	case errors.Is(err, db.ErrContentNotFound):
		return PublishEditorialContent404JSONResponse{NotFoundJSONResponse{Message: "document not found"}}, nil
	case err != nil:
		slog.Error("PublishEditorialContent: DB error", "module", module, "error", err)
		return PublishEditorialContent500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	return PublishEditorialContent200JSONResponse(EditorialContentEnvelope{Data: contentDocFromRow(request.Module, row)}), nil
}

// UnpublishEditorialContent retracts the published snapshot.
func (s *Server) UnpublishEditorialContent(ctx context.Context, request UnpublishEditorialContentRequestObject) (UnpublishEditorialContentResponseObject, error) {
	module, ok := contentModuleFromParam(request.Module)
	if !ok {
		return UnpublishEditorialContent404JSONResponse{NotFoundJSONResponse{Message: "unknown content module"}}, nil
	}
	userID, err := s.resolveContentActor(ctx, module, true)
	if err != nil {
		slog.Error("UnpublishEditorialContent: resolve actor", "error", err)
		return UnpublishEditorialContent500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if userID == 0 {
		return UnpublishEditorialContent403JSONResponse{ForbiddenJSONResponse{Message: "module write access required"}}, nil
	}

	row, err := s.DB.UnpublishContentDoc(ctx, module, int(request.DocId), userID)
	switch {
	case errors.Is(err, db.ErrContentNotFound):
		return UnpublishEditorialContent404JSONResponse{NotFoundJSONResponse{Message: "document not found"}}, nil
	case err != nil:
		slog.Error("UnpublishEditorialContent: DB error", "module", module, "error", err)
		return UnpublishEditorialContent500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	return UnpublishEditorialContent200JSONResponse(EditorialContentEnvelope{Data: contentDocFromRow(request.Module, row)}), nil
}
