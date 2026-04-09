package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/db"
)

const (
	ContextKeyUID   = "supabase_uid"
	ContextKeyEmail = "supabase_email"
	ContextKeyRole  = "user_role"
)

// RequireAuth validates the Bearer JWT and stores uid/email in the context.
func RequireAuth(jwks *JWKSCache) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")
		uid, email, err := jwks.VerifyToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set(ContextKeyUID, uid)
		c.Set(ContextKeyEmail, email)
		c.Next()
	}
}

// RequireAdmin additionally checks that the user has role=admin in the DB.
func RequireAdmin(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, exists := c.Get(ContextKeyUID)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}
		user, err := database.FetchUserBySupabaseUID(c.Request.Context(), uid.(string))
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
