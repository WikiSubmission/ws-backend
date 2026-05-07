package sanity

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Community is the public DTO for a Sanity-authored community entry. Optional
// fields use pointers so JSON marshalling omits them on the wrong kind.
type Community struct {
	ID          string   `json:"_id"`
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	Slug        *string  `json:"slug,omitempty"`
	Language    *string  `json:"language,omitempty"`
	Description *string  `json:"description,omitempty"`
	ImageURL    *string  `json:"imageUrl,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	IsActive    bool     `json:"isActive"`

	// Online-only
	Platform    *string `json:"platform,omitempty"`
	URL         *string `json:"url,omitempty"`
	InviteCode  *string `json:"inviteCode,omitempty"`
	MemberCount *int    `json:"memberCount,omitempty"`

	// Physical-only
	Address         *string   `json:"address,omitempty"`
	City            *string   `json:"city,omitempty"`
	Country         *string   `json:"country,omitempty"`
	Geo             *GeoPoint `json:"geo,omitempty"`
	ContactEmail    *string   `json:"contactEmail,omitempty"`
	ContactPhone    *string   `json:"contactPhone,omitempty"`
	MeetingSchedule *string   `json:"meetingSchedule,omitempty"`
}

type GeoPoint struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// CommunityFilter narrows the result set. Empty fields disable that filter.
type CommunityFilter struct {
	Kind     string // "online" | "physical"
	Language string // ISO 639-1
	Country  string // ISO 3166-1 alpha-2 (uppercased)
}

const communityProjection = `{
  _id, kind, name, "slug": slug.current, language, description,
  "imageUrl": image.asset->url, tags, isActive,
  platform, url, inviteCode, memberCount,
  address, city, country, geo, contactEmail, contactPhone, meetingSchedule
}`

// GetCommunities fetches active communities matching the filter, with the
// image URL rewritten through the local proxy. The result is cached.
func (c *Client) GetCommunities(ctx context.Context, f CommunityFilter) ([]Community, error) {
	conds := []string{`_type == "community"`, `isActive == true`}
	params := map[string]any{}
	if f.Kind != "" {
		conds = append(conds, `kind == $kind`)
		params["kind"] = f.Kind
	}
	if f.Language != "" {
		conds = append(conds, `language == $language`)
		params["language"] = f.Language
	}
	if f.Country != "" {
		conds = append(conds, `upper(country) == $country`)
		params["country"] = strings.ToUpper(f.Country)
	}
	groq := fmt.Sprintf(`*[%s] | order(name asc) %s`, strings.Join(conds, " && "), communityProjection)

	raw, err := c.Query(ctx, groq, params, "community")
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return []Community{}, nil
	}
	var out []Community
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("sanity: decode communities: %w", err)
	}
	for i := range out {
		if out[i].ImageURL != nil {
			proxied := c.ProxyImageURL(*out[i].ImageURL)
			out[i].ImageURL = &proxied
		}
	}
	return out, nil
}
