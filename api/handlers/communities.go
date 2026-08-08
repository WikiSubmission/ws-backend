package handlers

import (
	"context"
	"log/slog"
	"strconv"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/wikiSubmission/ws-backend/db"
)

// GetCommunities returns active, published communities with optional kind /
// language / country filters. Reads the first-party editorial store, which
// replaced the Sanity proxy this endpoint used to front; the documents were
// ported into editorial_communities by studio's sync:content and are now edited
// in /editor.
//
// TODO(auth): /communities is currently public read. Add auth gating once the
// consumer site needs user-specific filtering or write access.
func (s *Server) GetCommunities(ctx context.Context, request GetCommunitiesRequestObject) (GetCommunitiesResponseObject, error) {
	filter := db.CommunityFilter{}
	if request.Params.Kind != nil {
		filter.Kind = string(*request.Params.Kind)
	}
	if request.Params.Language != nil {
		filter.Language = *request.Params.Language
	}
	if request.Params.Country != nil {
		filter.Country = *request.Params.Country
	}

	rows, err := s.DB.ListPublishedCommunities(ctx, filter)
	if err != nil {
		slog.Error("Failed to list published communities", "error", err)
		return GetCommunities500JSONResponse{
			InternalServerErrrorJSONResponse{Message: "Failed to load communities"},
		}, nil
	}

	out := make([]Community, len(rows))
	for i := range rows {
		out[i] = editorialCommunityToAPI(rows[i])
	}
	return GetCommunities200JSONResponse(out), nil
}

func editorialCommunityToAPI(c db.PublishedCommunity) Community {
	api := Community{
		// The public id was a Sanity document id; it is now the editorial row id.
		UnderscoreId:    strconv.Itoa(c.ID),
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
