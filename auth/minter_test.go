package auth

import (
	"testing"
	"time"
)

// testSecret is a fixed 32+ char HS256 secret for the round-trip tests.
const testSecret = "test-secret-test-secret-test-secret-1234"

func TestMinterDisabledWithoutSecret(t *testing.T) {
	m, err := NewMinter("")
	if err != nil {
		t.Fatalf("NewMinter(\"\"): %v", err)
	}
	if m.Configured() {
		t.Fatal("minter built from empty secret should not be configured")
	}
	if _, err := m.MintAccessToken(Identity{AuthID: "google:1"}, time.Hour, time.Now()); err != ErrMinterNotConfigured {
		t.Fatalf("expected ErrMinterNotConfigured, got %v", err)
	}
}

func TestMintAccessTokenRoundTrip(t *testing.T) {
	m, err := NewMinter(testSecret)
	if err != nil {
		t.Fatalf("NewMinter: %v", err)
	}
	v, err := NewAuthJSVerifier(testSecret)
	if err != nil {
		t.Fatalf("NewAuthJSVerifier: %v", err)
	}

	id := Identity{AuthID: "google:12345", Email: "user@example.com", Name: "User"}
	tok, err := m.MintAccessToken(id, time.Hour, time.Now())
	if err != nil {
		t.Fatalf("MintAccessToken: %v", err)
	}

	authID, email, name, err := v.VerifyToken(tok)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if authID != id.AuthID || email != id.Email || name != id.Name {
		t.Fatalf("round trip mismatch: got %q/%q/%q want %q/%q/%q",
			authID, email, name, id.AuthID, id.Email, id.Name)
	}
}

func TestExpiredAccessTokenRejected(t *testing.T) {
	m, _ := NewMinter(testSecret)
	v, _ := NewAuthJSVerifier(testSecret)
	id := Identity{AuthID: "google:1", Email: "u@example.com", Name: "U"}
	// Issued in the past and already expired (well beyond the 30s skew).
	tok, err := m.MintAccessToken(id, -time.Hour, time.Now())
	if err != nil {
		t.Fatalf("MintAccessToken: %v", err)
	}
	if _, _, _, err := v.VerifyToken(tok); err == nil {
		t.Fatal("expected expired access token to be rejected")
	}
}

func TestRefreshTokenRejectedOnAccessPath(t *testing.T) {
	m, _ := NewMinter(testSecret)
	v, _ := NewAuthJSVerifier(testSecret)
	id := Identity{AuthID: "apple:abc", Email: "a@example.com", Name: "A"}
	rt, err := m.MintRefreshToken(id, time.Hour, time.Now())
	if err != nil {
		t.Fatalf("MintRefreshToken: %v", err)
	}
	if _, _, _, err := v.VerifyToken(rt); err == nil {
		t.Fatal("expected refresh token to be rejected on the access path")
	}
}

func TestVerifyRefreshToken(t *testing.T) {
	m, _ := NewMinter(testSecret)
	v, _ := NewAuthJSVerifier(testSecret)
	id := Identity{AuthID: "apple:abc", Email: "a@example.com", Name: "A"}

	rt, err := m.MintRefreshToken(id, time.Hour, time.Now())
	if err != nil {
		t.Fatalf("MintRefreshToken: %v", err)
	}
	got, err := v.VerifyRefreshToken(rt)
	if err != nil {
		t.Fatalf("VerifyRefreshToken: %v", err)
	}
	if got.AuthID != id.AuthID || got.Email != id.Email || got.Name != id.Name {
		t.Fatalf("refresh identity mismatch: got %+v want %+v", got, id)
	}

	// An access token must not validate as a refresh token.
	at, _ := m.MintAccessToken(id, time.Hour, time.Now())
	if _, err := v.VerifyRefreshToken(at); err == nil {
		t.Fatal("expected access token to be rejected as a refresh token")
	}

	// An expired refresh token must be rejected.
	expired, _ := m.MintRefreshToken(id, -time.Hour, time.Now())
	if _, err := v.VerifyRefreshToken(expired); err == nil {
		t.Fatal("expected expired refresh token to be rejected")
	}
}
