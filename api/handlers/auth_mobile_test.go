package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wikiSubmission/ws-backend/auth"
	"github.com/wikiSubmission/ws-backend/db"
)

const mobileTestSecret = "test-secret-test-secret-test-secret-1234"

// fakeStore is an in-memory nativeAuthStore for handler tests.
type fakeStore struct {
	users     map[string]*db.UserRow
	upsertErr error
	fetchErr  error
	// template, when set, is returned by UpsertUser with auth_id/email applied,
	// so a test can control role / permissions / active state.
	template *db.UserRow
}

func (f *fakeStore) UpsertUser(_ context.Context, authID, email string, displayName *string) (*db.UserRow, error) {
	if f.upsertErr != nil {
		return nil, f.upsertErr
	}
	u := &db.UserRow{ID: 1, AuthID: authID, Email: email, Role: "member", IsActive: true}
	if f.template != nil {
		clone := *f.template
		clone.AuthID = authID
		clone.Email = email
		u = &clone
	}
	if displayName != nil && u.DisplayName == nil {
		u.DisplayName = displayName
	}
	if f.users == nil {
		f.users = map[string]*db.UserRow{}
	}
	f.users[authID] = u
	return u, nil
}

func (f *fakeStore) FetchUserByAuthID(_ context.Context, authID string) (*db.UserRow, error) {
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	return f.users[authID], nil
}

type stubVerifier struct {
	identity auth.VerifiedIdentity
	err      error
}

func (s stubVerifier) Verify(_ context.Context, _, _ string) (auth.VerifiedIdentity, error) {
	return s.identity, s.err
}

func newTestHandler(t *testing.T, store nativeAuthStore, google, apple auth.IDTokenVerifier) *NativeAuthHandler {
	t.Helper()
	minter, err := auth.NewMinter(mobileTestSecret)
	if err != nil {
		t.Fatalf("NewMinter: %v", err)
	}
	verifier, err := auth.NewAuthJSVerifier(mobileTestSecret)
	if err != nil {
		t.Fatalf("NewAuthJSVerifier: %v", err)
	}
	return &NativeAuthHandler{
		Store:      store,
		Google:     google,
		Apple:      apple,
		Minter:     minter,
		Verifier:   verifier,
		AccessTTL:  time.Hour,
		RefreshTTL: 24 * time.Hour,
	}
}

func doJSON(t *testing.T, h *NativeAuthHandler, route, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	switch route {
	case "exchange":
		r.POST("/api/v1/auth/mobile/exchange", h.Exchange)
	case "refresh":
		r.POST("/api/v1/auth/mobile/refresh", h.Refresh)
	}
	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeAuth(t *testing.T, w *httptest.ResponseRecorder) mobileAuthResponse {
	t.Helper()
	var resp mobileAuthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	return resp
}

func TestExchangeGoogleSuccess(t *testing.T) {
	store := &fakeStore{}
	google := stubVerifier{identity: auth.VerifiedIdentity{
		Subject: "1029384756", Email: "user@example.com", Name: "User Example", EmailVerified: true,
	}}
	h := newTestHandler(t, store, google, nil)

	w := doJSON(t, h, "exchange", "/api/v1/auth/mobile/exchange", mobileExchangeRequest{
		Provider: "google", IDToken: "any",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	resp := decodeAuth(t, w)
	if resp.TokenType != "Bearer" || resp.ExpiresIn != 3600 {
		t.Fatalf("unexpected token envelope: %+v", resp)
	}
	if resp.User.AuthID != "google:1029384756" || resp.User.Email != "user@example.com" {
		t.Fatalf("unexpected user: %+v", resp.User)
	}
	if resp.User.IsAdmin || resp.User.IsEditor {
		t.Fatalf("member should not be admin/editor: %+v", resp.User)
	}

	// The minted access token must verify against the shared verifier with the
	// namespaced auth id, proving it is interchangeable with a web session token.
	v, _ := auth.NewAuthJSVerifier(mobileTestSecret)
	authID, email, _, err := v.VerifyToken(resp.AccessToken)
	if err != nil {
		t.Fatalf("minted access token failed verification: %v", err)
	}
	if authID != "google:1029384756" || email != "user@example.com" {
		t.Fatalf("token claims mismatch: %q %q", authID, email)
	}

	// The refresh token must NOT pass on the access path.
	if _, _, _, err := v.VerifyToken(resp.RefreshToken); err == nil {
		t.Fatal("refresh token must not verify as an access token")
	}
}

func TestExchangeAdminFlags(t *testing.T) {
	store := &fakeStore{template: &db.UserRow{ID: 7, Role: "admin", IsActive: true}}
	google := stubVerifier{identity: auth.VerifiedIdentity{Subject: "x", Email: "a@b.c", EmailVerified: true}}
	h := newTestHandler(t, store, google, nil)

	w := doJSON(t, h, "exchange", "/api/v1/auth/mobile/exchange", mobileExchangeRequest{Provider: "google", IDToken: "t"})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	resp := decodeAuth(t, w)
	if !resp.User.IsAdmin || !resp.User.IsEditor {
		t.Fatalf("admin should be admin and editor: %+v", resp.User)
	}
}

func TestExchangeRejections(t *testing.T) {
	valid := stubVerifier{identity: auth.VerifiedIdentity{Subject: "s", Email: "e@x.c", EmailVerified: true}}

	cases := []struct {
		name   string
		google auth.IDTokenVerifier
		apple  auth.IDTokenVerifier
		body   mobileExchangeRequest
		want   int
	}{
		{"unsupported provider", valid, valid, mobileExchangeRequest{Provider: "facebook", IDToken: "t"}, http.StatusBadRequest},
		{"missing id token", valid, valid, mobileExchangeRequest{Provider: "google"}, http.StatusBadRequest},
		{"provider not configured", nil, valid, mobileExchangeRequest{Provider: "google", IDToken: "t"}, http.StatusServiceUnavailable},
		{"invalid id token", stubVerifier{err: errors.New("bad")}, valid, mobileExchangeRequest{Provider: "google", IDToken: "t"}, http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(t, &fakeStore{}, tc.google, tc.apple)
			w := doJSON(t, h, "exchange", "/api/v1/auth/mobile/exchange", tc.body)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d (body=%s)", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestExchangeAppleNoEmailFallback(t *testing.T) {
	// Repeat Apple login: token has no email, but the row exists from first login.
	store := &fakeStore{users: map[string]*db.UserRow{
		"apple:appleSub": {ID: 3, AuthID: "apple:appleSub", Email: "first@example.com", Role: "member", IsActive: true},
	}}
	apple := stubVerifier{identity: auth.VerifiedIdentity{Subject: "appleSub"}} // no email
	h := newTestHandler(t, store, nil, apple)

	w := doJSON(t, h, "exchange", "/api/v1/auth/mobile/exchange", mobileExchangeRequest{
		Provider: "apple", IDToken: "t", Nonce: "n",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	resp := decodeAuth(t, w)
	if resp.User.Email != "first@example.com" {
		t.Fatalf("expected existing email, got %q", resp.User.Email)
	}

	// First Apple login with no email and no existing row must fail.
	store2 := &fakeStore{}
	h2 := newTestHandler(t, store2, nil, apple)
	w2 := doJSON(t, h2, "exchange", "/api/v1/auth/mobile/exchange", mobileExchangeRequest{Provider: "apple", IDToken: "t", Nonce: "n"})
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for first apple login without email, got %d", w2.Code)
	}
}

func TestRefreshFlow(t *testing.T) {
	store := &fakeStore{users: map[string]*db.UserRow{
		"google:42": {ID: 9, AuthID: "google:42", Email: "u@example.com", Role: "member", IsActive: true},
	}}
	h := newTestHandler(t, store, nil, nil)

	id := auth.Identity{AuthID: "google:42", Email: "u@example.com", Name: "u"}
	refreshTok, err := h.Minter.MintRefreshToken(id, time.Hour, time.Now())
	if err != nil {
		t.Fatalf("mint refresh: %v", err)
	}

	w := doJSON(t, h, "refresh", "/api/v1/auth/mobile/refresh", mobileRefreshRequest{RefreshToken: refreshTok})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	resp := decodeAuth(t, w)
	v, _ := auth.NewAuthJSVerifier(mobileTestSecret)
	if _, _, _, err := v.VerifyToken(resp.AccessToken); err != nil {
		t.Fatalf("refreshed access token invalid: %v", err)
	}

	// An access token presented to /refresh must be rejected.
	accessTok, _ := h.Minter.MintAccessToken(id, time.Hour, time.Now())
	wBad := doJSON(t, h, "refresh", "/api/v1/auth/mobile/refresh", mobileRefreshRequest{RefreshToken: accessTok})
	if wBad.Code != http.StatusUnauthorized {
		t.Fatalf("access token on refresh path: status = %d, want 401", wBad.Code)
	}

	// Empty body rejected.
	wEmpty := doJSON(t, h, "refresh", "/api/v1/auth/mobile/refresh", mobileRefreshRequest{})
	if wEmpty.Code != http.StatusBadRequest {
		t.Fatalf("empty refresh: status = %d, want 400", wEmpty.Code)
	}
}

func TestRefreshInactiveUser(t *testing.T) {
	store := &fakeStore{users: map[string]*db.UserRow{
		"google:42": {ID: 9, AuthID: "google:42", Email: "u@example.com", Role: "member", IsActive: false},
	}}
	h := newTestHandler(t, store, nil, nil)
	id := auth.Identity{AuthID: "google:42", Email: "u@example.com", Name: "u"}
	refreshTok, _ := h.Minter.MintRefreshToken(id, time.Hour, time.Now())

	w := doJSON(t, h, "refresh", "/api/v1/auth/mobile/refresh", mobileRefreshRequest{RefreshToken: refreshTok})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("inactive user refresh: status = %d, want 401", w.Code)
	}
}
