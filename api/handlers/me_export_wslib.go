package handlers

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
	"strconv"
	"time"
)

const (
	// wslibExportTTL is how long the signed download link stays valid. ws-lib
	// caps this server-side; it should match the export object lifecycle.
	wslibExportTTL = 30 * 24 * time.Hour
	// wslibExportDisposition forces browsers to download rather than render. The
	// header is stored on the S3 object by ws-lib, so it applies even though the
	// link is cross-origin (where the HTML download attribute is ignored).
	wslibExportDisposition = `attachment; filename="wikisubmission-export.json"`
	wslibStorePath         = "/private/store"
)

// WSLibExportUploader stores the export via ws-lib's authenticated /private/store
// endpoint, which owns all S3 + CloudFront access. ws-backend holds no AWS
// credentials or signing keys: only the ws-lib base URL and a shared HMAC secret.
type WSLibExportUploader struct {
	BaseURL string
	Secret  []byte
	Client  *http.Client
}

type wslibStoreRequest struct {
	Key           string `json:"key"`
	ContentBase64 string `json:"content_base64"`
	ContentType   string `json:"content_type"`
	Disposition   string `json:"disposition"`
	TTLSeconds    int64  `json:"ttl_seconds"`
}

type wslibStoreResponse struct {
	Key string `json:"key"`
	URL string `json:"url"`
}

func (u WSLibExportUploader) UploadUserExport(ctx context.Context, userID int, payload []byte) (string, string, error) {
	key := fmt.Sprintf("private/exports/user-%d/export.json", userID)
	body, err := json.Marshal(wslibStoreRequest{
		Key:           key,
		ContentBase64: base64.StdEncoding.EncodeToString(payload),
		ContentType:   "application/json",
		Disposition:   wslibExportDisposition,
		TTLSeconds:    int64(wslibExportTTL.Seconds()),
	})
	if err != nil {
		return "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.BaseURL+wslibStorePath, bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	// HMAC over METHOD\nPATH\nTIMESTAMP, matching ws-lib's signature middleware.
	// The header names mirror that middleware (X-Health-*); the shared secret is
	// STORE_AUTH_SECRET, distinct from the health secret.
	ts := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	message := http.MethodPost + "\n" + wslibStorePath + "\n" + ts
	mac := hmac.New(sha256.New, u.Secret)
	mac.Write([]byte(message))
	req.Header.Set("X-Health-Timestamp", ts)
	req.Header.Set("X-Health-Signature", "v1="+hex.EncodeToString(mac.Sum(nil)))

	client := u.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("ws-lib store returned %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed wslibStoreResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", "", fmt.Errorf("ws-lib store: invalid response: %w", err)
	}
	if parsed.URL == "" {
		return "", "", fmt.Errorf("ws-lib store: empty url in response")
	}
	return parsed.Key, parsed.URL, nil
}
