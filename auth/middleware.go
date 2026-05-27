package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/api/observability"
	"github.com/wikiSubmission/ws-backend/db"
)

// ParseEmailAllowlist turns a comma-separated env value into a lookup set of
// lowercased, trimmed emails. Empty entries are ignored.
func ParseEmailAllowlist(raw string) map[string]struct{} {
	set := make(map[string]struct{})
	for part := range strings.SplitSeq(raw, ",") {
		e := strings.ToLower(strings.TrimSpace(part))
		if e != "" {
			set[e] = struct{}{}
		}
	}
	return set
}

// RequireEditor allows only authenticated users whose email is in the allowlist
// (built from GAMES_EDITOR_EMAILS via ParseEmailAllowlist). Must run after
// RequireAuth so the email claim is on the context. An empty allowlist denies
// everyone.
func RequireEditor(allowlist map[string]struct{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		email, _ := c.Get(ContextKeyEmail)
		emailStr, _ := email.(string)
		emailStr = strings.ToLower(strings.TrimSpace(emailStr))
		if emailStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}
		if _, ok := allowlist[emailStr]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "editor access required"})
			return
		}
		c.Next()
	}
}

const (
	ContextKeyUID   = "user_id"
	ContextKeyEmail = "user_email"
	ContextKeyRole  = "user_role"
)

// RequireAuth validates the Bearer JWT and stores authID/email in the Gin context.
func RequireAuth(verifier *AuthJSVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			observability.AuthAttemptsTotal.WithLabelValues("missing_token").Inc()
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")
		authID, email, err := verifier.VerifyToken(token)
		if err != nil {
			observability.AuthAttemptsTotal.WithLabelValues("invalid_token").Inc()
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		observability.AuthAttemptsTotal.WithLabelValues("success").Inc()
		reqCtx := context.WithValue(c.Request.Context(), ContextKeyUID, authID)
		reqCtx = context.WithValue(reqCtx, ContextKeyEmail, email)
		c.Request = c.Request.WithContext(reqCtx)
		c.Set(ContextKeyUID, authID)
		c.Set(ContextKeyEmail, email)
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
