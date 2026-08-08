package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Public, unauthenticated reads of published editorial communities. This is the
// first-party replacement for the Sanity proxy that used to back /communities:
// the documents were ported into editorial_communities by studio's sync:content,
// and only the `published` snapshot is ever exposed.

// GeoPoint is a physical community's coordinates.
type GeoPoint struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// PublishedCommunity mirrors the community payload written by the migration and
// edited in /editor. Optional fields are pointers so an absent value stays absent
// rather than becoming a zero value on the wire.
type PublishedCommunity struct {
	ID          int
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	Slug        *string  `json:"slug"`
	Language    *string  `json:"language"`
	Description *string  `json:"description"`
	ImageURL    *string  `json:"image_url"`
	Tags        []string `json:"tags"`
	IsActive    bool     `json:"is_active"`

	// Online-only
	Platform    *string `json:"platform"`
	URL         *string `json:"url"`
	InviteCode  *string `json:"invite_code"`
	MemberCount *int    `json:"member_count"`

	// Physical-only
	Address         *string   `json:"address"`
	City            *string   `json:"city"`
	Country         *string   `json:"country"`
	Geo             *GeoPoint `json:"geo"`
	ContactEmail    *string   `json:"contact_email"`
	ContactPhone    *string   `json:"contact_phone"`
	MeetingSchedule *string   `json:"meeting_schedule"`
}

// CommunityFilter narrows the result set. Empty fields disable that filter.
type CommunityFilter struct {
	Kind     string // "online" | "physical"
	Language string // ISO 639-1
	Country  string // ISO 3166-1 alpha-2, matched case-insensitively
}

// ListPublishedCommunities returns active, published communities matching the
// filter, ordered by name. Never returns nil on success, so the endpoint encodes
// an empty list rather than null.
func (db *DB) ListPublishedCommunities(ctx context.Context, f CommunityFilter) ([]PublishedCommunity, error) {
	defer observe("list_published_communities", time.Now())

	// is_active defaults to true: a payload predating the field should stay
	// visible rather than silently dropping out of the listing.
	conds := []string{
		"published IS NOT NULL",
		"COALESCE(published->>'is_active', 'true') = 'true'",
	}
	args := []any{}
	add := func(cond string, val any) {
		args = append(args, val)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if f.Kind != "" {
		add("published->>'kind' = $%d", f.Kind)
	}
	if f.Language != "" {
		add("published->>'language' = $%d", f.Language)
	}
	if f.Country != "" {
		add("upper(published->>'country') = $%d", strings.ToUpper(f.Country))
	}

	query := `SELECT id, published FROM editorial_communities WHERE ` +
		strings.Join(conds, " AND ") +
		` ORDER BY published->>'name' ASC, id ASC`

	rows, err := db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []PublishedCommunity{}
	for rows.Next() {
		var id int
		var payload []byte
		if err := rows.Scan(&id, &payload); err != nil {
			return nil, err
		}
		var c PublishedCommunity
		if err := json.Unmarshal(payload, &c); err != nil {
			return nil, fmt.Errorf("decode community %d: %w", id, err)
		}
		c.ID = id
		if c.Tags == nil {
			c.Tags = []string{}
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
