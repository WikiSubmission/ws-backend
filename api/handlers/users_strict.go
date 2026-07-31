package handlers

import (
	"context"
	"encoding/json"
	"log/slog"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/wikiSubmission/ws-backend/auth"
	"github.com/wikiSubmission/ws-backend/db"
)

func userFromRow(u *db.UserRow) (User, error) {
	permissions := map[string]interface{}{}
	if len(u.Permissions) > 0 {
		if err := json.Unmarshal(u.Permissions, &permissions); err != nil {
			return User{}, err
		}
	}

	return User{
		Id:          int64(u.ID),
		AuthId:      u.AuthID,
		Email:       openapi_types.Email(u.Email),
		DisplayName: u.DisplayName,
		Role:        UserRole(u.Role),
		Permissions: &permissions,
		IsActive:    u.IsActive,
		CreatedAt:   u.CreatedAt,
		UpdatedAt:   u.UpdatedAt,
	}, nil
}

// callerIsAdmin resolves the authenticated caller from the request context and
// reports whether they are an active admin. The user-administration endpoints
// are declared "admin only" in openapi.yaml but the bearerAuth scheme only
// proves a valid session; the role check must happen here.
func (s *Server) callerIsAdmin(ctx context.Context) (bool, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return false, nil
	}
	caller, err := s.DB.FetchUserByAuthID(ctx, authID)
	if err != nil {
		return false, err
	}
	return caller != nil && caller.IsActive && caller.Role == "admin", nil
}

func (s *Server) ListUsersAdmin(ctx context.Context, request ListUsersAdminRequestObject) (ListUsersAdminResponseObject, error) {
	isAdmin, err := s.callerIsAdmin(ctx)
	if err != nil {
		slog.Error("ListUsersAdmin: admin check failed", "error", err)
		return ListUsersAdmin500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !isAdmin {
		return ListUsersAdmin403JSONResponse{ForbiddenJSONResponse{Message: "admin access required"}}, nil
	}

	limit := 50
	offset := 0
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}
	if limit <= 0 || offset < 0 {
		return ListUsersAdmin400JSONResponse{BadRequestJSONResponse{Message: "invalid pagination parameters"}}, nil
	}

	rows, err := s.DB.FetchAllUsers(ctx, limit, offset)
	if err != nil {
		slog.Error("ListUsersAdmin: DB error", "error", err)
		return ListUsersAdmin500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	users := make([]User, len(rows))
	for i := range rows {
		item, convErr := userFromRow(&rows[i])
		if convErr != nil {
			slog.Error("ListUsersAdmin: invalid permissions payload", "error", convErr, "user_id", rows[i].ID)
			return ListUsersAdmin500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
		}
		users[i] = item
	}

	return ListUsersAdmin200JSONResponse(UserListEnvelope{Data: users}), nil
}

func (s *Server) GetCurrentUserAccess(ctx context.Context, _ GetCurrentUserAccessRequestObject) (GetCurrentUserAccessResponseObject, error) {
	authID, ok := authIDFromContext(ctx)
	if !ok {
		return GetCurrentUserAccess500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	user, err := s.DB.FetchUserByAuthID(ctx, authID)
	if err != nil {
		slog.Error("GetCurrentUserAccess: DB error", "error", err)
		return GetCurrentUserAccess500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if user == nil {
		slog.Warn("GetCurrentUserAccess: user row not found",
			"provider", auth.ProviderFromAuthID(authID),
			"has_email", contextHasEmail(ctx),
			"has_name", contextHasName(ctx),
		)
		return GetCurrentUserAccess404JSONResponse{NotFoundJSONResponse{Message: "user not found"}}, nil
	}

	item, err := userFromRow(user)
	if err != nil {
		slog.Error("GetCurrentUserAccess: invalid permissions payload", "error", err, "user_id", user.ID)
		return GetCurrentUserAccess500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	return GetCurrentUserAccess200JSONResponse(UserEnvelope{Data: item}), nil
}

func (s *Server) UpdateUserAdmin(ctx context.Context, request UpdateUserAdminRequestObject) (UpdateUserAdminResponseObject, error) {
	isAdmin, err := s.callerIsAdmin(ctx)
	if err != nil {
		slog.Error("UpdateUserAdmin: admin check failed", "error", err)
		return UpdateUserAdmin500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if !isAdmin {
		return UpdateUserAdmin403JSONResponse{ForbiddenJSONResponse{Message: "admin access required"}}, nil
	}

	if request.Id <= 0 {
		return UpdateUserAdmin400JSONResponse{BadRequestJSONResponse{Message: "invalid user id"}}, nil
	}
	if request.Body == nil {
		return UpdateUserAdmin400JSONResponse{BadRequestJSONResponse{Message: "request body is required"}}, nil
	}

	id := int(request.Id)
	req := request.Body

	// Refuse to strip the last active admin, whether by demoting their role or by
	// deactivating them — either one locks everyone out of /admin/access, which is
	// the only surface that can grant it back. Checked before any write, so a
	// rejected request changes nothing.
	demotesRole := req.Role != nil && string(*req.Role) != "admin"
	deactivates := req.IsActive != nil && !*req.IsActive
	if demotesRole || deactivates {
		isLast, err := s.DB.IsLastActiveAdmin(ctx, id)
		if err != nil {
			slog.Error("UpdateUserAdmin: last-admin check failed", "error", err, "user_id", id)
			return UpdateUserAdmin500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
		}
		if isLast {
			return UpdateUserAdmin400JSONResponse{BadRequestJSONResponse{
				Message: "cannot remove the last active admin; grant admin to another account first",
			}}, nil
		}
	}

	if req.Role != nil {
		if err := s.DB.UpdateUserRole(ctx, id, string(*req.Role)); err != nil {
			slog.Error("UpdateUserAdmin: UpdateUserRole error", "error", err)
			return UpdateUserAdmin500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
		}
	}

	if req.Permissions != nil {
		permissionsRaw, err := json.Marshal(req.Permissions)
		if err != nil {
			return UpdateUserAdmin400JSONResponse{BadRequestJSONResponse{Message: "invalid permissions payload"}}, nil
		}
		if err := s.DB.UpdateUserPermissions(ctx, id, permissionsRaw); err != nil {
			slog.Error("UpdateUserAdmin: UpdateUserPermissions error", "error", err)
			return UpdateUserAdmin500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
		}
	}

	if req.IsActive != nil && !*req.IsActive {
		if err := s.DB.DeactivateUser(ctx, id); err != nil {
			slog.Error("UpdateUserAdmin: DeactivateUser error", "error", err)
			return UpdateUserAdmin500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
		}
	}

	user, err := s.DB.FetchUserByID(ctx, id)
	if err != nil {
		slog.Error("UpdateUserAdmin: FetchUserByID error", "error", err)
		return UpdateUserAdmin500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}
	if user == nil {
		return UpdateUserAdmin404JSONResponse{NotFoundJSONResponse{Message: "user not found"}}, nil
	}

	item, err := userFromRow(user)
	if err != nil {
		slog.Error("UpdateUserAdmin: invalid permissions payload", "error", err, "user_id", user.ID)
		return UpdateUserAdmin500JSONResponse{InternalServerErrrorJSONResponse{Message: "internal server error"}}, nil
	}

	return UpdateUserAdmin200JSONResponse(UserEnvelope{Data: item}), nil
}
