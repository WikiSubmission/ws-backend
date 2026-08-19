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

// PermissionGamesEditor is the legacy JSONB key on users.permissions that used
// to grant access to the games editorial surface as a single all-or-nothing
// boolean. Enforcement moved to the grant tables in migration 027
// (editor_module_grants module='game' for all games, editor_game_grants for a
// single game), so nothing reads this key to make an access decision any more.
// The column data is left in place so a rollback stays non-destructive.
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

// ContextKeyGamePerms is where the game gates stash the caller's resolved
// permission snapshot, so a handler can make a finer decision (for example a
// read-gated route that writes only for CanWriteGame holders) without a second
// round-trip to the database.
const ContextKeyGamePerms = "game_perms"

// requireGame is the shared body of the per-game gates. It resolves the caller
// and delegates the verdict to allow. Must run after RequireAuth so the auth id
// is on the context.
//
// gameKey is supplied by the caller at route-registration time, NOT read from
// the request. That is deliberate and load-bearing: the handlers behind these
// routes each serve one specific game's data and never inspect the path, so a
// gate that authorized whatever key the request carried would hand a holder of
// game B's grant the contents of game A. Binding the key at registration means
// the authorized game and the served game are the same by construction, and a
// game added later cannot inherit another game's routes.
func requireGame(
	database *db.DB,
	gameKey string,
	allow func(perms *db.EditorPermissions, gameKey string) bool,
) gin.HandlerFunc {
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
		if !user.HasRole("admin") && !user.HasRole("game_editor") {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "game editor role required"})
			return
		}
		perms, err := database.FetchEditorPermissions(c.Request.Context(), user.ID, user.HasRole("admin"))
		if err != nil {
			slog.Error("game gate: fetch permissions", "error", err, "path", c.FullPath())
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}
		if gameKey == "" || !allow(perms, gameKey) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "editor access required"})
			return
		}
		c.Set(ContextKeyRole, user.Role)
		c.Set(ContextKeyGamePerms, perms)
		c.Next()
	}
}

// RequireGameEditor allows admins, holders of the global games grant, and
// holders of a per-game grant on gameKey. Use for reads. gameKey must name the
// game whose data the routes behind this gate actually serve.
func RequireGameEditor(database *db.DB, gameKey string) gin.HandlerFunc {
	return requireGame(database, gameKey, func(p *db.EditorPermissions, key string) bool {
		return p.CanReadGame(key)
	})
}

// RequireGameWriter is RequireGameEditor for mutations — curation, passage
// status changes and the maintenance jobs, which are destructive and so demand
// write rather than mere read on the game.
func RequireGameWriter(database *db.DB, gameKey string) gin.HandlerFunc {
	return requireGame(database, gameKey, func(p *db.EditorPermissions, key string) bool {
		return p.CanWriteGame(key)
	})
}

// HasGamesEditorAccess reports whether the user can reach any games surface.
// Exposed so handlers serving the session shape can populate session.isEditor
// consistently with the gates above. Kept as a pure decision function — the
// caller resolves perms (via db.FetchEditorPermissions) and passes it in, so
// this stays unit-testable without a database.
//
// perms may be nil, which denies every non-admin.
func HasGamesEditorAccess(u *db.UserRow, perms *db.EditorPermissions) bool {
	if u == nil || !u.IsActive {
		return false
	}
	if u.HasRole("admin") {
		return true
	}
	return u.HasRole("game_editor") && perms.HasAnyGameAccess()
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

// OptionalAuth populates the auth context when a valid Bearer JWT is present
// and continues silently otherwise. Used by endpoints that accept anonymous
// callers but want to attach an identity when one is offered (e.g. FCM device
// registration).
func OptionalAuth(verifier *AuthJSVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.Next()
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")
		authID, email, name, err := verifier.VerifyToken(token)
		if err != nil || authID == "" {
			// A bad token on an optional endpoint is treated as anonymous, not
			// rejected — the caller may hold an expired session.
			c.Next()
			return
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

// RequireAdmin additionally checks that the authenticated user has the admin role.
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
		if !user.HasRole("admin") {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			return
		}
		c.Set(ContextKeyRole, user.Role)
		c.Next()
	}
}
