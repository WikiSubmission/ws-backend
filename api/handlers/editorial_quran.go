package handlers

import (
	"context"
	"errors"
	"log/slog"

	"github.com/wikiSubmission/ws-backend/db"
)

// Quran editorial write-path handlers (Phase 1). Authorization is re-checked
// here on every call from the resolved permission snapshot; the session
// response only shapes the UI. Publishing never touches the live reader corpus
// (see migration 019 / docs/quran-version-keying-migration.md).

const (
	changelogDefaultLimit = 50
	changelogMaxLimit     = 200
)

// ListEditorialQuranChapters returns chapter navigation + progress for a version.
func (s *Server) ListEditorialQuranChapters(ctx context.Context, request ListEditorialQuranChaptersRequestObject) (ListEditorialQuranChaptersResponseObject, error) {
	versionID := int(request.VersionId)
	perms, err := s.resolveEditorialPermissions(ctx)
	if err != nil {
		slog.Error("ListEditorialQuranChapters: resolve permissions", "error", err)
		return ListEditorialQuranChapters500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !perms.CanReadQuranVersion(versionID) {
		return ListEditorialQuranChapters403JSONResponse{ForbiddenJSONResponse{Message: "quran version access required"}}, nil
	}
	version, err := s.DB.FetchQuranVersionByID(ctx, versionID)
	if err != nil {
		slog.Error("ListEditorialQuranChapters: fetch version", "error", err)
		return ListEditorialQuranChapters500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if version == nil {
		return ListEditorialQuranChapters404JSONResponse{NotFoundJSONResponse{Message: "quran version not found"}}, nil
	}
	progress, err := s.DB.FetchQuranChapterProgress(ctx, versionID)
	if err != nil {
		slog.Error("ListEditorialQuranChapters: fetch progress", "error", err)
		return ListEditorialQuranChapters500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	pending, err := s.DB.FetchPendingPublishRequestsByVersion(ctx, versionID)
	if err != nil {
		slog.Error("ListEditorialQuranChapters: fetch pending", "error", err)
		return ListEditorialQuranChapters500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	pendingByChapter := make(map[int]*db.PublishRequestRow, len(pending))
	for i := range pending {
		pendingByChapter[pending[i].ChapterNumber] = &pending[i]
	}

	out := make([]QuranChapterSummary, 0, len(progress))
	for _, p := range progress {
		summary := QuranChapterSummary{
			ChapterNumber:   p.ChapterNumber,
			VerseCount:      p.VerseCount,
			Title:           p.Title,
			HasTitleDraft:   p.HasTitleDraft,
			DraftVerseCount: p.DraftVerseCount,
		}
		if req, ok := pendingByChapter[p.ChapterNumber]; ok {
			pr := toPublishRequest(req)
			summary.PendingRequest = &pr
		}
		out = append(out, summary)
	}
	return ListEditorialQuranChapters200JSONResponse(EditorialQuranChapterListEnvelope{Data: out}), nil
}

// GetEditorialQuranChapter returns the full chapter editing surface.
func (s *Server) GetEditorialQuranChapter(ctx context.Context, request GetEditorialQuranChapterRequestObject) (GetEditorialQuranChapterResponseObject, error) {
	versionID := int(request.VersionId)
	chapterNumber := int(request.ChapterNumber)
	perms, err := s.resolveEditorialPermissions(ctx)
	if err != nil {
		slog.Error("GetEditorialQuranChapter: resolve permissions", "error", err)
		return GetEditorialQuranChapter500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !perms.CanReadQuranVersion(versionID) {
		return GetEditorialQuranChapter403JSONResponse{ForbiddenJSONResponse{Message: "quran version access required"}}, nil
	}
	version, err := s.DB.FetchQuranVersionByID(ctx, versionID)
	if err != nil {
		slog.Error("GetEditorialQuranChapter: fetch version", "error", err)
		return GetEditorialQuranChapter500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if version == nil {
		return GetEditorialQuranChapter404JSONResponse{NotFoundJSONResponse{Message: "quran version not found"}}, nil
	}
	chapter, err := s.DB.FetchQuranChapterByNumber(ctx, chapterNumber)
	if err != nil {
		slog.Error("GetEditorialQuranChapter: fetch chapter", "error", err)
		return GetEditorialQuranChapter500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if chapter == nil {
		return GetEditorialQuranChapter404JSONResponse{NotFoundJSONResponse{Message: "chapter not found"}}, nil
	}

	refVersionID, refLangID, err := s.resolveReference(ctx, version)
	if err != nil {
		slog.Error("GetEditorialQuranChapter: resolve reference", "error", err)
		return GetEditorialQuranChapter500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	verses, err := s.DB.FetchQuranChapterEditorVerses(ctx, versionID, chapter.ID, refVersionID, refLangID)
	if err != nil {
		slog.Error("GetEditorialQuranChapter: fetch verses", "error", err)
		return GetEditorialQuranChapter500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	titleDraft, err := s.DB.FetchChapterTitleDraft(ctx, versionID, chapter.ID)
	if err != nil {
		slog.Error("GetEditorialQuranChapter: fetch title draft", "error", err)
		return GetEditorialQuranChapter500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	titlePublished, err := s.DB.FetchChapterTitlePublished(ctx, versionID, chapter.ID)
	if err != nil {
		slog.Error("GetEditorialQuranChapter: fetch title published", "error", err)
		return GetEditorialQuranChapter500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	pendingReq, err := s.DB.FetchPendingPublishRequest(ctx, versionID, chapterNumber)
	if err != nil {
		slog.Error("GetEditorialQuranChapter: fetch pending", "error", err)
		return GetEditorialQuranChapter500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	editor := QuranChapterEditor{
		ChapterNumber:  chapter.ChapterNumber,
		VerseCount:     chapter.VerseCount,
		TitleDraft:     titleDraft,
		TitlePublished: titlePublished,
		CanWrite:       perms.CanWriteQuranVersion(versionID),
		Verses:         make([]QuranVerseDraft, 0, len(verses)),
	}
	if version.ReferenceVersionID != nil {
		rv := int64(*version.ReferenceVersionID)
		editor.ReferenceVersionId = &rv
	}
	for _, v := range verses {
		editor.Verses = append(editor.Verses, toQuranVerseDraft(v))
	}
	if pendingReq != nil {
		pr := toPublishRequest(pendingReq)
		editor.PendingRequest = &pr
	}
	return GetEditorialQuranChapter200JSONResponse(EditorialQuranChapterEnvelope{Data: editor}), nil
}

// resolveReference returns the reference version id + its lang id for the rail,
// or (nil, nil) when no reference version is configured.
func (s *Server) resolveReference(ctx context.Context, version *db.QuranVersionRow) (*int, *int, error) {
	if version.ReferenceVersionID == nil {
		return nil, nil, nil
	}
	ref, err := s.DB.FetchQuranVersionByID(ctx, *version.ReferenceVersionID)
	if err != nil {
		return nil, nil, err
	}
	if ref == nil {
		return nil, nil, nil
	}
	refVersionID := ref.ID
	refLangID := int(ref.LangID)
	return &refVersionID, &refLangID, nil
}

// UpsertEditorialQuranVerseDraft saves one verse draft.
func (s *Server) UpsertEditorialQuranVerseDraft(ctx context.Context, request UpsertEditorialQuranVerseDraftRequestObject) (UpsertEditorialQuranVerseDraftResponseObject, error) {
	versionID := int(request.VersionId)
	chapterNumber := int(request.ChapterNumber)
	verseNumber := int(request.VerseNumber)

	userID, perms, err := s.resolveEditorialActor(ctx)
	if err != nil {
		slog.Error("UpsertEditorialQuranVerseDraft: resolve actor", "error", err)
		return UpsertEditorialQuranVerseDraft500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !perms.CanWriteQuranVersion(versionID) {
		return UpsertEditorialQuranVerseDraft403JSONResponse{ForbiddenJSONResponse{Message: "quran version write access required"}}, nil
	}
	version, err := s.DB.FetchQuranVersionByID(ctx, versionID)
	if err != nil {
		slog.Error("UpsertEditorialQuranVerseDraft: fetch version", "error", err)
		return UpsertEditorialQuranVerseDraft500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if version == nil {
		return UpsertEditorialQuranVerseDraft404JSONResponse{NotFoundJSONResponse{Message: "quran version not found"}}, nil
	}
	chapter, err := s.DB.FetchQuranChapterByNumber(ctx, chapterNumber)
	if err != nil {
		slog.Error("UpsertEditorialQuranVerseDraft: fetch chapter", "error", err)
		return UpsertEditorialQuranVerseDraft500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if chapter == nil {
		return UpsertEditorialQuranVerseDraft404JSONResponse{NotFoundJSONResponse{Message: "chapter not found"}}, nil
	}
	if verseNumber < 1 || verseNumber > chapter.VerseCount {
		return UpsertEditorialQuranVerseDraft400JSONResponse{BadRequestJSONResponse{Message: "verse number out of range for chapter"}}, nil
	}
	verseID, ok, err := s.DB.FetchVerseID(ctx, chapter.ID, verseNumber)
	if err != nil {
		slog.Error("UpsertEditorialQuranVerseDraft: fetch verse id", "error", err)
		return UpsertEditorialQuranVerseDraft500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !ok {
		return UpsertEditorialQuranVerseDraft404JSONResponse{NotFoundJSONResponse{Message: "verse not found"}}, nil
	}

	fields := db.QuranText{}
	if request.Body != nil {
		fields = db.QuranText{Subtitle: request.Body.Subtitle, Content: request.Body.Content, Footer: request.Body.Footer}
	}
	row, err := s.DB.UpsertVerseDraft(ctx, versionID, verseID, int(version.LangID), fields, userID)
	if err != nil {
		slog.Error("UpsertEditorialQuranVerseDraft: upsert", "error", err)
		return UpsertEditorialQuranVerseDraft500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	row.VerseNumber = verseNumber
	ch, vn := chapterNumber, verseNumber
	if err := s.DB.AppendChangelog(ctx, db.ChangelogEntry{
		VersionID: versionID, ChapterNumber: &ch, VerseNumber: &vn,
		Action: db.ActionVerseDraftSaved, ActorID: userID,
	}); err != nil {
		slog.Error("UpsertEditorialQuranVerseDraft: changelog", "error", err)
	}
	return UpsertEditorialQuranVerseDraft200JSONResponse(EditorialQuranVerseDraftEnvelope{Data: toQuranVerseDraft(*row)}), nil
}

// UpsertEditorialQuranChapterDraft saves the chapter-title draft.
func (s *Server) UpsertEditorialQuranChapterDraft(ctx context.Context, request UpsertEditorialQuranChapterDraftRequestObject) (UpsertEditorialQuranChapterDraftResponseObject, error) {
	versionID := int(request.VersionId)
	chapterNumber := int(request.ChapterNumber)

	userID, perms, err := s.resolveEditorialActor(ctx)
	if err != nil {
		slog.Error("UpsertEditorialQuranChapterDraft: resolve actor", "error", err)
		return UpsertEditorialQuranChapterDraft500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !perms.CanWriteQuranVersion(versionID) {
		return UpsertEditorialQuranChapterDraft403JSONResponse{ForbiddenJSONResponse{Message: "quran version write access required"}}, nil
	}
	version, err := s.DB.FetchQuranVersionByID(ctx, versionID)
	if err != nil {
		slog.Error("UpsertEditorialQuranChapterDraft: fetch version", "error", err)
		return UpsertEditorialQuranChapterDraft500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if version == nil {
		return UpsertEditorialQuranChapterDraft404JSONResponse{NotFoundJSONResponse{Message: "quran version not found"}}, nil
	}
	chapter, err := s.DB.FetchQuranChapterByNumber(ctx, chapterNumber)
	if err != nil {
		slog.Error("UpsertEditorialQuranChapterDraft: fetch chapter", "error", err)
		return UpsertEditorialQuranChapterDraft500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if chapter == nil {
		return UpsertEditorialQuranChapterDraft404JSONResponse{NotFoundJSONResponse{Message: "chapter not found"}}, nil
	}

	var title *string
	if request.Body != nil {
		title = request.Body.Title
	}
	if err := s.DB.UpsertChapterDraft(ctx, versionID, chapter.ID, int(version.LangID), title, userID); err != nil {
		slog.Error("UpsertEditorialQuranChapterDraft: upsert", "error", err)
		return UpsertEditorialQuranChapterDraft500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	ch := chapterNumber
	if err := s.DB.AppendChangelog(ctx, db.ChangelogEntry{
		VersionID: versionID, ChapterNumber: &ch,
		Action: db.ActionChapterDraftSaved, ActorID: userID,
	}); err != nil {
		slog.Error("UpsertEditorialQuranChapterDraft: changelog", "error", err)
	}
	resp := EditorialQuranChapterDraftEnvelope{}
	resp.Data.ChapterNumber = chapterNumber
	resp.Data.Title = title
	return UpsertEditorialQuranChapterDraft200JSONResponse(resp), nil
}

// CreateEditorialQuranPublishRequest opens a pending request for a chapter.
func (s *Server) CreateEditorialQuranPublishRequest(ctx context.Context, request CreateEditorialQuranPublishRequestRequestObject) (CreateEditorialQuranPublishRequestResponseObject, error) {
	versionID := int(request.VersionId)
	chapterNumber := int(request.ChapterNumber)

	userID, perms, err := s.resolveEditorialActor(ctx)
	if err != nil {
		slog.Error("CreateEditorialQuranPublishRequest: resolve actor", "error", err)
		return CreateEditorialQuranPublishRequest500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !perms.CanWriteQuranVersion(versionID) {
		return CreateEditorialQuranPublishRequest403JSONResponse{ForbiddenJSONResponse{Message: "quran version write access required"}}, nil
	}
	version, err := s.DB.FetchQuranVersionByID(ctx, versionID)
	if err != nil {
		slog.Error("CreateEditorialQuranPublishRequest: fetch version", "error", err)
		return CreateEditorialQuranPublishRequest500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if version == nil {
		return CreateEditorialQuranPublishRequest404JSONResponse{NotFoundJSONResponse{Message: "quran version not found"}}, nil
	}
	chapter, err := s.DB.FetchQuranChapterByNumber(ctx, chapterNumber)
	if err != nil {
		slog.Error("CreateEditorialQuranPublishRequest: fetch chapter", "error", err)
		return CreateEditorialQuranPublishRequest500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if chapter == nil {
		return CreateEditorialQuranPublishRequest404JSONResponse{NotFoundJSONResponse{Message: "chapter not found"}}, nil
	}

	var note *string
	if request.Body != nil {
		note = request.Body.Note
	}
	req, err := s.DB.CreatePublishRequest(ctx, versionID, chapterNumber, userID, note)
	if errors.Is(err, db.ErrPublishRequestPending) {
		return CreateEditorialQuranPublishRequest409JSONResponse{ConflictJSONResponse{Message: "a publish request is already pending for this chapter"}}, nil
	}
	if err != nil {
		slog.Error("CreateEditorialQuranPublishRequest: create", "error", err)
		return CreateEditorialQuranPublishRequest500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	return CreateEditorialQuranPublishRequest201JSONResponse(EditorialQuranPublishRequestEnvelope{Data: toPublishRequest(req)}), nil
}

// ListEditorialQuranPublishRequests returns the requests visible to the caller.
func (s *Server) ListEditorialQuranPublishRequests(ctx context.Context, request ListEditorialQuranPublishRequestsRequestObject) (ListEditorialQuranPublishRequestsResponseObject, error) {
	userID, perms, err := s.resolveEditorialActor(ctx)
	if err != nil {
		slog.Error("ListEditorialQuranPublishRequests: resolve actor", "error", err)
		return ListEditorialQuranPublishRequests500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !perms.CanReadModule(db.ModuleQuran) {
		return ListEditorialQuranPublishRequests403JSONResponse{ForbiddenJSONResponse{Message: "quran editorial access required"}}, nil
	}

	var status *string
	if request.Params.Status != nil {
		st := string(*request.Params.Status)
		status = &st
	}
	var versionFilter *int
	if request.Params.VersionId != nil {
		v := int(*request.Params.VersionId)
		versionFilter = &v
	}
	rows, err := s.DB.ListPublishRequests(ctx, userID, perms.IsAdmin, perms.ApprovableQuranVersionIDs(), status, versionFilter)
	if err != nil {
		slog.Error("ListEditorialQuranPublishRequests: list", "error", err)
		return ListEditorialQuranPublishRequests500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	out := make([]QuranPublishRequest, 0, len(rows))
	for i := range rows {
		out = append(out, toPublishRequest(&rows[i]))
	}
	return ListEditorialQuranPublishRequests200JSONResponse(EditorialQuranPublishRequestListEnvelope{Data: out}), nil
}

// ApproveEditorialQuranPublishRequest approves a request and syncs the chapter.
func (s *Server) ApproveEditorialQuranPublishRequest(ctx context.Context, request ApproveEditorialQuranPublishRequestRequestObject) (ApproveEditorialQuranPublishRequestResponseObject, error) {
	userID, perms, err := s.resolveEditorialActor(ctx)
	if err != nil {
		slog.Error("ApproveEditorialQuranPublishRequest: resolve actor", "error", err)
		return ApproveEditorialQuranPublishRequest500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	req, err := s.DB.FetchPublishRequestByID(ctx, int(request.RequestId))
	if errors.Is(err, db.ErrPublishRequestNotFound) {
		return ApproveEditorialQuranPublishRequest404JSONResponse{NotFoundJSONResponse{Message: "publish request not found"}}, nil
	}
	if err != nil {
		slog.Error("ApproveEditorialQuranPublishRequest: fetch", "error", err)
		return ApproveEditorialQuranPublishRequest500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !perms.CanApproveQuranVersion(req.VersionID) {
		return ApproveEditorialQuranPublishRequest403JSONResponse{ForbiddenJSONResponse{Message: "approve permission required for this version"}}, nil
	}
	var note *string
	if request.Body != nil {
		note = request.Body.Note
	}
	updated, err := s.DB.ApprovePublishRequest(ctx, req.ID, userID, note)
	if errors.Is(err, db.ErrPublishRequestNotPending) {
		return ApproveEditorialQuranPublishRequest409JSONResponse{ConflictJSONResponse{Message: "publish request is not pending"}}, nil
	}
	if errors.Is(err, db.ErrPublishRequestNotFound) {
		return ApproveEditorialQuranPublishRequest404JSONResponse{NotFoundJSONResponse{Message: "publish request not found"}}, nil
	}
	if err != nil {
		slog.Error("ApproveEditorialQuranPublishRequest: approve", "error", err)
		return ApproveEditorialQuranPublishRequest500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	return ApproveEditorialQuranPublishRequest200JSONResponse(EditorialQuranPublishRequestEnvelope{Data: toPublishRequest(updated)}), nil
}

// RejectEditorialQuranPublishRequest rejects a request without publishing.
func (s *Server) RejectEditorialQuranPublishRequest(ctx context.Context, request RejectEditorialQuranPublishRequestRequestObject) (RejectEditorialQuranPublishRequestResponseObject, error) {
	userID, perms, err := s.resolveEditorialActor(ctx)
	if err != nil {
		slog.Error("RejectEditorialQuranPublishRequest: resolve actor", "error", err)
		return RejectEditorialQuranPublishRequest500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	req, err := s.DB.FetchPublishRequestByID(ctx, int(request.RequestId))
	if errors.Is(err, db.ErrPublishRequestNotFound) {
		return RejectEditorialQuranPublishRequest404JSONResponse{NotFoundJSONResponse{Message: "publish request not found"}}, nil
	}
	if err != nil {
		slog.Error("RejectEditorialQuranPublishRequest: fetch", "error", err)
		return RejectEditorialQuranPublishRequest500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !perms.CanApproveQuranVersion(req.VersionID) {
		return RejectEditorialQuranPublishRequest403JSONResponse{ForbiddenJSONResponse{Message: "approve permission required for this version"}}, nil
	}
	var note *string
	if request.Body != nil {
		note = request.Body.Note
	}
	updated, err := s.DB.RejectPublishRequest(ctx, req.ID, userID, note)
	if errors.Is(err, db.ErrPublishRequestNotPending) {
		return RejectEditorialQuranPublishRequest409JSONResponse{ConflictJSONResponse{Message: "publish request is not pending"}}, nil
	}
	if errors.Is(err, db.ErrPublishRequestNotFound) {
		return RejectEditorialQuranPublishRequest404JSONResponse{NotFoundJSONResponse{Message: "publish request not found"}}, nil
	}
	if err != nil {
		slog.Error("RejectEditorialQuranPublishRequest: reject", "error", err)
		return RejectEditorialQuranPublishRequest500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	return RejectEditorialQuranPublishRequest200JSONResponse(EditorialQuranPublishRequestEnvelope{Data: toPublishRequest(updated)}), nil
}

// CancelEditorialQuranPublishRequest cancels an open request. Allowed for the
// original requester, an approver of the version, or an admin.
func (s *Server) CancelEditorialQuranPublishRequest(ctx context.Context, request CancelEditorialQuranPublishRequestRequestObject) (CancelEditorialQuranPublishRequestResponseObject, error) {
	userID, perms, err := s.resolveEditorialActor(ctx)
	if err != nil {
		slog.Error("CancelEditorialQuranPublishRequest: resolve actor", "error", err)
		return CancelEditorialQuranPublishRequest500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	req, err := s.DB.FetchPublishRequestByID(ctx, int(request.RequestId))
	if errors.Is(err, db.ErrPublishRequestNotFound) {
		return CancelEditorialQuranPublishRequest404JSONResponse{NotFoundJSONResponse{Message: "publish request not found"}}, nil
	}
	if err != nil {
		slog.Error("CancelEditorialQuranPublishRequest: fetch", "error", err)
		return CancelEditorialQuranPublishRequest500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if perms == nil || (userID != req.RequestedBy && !perms.CanApproveQuranVersion(req.VersionID)) {
		return CancelEditorialQuranPublishRequest403JSONResponse{ForbiddenJSONResponse{Message: "not permitted to cancel this request"}}, nil
	}
	updated, err := s.DB.CancelPublishRequest(ctx, req.ID, userID)
	if errors.Is(err, db.ErrPublishRequestNotPending) {
		return CancelEditorialQuranPublishRequest409JSONResponse{ConflictJSONResponse{Message: "publish request is not pending"}}, nil
	}
	if errors.Is(err, db.ErrPublishRequestNotFound) {
		return CancelEditorialQuranPublishRequest404JSONResponse{NotFoundJSONResponse{Message: "publish request not found"}}, nil
	}
	if err != nil {
		slog.Error("CancelEditorialQuranPublishRequest: cancel", "error", err)
		return CancelEditorialQuranPublishRequest500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	return CancelEditorialQuranPublishRequest200JSONResponse(EditorialQuranPublishRequestEnvelope{Data: toPublishRequest(updated)}), nil
}

// ListEditorialQuranChapterChangelog returns recent changelog entries.
func (s *Server) ListEditorialQuranChapterChangelog(ctx context.Context, request ListEditorialQuranChapterChangelogRequestObject) (ListEditorialQuranChapterChangelogResponseObject, error) {
	versionID := int(request.VersionId)
	chapterNumber := int(request.ChapterNumber)
	perms, err := s.resolveEditorialPermissions(ctx)
	if err != nil {
		slog.Error("ListEditorialQuranChapterChangelog: resolve permissions", "error", err)
		return ListEditorialQuranChapterChangelog500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !perms.CanReadQuranVersion(versionID) {
		return ListEditorialQuranChapterChangelog403JSONResponse{ForbiddenJSONResponse{Message: "quran version access required"}}, nil
	}
	chapter, err := s.DB.FetchQuranChapterByNumber(ctx, chapterNumber)
	if err != nil {
		slog.Error("ListEditorialQuranChapterChangelog: fetch chapter", "error", err)
		return ListEditorialQuranChapterChangelog500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if chapter == nil {
		return ListEditorialQuranChapterChangelog404JSONResponse{NotFoundJSONResponse{Message: "chapter not found"}}, nil
	}

	limit := changelogDefaultLimit
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
		if limit < 1 {
			limit = changelogDefaultLimit
		}
		if limit > changelogMaxLimit {
			limit = changelogMaxLimit
		}
	}
	rows, err := s.DB.ListChangelog(ctx, versionID, chapterNumber, limit)
	if err != nil {
		slog.Error("ListEditorialQuranChapterChangelog: list", "error", err)
		return ListEditorialQuranChapterChangelog500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	out := make([]QuranChangelogEntry, 0, len(rows))
	for i := range rows {
		out = append(out, toChangelogEntry(rows[i]))
	}
	return ListEditorialQuranChapterChangelog200JSONResponse(EditorialQuranChangelogListEnvelope{Data: out}), nil
}
