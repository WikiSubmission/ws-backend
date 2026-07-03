package publish

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestUploadSendsSignedStoreRequest(t *testing.T) {
	secret := []byte("test-secret")
	payload := []byte("sqlite-bytes")

	var got struct {
		method, path, contentType string
		timestamp, signature      string
		body                      storePublicRequest
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.path = r.URL.Path
		got.contentType = r.Header.Get("Content-Type")
		got.timestamp = r.Header.Get("X-Health-Timestamp")
		got.signature = r.Header.Get("X-Health-Signature")
		if err := json.NewDecoder(r.Body).Decode(&got.body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		json.NewEncoder(w).Encode(map[string]string{
			"key": got.body.Key,
			"url": "https://cdn.wikisubmission.org/" + got.body.Key,
		})
	}))
	defer srv.Close()

	store := WSLibPublicStore{BaseURL: srv.URL, Secret: secret, Client: srv.Client()}
	url, err := store.Upload(context.Background(), "offline/quran-en-v1.db", "application/octet-stream", BundleCacheControl, payload)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if url != "https://cdn.wikisubmission.org/offline/quran-en-v1.db" {
		t.Fatalf("unexpected url: %s", url)
	}

	if got.method != http.MethodPost || got.path != "/private/store-public" {
		t.Fatalf("expected POST /private/store-public, got %s %s", got.method, got.path)
	}
	if got.contentType != "application/json" {
		t.Fatalf("unexpected content type header: %s", got.contentType)
	}

	// Body fields: key under offline/, declared content type + cache control,
	// payload round-trips through base64.
	if !strings.HasPrefix(got.body.Key, "offline/") {
		t.Fatalf("key not under offline/: %s", got.body.Key)
	}
	if got.body.ContentType != "application/octet-stream" {
		t.Fatalf("unexpected content_type: %s", got.body.ContentType)
	}
	if got.body.CacheControl != BundleCacheControl {
		t.Fatalf("unexpected cache_control: %s", got.body.CacheControl)
	}
	decoded, err := base64.StdEncoding.DecodeString(got.body.ContentBase64)
	if err != nil || string(decoded) != string(payload) {
		t.Fatalf("payload did not round-trip: %q, err=%v", decoded, err)
	}

	// HMAC headers: timestamp is current, signature verifies over
	// METHOD\nPATH\nTIMESTAMP with the shared secret.
	unix, err := strconv.ParseInt(got.timestamp, 10, 64)
	if err != nil {
		t.Fatalf("invalid timestamp %q: %v", got.timestamp, err)
	}
	if drift := time.Since(time.Unix(unix, 0)); drift < -30*time.Second || drift > 30*time.Second {
		t.Fatalf("timestamp drift too large: %v", drift)
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("POST\n/private/store-public\n" + got.timestamp))
	expected := "v1=" + hex.EncodeToString(mac.Sum(nil))
	if got.signature != expected {
		t.Fatalf("signature mismatch: got %s, want %s", got.signature, expected)
	}
}

func TestUploadErrorPaths(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		wantSub string
	}{
		{
			name: "non-200 status surfaces body",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, `{"error":"key must be under a public prefix"}`, http.StatusBadRequest)
			},
			wantSub: "returned 400",
		},
		{
			name: "empty url rejected",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`{"key":"offline/x","url":""}`))
			},
			wantSub: "empty url",
		},
		{
			name: "invalid json rejected",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`not-json`))
			},
			wantSub: "invalid response",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			store := WSLibPublicStore{BaseURL: srv.URL, Secret: []byte("s"), Client: srv.Client()}
			_, err := store.Upload(context.Background(), "offline/x", "application/json", ManifestCacheControl, []byte("{}"))
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("expected error containing %q, got %v", tc.wantSub, err)
			}
		})
	}
}

func TestKeyPrefixFromBaseURL(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"default cdn url", "https://cdn.wikisubmission.org/offline", "offline", false},
		{"trailing slash", "https://cdn.wikisubmission.org/offline/", "offline", false},
		{"nested prefix", "https://cdn.wikisubmission.org/offline/v2", "offline/v2", false},
		{"no path", "https://cdn.wikisubmission.org", "", true},
		{"root path only", "https://cdn.wikisubmission.org/", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := KeyPrefixFromBaseURL(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %q", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("prefix mismatch: got %q, want %q", got, tc.want)
			}
		})
	}
}
