package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/db"
)

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
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")
		authID, email, err := verifier.VerifyToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
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
