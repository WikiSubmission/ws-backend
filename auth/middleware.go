package auth

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/api/observability"
	"github.com/wikiSubmission/ws-backend/db"
)

// PermissionGamesEditor is the JSONB key on users.permissions that grants
// access to the games editorial surface. Admins are always allowed; other
// users need this permission set to true.
const PermissionGamesEditor = "games_editor"

// HasPermission reports whether the user's permissions JSONB blob has the
// given key set to true.
func HasPermission(raw json.RawMessage, key string) bool {
	if len(raw) == 0 {
		return false
	}
	var perms map[string]any
	if err := json.Unmarshal(raw, &perms); err != nil {
		return false
	}
	v, ok := perms[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

// RequireEditor allows admins and users granted the games_editor permission
// via the admin access page. Must run after RequireAuth so the auth id is on
// the context. The env-based allowlist (GAMES_EDITOR_EMAILS) it replaced has
// been retired in favor of DB-backed access control.
func RequireEditor(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		authID, exists := c.Get(ContextKeyUID)
		authIDStr, _ := authID.(string)
		if !exists || authIDStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}
		user, err := database.FetchUserByAuthID(c.Request.Context(), authIDStr)
		if err != nil || user == nil || !user.IsActive {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "editor access required"})
			return
		}
		if user.Role != "admin" && !HasPermission(user.Permissions, PermissionGamesEditor) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "editor access required"})
			return
		}
		c.Set(ContextKeyRole, user.Role)
		c.Next()
	}
}

// HasGamesEditorAccess reports whether the given user passes the same gate
// as RequireEditor. Exposed so handlers serving the session shape can populate
// session.isEditor consistently with the middleware.
func HasGamesEditorAccess(u *db.UserRow) bool {
	if u == nil || !u.IsActive {
		return false
	}
	if u.Role == "admin" {
		return true
	}
	return HasPermission(u.Permissions, PermissionGamesEditor)
}

const (
	ContextKeyUID   = "user_id"
	ContextKeyEmail = "user_email"
	ContextKeyName  = "user_name"
	ContextKeyRole  = "user_role"
)

// RequireAuth validates the Bearer JWT and stores authID/email in the Gin context.
func RequireAuth(verifier *AuthJSVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			observability.AuthAttemptsTotal.WithLabelValues("missing_token").Inc()
			slog.Warn("auth request missing bearer token", "method", c.Request.Method, "path", c.FullPath())
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")
		authID, email, name, err := verifier.VerifyToken(token)
		if err != nil {
			observability.AuthAttemptsTotal.WithLabelValues("invalid_token").Inc()
			slog.Warn("auth request rejected invalid token", "method", c.Request.Method, "path", c.FullPath(), "error", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		observability.AuthAttemptsTotal.WithLabelValues("success").Inc()
		if authID == "" || email == "" || name == "" {
			slog.Warn("auth request accepted with incomplete claims",
				"method", c.Request.Method,
				"path", c.FullPath(),
				"provider", ProviderFromAuthID(authID),
				"has_subject", authID != "",
				"has_email", email != "",
				"has_name", name != "",
			)
		}
		reqCtx := context.WithValue(c.Request.Context(), ContextKeyUID, authID)
		reqCtx = context.WithValue(reqCtx, ContextKeyEmail, email)
		reqCtx = context.WithValue(reqCtx, ContextKeyName, name)
		c.Request = c.Request.WithContext(reqCtx)
		c.Set(ContextKeyUID, authID)
		c.Set(ContextKeyEmail, email)
		c.Set(ContextKeyName, name)
		c.Next()
	}
}

// RequireAdmin additionally checks that the authenticated user has role=admin in the DB.
func RequireAdmin(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		authID, exists := c.Get(ContextKeyUID)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}
		user, err := database.FetchUserByAuthID(c.Request.Context(), authID.(string))
		if err != nil || user == nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "user not found"})
			return
		}
		if user.Role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			return
		}
		c.Set(ContextKeyRole, user.Role)
		c.Next()
	}
}
