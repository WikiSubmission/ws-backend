package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/auth"
	"github.com/wikiSubmission/ws-backend/db"
)

// UserHandler holds the DB reference for user-related route handlers.
type UserHandler struct {
	DB *db.DB
}

// UserResponse is the JSON shape returned for a single user.
type UserResponse struct {
	ID          int             `json:"id"`
	AuthID      string          `json:"auth_id"`
	Email       string          `json:"email"`
	DisplayName *string         `json:"display_name,omitempty"`
	Role        string          `json:"role"`
	Permissions json.RawMessage `json:"permissions"`
	IsActive    bool            `json:"is_active"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// UpsertUserRequest is the body for POST /users.
// Email is intentionally absent — it is sourced from the verified JWT only.
type UpsertUserRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
}

func displayNameFromGin(c *gin.Context) *string {
	name := strings.TrimSpace(c.GetString(auth.ContextKeyName))
	if name == "" {
		return nil
	}
	return &name
}

func userRowToResponse(u *db.UserRow) UserResponse {
	return UserResponse{
		ID:          u.ID,
		AuthID:      u.AuthID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		Role:        u.Role,
		Permissions: u.Permissions,
		IsActive:    u.IsActive,
		CreatedAt:   u.CreatedAt,
		UpdatedAt:   u.UpdatedAt,
	}
}

// GetUsers handles GET /api/v1/users (admin only).
func (h *UserHandler) GetUsers(c *gin.Context) {
	limit := 50
	offset := 0
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	if o := c.Query("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	users, err := h.DB.FetchAllUsers(c.Request.Context(), limit, offset)
	if err != nil {
		slog.Error("GetUsers: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"data": nil, "error": "internal server error"})
		return
	}

	result := make([]UserResponse, len(users))
	for i := range users {
		result[i] = userRowToResponse(&users[i])
	}
	c.JSON(http.StatusOK, gin.H{"data": result, "error": nil})
}

// GetCurrentUser handles GET /api/v1/users/me.
func (h *UserHandler) GetCurrentUser(c *gin.Context) {
	authID := c.GetString(auth.ContextKeyUID)
	if authID == "" {
		slog.Warn("GetCurrentUser: missing auth subject")
		c.JSON(http.StatusUnauthorized, gin.H{"data": nil, "error": "not authenticated"})
		return
	}

	user, err := h.DB.FetchUserByAuthID(c.Request.Context(), authID)
	if err != nil {
		slog.Error("GetCurrentUser: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"data": nil, "error": "internal server error"})
		return
	}
	if user == nil {
		slog.Warn("GetCurrentUser: user row not found", "provider", auth.ProviderFromAuthID(authID))
		c.JSON(http.StatusNotFound, gin.H{"data": nil, "error": "user not found"})
		return
	}

	slog.Debug("GetCurrentUser: user row found", "provider", auth.ProviderFromAuthID(authID), "user_id", user.ID, "has_display_name", user.DisplayName != nil && *user.DisplayName != "")
	c.JSON(http.StatusOK, gin.H{"data": userRowToResponse(user), "error": nil})
}

// GetUserByID handles GET /api/v1/users/:id.
func (h *UserHandler) GetUserByID(c *gin.Context) {
	idParam := c.Param("id")

	if idParam == "me" {
		authID := c.GetString(auth.ContextKeyUID)
		if authID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"data": nil, "error": "not authenticated"})
			return
		}
		user, err := h.DB.FetchUserByAuthID(c.Request.Context(), authID)
		if err != nil {
			slog.Error("GetUserByID(me): DB error", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"data": nil, "error": "internal server error"})
			return
		}
		if user == nil {
			slog.Warn("GetUserByID(me): user row not found", "provider", auth.ProviderFromAuthID(authID))
			c.JSON(http.StatusNotFound, gin.H{"data": nil, "error": "user not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": userRowToResponse(user), "error": nil})
		return
	}

	id, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"data": nil, "error": "invalid user id"})
		return
	}

	callerAuthID := c.GetString(auth.ContextKeyUID)
	callerRole := c.GetString(auth.ContextKeyRole)
	if callerRole != "admin" {
		caller, err := h.DB.FetchUserByAuthID(c.Request.Context(), callerAuthID)
		if err != nil || caller == nil || caller.ID != id {
			c.JSON(http.StatusForbidden, gin.H{"data": nil, "error": "forbidden"})
			return
		}
	}

	user, err := h.DB.FetchUserByID(c.Request.Context(), id)
	if err != nil {
		slog.Error("GetUserByID: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"data": nil, "error": "internal server error"})
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, gin.H{"data": nil, "error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": userRowToResponse(user), "error": nil})
}

// UpsertUser handles POST /api/v1/users (upsert on first login).
// The auth_id comes from the verified JWT; the body only provides optional display name.
func (h *UserHandler) UpsertUser(c *gin.Context) {
	var req UpsertUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		slog.Warn("UpsertUser: invalid request body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"data": nil, "error": err.Error()})
		return
	}

	authID := c.GetString(auth.ContextKeyUID)
	if authID == "" {
		slog.Warn("UpsertUser: missing auth subject")
		c.JSON(http.StatusUnauthorized, gin.H{"data": nil, "error": "not authenticated"})
		return
	}
	// Email must come exclusively from the verified JWT — never from the request body.
	// Trusting the body would allow a caller with a blank email claim to hijack any
	// existing user row by supplying a victim email (ON CONFLICT updates auth_id).
	email := c.GetString(auth.ContextKeyEmail)
	if email == "" {
		slog.Warn("UpsertUser: missing email claim",
			"provider", auth.ProviderFromAuthID(authID),
			"has_name", c.GetString(auth.ContextKeyName) != "",
		)
		c.JSON(http.StatusBadRequest, gin.H{"data": nil, "error": "email is required"})
		return
	}

	displayName := req.DisplayName
	if displayName == nil {
		displayName = displayNameFromGin(c)
	}

	slog.Info("UpsertUser: provisioning user",
		"provider", auth.ProviderFromAuthID(authID),
		"has_display_name", displayName != nil,
	)
	user, err := h.DB.UpsertUser(c.Request.Context(), authID, email, displayName)
	if err != nil {
		slog.Error("UpsertUser: DB error", "provider", auth.ProviderFromAuthID(authID), "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"data": nil, "error": "internal server error"})
		return
	}
	slog.Info("UpsertUser: user provisioned", "provider", auth.ProviderFromAuthID(authID), "user_id", user.ID, "has_display_name", user.DisplayName != nil && *user.DisplayName != "")
	c.JSON(http.StatusOK, gin.H{"data": userRowToResponse(user), "error": nil})
}

// UpdateUser handles PATCH /api/v1/users/:id (admin only).
func (h *UserHandler) UpdateUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"data": nil, "error": "invalid user id"})
		return
	}

	var req UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"data": nil, "error": err.Error()})
		return
	}

	if req.Role != nil {
		if err := h.DB.UpdateUserRole(c.Request.Context(), id, string(*req.Role)); err != nil {
			slog.Error("UpdateUser: UpdateUserRole error", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"data": nil, "error": err.Error()})
			return
		}
	}

	if req.Permissions != nil {
		permissionsRaw, err := json.Marshal(req.Permissions)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"data": nil, "error": "invalid permissions payload"})
			return
		}
		if err := h.DB.UpdateUserPermissions(c.Request.Context(), id, permissionsRaw); err != nil {
			slog.Error("UpdateUser: UpdateUserPermissions error", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"data": nil, "error": err.Error()})
			return
		}
	}

	if req.IsActive != nil && !*req.IsActive {
		if err := h.DB.DeactivateUser(c.Request.Context(), id); err != nil {
			slog.Error("UpdateUser: DeactivateUser error", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"data": nil, "error": err.Error()})
			return
		}
	}

	user, err := h.DB.FetchUserByID(c.Request.Context(), id)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"data": nil, "error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": userRowToResponse(user), "error": nil})
}
