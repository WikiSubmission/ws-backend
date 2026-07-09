package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/auth"
	"github.com/wikiSubmission/ws-backend/db"
)

func internalServerResponse() InternalServerErrrorJSONResponse {
	return InternalServerErrrorJSONResponse{Message: "internal server error"}
}

func (s *Server) ensureUserForActivity(ctx context.Context, authID string) error {
	user, err := s.DB.FetchUserByAuthID(ctx, authID)
	if err != nil || user != nil {
		if err != nil {
			slog.Error("ensureUserForActivity: FetchUserByAuthID", "provider", auth.ProviderFromAuthID(authID), "error", err)
		}
		return err
	}
	email, ok := emailFromContext(ctx)
	if !ok {
		slog.Warn("ensureUserForActivity: cannot provision user without email claim",
			"provider", auth.ProviderFromAuthID(authID),
			"has_name", contextHasName(ctx),
		)
		return fmt.Errorf("email claim required")
	}
	var displayName *string
	if name, ok := nameFromContext(ctx); ok {
		n := name
		displayName = &n
	}
	slog.Info("ensureUserForActivity: provisioning user", "provider", auth.ProviderFromAuthID(authID), "has_display_name", displayName != nil)
	_, err = s.DB.UpsertUser(ctx, authID, email, displayName)
	if err != nil {
		slog.Error("ensureUserForActivity: UpsertUser", "provider", auth.ProviderFromAuthID(authID), "error", err)
	}
	return err
}

func activityFromRow(row db.ActivityRow) ActivityEntry {
	return ActivityEntry{
		Id:        row.ID,
		Kind:      ActivityKind(row.Kind),
		Scripture: ActivityEntryScripture(row.Scripture),
		VerseKey:  row.VerseKey,
		Query:     row.Query,
		CreatedAt: row.CreatedAt.UTC(),
	}
}

func exportFromRow(row *db.UserDataExportRow) *UserDataExport {
	if row == nil {
		return nil
	}
	return &UserDataExport{
		Id:          row.ID,
		UserId:      int64(row.UserID),
		Status:      UserDataExportStatus(row.Status),
		ObjectKey:   row.ObjectKey,
		DownloadUrl: row.DownloadURL,
		Error:       row.Error,
		RequestedAt: row.RequestedAt.UTC(),
		SentAt:      row.SentAt,
	}
}

func deletionFromRow(row *db.UserDataDeletionRow) *UserDataDeletion {
	if row == nil {
		return nil
	}
	categories := make([]DeleteContentCategory, len(row.Categories))
	for i, category := range row.Categories {
		categories[i] = DeleteContentCategory(category)
	}
	var counts *map[string]int64
	if row.Counts != nil {
		parsed := map[string]int64{}
		if err := json.Unmarshal(*row.Counts, &parsed); err == nil {
			counts = &parsed
		}
	}
	return &UserDataDeletion{
		Id:          row.ID,
		UserId:      int64(row.UserID),
		Status:      UserDataDeletionStatus(row.Status),
		Categories:  categories,
		Counts:      counts,
		Error:       row.Error,
		RequestedAt: row.RequestedAt.UTC(),
		CompletedAt: row.CompletedAt,
	}
}

func (s *Server) resolveCurrentUser(ctx context.Context) (*db.UserRow, string, string, bool) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return nil, "", "", false
	}
	email, ok := emailFromContext(ctx)
	if !ok {
		slog.Warn("resolveCurrentUser: missing email claim", "provider", auth.ProviderFromAuthID(authID), "has_name", contextHasName(ctx))
		return nil, "", "", false
	}
	var displayName *string
	if name, ok := nameFromContext(ctx); ok {
		n := name
		displayName = &n
	}
	if _, err := s.DB.UpsertUser(ctx, authID, email, displayName); err != nil {
		slog.Error("resolveCurrentUser: UpsertUser", "provider", auth.ProviderFromAuthID(authID), "error", err)
		return nil, "", "", false
	}
	user, err := s.DB.FetchUserByAuthID(ctx, authID)
	if err != nil || user == nil {
		slog.Error("resolveCurrentUser: FetchUserByAuthID", "provider", auth.ProviderFromAuthID(authID), "error", err)
		return nil, "", "", false
	}
	return user, authID, email, true
}

func (s *Server) ListMeActivity(ctx context.Context, request ListMeActivityRequestObject) (ListMeActivityResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return ListMeActivity500JSONResponse{internalServerResponse()}, nil
	}
	limit := 0
	offset := 0
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}
	rows, err := s.DB.ListActivity(ctx, authID, limit, offset)
	if err != nil {
		slog.Error("ListMeActivity: ListActivity", "error", err)
		return ListMeActivity500JSONResponse{internalServerResponse()}, nil
	}
	data := make([]ActivityEntry, len(rows))
	for i := range rows {
		data[i] = activityFromRow(rows[i])
	}
	return ListMeActivity200JSONResponse(ActivityListEnvelope{Data: data}), nil
}

func (s *Server) RecordMeActivity(ctx context.Context, request RecordMeActivityRequestObject) (RecordMeActivityResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return RecordMeActivity500JSONResponse{internalServerResponse()}, nil
	}
	if request.Body == nil {
		return RecordMeActivity400JSONResponse{BadRequestJSONResponse{Message: "request body is required"}}, nil
	}
	req := *request.Body
	if req.Query != nil {
		q := strings.TrimSpace(*req.Query)
		if q == "" {
			req.Query = nil
		} else {
			if len(q) > 500 {
				q = q[:500]
			}
			req.Query = &q
		}
	}
	if req.VerseKey != nil {
		k := strings.TrimSpace(*req.VerseKey)
		if k == "" {
			req.VerseKey = nil
		} else {
			req.VerseKey = &k
		}
	}
	if err := s.ensureUserForActivity(ctx, authID); err != nil {
		slog.Error("RecordMeActivity: ensure user", "error", err)
		return RecordMeActivity500JSONResponse{internalServerResponse()}, nil
	}
	stored, err := s.DB.InsertActivity(ctx, authID, string(req.Kind), string(req.Scripture), req.VerseKey, req.Query)
	if err != nil {
		slog.Error("RecordMeActivity: InsertActivity", "error", err)
		return RecordMeActivity500JSONResponse{internalServerResponse()}, nil
	}
	if !stored {
		return RecordMeActivity202JSONResponse(ActivityStoredResponse{Stored: false}), nil
	}
	return RecordMeActivity200JSONResponse(ActivityStoredResponse{Stored: true}), nil
}

func (s *Server) ClearMeActivity(ctx context.Context, _ ClearMeActivityRequestObject) (ClearMeActivityResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return ClearMeActivity500JSONResponse{internalServerResponse()}, nil
	}
	deleted, err := s.DB.ClearActivity(ctx, authID)
	if err != nil {
		slog.Error("ClearMeActivity: ClearActivity", "error", err)
		return ClearMeActivity500JSONResponse{internalServerResponse()}, nil
	}
	return ClearMeActivity200JSONResponse(DeletedCountResponse{Deleted: deleted}), nil
}

func (s *Server) GetMeActivityConsent(ctx context.Context, _ GetMeActivityConsentRequestObject) (GetMeActivityConsentResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return GetMeActivityConsent500JSONResponse{internalServerResponse()}, nil
	}
	consent, err := s.DB.GetActivityConsent(ctx, authID)
	if err != nil {
		slog.Error("GetMeActivityConsent: GetActivityConsent", "error", err)
		return GetMeActivityConsent500JSONResponse{internalServerResponse()}, nil
	}
	return GetMeActivityConsent200JSONResponse(ActivityConsentResponse{Consent: consent}), nil
}

func (s *Server) SetMeActivityConsent(ctx context.Context, request SetMeActivityConsentRequestObject) (SetMeActivityConsentResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return SetMeActivityConsent500JSONResponse{internalServerResponse()}, nil
	}
	if request.Body == nil {
		return SetMeActivityConsent400JSONResponse{BadRequestJSONResponse{Message: "request body is required"}}, nil
	}
	if err := s.ensureUserForActivity(ctx, authID); err != nil {
		slog.Error("SetMeActivityConsent: ensure user", "error", err)
		return SetMeActivityConsent500JSONResponse{internalServerResponse()}, nil
	}
	consent, err := s.DB.SetActivityConsent(ctx, authID, request.Body.Consent)
	if err != nil {
		slog.Error("SetMeActivityConsent: SetActivityConsent", "error", err)
		return SetMeActivityConsent500JSONResponse{internalServerResponse()}, nil
	}
	return SetMeActivityConsent200JSONResponse(ActivityConsentResponse{Consent: consent}), nil
}

func (s *Server) RequestMeExport(ctx context.Context, _ RequestMeExportRequestObject) (RequestMeExportResponseObject, error) {
	user, authID, email, ok := s.resolveCurrentUser(ctx)
	if !ok {
		return RequestMeExport500JSONResponse{internalServerResponse()}, nil
	}
	now := time.Now().UTC()
	blocker, err := s.DB.FindExportBlocker(ctx, user.ID, now)
	if err != nil {
		return RequestMeExport500JSONResponse{internalServerResponse()}, nil
	}
	if blocker != nil {
		return RequestMeExport429JSONResponse(RetryAfterResponse{RetryAfterSeconds: computeRetryAfterSeconds(blocker, now)}), nil
	}
	row, err := s.DB.CreateQueuedUserDataExport(ctx, user.ID)
	if err != nil {
		return RequestMeExport500JSONResponse{internalServerResponse()}, nil
	}
	uploader := s.ExportUploader
	if uploader == nil {
		uploader = NoopExportUploader{}
	}
	handler := MeExportHandler{DB: s.DB, Uploader: uploader}
	go handler.runExport(row.ID, user.ID, authID, email)
	return RequestMeExport202JSONResponse(UserDataExportQueuedResponse{Status: UserDataExportQueuedResponseStatusQueued, EstimatedMinutes: 15}), nil
}

func (s *Server) GetMeExportStatus(ctx context.Context, _ GetMeExportStatusRequestObject) (GetMeExportStatusResponseObject, error) {
	user, _, _, ok := s.resolveCurrentUser(ctx)
	if !ok {
		return GetMeExportStatus500JSONResponse{internalServerResponse()}, nil
	}
	row, err := s.DB.LatestUserDataExport(ctx, user.ID)
	if err != nil {
		return GetMeExportStatus500JSONResponse{internalServerResponse()}, nil
	}
	return GetMeExportStatus200JSONResponse(UserDataExportEnvelope{Data: exportFromRow(row)}), nil
}

func (s *Server) RequestMeContentDeletion(ctx context.Context, request RequestMeContentDeletionRequestObject) (RequestMeContentDeletionResponseObject, error) {
	user, authID, _, ok := s.resolveCurrentUser(ctx)
	if !ok {
		return RequestMeContentDeletion500JSONResponse{internalServerResponse()}, nil
	}
	if request.Body == nil || len(request.Body.Categories) == 0 {
		return RequestMeContentDeletion400JSONResponse{BadRequestJSONResponse{Message: "no categories selected"}}, nil
	}
	categories := make([]string, len(request.Body.Categories))
	seen := make(map[string]bool, len(request.Body.Categories))
	for i, category := range request.Body.Categories {
		name := string(category)
		if !db.IsValidDeletionCategory(name) {
			return RequestMeContentDeletion400JSONResponse{BadRequestJSONResponse{Message: fmt.Sprintf("unknown category: %s", name)}}, nil
		}
		if seen[name] {
			return RequestMeContentDeletion400JSONResponse{BadRequestJSONResponse{Message: fmt.Sprintf("duplicate category: %s", name)}}, nil
		}
		seen[name] = true
		categories[i] = name
	}
	latestExport, err := s.DB.LatestUserDataExport(ctx, user.ID)
	if err != nil {
		return RequestMeContentDeletion500JSONResponse{internalServerResponse()}, nil
	}
	if !exportGateSatisfied(latestExport, time.Now().UTC()) {
		return RequestMeContentDeletion409JSONResponse(ReasonResponse{Reason: "export_required"}), nil
	}
	blocker, err := s.DB.FindDeletionBlocker(ctx, user.ID)
	if err != nil {
		return RequestMeContentDeletion500JSONResponse{internalServerResponse()}, nil
	}
	if blocker != nil {
		return RequestMeContentDeletion429JSONResponse(ReasonResponse{Reason: "deletion_in_progress"}), nil
	}
	row, err := s.DB.CreateQueuedDeletion(ctx, user.ID, categories)
	if err != nil {
		return RequestMeContentDeletion500JSONResponse{internalServerResponse()}, nil
	}
	handler := MeDeleteHandler{DB: s.DB}
	go handler.runDeletion(row.ID, authID, categories)
	return RequestMeContentDeletion202JSONResponse(UserDataDeletionQueuedResponse{Status: UserDataDeletionQueuedResponseStatusQueued}), nil
}

func (s *Server) GetMeContentDeletionStatus(ctx context.Context, _ GetMeContentDeletionStatusRequestObject) (GetMeContentDeletionStatusResponseObject, error) {
	user, _, _, ok := s.resolveCurrentUser(ctx)
	if !ok {
		return GetMeContentDeletionStatus500JSONResponse{internalServerResponse()}, nil
	}
	row, err := s.DB.LatestDeletion(ctx, user.ID)
	if err != nil {
		return GetMeContentDeletionStatus500JSONResponse{internalServerResponse()}, nil
	}
	return GetMeContentDeletionStatus200JSONResponse(UserDataDeletionEnvelope{Data: deletionFromRow(row)}), nil
}

func (s *Server) DeleteMeAccount(ctx context.Context, _ DeleteMeAccountRequestObject) (DeleteMeAccountResponseObject, error) {
	user, authID, _, ok := s.resolveCurrentUser(ctx)
	if !ok {
		return DeleteMeAccount500JSONResponse{internalServerResponse()}, nil
	}

	// Export gate: the user must have a recent, completed export so a usable copy
	// of their data exists before this irreversible deletion. Enforced here, not
	// just in the UI, so it cannot be bypassed.
	latestExport, err := s.DB.LatestUserDataExport(ctx, user.ID)
	if err != nil {
		return DeleteMeAccount500JSONResponse{internalServerResponse()}, nil
	}
	if !exportGateSatisfied(latestExport, time.Now().UTC()) {
		return DeleteMeAccount409JSONResponse(ReasonResponse{Reason: "export_required"}), nil
	}

	// The export S3 object is left to expire on its own (30-day TTL); ws-lib
	// exposes no delete endpoint, and the object is orphaned once its
	// user_data_exports row is removed by the purge below.
	counts, err := s.DB.PurgeUserAccount(ctx, authID)
	if err != nil {
		slog.Error("DeleteMeAccount: PurgeUserAccount", "provider", auth.ProviderFromAuthID(authID), "error", err)
		return DeleteMeAccount500JSONResponse{internalServerResponse()}, nil
	}
	slog.Info("DeleteMeAccount: account purged", "user_id", user.ID, "counts", counts)

	return DeleteMeAccount200JSONResponse(AccountDeletionResponse{
		Status: Deleted,
		Counts: &counts,
	}), nil
}

func (s *Server) SubscribeMePush(ctx context.Context, request SubscribeMePushRequestObject) (SubscribeMePushResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return SubscribeMePush500JSONResponse{internalServerResponse()}, nil
	}
	if request.Body == nil {
		return SubscribeMePush400JSONResponse{BadRequestJSONResponse{Message: "request body is required"}}, nil
	}
	req := request.Body
	if !strings.HasPrefix(req.Endpoint, "https://") || req.Keys.P256dh == "" || req.Keys.Auth == "" {
		return SubscribeMePush400JSONResponse{BadRequestJSONResponse{Message: "endpoint and keys are required"}}, nil
	}
	categories := []string(nil)
	if req.Categories != nil {
		categories = *req.Categories
	}
	var userAgent *string
	if r, ok := requestFromContext(ctx); ok {
		if ua := r.UserAgent(); ua != "" {
			userAgent = &ua
		}
	}
	row, err := s.DB.UpsertPushSubscription(ctx, authID, req.Endpoint, req.Keys.P256dh, req.Keys.Auth, categories, req.Locale, req.Timezone, userAgent)
	if err != nil {
		slog.Error("SubscribeMePush: UpsertPushSubscription", "error", err)
		return SubscribeMePush500JSONResponse{internalServerResponse()}, nil
	}
	return SubscribeMePush200JSONResponse(PushSubscribeResponse{Id: row.ID, Categories: row.Categories}), nil
}

func requestFromContext(ctx context.Context) (*http.Request, bool) {
	if gc, ok := ctx.(*gin.Context); ok && gc.Request != nil {
		return gc.Request, true
	}
	return nil, false
}

func (s *Server) UnsubscribeMePush(ctx context.Context, request UnsubscribeMePushRequestObject) (UnsubscribeMePushResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return UnsubscribeMePush500JSONResponse{internalServerResponse()}, nil
	}
	if request.Body == nil || request.Body.Endpoint == "" {
		return UnsubscribeMePush400JSONResponse{BadRequestJSONResponse{Message: "endpoint is required"}}, nil
	}
	if _, err := s.DB.DeletePushSubscription(ctx, authID, request.Body.Endpoint); err != nil {
		slog.Error("UnsubscribeMePush: DeletePushSubscription", "error", err)
		return UnsubscribeMePush500JSONResponse{internalServerResponse()}, nil
	}
	return UnsubscribeMePush200JSONResponse(OkResponse{Ok: true}), nil
}

func (s *Server) SendMePushTest(ctx context.Context, _ SendMePushTestRequestObject) (SendMePushTestResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return SendMePushTest500JSONResponse{internalServerResponse()}, nil
	}
	if !s.PushSender.Configured() {
		return SendMePushTest503JSONResponse{Message: "push is not configured"}, nil
	}
	subs, err := s.DB.FetchPushSubscriptionsByAuthID(ctx, authID)
	if err != nil {
		slog.Error("SendMePushTest: FetchPushSubscriptionsByAuthID", "error", err)
		return SendMePushTest500JSONResponse{internalServerResponse()}, nil
	}
	msg := PushMessage{Title: "WikiSubmission", Body: "Notifications are enabled.", URL: "/", Tag: "push-test"}
	var sent int
	for _, sub := range subs {
		gone, sendErr := s.PushSender.Send(ctx, sub, msg)
		if gone {
			_ = s.DB.DeletePushSubscriptionByEndpoint(ctx, sub.Endpoint)
			continue
		}
		if sendErr != nil {
			slog.Warn("SendMePushTest: send failed", "error", sendErr)
			continue
		}
		sent++
	}
	return SendMePushTest200JSONResponse(PushTestResponse{Sent: sent}), nil
}
