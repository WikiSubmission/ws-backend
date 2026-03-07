package handlers

import (
	"context"

	"github.com/wikiSubmission/ws-backend/db"
)

type Server struct {
	DB *db.DB
}

// 1. List all verse drafts
func (s *Server) ListVerseDrafts(ctx context.Context, request ListVerseDraftsRequestObject) (ListVerseDraftsResponseObject, error) {
	return ListVerseDrafts501JSONResponse{
		NotImplementedErrorJSONResponse{
			Message: "ListVerseDrafts is not implemented yet",
			Code:    "NOT_IMPLEMENTED"},
	}, nil
}

// 2. Create a new translation draft
func (s *Server) CreateVerseDraft(ctx context.Context, request CreateVerseDraftRequestObject) (CreateVerseDraftResponseObject, error) {
	return CreateVerseDraft501JSONResponse{
		NotImplementedErrorJSONResponse{
			Message: "CreateVerseDraft is not implemented yet",
			Code:    "NOT_IMPLEMENTED"},
	}, nil
}

// 3. Discard a draft
func (s *Server) DeleteVerseDraft(ctx context.Context, request DeleteVerseDraftRequestObject) (DeleteVerseDraftResponseObject, error) {
	return DeleteVerseDraft501JSONResponse{
		NotImplementedErrorJSONResponse{
			Message: "DeleteVerseDraft is not implemented yet",
			Code:    "NOT_IMPLEMENTED"},
	}, nil
}

// 4. Get a specific draft
func (s *Server) GetVerseDraft(ctx context.Context, request GetVerseDraftRequestObject) (GetVerseDraftResponseObject, error) {
	return GetVerseDraft501JSONResponse{
		NotImplementedErrorJSONResponse{
			Message: "GetVerseDraft is not implemented yet",
			Code:    "NOT_IMPLEMENTED"},
	}, nil
}

// 5. Update a draft
func (s *Server) UpdateVerseDraft(ctx context.Context, request UpdateVerseDraftRequestObject) (UpdateVerseDraftResponseObject, error) {
	return UpdateVerseDraft501JSONResponse{
		NotImplementedErrorJSONResponse{
			Message: "UpdateVerseDraft is not implemented yet",
			Code:    "NOT_IMPLEMENTED"},
	}, nil
}

// 6. View audit logs
func (s *Server) GetVerseHistory(ctx context.Context, request GetVerseHistoryRequestObject) (GetVerseHistoryResponseObject, error) {
	return GetVerseHistory501JSONResponse{
		NotImplementedErrorJSONResponse{
			Message: "GetVerseHistory is not implemented yet",
			Code:    "NOT_IMPLEMENTED",
		},
	}, nil
}

// 7. Publish a draft
func (s *Server) PublishVerseText(ctx context.Context, request PublishVerseTextRequestObject) (PublishVerseTextResponseObject, error) {
	return PublishVerseText501JSONResponse{
		NotImplementedErrorJSONResponse{
			Message: "PublishVerseText is not implemented yet",
			Code:    "NOT_IMPLEMENTED",
		},
	}, nil
}

// 8. Fetch a range of verses
func (s *Server) GetQuran(ctx context.Context, request GetQuranRequestObject) (GetQuranResponseObject, error) {
	return GetQuran501JSONResponse{
		NotImplementedErrorJSONResponse{
			Message: "GetQuran is not implemented yet",
			Code:    "NOT_IMPLEMENTED",
		},
	}, nil
}
