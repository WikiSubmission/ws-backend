package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
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
	SupabaseUID string          `json:"supabase_uid"`
	Email       string          `json:"email"`
	DisplayName *string         `json:"display_name,omitempty"`
	Role        string          `json:"role"`
	Permissions json.RawMessage `json:"permissions"`
	IsActive    bool            `json:"is_active"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// UpsertUserRequest is the body for POST /users.
type UpsertUserRequest struct {
	SupabaseUID string  `json:"supabase_uid" binding:"required"`
	Email       string  `json:"email"        binding:"required"`
	DisplayName *string `json:"display_name,omitempty"`
}

// UpdateUserRequest is the body for PATCH /users/:id.
type UpdateUserRequest struct {
	Role        *string         `json:"role,omitempty"`
	Permissions json.RawMessage `json:"permissions,omitempty"`
	IsActive    *bool           `json:"is_active,omitempty"`
}

func userRowToResponse(u *db.UserRow) UserResponse {
	return UserResponse{
		ID:          u.ID,
		SupabaseUID: u.SupabaseUID,
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

// GetUserByID handles GET /api/v1/users/:id.
func (h *UserHandler) GetUserByID(c *gin.Context) {
	// Allow fetching by numeric id or "me" (self lookup by JWT uid)
	idParam := c.Param("id")

	if idParam == "me" {
		uid := c.GetString(auth.ContextKeyUID)
		if uid == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"data": nil, "error": "not authenticated"})
			return
		}
		user, err := h.DB.FetchUserBySupabaseUID(c.Request.Context(), uid)
		if err != nil {
			slog.Error("GetUserByID(me): DB error", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"data": nil, "error": "internal server error"})
			return
		}
		if user == nil {
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

	// Non-admin users may only fetch their own record.
	callerUID := c.GetString(auth.ContextKeyUID)
	callerRole := c.GetString(auth.ContextKeyRole)
	if callerRole != "admin" {
		// Resolve caller's DB record to compare ids.
		caller, err := h.DB.FetchUserBySupabaseUID(c.Request.Context(), callerUID)
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
func (h *UserHandler) UpsertUser(c *gin.Context) {
	var req UpsertUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"data": nil, "error": err.Error()})
		return
	}

	// Verify the JWT uid matches the requested supabase_uid to prevent spoofing.
	callerUID := c.GetString(auth.ContextKeyUID)
	if callerUID != req.SupabaseUID {
		c.JSON(http.StatusForbidden, gin.H{"data": nil, "error": "supabase_uid mismatch"})
		return
	}

	user, err := h.DB.UpsertUser(c.Request.Context(), req.SupabaseUID, req.Email, req.DisplayName)
	if err != nil {
		slog.Error("UpsertUser: DB error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"data": nil, "error": "internal server error"})
		return
	}
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
		if err := h.DB.UpdateUserRole(c.Request.Context(), id, *req.Role); err != nil {
			slog.Error("UpdateUser: UpdateUserRole error", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"data": nil, "error": err.Error()})
			return
		}
	}

	if req.Permissions != nil {
		if err := h.DB.UpdateUserPermissions(c.Request.Context(), id, req.Permissions); err != nil {
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

	// Return the updated user.
	user, err := h.DB.FetchUserByID(c.Request.Context(), id)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"data": nil, "error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": userRowToResponse(user), "error": nil})
}
