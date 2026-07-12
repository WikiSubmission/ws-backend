package handlers

// Anonymous client crash/error ingestion for the web and mobile frontends
// (POST /client-errors). Reports are structured-logged for operational
// triage — no database table, no PII beyond what the client chooses to send.
// Outside the OpenAPI surface (registered before the validator), input sizes
// bounded and per-IP rate limited like the other anonymous write endpoints.

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	ceMaxBatch      = 10
	ceMaxMessageLen = 2000
	ceMaxStackLen   = 8000
	ceMaxFieldLen   = 256
)

var ceSources = map[string]struct{}{
	"error-boundary":     {},
	"global-error":       {},
	"window-error":       {},
	"unhandledrejection": {},
}

type ClientErrorHandler struct {
	limiter *keyedLimiter // per-IP fixed window
}

func NewClientErrorHandler() ClientErrorHandler {
	return ClientErrorHandler{limiter: newKeyedLimiter(20, time.Minute)}
}

type clientErrorReport struct {
	Message    string  `json:"message"`
	Stack      *string `json:"stack,omitempty"`
	Source     string  `json:"source"`
	Route      *string `json:"route,omitempty"`
	Platform   *string `json:"platform,omitempty"`
	AppVersion *string `json:"app_version,omitempty"`
	OccurredAt *int64  `json:"occurred_at,omitempty"` // epoch ms, client clock
}

type clientErrorRequest struct {
	Errors []clientErrorReport `json:"errors"`
}

func ceTruncate(s string, max int) string {
	if len(s) > max {
		return s[:max]
	}
	return s
}

func cePtr(s *string, max int) string {
	if s == nil {
		return ""
	}
	return ceTruncate(*s, max)
}

// Report logs a batch of client-side errors.
func (h ClientErrorHandler) Report(c *gin.Context) {
	if !h.limiter.Allow(c.ClientIP()) {
		c.JSON(http.StatusTooManyRequests, gin.H{"message": "rate limit exceeded — please slow down"})
		return
	}

	var req clientErrorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid request body"})
		return
	}
	if len(req.Errors) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "errors must not be empty"})
		return
	}
	if len(req.Errors) > ceMaxBatch {
		req.Errors = req.Errors[:ceMaxBatch]
	}

	for _, e := range req.Errors {
		if e.Message == "" {
			continue
		}
		source := e.Source
		if _, ok := ceSources[source]; !ok {
			source = "unknown"
		}
		slog.Error("client error",
			"message", ceTruncate(e.Message, ceMaxMessageLen),
			"stack", cePtr(e.Stack, ceMaxStackLen),
			"source", source,
			"route", cePtr(e.Route, ceMaxFieldLen),
			"platform", cePtr(e.Platform, ceMaxFieldLen),
			"app_version", cePtr(e.AppVersion, ceMaxFieldLen),
			"user_agent", ceTruncate(c.Request.UserAgent(), ceMaxFieldLen),
		)
	}

	c.Status(http.StatusNoContent)
}
