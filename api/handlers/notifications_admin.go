package handlers

// Admin handlers for the notifications surface: zikr entries CRUD and
// scheduled-notification CRUD plus immediate dispatch. Plain Gin handlers (not
// the OpenAPI strict server) registered under /api/v1/admin/notifications and
// protected by auth.RequireAuth + auth.RequireAdmin. They back the
// /admin/notifications web page.

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/auth"
	"github.com/wikiSubmission/ws-backend/db"
	"github.com/wikiSubmission/ws-backend/notify"
)

var (
	scheduleDestinations = map[string]struct{}{"all": {}, "web_push": {}, "fcm": {}}
	scheduleFrequencies  = map[string]struct{}{"once": {}, "daily": {}, "weekly": {}, "biweekly": {}, "monthly": {}}
)

type NotificationsAdminHandler struct {
	DB         *db.DB
	Sender     WebPushSender      // caller-only test sends
	Dispatcher *notify.Dispatcher // full fan-out (send-now, broadcast test)
}

// ── Zikr CRUD ────────────────────────────────────────────────────────────────

// ListZikr returns every entry (active and inactive) for the admin table.
func (h NotificationsAdminHandler) ListZikr(c *gin.Context) {
	rows, err := h.DB.ListZikr(c.Request.Context(), false, c.Query("lang"))
	if err != nil {
		slog.Error("notifications admin: ListZikr", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	out := make([]zikrEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, zikrEntryFromRow(r))
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

type zikrCreateRequest struct {
	Text   string  `json:"text"`
	Lang   *string `json:"lang,omitempty"`
	Source *string `json:"source,omitempty"`
	Active *bool   `json:"active,omitempty"`
	Weight *int    `json:"weight,omitempty"`
}

func (h NotificationsAdminHandler) CreateZikr(c *gin.Context) {
	var req zikrCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "text is required"})
		return
	}
	lang := "en"
	if req.Lang != nil && *req.Lang != "" {
		lang = *req.Lang
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	weight := 0
	if req.Weight != nil {
		weight = *req.Weight
	}
	row, err := h.DB.InsertZikr(c.Request.Context(), req.Text, lang, req.Source, active, weight)
	if err != nil {
		slog.Error("notifications admin: InsertZikr", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": zikrEntryFromRow(*row)})
}

type zikrUpdateRequest struct {
	Text   *string `json:"text,omitempty"`
	Lang   *string `json:"lang,omitempty"`
	Source *string `json:"source,omitempty"`
	Active *bool   `json:"active,omitempty"`
	Weight *int    `json:"weight,omitempty"`
}

func (h NotificationsAdminHandler) UpdateZikr(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid id"})
		return
	}
	var req zikrUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}
	if req.Text != nil && *req.Text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "text cannot be empty"})
		return
	}
	row, err := h.DB.UpdateZikr(c.Request.Context(), id, req.Text, req.Lang, req.Source, req.Active, req.Weight)
	if err != nil {
		slog.Error("notifications admin: UpdateZikr", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	if row == nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "zikr entry not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": zikrEntryFromRow(*row)})
}

func (h NotificationsAdminHandler) DeleteZikr(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid id"})
		return
	}
	found, err := h.DB.DeleteZikr(c.Request.Context(), id)
	if err != nil {
		slog.Error("notifications admin: DeleteZikr", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"message": "zikr entry not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"ok": true}})
}

// ── Scheduled notifications ──────────────────────────────────────────────────

type scheduleJSON struct {
	ID             int        `json:"id"`
	Title          string     `json:"title"`
	Body           string     `json:"body"`
	URL            *string    `json:"url,omitempty"`
	Category       *string    `json:"category,omitempty"`
	Destination    string     `json:"destination"`
	CategoryFilter *string    `json:"category_filter,omitempty"`
	Frequency      string     `json:"frequency"`
	StartsAt       time.Time  `json:"starts_at"`
	Timezone       string     `json:"timezone"`
	Enabled        bool       `json:"enabled"`
	NextRunAt      time.Time  `json:"next_run_at"`
	LastRunAt      *time.Time `json:"last_run_at,omitempty"`
	LastStatus     *string    `json:"last_status,omitempty"`
	LastError      *string    `json:"last_error,omitempty"`
	LastWebSent    *int       `json:"last_web_sent,omitempty"`
	LastFCMSent    *int       `json:"last_fcm_sent,omitempty"`
	CreatedBy      *string    `json:"created_by,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func scheduleFromRow(r db.ScheduledNotificationRow) scheduleJSON {
	return scheduleJSON{
		ID: r.ID, Title: r.Title, Body: r.Body, URL: r.URL, Category: r.Category,
		Destination: r.Destination, CategoryFilter: r.CategoryFilter, Frequency: r.Frequency,
		StartsAt: r.StartsAt, Timezone: r.Timezone, Enabled: r.Enabled, NextRunAt: r.NextRunAt,
		LastRunAt: r.LastRunAt, LastStatus: r.LastStatus, LastError: r.LastError,
		LastWebSent: r.LastWebSent, LastFCMSent: r.LastFCMSent, CreatedBy: r.CreatedBy,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func (h NotificationsAdminHandler) ListSchedules(c *gin.Context) {
	rows, err := h.DB.ListScheduledNotifications(c.Request.Context())
	if err != nil {
		slog.Error("notifications admin: ListScheduledNotifications", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	out := make([]scheduleJSON, 0, len(rows))
	for _, r := range rows {
		out = append(out, scheduleFromRow(r))
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

type scheduleCreateRequest struct {
	Title          string  `json:"title"`
	Body           string  `json:"body"`
	URL            *string `json:"url,omitempty"`
	Category       *string `json:"category,omitempty"`
	Destination    string  `json:"destination"`
	CategoryFilter *string `json:"category_filter,omitempty"`
	Frequency      string  `json:"frequency"`
	StartsAt       string  `json:"starts_at"` // RFC3339
	Timezone       string  `json:"timezone"`  // IANA zone
	Enabled        *bool   `json:"enabled,omitempty"`
}

// computeNextRun derives the first dispatch time for a schedule: starts_at
// itself while it is still ahead, otherwise the next recurrence after now
// ('once' schedules in the past are rejected upstream).
func computeNextRun(frequency string, startsAt, now time.Time, loc *time.Location) time.Time {
	if startsAt.After(now) {
		return startsAt
	}
	return notify.NextRunAt(frequency, startsAt, now, loc)
}

// parseStartsAt accepts either an RFC3339 timestamp or a wall-clock timestamp
// without offset (the admin form's datetime-local value), which is interpreted
// in the schedule's timezone.
func parseStartsAt(value string, loc *time.Location) (time.Time, error) {
	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts, nil
	}
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04"} {
		if ts, err := time.ParseInLocation(layout, value, loc); err == nil {
			return ts, nil
		}
	}
	return time.Time{}, errInvalidStartsAt
}

var errInvalidStartsAt = errors.New("starts_at must be RFC3339 or YYYY-MM-DDTHH:MM")

func (h NotificationsAdminHandler) CreateSchedule(c *gin.Context) {
	var req scheduleCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}
	if req.Title == "" || req.Body == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "title and body are required"})
		return
	}
	if _, ok := scheduleDestinations[req.Destination]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"message": "destination must be one of all, web_push, fcm"})
		return
	}
	if _, ok := scheduleFrequencies[req.Frequency]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{"message": "frequency must be one of once, daily, weekly, biweekly, monthly"})
		return
	}
	if req.Timezone == "" {
		req.Timezone = "UTC"
	}
	loc, err := time.LoadLocation(req.Timezone)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid timezone"})
		return
	}
	startsAt, err := parseStartsAt(req.StartsAt, loc)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	now := time.Now()
	if req.Frequency == "once" && !startsAt.After(now) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "a one-time notification must start in the future"})
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	var createdBy *string
	if v, ok := c.Get(auth.ContextKeyEmail); ok {
		if email, _ := v.(string); email != "" {
			createdBy = &email
		}
	}

	row, err := h.DB.InsertScheduledNotification(c.Request.Context(), db.NewScheduledNotification{
		Title: req.Title, Body: req.Body, URL: req.URL, Category: req.Category,
		Destination: req.Destination, CategoryFilter: req.CategoryFilter,
		Frequency: req.Frequency, StartsAt: startsAt, Timezone: req.Timezone,
		Enabled:   enabled,
		NextRunAt: computeNextRun(req.Frequency, startsAt, now, loc),
		CreatedBy: createdBy,
	})
	if err != nil {
		slog.Error("notifications admin: InsertScheduledNotification", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": scheduleFromRow(*row)})
}

type scheduleUpdateRequest struct {
	Title          *string `json:"title,omitempty"`
	Body           *string `json:"body,omitempty"`
	URL            *string `json:"url,omitempty"`
	Category       *string `json:"category,omitempty"`
	Destination    *string `json:"destination,omitempty"`
	CategoryFilter *string `json:"category_filter,omitempty"`
	Frequency      *string `json:"frequency,omitempty"`
	StartsAt       *string `json:"starts_at,omitempty"` // RFC3339
	Timezone       *string `json:"timezone,omitempty"`
	Enabled        *bool   `json:"enabled,omitempty"`
}

func (h NotificationsAdminHandler) UpdateSchedule(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid id"})
		return
	}
	var req scheduleUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}
	if req.Title != nil && *req.Title == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "title cannot be empty"})
		return
	}
	if req.Body != nil && *req.Body == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "body cannot be empty"})
		return
	}
	if req.Destination != nil {
		if _, ok := scheduleDestinations[*req.Destination]; !ok {
			c.JSON(http.StatusBadRequest, gin.H{"message": "destination must be one of all, web_push, fcm"})
			return
		}
	}
	if req.Frequency != nil {
		if _, ok := scheduleFrequencies[*req.Frequency]; !ok {
			c.JSON(http.StatusBadRequest, gin.H{"message": "frequency must be one of once, daily, weekly, biweekly, monthly"})
			return
		}
	}
	if req.Timezone != nil {
		if _, err := time.LoadLocation(*req.Timezone); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "invalid timezone"})
			return
		}
	}
	existing, err := h.DB.GetScheduledNotification(c.Request.Context(), id)
	if err != nil {
		slog.Error("notifications admin: GetScheduledNotification", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "schedule not found"})
		return
	}

	// Merge the timezone first: a wall-clock starts_at in the request is
	// interpreted in the schedule's (possibly updated) timezone.
	timezone := existing.Timezone
	if req.Timezone != nil {
		timezone = *req.Timezone
	}
	loc, locErr := time.LoadLocation(timezone)
	if locErr != nil {
		loc = time.UTC
	}

	var startsAt *time.Time
	if req.StartsAt != nil {
		parsed, parseErr := parseStartsAt(*req.StartsAt, loc)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": parseErr.Error()})
			return
		}
		startsAt = &parsed
	}

	// Recompute next_run_at whenever a timing input changes or the schedule is
	// re-enabled, using the merged (request-over-existing) timing values.
	var nextRunAt *time.Time
	timingChanged := req.Frequency != nil || startsAt != nil || req.Timezone != nil
	reEnabled := req.Enabled != nil && *req.Enabled && !existing.Enabled
	if timingChanged || reEnabled {
		frequency := existing.Frequency
		if req.Frequency != nil {
			frequency = *req.Frequency
		}
		mergedStartsAt := existing.StartsAt
		if startsAt != nil {
			mergedStartsAt = *startsAt
		}
		now := time.Now()
		if frequency == "once" && !mergedStartsAt.After(now) {
			c.JSON(http.StatusBadRequest, gin.H{"message": "a one-time notification must start in the future"})
			return
		}
		next := computeNextRun(frequency, mergedStartsAt, now, loc)
		nextRunAt = &next
	}

	row, err := h.DB.UpdateScheduledNotification(c.Request.Context(), id,
		req.Title, req.Body, req.URL, req.Category, req.Destination, req.CategoryFilter, req.Frequency,
		startsAt, req.Timezone, req.Enabled, nextRunAt)
	if err != nil {
		slog.Error("notifications admin: UpdateScheduledNotification", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	if row == nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "schedule not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": scheduleFromRow(*row)})
}

func (h NotificationsAdminHandler) DeleteSchedule(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid id"})
		return
	}
	found, err := h.DB.DeleteScheduledNotification(c.Request.Context(), id)
	if err != nil {
		slog.Error("notifications admin: DeleteScheduledNotification", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	if !found {
		c.JSON(http.StatusNotFound, gin.H{"message": "schedule not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"ok": true}})
}

// SendNow dispatches a schedule immediately without touching next_run_at, so
// an admin can push an occurrence out of band. The result is recorded on the
// row like a scheduled run.
func (h NotificationsAdminHandler) SendNow(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid id"})
		return
	}
	row, err := h.DB.GetScheduledNotification(c.Request.Context(), id)
	if err != nil {
		slog.Error("notifications admin: GetScheduledNotification", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}
	if row == nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "schedule not found"})
		return
	}

	webSent, fcmSent, dispatchErr := h.Dispatcher.Dispatch(c.Request.Context(), *row)
	status := "ok"
	var errText *string
	if dispatchErr != nil {
		msg := dispatchErr.Error()
		errText = &msg
		if webSent > 0 || fcmSent > 0 {
			status = "partial"
		} else {
			status = "error"
		}
	}
	if recordErr := h.DB.RecordScheduleResult(c.Request.Context(), id, status, errText, webSent, fcmSent); recordErr != nil {
		slog.Error("notifications admin: RecordScheduleResult", "error", recordErr)
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"web_sent": webSent, "fcm_sent": fcmSent, "status": status}})
}

type testSendRequest struct {
	Title          string  `json:"title"`
	Body           string  `json:"body"`
	URL            *string `json:"url,omitempty"`
	Destination    *string `json:"destination,omitempty"` // used with broadcast
	CategoryFilter *string `json:"category_filter,omitempty"`
	Broadcast      bool    `json:"broadcast,omitempty"`
}

// SendTest delivers an ad-hoc notification. By default it goes only to the
// calling admin's own web-push endpoints (a safe preview); broadcast=true
// dispatches to the real destination.
func (h NotificationsAdminHandler) SendTest(c *gin.Context) {
	var req testSendRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Title == "" || req.Body == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "title and body are required"})
		return
	}

	if req.Broadcast {
		destination := "all"
		if req.Destination != nil {
			if _, ok := scheduleDestinations[*req.Destination]; !ok {
				c.JSON(http.StatusBadRequest, gin.H{"message": "destination must be one of all, web_push, fcm"})
				return
			}
			destination = *req.Destination
		}
		webSent, fcmSent, dispatchErr := h.Dispatcher.Dispatch(c.Request.Context(), db.ScheduledNotificationRow{
			Title: req.Title, Body: req.Body, URL: req.URL,
			Destination: destination, CategoryFilter: req.CategoryFilter,
		})
		if dispatchErr != nil {
			slog.Warn("notifications admin: broadcast test dispatch", "error", dispatchErr)
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"web_sent": webSent, "fcm_sent": fcmSent, "broadcast": true}})
		return
	}

	authID, ok := authIDFromGin(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "not authenticated"})
		return
	}
	if !h.Sender.Configured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "push is not configured"})
		return
	}
	subs, err := h.DB.FetchPushSubscriptionsByAuthID(c.Request.Context(), authID)
	if err != nil {
		slog.Error("notifications admin: FetchPushSubscriptionsByAuthID", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
		return
	}

	msg := PushMessage{Title: req.Title, Body: req.Body, Tag: "admin-test"}
	if req.URL != nil {
		msg.URL = *req.URL
	}
	var sent int
	for _, sub := range subs {
		gone, sendErr := h.Sender.Send(c.Request.Context(), sub, msg)
		if gone {
			_ = h.DB.DeletePushSubscriptionByEndpoint(c.Request.Context(), sub.Endpoint)
			continue
		}
		if sendErr != nil {
			slog.Warn("notifications admin: test send failed", "error", sendErr)
			continue
		}
		sent++
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"web_sent": sent, "broadcast": false}})
}
