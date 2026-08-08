package handlers

import (
	"testing"

	"github.com/wikiSubmission/ws-backend/db"
)

func strptr(s string) *string { return &s }
func intptr(i int) *int       { return &i }

// The public shape predates the editorial store — it was fed by Sanity — so the
// mapping is what keeps existing consumers working after the cutover.
func TestEditorialCommunityToAPI_Online(t *testing.T) {
	api := editorialCommunityToAPI(db.PublishedCommunity{
		ID:          7,
		Kind:        "online",
		Name:        "Submission Server",
		Slug:        strptr("submission-server"),
		Language:    strptr("en"),
		IsActive:    true,
		Tags:        []string{"discord", "study"},
		Platform:    strptr("discord"),
		URL:         strptr("https://discord.gg/example"),
		InviteCode:  strptr("abc123"),
		MemberCount: intptr(420),
	})

	if api.UnderscoreId != "7" {
		t.Errorf("row id must be stringified for the public _id: got %q", api.UnderscoreId)
	}
	if api.Kind != "online" || api.Name != "Submission Server" || !api.IsActive {
		t.Errorf("core fields not carried over: %+v", api)
	}
	if api.Platform == nil || *api.Platform != "discord" {
		t.Errorf("platform not mapped: %v", api.Platform)
	}
	if api.MemberCount == nil || *api.MemberCount != 420 {
		t.Errorf("member_count not mapped: %v", api.MemberCount)
	}
	if api.Tags == nil || len(*api.Tags) != 2 {
		t.Errorf("tags not mapped: %v", api.Tags)
	}
	// Physical-only fields stay absent so they are omitted from the JSON.
	if api.Address != nil || api.Geo != nil || api.ContactEmail != nil {
		t.Errorf("physical fields leaked onto an online community: %+v", api)
	}
}

func TestEditorialCommunityToAPI_Physical(t *testing.T) {
	api := editorialCommunityToAPI(db.PublishedCommunity{
		ID:           12,
		Kind:         "physical",
		Name:         "Toronto",
		IsActive:     true,
		Address:      strptr("Toronto ON, Canada"),
		City:         strptr("Toronto"),
		Country:      strptr("CA"),
		Geo:          &db.GeoPoint{Lat: 43.65, Lng: -79.38},
		ContactEmail: strptr("hi@example.org"),
	})

	if api.Geo == nil || api.Geo.Lat == nil || *api.Geo.Lat != 43.65 {
		t.Fatalf("geo lat not mapped: %+v", api.Geo)
	}
	if api.Geo.Lng == nil || *api.Geo.Lng != -79.38 {
		t.Errorf("geo lng not mapped: %+v", api.Geo)
	}
	if api.ContactEmail == nil || string(*api.ContactEmail) != "hi@example.org" {
		t.Errorf("contact email not mapped: %v", api.ContactEmail)
	}
	if api.Platform != nil || api.MemberCount != nil {
		t.Errorf("online fields leaked onto a physical community: %+v", api)
	}
}

// An empty tag list must omit the key rather than serialise [] — the migration
// writes `tags: []` for every community that had none in the CMS.
func TestEditorialCommunityToAPI_EmptyTagsOmitted(t *testing.T) {
	api := editorialCommunityToAPI(db.PublishedCommunity{
		ID: 1, Kind: "online", Name: "X", Tags: []string{},
	})
	if api.Tags != nil {
		t.Errorf("empty tags must be omitted, got %v", api.Tags)
	}
}
