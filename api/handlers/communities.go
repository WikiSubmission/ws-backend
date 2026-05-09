package handlers

import (
	"context"
	"log/slog"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/wikiSubmission/ws-backend/sanity"
)

// GetCommunities returns active communities from Sanity, with optional kind /
// language / country filters. Returns 502 if Sanity is unreachable.
//
// TODO(auth): /communities is currently public read. Add auth gating once the
// consumer site needs user-specific filtering or write access.
func (s *Server) GetCommunities(ctx context.Context, request GetCommunitiesRequestObject) (GetCommunitiesResponseObject, error) {
	if s.Sanity == nil {
		slog.Error("GetCommunities called but sanity client is not configured")
		return GetCommunities502JSONResponse{
			BadGatewayJSONResponse{Message: "Sanity proxy is not configured on this instance"},
		}, nil
	}

	filter := sanity.CommunityFilter{}
	if request.Params.Kind != nil {
		filter.Kind = string(*request.Params.Kind)
	}
	if request.Params.Language != nil {
		filter.Language = *request.Params.Language
	}
	if request.Params.Country != nil {
		filter.Country = *request.Params.Country
	}

	rows, err := s.Sanity.GetCommunities(ctx, filter)
	if err != nil {
		slog.Error("Failed to fetch communities from Sanity", "error", err)
		return GetCommunities502JSONResponse{
			BadGatewayJSONResponse{Message: "Upstream content source is unavailable"},
		}, nil
	}

	out := make([]Community, len(rows))
	for i := range rows {
		out[i] = sanityCommunityToAPI(rows[i])
	}
	return GetCommunities200JSONResponse(out), nil
}

func sanityCommunityToAPI(c sanity.Community) Community {
	api := Community{
		UnderscoreId:    c.ID,
		Name:            c.Name,
		Kind:            CommunityKind(c.Kind),
		IsActive:        c.IsActive,
		Slug:            c.Slug,
		Language:        c.Language,
		Description:     c.Description,
		ImageUrl:        c.ImageURL,
		Tags:            optStringSlice(c.Tags),
		Url:             c.URL,
		InviteCode:      c.InviteCode,
		MemberCount:     c.MemberCount,
		Address:         c.Address,
		City:            c.City,
		Country:         c.Country,
		ContactPhone:    c.ContactPhone,
		MeetingSchedule: c.MeetingSchedule,
	}
	if c.ContactEmail != nil {
		email := openapi_types.Email(*c.ContactEmail)
		api.ContactEmail = &email
	}
	if c.Platform != nil {
		p := CommunityPlatform(*c.Platform)
		api.Platform = &p
	}
	if c.Geo != nil {
		api.Geo = &GeoPoint{Lat: &c.Geo.Lat, Lng: &c.Geo.Lng}
	}
	return api
}

func optStringSlice(s []string) *[]string {
	if len(s) == 0 {
		return nil
	}
	return &s
}
