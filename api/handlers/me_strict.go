package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/wikiSubmission/ws-backend/auth"
	"github.com/wikiSubmission/ws-backend/db"
)

func authIDFromContext(ctx context.Context) (string, bool) {
	v := ctx.Value(auth.ContextKeyUID)
	s, ok := v.(string)
	return s, ok
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

func noteFromRow(n *db.NoteRow) Note {
	return Note{
		Id:        n.ID,
		Scripture: NoteScripture(n.Scripture),
		VerseKey:  n.VerseKey,
		Lang:      n.Lang,
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

func (s *Server) GetMeBookmarks(ctx context.Context, request GetMeBookmarksRequestObject) (GetMeBookmarksResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return GetMeBookmarks500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	scripture := "quran"
	if request.Params.Scripture != nil {
		scripture = string(*request.Params.Scripture)
	}
	if !validScripture(scripture) {
		return GetMeBookmarks400JSONResponse{BadRequestJSONResponse{Message: "scripture must be 'quran' or 'bible'"}}, nil
	}

	bookmarks, err := s.DB.FetchBookmarks(ctx, authID, scripture)
	if err != nil {
		slog.Error("GetMeBookmarks: DB error", "error", err)
		return GetMeBookmarks500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	result := make([]Bookmark, len(bookmarks))
	for i := range bookmarks {
		result[i] = bookmarkFromRow(&bookmarks[i])
	}
	return GetMeBookmarks200JSONResponse(BookmarkListEnvelope{Data: result}), nil
}

func (s *Server) CreateMeBookmark(ctx context.Context, request CreateMeBookmarkRequestObject) (CreateMeBookmarkResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return CreateMeBookmark500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if request.Body == nil {
		return CreateMeBookmark400JSONResponse{BadRequestJSONResponse{Message: "request body is required"}}, nil
	}

	req := *request.Body
	scripture := string(req.Scripture)
	if !validScripture(scripture) {
		return CreateMeBookmark400JSONResponse{BadRequestJSONResponse{Message: "scripture must be 'quran' or 'bible'"}}, nil
	}
	color := "amber"
	if req.Color != nil && *req.Color != "" {
		color = *req.Color
	}
	name := ""
	if req.Name != nil {
		name = *req.Name
	}
	kind := "normal"
	if req.Kind != nil && *req.Kind != "" {
		kind = string(*req.Kind)
	}
	if kind != "normal" && kind != "cover_to_cover" {
		return CreateMeBookmark400JSONResponse{BadRequestJSONResponse{Message: "kind must be 'normal' or 'cover_to_cover'"}}, nil
	}

	b, err := s.DB.CreateBookmark(ctx, authID, scripture, req.VerseKey, name, color, kind)
	if err != nil {
		slog.Error("CreateMeBookmark: DB error", "error", err)
		return CreateMeBookmark500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	return CreateMeBookmark201JSONResponse(BookmarkEnvelope{Data: bookmarkFromRow(b)}), nil
}

func (s *Server) UpdateMeBookmark(ctx context.Context, request UpdateMeBookmarkRequestObject) (UpdateMeBookmarkResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return UpdateMeBookmark500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if request.Body == nil {
		return UpdateMeBookmark400JSONResponse{BadRequestJSONResponse{Message: "request body is required"}}, nil
	}

	req := *request.Body
	existing, err := s.DB.FetchBookmarks(ctx, authID, "quran")
	if err != nil {
		slog.Error("UpdateMeBookmark: fetch error", "error", err)
		return UpdateMeBookmark500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	existingBible, err := s.DB.FetchBookmarks(ctx, authID, "bible")
	if err != nil {
		slog.Error("UpdateMeBookmark: fetch bible error", "error", err)
		return UpdateMeBookmark500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	existing = append(existing, existingBible...)

	var currentName, currentColor string
	for i := range existing {
		if existing[i].ID == request.Id {
			currentName = existing[i].Name
			currentColor = existing[i].Color
			break
		}
	}
	if req.Name != nil {
		currentName = *req.Name
	}
	if req.Color != nil {
		currentColor = *req.Color
	}

	b, err := s.DB.UpdateBookmark(ctx, request.Id, authID, currentName, currentColor)
	if err != nil {
		slog.Error("UpdateMeBookmark: DB error", "error", err)
		return UpdateMeBookmark500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if b == nil {
		return UpdateMeBookmark404JSONResponse{NotFoundJSONResponse{Message: "bookmark not found"}}, nil
	}
	return UpdateMeBookmark200JSONResponse(BookmarkEnvelope{Data: bookmarkFromRow(b)}), nil
}

func (s *Server) DeleteMeBookmark(ctx context.Context, request DeleteMeBookmarkRequestObject) (DeleteMeBookmarkResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return DeleteMeBookmark500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	deleted, err := s.DB.DeleteBookmark(ctx, request.Id, authID)
	if err != nil {
		slog.Error("DeleteMeBookmark: DB error", "error", err)
		return DeleteMeBookmark500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !deleted {
		return DeleteMeBookmark404JSONResponse{NotFoundJSONResponse{Message: "bookmark not found"}}, nil
	}
	return DeleteMeBookmark200JSONResponse(nullDataEnvelope()), nil
}

func (s *Server) PutMeCoverToCover(ctx context.Context, request PutMeCoverToCoverRequestObject) (PutMeCoverToCoverResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
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

	bookmarks, err := s.DB.FetchChapterBookmarks(ctx, authID, scripture, request.Params.Chapter)
	if err != nil {
		slog.Error("GetMeScriptureState: bookmarks DB error", "error", err)
		return GetMeScriptureState500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	notes, err := s.DB.FetchChapterNotes(ctx, authID, scripture, request.Params.Chapter)
	if err != nil {
		slog.Error("GetMeScriptureState: notes DB error", "error", err)
		return GetMeScriptureState500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	bmMap := make(map[string]Bookmark, len(bookmarks))
	for i := range bookmarks {
		bm := bookmarkFromRow(&bookmarks[i])
		bmMap[bm.VerseKey] = bm
	}
	noteMap := make(map[string][]Note)
	for i := range notes {
		n := noteFromRow(&notes[i])
		noteMap[n.VerseKey] = append(noteMap[n.VerseKey], n)
	}

	return GetMeScriptureState200JSONResponse(ScriptureState{Bookmarks: bmMap, Notes: noteMap}), nil
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

	notes, err := s.DB.FetchNotes(ctx, authID, scripture, request.Params.VerseKey, request.Params.Lang)
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

func (s *Server) CreateMeNote(ctx context.Context, request CreateMeNoteRequestObject) (CreateMeNoteResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return CreateMeNote500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if request.Body == nil {
		return CreateMeNote400JSONResponse{BadRequestJSONResponse{Message: "request body is required"}}, nil
	}
	req := *request.Body

	scripture := string(req.Scripture)
	if !validScripture(scripture) {
		return CreateMeNote400JSONResponse{BadRequestJSONResponse{Message: "scripture must be 'quran' or 'bible'"}}, nil
	}
	lang := "en"
	if req.Lang != nil && *req.Lang != "" {
		lang = *req.Lang
	}

	n, err := s.DB.CreateNote(ctx, authID, scripture, req.VerseKey, lang, req.Content)
	if err != nil {
		slog.Error("CreateMeNote: DB error", "error", err)
		return CreateMeNote500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	return CreateMeNote201JSONResponse(NoteEnvelope{Data: noteFromRow(n)}), nil
}

func (s *Server) UpdateMeNote(ctx context.Context, request UpdateMeNoteRequestObject) (UpdateMeNoteResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return UpdateMeNote500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if request.Body == nil {
		return UpdateMeNote400JSONResponse{BadRequestJSONResponse{Message: "request body is required"}}, nil
	}

	n, err := s.DB.UpdateNote(ctx, request.Id, authID, request.Body.Content)
	if err != nil {
		slog.Error("UpdateMeNote: DB error", "error", err)
		return UpdateMeNote500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if n == nil {
		return UpdateMeNote404JSONResponse{NotFoundJSONResponse{Message: "note not found"}}, nil
	}

	return UpdateMeNote200JSONResponse(NoteEnvelope{Data: noteFromRow(n)}), nil
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
