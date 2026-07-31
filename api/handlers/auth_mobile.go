package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/auth"
	"github.com/wikiSubmission/ws-backend/db"
	"github.com/wikiSubmission/ws-backend/email"
)

// nativeAuthStore is the slice of the DB the native-auth handlers need. Keeping
// it small lets the handler tests inject a fake without a live database, in
// line with this package's DB-free test convention (DB calls are covered in db/).
type nativeAuthStore interface {
	UpsertUser(ctx context.Context, authID, email string, displayName *string) (*db.UserRow, error)
	FetchUserByAuthID(ctx context.Context, authID string) (*db.UserRow, error)
	// FetchEditorPermissions resolves the grant snapshot behind the session's
	// isEditor flag. Games access moved from a JSONB flag to grant tables in
	// migration 027, so it can no longer be read off the user row.
	FetchEditorPermissions(ctx context.Context, userID int, isAdmin bool) (*db.EditorPermissions, error)
}

// NativeAuthHandler serves the mobile native-auth endpoints. A mobile client
// cannot mint the backend session JWT (WS_BACKEND_JWT_SECRET never ships to a
// device), so it signs in with a native Google / Apple flow, hands the provider
// ID token here, and receives the same HS256 session token the rest of the API
// already trusts plus a long-lived refresh token.
type NativeAuthHandler struct {
	Store      nativeAuthStore
	Google     auth.IDTokenVerifier
	Apple      auth.IDTokenVerifier
	Minter     *auth.Minter
	Verifier   *auth.AuthJSVerifier
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	// OTP issues and verifies email one-time codes for passwordless sign-in.
	// Nil/unconfigured disables the /auth/mobile/otp/* endpoints (503).
	OTP *auth.OTPManager
	// Mailer delivers the OTP code. Nil means email delivery is unconfigured;
	// OTPDevLog then decides between a dev log and a 503.
	Mailer email.Sender
	// OTPDevLog, when true, logs the code instead of failing when Mailer is nil.
	// Intended for local development only, mirroring the web send-otp dev path.
	OTPDevLog bool
	// otpIPLimiter / otpEmailLimiter throttle OTP requests. Lazily initialised.
	otpIPLimiter    *rateLimiter
	otpEmailLimiter *rateLimiter
	// Now is injectable so tests can pin issuance time; defaults to time.Now.
	Now func() time.Time
}

type mobileExchangeRequest struct {
	Provider string `json:"provider"`
	IDToken  string `json:"id_token"`
	Nonce    string `json:"nonce"`
}

type mobileRefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type mobileAuthUser struct {
	ID          int64          `json:"id"`
	AuthID      string         `json:"auth_id"`
	Email       string         `json:"email"`
	DisplayName *string        `json:"display_name,omitempty"`
	Role        string         `json:"role"`
	Permissions map[string]any `json:"permissions"`
	IsActive    bool           `json:"is_active"`
	IsAdmin     bool           `json:"is_admin"`
	IsEditor    bool           `json:"is_editor"`
}

type mobileAuthResponse struct {
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token"`
	TokenType    string         `json:"token_type"`
	ExpiresIn    int            `json:"expires_in"`
	User         mobileAuthUser `json:"user"`
}

func (h *NativeAuthHandler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

// Exchange verifies a provider ID token and returns a backend session.
// POST /api/v1/auth/mobile/exchange
func (h *NativeAuthHandler) Exchange(c *gin.Context) {
	if h.Minter == nil || !h.Minter.Configured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "native auth not configured"})
		return
	}

	var req mobileExchangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	req.Provider = strings.ToLower(strings.TrimSpace(req.Provider))
	req.IDToken = strings.TrimSpace(req.IDToken)
	if req.IDToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id_token is required"})
		return
	}

	var verifier auth.IDTokenVerifier
	switch req.Provider {
	case "google":
		verifier = h.Google
	case "apple":
		verifier = h.Apple
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported provider"})
		return
	}
	if verifier == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "provider not configured"})
		return
	}

	identity, err := verifier.Verify(c.Request.Context(), req.IDToken, req.Nonce)
	if err != nil {
		_ = c.Error(err) // surfaced via the request error logger middleware
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid id token"})
		return
	}

	authID := req.Provider + ":" + identity.Subject

	user, err := h.resolveUser(c, authID, identity)
	if err != nil {
		switch err {
		case errEmailUnavailable:
			c.JSON(http.StatusUnauthorized, gin.H{"error": "email not available from provider"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		}
		return
	}
	if !user.IsActive {
		c.JSON(http.StatusForbidden, gin.H{"error": "account is not active"})
		return
	}

	h.respondWithSession(c, user)
}

// Refresh exchanges a valid refresh token for a fresh access token and a
// rotated refresh token. A deactivated account is rejected here, so revocation
// takes effect within one access-token lifetime.
// POST /api/v1/auth/mobile/refresh
func (h *NativeAuthHandler) Refresh(c *gin.Context) {
	if h.Minter == nil || !h.Minter.Configured() || h.Verifier == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "native auth not configured"})
		return
	}

	var req mobileRefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.RefreshToken) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "refresh_token is required"})
		return
	}

	id, err := h.Verifier.VerifyRefreshToken(strings.TrimSpace(req.RefreshToken))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
		return
	}

	user, err := h.Store.FetchUserByAuthID(c.Request.Context(), id.AuthID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	if user == nil || !user.IsActive {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "account is not active"})
		return
	}

	h.respondWithSession(c, user)
}

var errEmailUnavailable = &nativeAuthError{"email unavailable"}

type nativeAuthError struct{ msg string }

func (e *nativeAuthError) Error() string { return e.msg }

// resolveUser upserts the user from the verified identity. Apple omits the email
// on repeat logins, so when the email is absent we fall back to the existing row
// keyed by auth_id (created on first sign-in) rather than failing.
func (h *NativeAuthHandler) resolveUser(c *gin.Context, authID string, identity auth.VerifiedIdentity) (*db.UserRow, error) {
	ctx := c.Request.Context()
	if identity.Email == "" {
		existing, err := h.Store.FetchUserByAuthID(ctx, authID)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, errEmailUnavailable
		}
		return existing, nil
	}

	var displayName *string
	if name := strings.TrimSpace(identity.Name); name != "" {
		displayName = &name
	}
	return h.Store.UpsertUser(ctx, authID, identity.Email, displayName)
}

func (h *NativeAuthHandler) respondWithSession(c *gin.Context, user *db.UserRow) {
	now := h.now()
	tokenName := displayNameOrFallback(user)
	identity := auth.Identity{AuthID: user.AuthID, Email: user.Email, Name: tokenName}

	access, err := h.Minter.MintAccessToken(identity, h.AccessTTL, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	refresh, err := h.Minter.MintRefreshToken(identity, h.RefreshTTL, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	// isEditor now comes from the grant tables rather than a JSONB flag, so the
	// snapshot has to be resolved here. A failure degrades to "no games access"
	// rather than failing the sign-in — the backend re-checks on every request.
	perms, err := h.Store.FetchEditorPermissions(c.Request.Context(), user.ID, user.Role == "admin")
	if err != nil {
		slog.Error("mobile auth: fetch editor permissions", "error", err, "user_id", user.ID)
		perms = nil
	}

	c.JSON(http.StatusOK, mobileAuthResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int(h.AccessTTL.Seconds()),
		User:         mobileUserFromRow(user, perms),
	})
}

// displayNameOrFallback ensures the minted token always carries a non-empty
// name claim (the verifier warns otherwise), mirroring the web credentials path
// that falls back to the email local part.
func displayNameOrFallback(user *db.UserRow) string {
	if user.DisplayName != nil && strings.TrimSpace(*user.DisplayName) != "" {
		return *user.DisplayName
	}
	if local, _, ok := strings.Cut(user.Email, "@"); ok && local != "" {
		return local
	}
	return user.Email
}

func mobileUserFromRow(user *db.UserRow, perms *db.EditorPermissions) mobileAuthUser {
	permissions := map[string]any{}
	if len(user.Permissions) > 0 {
		_ = json.Unmarshal(user.Permissions, &permissions)
	}
	return mobileAuthUser{
		ID:          int64(user.ID),
		AuthID:      user.AuthID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Role:        user.Role,
		Permissions: permissions,
		IsActive:    user.IsActive,
		IsAdmin:     user.Role == "admin",
		IsEditor:    auth.HasGamesEditorAccess(user, perms),
	}
}
