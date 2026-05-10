package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/auth"
	"github.com/wikiSubmission/ws-backend/db"
)

func authIDFromContext(ctx context.Context) (string, bool) {
	if v := ctx.Value(auth.ContextKeyUID); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s, true
		}
	}

	// Strict handlers receive context.Context, but at runtime this is usually
	// *gin.Context. Read both Gin keys and the underlying request context.
	if gc, ok := ctx.(*gin.Context); ok {
		if v, exists := gc.Get(auth.ContextKeyUID); exists {
			if s, ok := v.(string); ok && s != "" {
				return s, true
			}
		}
		if gc.Request != nil {
			if v := gc.Request.Context().Value(auth.ContextKeyUID); v != nil {
				if s, ok := v.(string); ok && s != "" {
					return s, true
				}
			}
		}
	}

	return "", false
}

func emailFromContext(ctx context.Context) (string, bool) {
	if v := ctx.Value(auth.ContextKeyEmail); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s, true
		}
	}

	if gc, ok := ctx.(*gin.Context); ok {
		if v, exists := gc.Get(auth.ContextKeyEmail); exists {
			if s, ok := v.(string); ok && s != "" {
				return s, true
			}
		}
		if gc.Request != nil {
			if v := gc.Request.Context().Value(auth.ContextKeyEmail); v != nil {
				if s, ok := v.(string); ok && s != "" {
					return s, true
				}
			}
		}
	}

	return "", false
}

// ensureCurrentUser provisions a users row on first /me write so insert-select
// queries that rely on users.auth_id always have a matching user id. Also
// ensures the user has a default bookmark category to land their first save.
func (s *Server) ensureCurrentUser(ctx context.Context, authID string) error {
	user, err := s.DB.FetchUserByAuthID(ctx, authID)
	if err != nil {
		return err
	}
	if user == nil {
		email, ok := emailFromContext(ctx)
		if !ok {
			return fmt.Errorf("no users row for auth id %q and no email claim available", authID)
		}
		if _, err := s.DB.UpsertUser(ctx, authID, email, nil); err != nil {
			return err
		}
	}

	if err := s.DB.EnsureDefaultBookmarkCategory(ctx, authID); err != nil {
		return fmt.Errorf("ensure default category: %w", err)
	}
	return nil
}

func validScripture(scripture string) bool {
	return scripture == "quran" || scripture == "bible"
}

func bookmarkFromRow(b *db.BookmarkRow) Bookmark {
	return Bookmark{
		Id:        b.ID,
		Scripture: BookmarkScripture(b.Scripture),
		VerseKey:  b.VerseKey,
		Name:      b.Name,
		Color:     b.Color,
		Kind:      BookmarkKind(b.Kind),
		CreatedAt: b.CreatedAt.UTC(),
		UpdatedAt: b.UpdatedAt.UTC(),
	}
}

func categoryFromRow(c *db.BookmarkCategoryRow) BookmarkCategory {
	return BookmarkCategory{
		Id:         c.ID,
		Name:       c.Name,
		Color:      c.Color,
		EntryCount: c.EntryCount,
		CreatedAt:  c.CreatedAt.UTC(),
		UpdatedAt:  c.UpdatedAt.UTC(),
	}
}

func entryFromRow(e *db.BookmarkEntryRow) BookmarkEntry {
	return BookmarkEntry{
		Id:         e.ID,
		CategoryId: e.CategoryID,
		Scripture:  BookmarkEntryScripture(e.Scripture),
		VerseKey:   e.VerseKey,
		CreatedAt:  e.CreatedAt.UTC(),
	}
}

func noteFromRow(n *db.NoteRow) Note {
	return Note{
		Id:        n.ID,
		Scripture: NoteScripture(n.Scripture),
		VerseKey:  n.VerseKey,
		Content:   n.Content,
		CreatedAt: n.CreatedAt.UTC(),
		UpdatedAt: n.UpdatedAt.UTC(),
	}
}

func collectionFromRow(c *db.CollectionRow) Collection {
	return Collection{
		Id:          c.ID,
		Name:        c.Name,
		Description: c.Description,
		IsPublic:    c.IsPublic,
		ShareToken:  c.ShareToken,
		CreatedAt:   c.CreatedAt.UTC(),
		UpdatedAt:   c.UpdatedAt.UTC(),
	}
}

func collectionVerseFromRow(v *db.CollectionVerseRow) CollectionVerse {
	return CollectionVerse{
		Id:           v.ID,
		CollectionId: v.CollectionID,
		Scripture:    CollectionVerseScripture(v.Scripture),
		VerseKey:     v.VerseKey,
		Note:         v.Note,
		Position:     v.Position,
		CreatedAt:    v.CreatedAt.UTC(),
	}
}

func nullDataEnvelope() NullDataEnvelope {
	return NullDataEnvelope{Data: nil}
}

func (s *Server) ListMeBookmarkCategories(ctx context.Context, _ ListMeBookmarkCategoriesRequestObject) (ListMeBookmarkCategoriesResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return ListMeBookmarkCategories500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if err := s.ensureCurrentUser(ctx, authID); err != nil {
		slog.Error("ListMeBookmarkCategories: ensureCurrentUser failed", "error", err)
	}
	cats, err := s.DB.ListBookmarkCategories(ctx, authID)
	if err != nil {
		slog.Error("ListMeBookmarkCategories: DB error", "error", err)
		return ListMeBookmarkCategories500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	result := make([]BookmarkCategory, len(cats))
	for i := range cats {
		result[i] = categoryFromRow(&cats[i])
	}
	return ListMeBookmarkCategories200JSONResponse(BookmarkCategoryListEnvelope{Data: result}), nil
}

func (s *Server) CreateMeBookmarkCategory(ctx context.Context, request CreateMeBookmarkCategoryRequestObject) (CreateMeBookmarkCategoryResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return CreateMeBookmarkCategory500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if err := s.ensureCurrentUser(ctx, authID); err != nil {
		slog.Error("CreateMeBookmarkCategory: ensureCurrentUser failed", "error", err)
		return CreateMeBookmarkCategory500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if request.Body == nil {
		return CreateMeBookmarkCategory400JSONResponse{BadRequestJSONResponse{Message: "request body is required"}}, nil
	}
	req := *request.Body
	if req.Name == "" {
		return CreateMeBookmarkCategory400JSONResponse{BadRequestJSONResponse{Message: "name is required"}}, nil
	}
	color := "amber"
	if req.Color != nil && *req.Color != "" {
		color = *req.Color
	}
	c, err := s.DB.CreateBookmarkCategory(ctx, authID, req.Name, color)
	if err != nil {
		slog.Error("CreateMeBookmarkCategory: DB error", "error", err)
		return CreateMeBookmarkCategory500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	return CreateMeBookmarkCategory201JSONResponse(BookmarkCategoryEnvelope{Data: categoryFromRow(c)}), nil
}

func (s *Server) UpdateMeBookmarkCategory(ctx context.Context, request UpdateMeBookmarkCategoryRequestObject) (UpdateMeBookmarkCategoryResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return UpdateMeBookmarkCategory500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if request.Body == nil {
		return UpdateMeBookmarkCategory400JSONResponse{BadRequestJSONResponse{Message: "request body is required"}}, nil
	}
	req := *request.Body

	// Fetch current to merge optional fields.
	cats, err := s.DB.ListBookmarkCategories(ctx, authID)
	if err != nil {
		slog.Error("UpdateMeBookmarkCategory: list error", "error", err)
		return UpdateMeBookmarkCategory500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	var currentName, currentColor string
	for i := range cats {
		if cats[i].ID == request.Id {
			currentName = cats[i].Name
			currentColor = cats[i].Color
			break
		}
	}
	if currentName == "" {
		return UpdateMeBookmarkCategory404JSONResponse{NotFoundJSONResponse{Message: "category not found"}}, nil
	}
	if req.Name != nil && *req.Name != "" {
		currentName = *req.Name
	}
	if req.Color != nil && *req.Color != "" {
		currentColor = *req.Color
	}

	c, err := s.DB.UpdateBookmarkCategory(ctx, request.Id, authID, currentName, currentColor)
	if err != nil {
		slog.Error("UpdateMeBookmarkCategory: DB error", "error", err)
		return UpdateMeBookmarkCategory500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if c == nil {
		return UpdateMeBookmarkCategory404JSONResponse{NotFoundJSONResponse{Message: "category not found"}}, nil
	}
	return UpdateMeBookmarkCategory200JSONResponse(BookmarkCategoryEnvelope{Data: categoryFromRow(c)}), nil
}

func (s *Server) DeleteMeBookmarkCategory(ctx context.Context, request DeleteMeBookmarkCategoryRequestObject) (DeleteMeBookmarkCategoryResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return DeleteMeBookmarkCategory500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	deleted, err := s.DB.DeleteBookmarkCategory(ctx, request.Id, authID)
	if err != nil {
		slog.Error("DeleteMeBookmarkCategory: DB error", "error", err)
		return DeleteMeBookmarkCategory500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !deleted {
		return DeleteMeBookmarkCategory404JSONResponse{NotFoundJSONResponse{Message: "category not found"}}, nil
	}
	return DeleteMeBookmarkCategory200JSONResponse(nullDataEnvelope()), nil
}

func (s *Server) ListMeBookmarkCategoryEntries(ctx context.Context, request ListMeBookmarkCategoryEntriesRequestObject) (ListMeBookmarkCategoryEntriesResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return ListMeBookmarkCategoryEntries500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	entries, err := s.DB.ListBookmarkCategoryEntries(ctx, request.Id, authID)
	if err != nil {
		slog.Error("ListMeBookmarkCategoryEntries: DB error", "error", err)
		return ListMeBookmarkCategoryEntries500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	result := make([]BookmarkEntry, len(entries))
	for i := range entries {
		result[i] = entryFromRow(&entries[i])
	}
	return ListMeBookmarkCategoryEntries200JSONResponse(BookmarkEntryListEnvelope{Data: result}), nil
}

func (s *Server) CreateMeBookmarkEntry(ctx context.Context, request CreateMeBookmarkEntryRequestObject) (CreateMeBookmarkEntryResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return CreateMeBookmarkEntry500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if err := s.ensureCurrentUser(ctx, authID); err != nil {
		slog.Error("CreateMeBookmarkEntry: ensureCurrentUser failed", "error", err)
		return CreateMeBookmarkEntry500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if request.Body == nil {
		return CreateMeBookmarkEntry400JSONResponse{BadRequestJSONResponse{Message: "request body is required"}}, nil
	}
	req := *request.Body
	scripture := string(req.Scripture)
	if !validScripture(scripture) {
		return CreateMeBookmarkEntry400JSONResponse{BadRequestJSONResponse{Message: "scripture must be 'quran' or 'bible'"}}, nil
	}
	e, err := s.DB.CreateBookmarkEntry(ctx, req.CategoryId, authID, scripture, req.VerseKey)
	if err != nil {
		slog.Error("CreateMeBookmarkEntry: DB error", "error", err)
		return CreateMeBookmarkEntry500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if e == nil {
		return CreateMeBookmarkEntry409JSONResponse{ConflictJSONResponse{Message: "entry already exists or category not found"}}, nil
	}
	return CreateMeBookmarkEntry201JSONResponse(BookmarkEntryEnvelope{Data: entryFromRow(e)}), nil
}

func (s *Server) DeleteMeBookmarkEntry(ctx context.Context, request DeleteMeBookmarkEntryRequestObject) (DeleteMeBookmarkEntryResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return DeleteMeBookmarkEntry500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	deleted, err := s.DB.DeleteBookmarkEntry(ctx, request.Id, authID)
	if err != nil {
		slog.Error("DeleteMeBookmarkEntry: DB error", "error", err)
		return DeleteMeBookmarkEntry500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !deleted {
		return DeleteMeBookmarkEntry404JSONResponse{NotFoundJSONResponse{Message: "entry not found"}}, nil
	}
	return DeleteMeBookmarkEntry200JSONResponse(nullDataEnvelope()), nil
}

func (s *Server) PutMeCoverToCover(ctx context.Context, request PutMeCoverToCoverRequestObject) (PutMeCoverToCoverResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return PutMeCoverToCover500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if err := s.ensureCurrentUser(ctx, authID); err != nil {
		slog.Error("PutMeCoverToCover: ensureCurrentUser failed", "error", err)
		return PutMeCoverToCover500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if request.Body == nil {
		return PutMeCoverToCover400JSONResponse{BadRequestJSONResponse{Message: "request body is required"}}, nil
	}
	req := *request.Body

	scripture := string(req.Scripture)
	if !validScripture(scripture) {
		return PutMeCoverToCover400JSONResponse{BadRequestJSONResponse{Message: "scripture must be 'quran' or 'bible'"}}, nil
	}
	b, err := s.DB.UpsertCoverToCover(ctx, authID, scripture, req.VerseKey)
	if err != nil {
		slog.Error("PutMeCoverToCover: DB error", "error", err)
		return PutMeCoverToCover500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	return PutMeCoverToCover200JSONResponse(BookmarkEnvelope{Data: bookmarkFromRow(b)}), nil
}

func (s *Server) GetMeScriptureState(ctx context.Context, request GetMeScriptureStateRequestObject) (GetMeScriptureStateResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return GetMeScriptureState500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	scripture := "quran"
	if request.Params.Scripture != nil {
		scripture = string(*request.Params.Scripture)
	}
	if !validScripture(scripture) {
		return GetMeScriptureState400JSONResponse{BadRequestJSONResponse{Message: "scripture must be 'quran' or 'bible'"}}, nil
	}
	if request.Params.Chapter < 1 {
		return GetMeScriptureState400JSONResponse{BadRequestJSONResponse{Message: "invalid chapter"}}, nil
	}

	entries, err := s.DB.FetchChapterBookmarkEntries(ctx, authID, scripture, request.Params.Chapter)
	if err != nil {
		slog.Error("GetMeScriptureState: entries DB error", "error", err)
		return GetMeScriptureState500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	notes, err := s.DB.FetchChapterNotes(ctx, authID, scripture, request.Params.Chapter)
	if err != nil {
		slog.Error("GetMeScriptureState: notes DB error", "error", err)
		return GetMeScriptureState500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	entryMap := make(map[string][]BookmarkEntry)
	for i := range entries {
		e := entryFromRow(&entries[i])
		entryMap[e.VerseKey] = append(entryMap[e.VerseKey], e)
	}
	noteMap := make(map[string]Note)
	for i := range notes {
		n := noteFromRow(&notes[i])
		noteMap[n.VerseKey] = n
	}

	return GetMeScriptureState200JSONResponse(ScriptureState{Bookmarks: entryMap, Notes: noteMap}), nil
}

func (s *Server) GetMeNotes(ctx context.Context, request GetMeNotesRequestObject) (GetMeNotesResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return GetMeNotes500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	scripture := "quran"
	if request.Params.Scripture != nil {
		scripture = string(*request.Params.Scripture)
	}
	if !validScripture(scripture) {
		return GetMeNotes400JSONResponse{BadRequestJSONResponse{Message: "scripture must be 'quran' or 'bible'"}}, nil
	}

	notes, err := s.DB.FetchNotes(ctx, authID, scripture, request.Params.VerseKey)
	if err != nil {
		slog.Error("GetMeNotes: DB error", "error", err)
		return GetMeNotes500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	result := make([]Note, len(notes))
	for i := range notes {
		result[i] = noteFromRow(&notes[i])
	}
	return GetMeNotes200JSONResponse(NoteListEnvelope{Data: result}), nil
}

func (s *Server) UpsertMeNote(ctx context.Context, request UpsertMeNoteRequestObject) (UpsertMeNoteResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return UpsertMeNote500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if err := s.ensureCurrentUser(ctx, authID); err != nil {
		slog.Error("UpsertMeNote: ensureCurrentUser failed", "error", err)
		return UpsertMeNote500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if request.Body == nil {
		return UpsertMeNote400JSONResponse{BadRequestJSONResponse{Message: "request body is required"}}, nil
	}
	req := *request.Body

	scripture := string(req.Scripture)
	if !validScripture(scripture) {
		return UpsertMeNote400JSONResponse{BadRequestJSONResponse{Message: "scripture must be 'quran' or 'bible'"}}, nil
	}
	if req.Content == "" {
		return UpsertMeNote400JSONResponse{BadRequestJSONResponse{Message: "content is required"}}, nil
	}

	n, err := s.DB.UpsertNote(ctx, authID, scripture, req.VerseKey, req.Content)
	if err != nil {
		slog.Error("UpsertMeNote: DB error", "error", err)
		return UpsertMeNote500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	return UpsertMeNote200JSONResponse(NoteEnvelope{Data: noteFromRow(n)}), nil
}

func (s *Server) DeleteMeNote(ctx context.Context, request DeleteMeNoteRequestObject) (DeleteMeNoteResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return DeleteMeNote500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	deleted, err := s.DB.DeleteNote(ctx, request.Id, authID)
	if err != nil {
		slog.Error("DeleteMeNote: DB error", "error", err)
		return DeleteMeNote500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !deleted {
		return DeleteMeNote404JSONResponse{NotFoundJSONResponse{Message: "note not found"}}, nil
	}
	return DeleteMeNote200JSONResponse(nullDataEnvelope()), nil
}

func (s *Server) GetMeSearch(ctx context.Context, request GetMeSearchRequestObject) (GetMeSearchResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return GetMeSearch500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	q := request.Params.Q
	if q == "" {
		return GetMeSearch400JSONResponse{BadRequestJSONResponse{Message: "q is required"}}, nil
	}

	var scripture *string
	if request.Params.Scripture != nil {
		sc := string(*request.Params.Scripture)
		if !validScripture(sc) {
			return GetMeSearch400JSONResponse{BadRequestJSONResponse{Message: "scripture must be 'quran' or 'bible'"}}, nil
		}
		scripture = &sc
	}

	matches, err := s.DB.SearchNotes(ctx, authID, q, scripture)
	if err != nil {
		slog.Error("GetMeSearch: DB error", "error", err)
		return GetMeSearch500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	results := make([]SearchResult, len(matches))
	for i, m := range matches {
		results[i] = SearchResult{
			Source:    SearchResultSource("note"),
			VerseKey:  m.VerseKey,
			Scripture: SearchResultScripture(m.Scripture),
			Excerpt:   m.Excerpt,
		}
	}
	return GetMeSearch200JSONResponse(MeSearchEnvelope{Data: results}), nil
}

func (s *Server) GetMeReadingProgress(ctx context.Context, request GetMeReadingProgressRequestObject) (GetMeReadingProgressResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return GetMeReadingProgress500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	scripture := "quran"
	if request.Params.Scripture != nil {
		scripture = string(*request.Params.Scripture)
	}
	if !validScripture(scripture) {
		return GetMeReadingProgress400JSONResponse{BadRequestJSONResponse{Message: "scripture must be 'quran' or 'bible'"}}, nil
	}

	rp, err := s.DB.GetReadingProgress(ctx, authID, scripture)
	if err != nil {
		slog.Error("GetMeReadingProgress: DB error", "error", err)
		return GetMeReadingProgress500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if rp == nil {
		return GetMeReadingProgress200JSONResponse(ReadingProgressEnvelope{Data: nil}), nil
	}

	resp := &ReadingProgress{
		Scripture: ReadingProgressScripture(rp.Scripture),
		VerseKey:  rp.VerseKey,
		UpdatedAt: rp.UpdatedAt.UTC(),
	}
	return GetMeReadingProgress200JSONResponse(ReadingProgressEnvelope{Data: resp}), nil
}

func (s *Server) PutMeReadingProgress(ctx context.Context, request PutMeReadingProgressRequestObject) (PutMeReadingProgressResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return PutMeReadingProgress500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if err := s.ensureCurrentUser(ctx, authID); err != nil {
		slog.Error("PutMeReadingProgress: ensureCurrentUser failed", "error", err)
		return PutMeReadingProgress500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if request.Body == nil {
		return PutMeReadingProgress400JSONResponse{BadRequestJSONResponse{Message: "request body is required"}}, nil
	}
	req := *request.Body

	scripture := string(req.Scripture)
	if !validScripture(scripture) {
		return PutMeReadingProgress400JSONResponse{BadRequestJSONResponse{Message: "scripture must be 'quran' or 'bible'"}}, nil
	}
	rp, err := s.DB.UpsertReadingProgress(ctx, authID, scripture, req.VerseKey)
	if err != nil {
		slog.Error("PutMeReadingProgress: DB error", "error", err)
		return PutMeReadingProgress500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	return PutMeReadingProgress200JSONResponse(ReadingProgressRequiredEnvelope{
		Data: ReadingProgress{
			Scripture: ReadingProgressScripture(rp.Scripture),
			VerseKey:  rp.VerseKey,
			UpdatedAt: rp.UpdatedAt.UTC(),
		},
	}), nil
}

func (s *Server) GetMeStreak(ctx context.Context, request GetMeStreakRequestObject) (GetMeStreakResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return GetMeStreak500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	scripture := "quran"
	if request.Params.Scripture != nil {
		scripture = string(*request.Params.Scripture)
	}
	if !validScripture(scripture) {
		return GetMeStreak400JSONResponse{BadRequestJSONResponse{Message: "scripture must be 'quran' or 'bible'"}}, nil
	}

	streak, err := s.DB.GetStreak(ctx, authID, scripture)
	if err != nil {
		slog.Error("GetMeStreak: DB error", "error", err)
		return GetMeStreak500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	var last *string
	if streak.LastActiveDay != "" {
		last = &streak.LastActiveDay
	}
	return GetMeStreak200JSONResponse(StreakEnvelope{
		Data: Streak{
			CurrentStreak:   streak.CurrentStreak,
			LongestStreak:   streak.LongestStreak,
			LastActiveDay:   last,
			TotalVersesRead: streak.TotalVersesRead,
		},
	}), nil
}

func (s *Server) PostMeReadingLog(ctx context.Context, request PostMeReadingLogRequestObject) (PostMeReadingLogResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return PostMeReadingLog500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if err := s.ensureCurrentUser(ctx, authID); err != nil {
		slog.Error("PostMeReadingLog: ensureCurrentUser failed", "error", err)
		return PostMeReadingLog500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if request.Body == nil {
		return PostMeReadingLog400JSONResponse{BadRequestJSONResponse{Message: "request body is required"}}, nil
	}
	req := *request.Body

	scripture := string(req.Scripture)
	if !validScripture(scripture) {
		return PostMeReadingLog400JSONResponse{BadRequestJSONResponse{Message: "scripture must be 'quran' or 'bible'"}}, nil
	}
	versesRead := 1
	if req.VersesRead != nil && *req.VersesRead > 0 {
		versesRead = *req.VersesRead
	}

	day := time.Now().UTC().Format("2006-01-02")
	if req.Day != nil {
		day = req.Day.String()
	}
	if err := s.DB.IncrementReadingLog(ctx, authID, scripture, day, versesRead); err != nil {
		slog.Error("PostMeReadingLog: DB error", "error", err)
		return PostMeReadingLog500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	return PostMeReadingLog200JSONResponse(nullDataEnvelope()), nil
}

func (s *Server) GetMePreferences(ctx context.Context, request GetMePreferencesRequestObject) (GetMePreferencesResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return GetMePreferences500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	scripture := "quran"
	if request.Params.Scripture != nil {
		scripture = string(*request.Params.Scripture)
	}
	if !validScripture(scripture) {
		return GetMePreferences400JSONResponse{BadRequestJSONResponse{Message: "scripture must be 'quran' or 'bible'"}}, nil
	}

	row, err := s.DB.GetPreferences(ctx, authID, scripture)
	if err != nil {
		slog.Error("GetMePreferences: DB error", "error", err)
		return GetMePreferences500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if row == nil {
		return GetMePreferences200JSONResponse(PreferencesEnvelope{Data: nil}), nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(row.Payload, &payload); err != nil {
		slog.Error("GetMePreferences: invalid payload JSON", "error", err)
		return GetMePreferences500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	return GetMePreferences200JSONResponse(PreferencesEnvelope{Data: &payload}), nil
}

func (s *Server) PutMePreferences(ctx context.Context, request PutMePreferencesRequestObject) (PutMePreferencesResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return PutMePreferences500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if err := s.ensureCurrentUser(ctx, authID); err != nil {
		slog.Error("PutMePreferences: ensureCurrentUser failed", "error", err)
		return PutMePreferences500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if request.Body == nil {
		return PutMePreferences400JSONResponse{BadRequestJSONResponse{Message: "request body is required"}}, nil
	}
	req := *request.Body

	scripture := string(req.Scripture)
	if !validScripture(scripture) {
		return PutMePreferences400JSONResponse{BadRequestJSONResponse{Message: "scripture must be 'quran' or 'bible'"}}, nil
	}

	payload, err := json.Marshal(req.Payload)
	if err != nil {
		return PutMePreferences400JSONResponse{BadRequestJSONResponse{Message: "invalid payload JSON"}}, nil
	}
	_, err = s.DB.UpsertPreferences(ctx, authID, scripture, payload)
	if err != nil {
		slog.Error("PutMePreferences: DB error", "error", err)
		return PutMePreferences500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	return PutMePreferences200JSONResponse(nullDataEnvelope()), nil
}

func (s *Server) ListMeCollections(ctx context.Context, _ ListMeCollectionsRequestObject) (ListMeCollectionsResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return ListMeCollections500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	cols, err := s.DB.ListCollections(ctx, authID)
	if err != nil {
		slog.Error("ListMeCollections: DB error", "error", err)
		return ListMeCollections500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	result := make([]Collection, len(cols))
	for i := range cols {
		result[i] = collectionFromRow(&cols[i])
	}
	return ListMeCollections200JSONResponse(CollectionListEnvelope{Data: result}), nil
}

func (s *Server) CreateMeCollection(ctx context.Context, request CreateMeCollectionRequestObject) (CreateMeCollectionResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return CreateMeCollection500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if err := s.ensureCurrentUser(ctx, authID); err != nil {
		slog.Error("CreateMeCollection: ensureCurrentUser failed", "error", err)
		return CreateMeCollection500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if request.Body == nil {
		return CreateMeCollection400JSONResponse{BadRequestJSONResponse{Message: "request body is required"}}, nil
	}
	req := *request.Body

	isPublic := false
	if req.IsPublic != nil {
		isPublic = *req.IsPublic
	}
	description := ""
	if req.Description != nil {
		description = *req.Description
	}
	col, err := s.DB.CreateCollection(ctx, authID, req.Name, description, isPublic)
	if err != nil {
		slog.Error("CreateMeCollection: DB error", "error", err)
		return CreateMeCollection500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	return CreateMeCollection201JSONResponse(CollectionEnvelope{Data: collectionFromRow(col)}), nil
}

func (s *Server) UpdateMeCollection(ctx context.Context, request UpdateMeCollectionRequestObject) (UpdateMeCollectionResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return UpdateMeCollection500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if request.Body == nil {
		return UpdateMeCollection400JSONResponse{BadRequestJSONResponse{Message: "request body is required"}}, nil
	}
	req := *request.Body

	description := ""
	if req.Description != nil {
		description = *req.Description
	}
	isPublic := false
	if req.IsPublic != nil {
		isPublic = *req.IsPublic
	}
	regenerateToken := false
	if req.RegenerateToken != nil {
		regenerateToken = *req.RegenerateToken
	}
	col, err := s.DB.UpdateCollection(ctx, request.Id, authID, req.Name, description, isPublic, regenerateToken)
	if err != nil {
		slog.Error("UpdateMeCollection: DB error", "error", err)
		return UpdateMeCollection500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if col == nil {
		return UpdateMeCollection404JSONResponse{NotFoundJSONResponse{Message: "collection not found"}}, nil
	}
	return UpdateMeCollection200JSONResponse(CollectionEnvelope{Data: collectionFromRow(col)}), nil
}

func (s *Server) DeleteMeCollection(ctx context.Context, request DeleteMeCollectionRequestObject) (DeleteMeCollectionResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return DeleteMeCollection500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	deleted, err := s.DB.DeleteCollection(ctx, request.Id, authID)
	if err != nil {
		slog.Error("DeleteMeCollection: DB error", "error", err)
		return DeleteMeCollection500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !deleted {
		return DeleteMeCollection404JSONResponse{NotFoundJSONResponse{Message: "collection not found"}}, nil
	}
	return DeleteMeCollection200JSONResponse(nullDataEnvelope()), nil
}

func (s *Server) GetMeCollectionDetail(ctx context.Context, request GetMeCollectionDetailRequestObject) (GetMeCollectionDetailResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return GetMeCollectionDetail500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	cols, err := s.DB.ListCollections(ctx, authID)
	if err != nil {
		slog.Error("GetMeCollectionDetail: list error", "error", err)
		return GetMeCollectionDetail500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	var col *db.CollectionRow
	for i := range cols {
		if cols[i].ID == request.Id {
			col = &cols[i]
			break
		}
	}
	if col == nil {
		return GetMeCollectionDetail404JSONResponse{NotFoundJSONResponse{Message: "collection not found"}}, nil
	}

	verses, err := s.DB.ListCollectionVerses(ctx, request.Id)
	if err != nil {
		slog.Error("GetMeCollectionDetail: verses error", "error", err)
		return GetMeCollectionDetail500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	vr := make([]CollectionVerse, len(verses))
	for i := range verses {
		vr[i] = collectionVerseFromRow(&verses[i])
	}

	resp := CollectionDetail{
		Id:          col.ID,
		Name:        col.Name,
		Description: col.Description,
		IsPublic:    col.IsPublic,
		ShareToken:  col.ShareToken,
		CreatedAt:   col.CreatedAt.UTC(),
		UpdatedAt:   col.UpdatedAt.UTC(),
		Verses:      vr,
	}
	return GetMeCollectionDetail200JSONResponse(CollectionDetailEnvelope{Data: resp}), nil
}

func (s *Server) AddVerseToMeCollection(ctx context.Context, request AddVerseToMeCollectionRequestObject) (AddVerseToMeCollectionResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return AddVerseToMeCollection500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if request.Body == nil {
		return AddVerseToMeCollection400JSONResponse{BadRequestJSONResponse{Message: "request body is required"}}, nil
	}
	req := *request.Body
	scripture := string(req.Scripture)
	if !validScripture(scripture) {
		return AddVerseToMeCollection400JSONResponse{BadRequestJSONResponse{Message: "scripture must be 'quran' or 'bible'"}}, nil
	}

	note := ""
	if req.Note != nil {
		note = *req.Note
	}
	cv, err := s.DB.AddVerseToCollection(ctx, request.Id, authID, scripture, req.VerseKey, note)
	if err != nil {
		slog.Error("AddVerseToMeCollection: DB error", "error", err)
		return AddVerseToMeCollection500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	return AddVerseToMeCollection201JSONResponse(CollectionVerseEnvelope{Data: collectionVerseFromRow(cv)}), nil
}

func (s *Server) RemoveVerseFromMeCollection(ctx context.Context, request RemoveVerseFromMeCollectionRequestObject) (RemoveVerseFromMeCollectionResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return RemoveVerseFromMeCollection500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	deleted, err := s.DB.RemoveVerseFromCollection(ctx, request.VerseId, authID)
	if err != nil {
		slog.Error("RemoveVerseFromMeCollection: DB error", "error", err)
		return RemoveVerseFromMeCollection500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !deleted {
		return RemoveVerseFromMeCollection404JSONResponse{NotFoundJSONResponse{Message: "verse not found in collection"}}, nil
	}
	return RemoveVerseFromMeCollection200JSONResponse(nullDataEnvelope()), nil
}

func (s *Server) GetSharedCollection(ctx context.Context, request GetSharedCollectionRequestObject) (GetSharedCollectionResponseObject, error) {
	col, err := s.DB.GetCollectionByShareToken(ctx, request.Token)
	if err != nil {
		slog.Error("GetSharedCollection: DB error", "error", err)
		return GetSharedCollection500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if col == nil {
		return GetSharedCollection404JSONResponse{NotFoundJSONResponse{Message: "collection not found"}}, nil
	}

	verses, err := s.DB.ListCollectionVerses(ctx, col.ID)
	if err != nil {
		slog.Error("GetSharedCollection: verses DB error", "error", err)
		return GetSharedCollection500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	vr := make([]CollectionVerse, len(verses))
	for i := range verses {
		vr[i] = collectionVerseFromRow(&verses[i])
	}

	resp := CollectionDetail{
		Id:          col.ID,
		Name:        col.Name,
		Description: col.Description,
		IsPublic:    col.IsPublic,
		ShareToken:  col.ShareToken,
		CreatedAt:   col.CreatedAt.UTC(),
		UpdatedAt:   col.UpdatedAt.UTC(),
		Verses:      vr,
	}
	return GetSharedCollection200JSONResponse(CollectionDetailEnvelope{Data: resp}), nil
}
