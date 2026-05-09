package sanity

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeSanity returns a test server that matches Sanity's
// /v{api}/data/query/{dataset} contract and returns the supplied result.
func fakeSanity(t *testing.T, projectID, dataset string, result string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := fmt.Sprintf("/v%s/data/query/%s", defaultAPIVersion, dataset)
		if !strings.HasPrefix(r.URL.Path, "/v") || !strings.Contains(r.URL.Path, "/data/query/"+dataset) {
			t.Errorf("unexpected path: got=%q want prefix=%q", r.URL.Path, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, fmt.Sprintf(`{"result": %s, "ms": 1, "query": ""}`, result))
	}))
	return srv
}

// newTestClient builds a Client whose HTTP transport is rerouted through the
// supplied test server.
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c, err := New(Config{
		ProjectID:  "puy3ce80",
		Dataset:    "production",
		APIVersion: defaultAPIVersion,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.http = srv.Client()
	// Redirect requests to the test server by replacing the host segment in the
	// URL via a custom transport. Cheaper than fully mocking.
	c.http.Transport = rewriteTransport{base: srv.URL}
	return c
}

type rewriteTransport struct{ base string }

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.URL.Scheme = "http"
	r.URL.Host = strings.TrimPrefix(rt.base, "http://")
	r.Host = r.URL.Host
	return http.DefaultTransport.RoundTrip(r)
}

func TestGetCommunities_OnlineRewritesImageURL(t *testing.T) {
	const projectID = "puy3ce80"
	const dataset = "production"
	const result = `[{
		"_id": "drafts.community-foo",
		"kind": "online",
		"name": "Submitters Discord",
		"slug": "submitters-discord",
		"language": "en",
		"description": "Test",
		"imageUrl": "https://cdn.sanity.io/images/puy3ce80/production/abc-1024x768.jpg",
		"isActive": true,
		"platform": "discord",
		"url": "https://discord.gg/example"
	}]`
	srv := fakeSanity(t, projectID, dataset, result)
	defer srv.Close()
	c := newTestClient(t, srv)

	got, err := c.GetCommunities(context.Background(), CommunityFilter{Kind: "online"})
	if err != nil {
		t.Fatalf("GetCommunities: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Kind != "online" || got[0].Name != "Submitters Discord" {
		t.Errorf("decoded = %+v", got[0])
	}
	if got[0].ImageURL == nil || *got[0].ImageURL != "/sanity/image/abc-1024x768.jpg" {
		t.Errorf("imageURL = %v, want proxied path", got[0].ImageURL)
	}
}

func TestGetCommunities_EmptyResultIsEmptySlice(t *testing.T) {
	srv := fakeSanity(t, "puy3ce80", "production", "null")
	defer srv.Close()
	c := newTestClient(t, srv)

	got, err := c.GetCommunities(context.Background(), CommunityFilter{})
	if err != nil {
		t.Fatalf("GetCommunities: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestProxyImageURL_NonSanityURL_PassesThrough(t *testing.T) {
	c, err := New(Config{ProjectID: "puy3ce80", Dataset: "production"})
	if err != nil {
		t.Fatal(err)
	}
	out := c.ProxyImageURL("https://example.com/foo.png")
	if out != "https://example.com/foo.png" {
		t.Errorf("got=%q, want untouched", out)
	}
}

func TestVerifyWebhook_RoundTrip(t *testing.T) {
	c, err := New(Config{ProjectID: "p", Dataset: "d", WebhookSecret: "shhh"})
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"_id":"x","_type":"community","operation":"update"}`)
	ts := time.Now().UnixMilli()

	mac := hmac.New(sha256.New, []byte("shhh"))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	header := fmt.Sprintf("t=%d,v1=%s", ts, base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))

	got, err := c.VerifyWebhook(header, body)
	if err != nil {
		t.Fatalf("VerifyWebhook: %v", err)
	}
	if got.ID != "x" || got.Type != "community" || got.Operation != "update" {
		t.Errorf("payload = %+v", got)
	}
}

func TestVerifyWebhook_RejectsTamperedBody(t *testing.T) {
	c, err := New(Config{ProjectID: "p", Dataset: "d", WebhookSecret: "shhh"})
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Now().UnixMilli()
	body := []byte(`{"_id":"x"}`)

	mac := hmac.New(sha256.New, []byte("shhh"))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	header := fmt.Sprintf("t=%d,v1=%s", ts, base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))

	if _, err := c.VerifyWebhook(header, []byte(`{"_id":"y"}`)); err == nil {
		t.Fatalf("expected signature mismatch error")
	}
}

func TestVerifyWebhook_RejectsStaleTimestamp(t *testing.T) {
	c, err := New(Config{ProjectID: "p", Dataset: "d", WebhookSecret: "shhh"})
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{}`)
	ts := time.Now().Add(-10 * time.Minute).UnixMilli()
	mac := hmac.New(sha256.New, []byte("shhh"))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	header := fmt.Sprintf("t=%d,v1=%s", ts, base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))

	if _, err := c.VerifyWebhook(header, body); err == nil {
		t.Fatalf("expected stale-timestamp error")
	}
}

func TestCacheHit(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"result": [], "ms": 1, "query": ""}`)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	for i := 0; i < 3; i++ {
		if _, err := c.Query(context.Background(), `*[_type=="x"]`, nil, "x"); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (cache should serve subsequent reads)", calls)
	}

	c.InvalidateByType("x")
	if _, err := c.Query(context.Background(), `*[_type=="x"]`, nil, "x"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2 after invalidation", calls)
	}
}

func TestQueryParamsAreEncoded(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"result": [], "ms": 1, "query": ""}`)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	_, err := c.Query(context.Background(), `*[_type==$t]`, map[string]any{"t": "community"}, "community")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotURL, `%24t=%22community%22`) {
		t.Errorf("query did not include $t param: %s", gotURL)
	}
}

// silence unused import if json is later needed
var _ = json.Marshal
