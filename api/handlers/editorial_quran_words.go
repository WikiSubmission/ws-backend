package handlers

import (
	"context"
	"log/slog"

	"github.com/wikiSubmission/ws-backend/db"
)

// Quran word-by-word editorial handlers (Phase 2). Authorization is re-checked
// here on every call. Word drafts publish with their chapter (the migration 019
// workflow extended in syncWordPublished); the live reader corpus is never
// written.

// derefInt returns *p or 0 when p is nil (the "no reference version" sentinel
// FetchQuranChapterWords / FetchQuranWord expect).
func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// GetEditorialQuranChapterWords returns the word-by-word editing surface.
func (s *Server) GetEditorialQuranChapterWords(ctx context.Context, request GetEditorialQuranChapterWordsRequestObject) (GetEditorialQuranChapterWordsResponseObject, error) {
	versionID := int(request.VersionId)
	chapterNumber := int(request.ChapterNumber)
	perms, err := s.resolveEditorialPermissions(ctx)
	if err != nil {
		slog.Error("GetEditorialQuranChapterWords: resolve permissions", "error", err)
		return GetEditorialQuranChapterWords500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !perms.CanReadQuranVersion(versionID) {
		return GetEditorialQuranChapterWords403JSONResponse{ForbiddenJSONResponse{Message: "quran version access required"}}, nil
	}
	version, err := s.DB.FetchQuranVersionByID(ctx, versionID)
	if err != nil {
		slog.Error("GetEditorialQuranChapterWords: fetch version", "error", err)
		return GetEditorialQuranChapterWords500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if version == nil {
		return GetEditorialQuranChapterWords404JSONResponse{NotFoundJSONResponse{Message: "quran version not found"}}, nil
	}
	chapter, err := s.DB.FetchQuranChapterByNumber(ctx, chapterNumber)
	if err != nil {
		slog.Error("GetEditorialQuranChapterWords: fetch chapter", "error", err)
		return GetEditorialQuranChapterWords500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if chapter == nil {
		return GetEditorialQuranChapterWords404JSONResponse{NotFoundJSONResponse{Message: "chapter not found"}}, nil
	}

	refVersionID, refLangID, err := s.resolveReference(ctx, version)
	if err != nil {
		slog.Error("GetEditorialQuranChapterWords: resolve reference", "error", err)
		return GetEditorialQuranChapterWords500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	verses, err := s.DB.FetchQuranChapterWords(ctx, versionID, chapter.ID, derefInt(refVersionID), derefInt(refLangID))
	if err != nil {
		slog.Error("GetEditorialQuranChapterWords: fetch words", "error", err)
		return GetEditorialQuranChapterWords500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	out := QuranChapterWords{
		ChapterNumber: chapter.ChapterNumber,
		CanWrite:      perms.CanWriteQuranVersion(versionID),
		Verses:        make([]QuranVerseWords, 0, len(verses)),
	}
	if version.ReferenceVersionID != nil {
		rv := int64(*version.ReferenceVersionID)
		out.ReferenceVersionId = &rv
	}
	for _, v := range verses {
		vw := QuranVerseWords{VerseNumber: v.VerseNumber, Words: make([]QuranWord, 0, len(v.Words))}
		for _, w := range v.Words {
			vw.Words = append(vw.Words, toQuranWord(w))
		}
		out.Verses = append(out.Verses, vw)
	}
	return GetEditorialQuranChapterWords200JSONResponse(EditorialQuranChapterWordsEnvelope{Data: out}), nil
}

// UpsertEditorialQuranWordDraft saves one per-word draft.
func (s *Server) UpsertEditorialQuranWordDraft(ctx context.Context, request UpsertEditorialQuranWordDraftRequestObject) (UpsertEditorialQuranWordDraftResponseObject, error) {
	versionID := int(request.VersionId)
	wordID := int(request.WordId)

	userID, perms, err := s.resolveEditorialActor(ctx)
	if err != nil {
		slog.Error("UpsertEditorialQuranWordDraft: resolve actor", "error", err)
		return UpsertEditorialQuranWordDraft500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !perms.CanWriteQuranVersion(versionID) {
		return UpsertEditorialQuranWordDraft403JSONResponse{ForbiddenJSONResponse{Message: "quran version write access required"}}, nil
	}
	version, err := s.DB.FetchQuranVersionByID(ctx, versionID)
	if err != nil {
		slog.Error("UpsertEditorialQuranWordDraft: fetch version", "error", err)
		return UpsertEditorialQuranWordDraft500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if version == nil {
		return UpsertEditorialQuranWordDraft404JSONResponse{NotFoundJSONResponse{Message: "quran version not found"}}, nil
	}
	loc, ok, err := s.DB.FetchWordLocation(ctx, wordID)
	if err != nil {
		slog.Error("UpsertEditorialQuranWordDraft: fetch word location", "error", err)
		return UpsertEditorialQuranWordDraft500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !ok {
		return UpsertEditorialQuranWordDraft404JSONResponse{NotFoundJSONResponse{Message: "word not found"}}, nil
	}

	fields := db.QuranWordFields{}
	if request.Body != nil {
		fields = db.QuranWordFields{Word: request.Body.Word, Meaning: request.Body.Meaning}
	}
	if err := s.DB.UpsertWordDraft(ctx, versionID, wordID, int(version.LangID), fields, userID); err != nil {
		slog.Error("UpsertEditorialQuranWordDraft: upsert", "error", err)
		return UpsertEditorialQuranWordDraft500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	ch, vn := loc.ChapterNumber, loc.VerseNumber
	if err := s.DB.AppendChangelog(ctx, db.ChangelogEntry{
		VersionID: versionID, ChapterNumber: &ch, VerseNumber: &vn,
		Action: db.ActionWordDraftSaved, ActorID: userID,
	}); err != nil {
		slog.Error("UpsertEditorialQuranWordDraft: changelog", "error", err)
	}

	refVersionID, refLangID, err := s.resolveReference(ctx, version)
	if err != nil {
		slog.Error("UpsertEditorialQuranWordDraft: resolve reference", "error", err)
		return UpsertEditorialQuranWordDraft500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	word, err := s.DB.FetchQuranWord(ctx, versionID, wordID, derefInt(refVersionID), derefInt(refLangID))
	if err != nil {
		slog.Error("UpsertEditorialQuranWordDraft: fetch word", "error", err)
		return UpsertEditorialQuranWordDraft500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if word == nil {
		return UpsertEditorialQuranWordDraft404JSONResponse{NotFoundJSONResponse{Message: "word not found"}}, nil
	}
	return UpsertEditorialQuranWordDraft200JSONResponse(EditorialQuranWordDraftEnvelope{Data: toQuranWord(*word)}), nil
}
