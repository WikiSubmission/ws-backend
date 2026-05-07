// Package sanity is a small read-only proxy in front of the Sanity content
// lake. It exposes typed query helpers (e.g. GetCommunities) backed by an
// in-process TTL cache. The studio still talks to Sanity directly; only the
// public consumer surface flows through here.
//
// The package is pure I/O — it never imports Gin or the api package.
package sanity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultAPIVersion = "2024-01-01"
	defaultCacheTTL   = 5 * time.Minute
	defaultTimeout    = 10 * time.Second
)

// Config carries the runtime parameters for the Sanity client. All values are
// read from environment variables in NewFromEnv.
type Config struct {
	ProjectID     string
	Dataset       string
	APIVersion    string
	ReadToken     string // optional; required for draft / private datasets
	WebhookSecret string // shared secret used to verify webhook signatures
	CacheTTL      time.Duration
	HTTPTimeout   time.Duration
}

// Client is the entry point for the package. It is safe for concurrent use.
type Client struct {
	cfg   Config
	http  *http.Client
	cache *cache
}

// New constructs a Client from an explicit Config. Use NewFromEnv in normal
// startup paths.
func New(cfg Config) (*Client, error) {
	if cfg.ProjectID == "" {
		return nil, errors.New("sanity: ProjectID is required")
	}
	if cfg.Dataset == "" {
		return nil, errors.New("sanity: Dataset is required")
	}
	if cfg.APIVersion == "" {
		cfg.APIVersion = defaultAPIVersion
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = defaultCacheTTL
	}
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = defaultTimeout
	}
	return &Client{
		cfg:   cfg,
		http:  &http.Client{Timeout: cfg.HTTPTimeout},
		cache: newCache(),
	}, nil
}

// NewFromEnv reads SANITY_* environment variables and constructs a Client.
// Returns (nil, nil) when SANITY_PROJECT_ID is unset so callers can opt in
// gracefully without the proxy enabled.
func NewFromEnv() (*Client, error) {
	projectID := os.Getenv("SANITY_PROJECT_ID")
	if projectID == "" {
		return nil, nil
	}
	cfg := Config{
		ProjectID:     projectID,
		Dataset:       os.Getenv("SANITY_DATASET"),
		APIVersion:    os.Getenv("SANITY_API_VERSION"),
		ReadToken:     os.Getenv("SANITY_READ_TOKEN"),
		WebhookSecret: os.Getenv("SANITY_WEBHOOK_SECRET"),
	}
	if ttl := os.Getenv("SANITY_CACHE_TTL"); ttl != "" {
		if d, err := time.ParseDuration(ttl); err == nil {
			cfg.CacheTTL = d
		}
	}
	return New(cfg)
}

// Cfg returns the active config. Exposed for handlers that need to build
// proxy URLs or verify webhook secrets.
func (c *Client) Cfg() Config { return c.cfg }

// queryResponse is the envelope returned by Sanity's HTTP query endpoint.
type queryResponse struct {
	Result json.RawMessage `json:"result"`
	Ms     int             `json:"ms"`
	Query  string          `json:"query"`
}

// Query executes a GROQ query and returns the unmarshalled `result` field.
// The cache is keyed by (groq, params, dataset). Callers pass tags so the
// webhook can invalidate by document type.
func (c *Client) Query(ctx context.Context, groq string, params map[string]any, tags ...string) (json.RawMessage, error) {
	cacheKey := buildCacheKey(c.cfg.Dataset, groq, params)
	if hit, ok := c.cache.get(cacheKey); ok {
		return hit, nil
	}

	endpoint := fmt.Sprintf(
		"https://%s.api.sanity.io/v%s/data/query/%s",
		c.cfg.ProjectID, c.cfg.APIVersion, c.cfg.Dataset,
	)
	q := url.Values{}
	q.Set("query", groq)
	for k, v := range params {
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("sanity: marshal param %q: %w", k, err)
		}
		q.Set("$"+k, string(raw))
	}
	full := endpoint + "?" + q.Encode()

	body, err := c.do(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, err
	}
	var resp queryResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("sanity: decode response: %w", err)
	}
	c.cache.put(cacheKey, resp.Result, c.cfg.CacheTTL, tags...)
	return resp.Result, nil
}

// do issues an HTTP request with one retry on transient 5xx / network errors.
func (c *Client) do(ctx context.Context, method, urlStr string, body io.Reader) ([]byte, error) {
	const maxAttempts = 2
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, urlStr, body)
		if err != nil {
			return nil, fmt.Errorf("sanity: build request: %w", err)
		}
		if c.cfg.ReadToken != "" {
			req.Header.Set("Authorization", "Bearer "+c.cfg.ReadToken)
		}
		req.Header.Set("Accept", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("sanity: transport error: %w", err)
			slog.Warn("sanity request failed", "attempt", attempt, "error", err)
			continue
		}
		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("sanity: read response: %w", readErr)
			continue
		}
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("sanity: upstream %d: %s", resp.StatusCode, truncate(string(respBody), 200))
			continue
		}
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("sanity: upstream %d: %s", resp.StatusCode, truncate(string(respBody), 200))
		}
		return respBody, nil
	}
	return nil, lastErr
}

// InvalidateByType clears all cache entries tagged with the given Sanity
// document type (e.g. "community"). Used by the webhook receiver.
func (c *Client) InvalidateByType(docType string) {
	c.cache.invalidateByTag(docType)
}

// InvalidateByID clears cache entries tagged with the given document ID.
func (c *Client) InvalidateByID(id string) {
	c.cache.invalidateByTag(id)
}

// PostJSON is exported only for the rare case (e.g. listening / preview) where
// callers need a raw POST against Sanity. It does not cache.
func (c *Client) PostJSON(ctx context.Context, path string, payload any) ([]byte, error) {
	endpoint := fmt.Sprintf("https://%s.api.sanity.io/v%s%s", c.cfg.ProjectID, c.cfg.APIVersion, path)
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("sanity: marshal payload: %w", err)
	}
	return c.do(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
}

func buildCacheKey(dataset, groq string, params map[string]any) string {
	if len(params) == 0 {
		return dataset + "|" + groq
	}
	// Stable encoding: sort keys for deterministic cache hits.
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sortStrings(keys)
	var b strings.Builder
	b.WriteString(dataset)
	b.WriteByte('|')
	b.WriteString(groq)
	for _, k := range keys {
		raw, _ := json.Marshal(params[k])
		b.WriteByte('|')
		b.WriteString(k)
		b.WriteByte('=')
		b.Write(raw)
	}
	return b.String()
}

func sortStrings(s []string) {
	// Tiny insertion sort — keys list is always small (≤ 5).
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
