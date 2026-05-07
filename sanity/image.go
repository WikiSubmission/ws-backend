package sanity

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ImageProxyPrefix is the URL path under which Sanity image assets are served
// by ws-backend. It must match the route registered in api/server.go.
const ImageProxyPrefix = "/sanity/image/"

// ProxyImageURL rewrites a Sanity CDN URL into the ws-backend image proxy
// path. URLs that do not match the expected Sanity shape are returned
// unchanged.
//
// Input:  https://cdn.sanity.io/images/<projectId>/<dataset>/<file>?<query>
// Output: /sanity/image/<file>?<query>
func (c *Client) ProxyImageURL(sanityURL string) string {
	prefix := fmt.Sprintf("https://cdn.sanity.io/images/%s/%s/", c.cfg.ProjectID, c.cfg.Dataset)
	if !strings.HasPrefix(sanityURL, prefix) {
		return sanityURL
	}
	return ImageProxyPrefix + sanityURL[len(prefix):]
}

// FetchImage proxies a single image asset by its sub-path and forwards the
// query string verbatim (preserves Sanity image transforms like ?w=512&h=512).
// Returns the upstream body, content-type, and content-length for the caller
// to write to the response.
func (c *Client) FetchImage(ctx context.Context, subPath, rawQuery string) (io.ReadCloser, string, int64, error) {
	if subPath == "" {
		return nil, "", 0, errors.New("sanity: empty image path")
	}
	if strings.Contains(subPath, "..") {
		return nil, "", 0, errors.New("sanity: invalid image path")
	}
	upstream := fmt.Sprintf(
		"https://cdn.sanity.io/images/%s/%s/%s",
		c.cfg.ProjectID, c.cfg.Dataset, strings.TrimLeft(subPath, "/"),
	)
	if rawQuery != "" {
		upstream += "?" + rawQuery
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream, nil)
	if err != nil {
		return nil, "", 0, fmt.Errorf("sanity: build image request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", 0, fmt.Errorf("sanity: fetch image: %w", err)
	}
	if resp.StatusCode >= 400 {
		_ = resp.Body.Close()
		return nil, "", 0, fmt.Errorf("sanity: upstream image %d", resp.StatusCode)
	}
	return resp.Body, resp.Header.Get("Content-Type"), resp.ContentLength, nil
}
