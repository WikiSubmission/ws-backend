// Package publish uploads offline bundle artifacts to the public CDN through
// ws-lib's authenticated /private/store-public endpoint. Like the user-data
// export path (api/handlers/me_export_wslib.go), all S3 + CloudFront access
// stays in ws-lib: ws-backend holds only the ws-lib base URL and the shared
// HMAC secret, never AWS credentials.
//
// Unlike exports, stored objects are public and stable: keys live under the
// offline/ prefix, the returned URL is the unsigned CloudFront URL, and cache
// lifetime is controlled per object via Cache-Control.
package publish

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const storePublicPath = "/private/store-public"

// Cache-Control values for the two artifact kinds: bundle filenames are
// version-stamped and immutable, the manifest is mutable and polled.
const (
	BundleCacheControl   = "public, max-age=31536000, immutable"
	ManifestCacheControl = "public, max-age=60"
)

// WSLibPublicStore uploads objects via ws-lib's /private/store-public
// endpoint, authenticated with the same HMAC scheme as /private/store.
type WSLibPublicStore struct {
	BaseURL string
	Secret  []byte
	Client  *http.Client
}

type storePublicRequest struct {
	Key           string `json:"key"`
	ContentBase64 string `json:"content_base64"`
	ContentType   string `json:"content_type"`
	CacheControl  string `json:"cache_control"`
}

type storePublicResponse struct {
	Key string `json:"key"`
	URL string `json:"url"`
}

// Upload stores payload at key and returns the public URL ws-lib reports.
func (s WSLibPublicStore) Upload(ctx context.Context, key, contentType, cacheControl string, payload []byte) (string, error) {
	body, err := json.Marshal(storePublicRequest{
		Key:           key,
		ContentBase64: base64.StdEncoding.EncodeToString(payload),
		ContentType:   contentType,
		CacheControl:  cacheControl,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL+storePublicPath, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	// HMAC over METHOD\nPATH\nTIMESTAMP, matching ws-lib's signature
	// middleware. Header names mirror that middleware (X-Health-*); the
	// shared secret is STORE_AUTH_SECRET.
	ts := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	message := http.MethodPost + "\n" + storePublicPath + "\n" + ts
	mac := hmac.New(sha256.New, s.Secret)
	mac.Write([]byte(message))
	req.Header.Set("X-Health-Timestamp", ts)
	req.Header.Set("X-Health-Signature", "v1="+hex.EncodeToString(mac.Sum(nil)))

	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ws-lib store-public returned %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed storePublicResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("ws-lib store-public: invalid response: %w", err)
	}
	if parsed.URL == "" {
		return "", fmt.Errorf("ws-lib store-public: empty url in response")
	}
	return parsed.URL, nil
}

// KeyPrefixFromBaseURL derives the S3 key prefix from the manifest base URL,
// so uploaded keys always match the URLs the manifest advertises. For
// https://cdn.wikisubmission.org/offline it returns "offline". An empty path
// is an error: ws-lib only stores public objects under an allowlisted prefix.
func KeyPrefixFromBaseURL(baseURL string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse base url %q: %w", baseURL, err)
	}
	prefix := strings.Trim(u.Path, "/")
	if prefix == "" {
		return "", fmt.Errorf("base url %q has no path prefix; expected e.g. https://cdn.wikisubmission.org/offline", baseURL)
	}
	return prefix, nil
}
