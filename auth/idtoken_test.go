package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// newTestKeySet returns a public JWKS plus the matching private signing key.
// Both carry the same kid so jwt.WithKeySet can select the verification key.
func newTestKeySet(t *testing.T) (jwk.Set, jwk.Key) {
	t.Helper()
	raw, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	priv, err := jwk.FromRaw(raw)
	if err != nil {
		t.Fatalf("jwk.FromRaw: %v", err)
	}
	_ = priv.Set(jwk.KeyIDKey, "test-kid")
	_ = priv.Set(jwk.AlgorithmKey, jwa.RS256)

	pub, err := priv.PublicKey()
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}
	set := jwk.NewSet()
	if err := set.AddKey(pub); err != nil {
		t.Fatalf("AddKey: %v", err)
	}
	return set, priv
}

type claims struct {
	iss           string
	aud           string
	sub           string
	email         string
	name          string
	nonce         string
	emailVerified any
	exp           time.Time
}

func signIDToken(t *testing.T, key jwk.Key, c claims) string {
	t.Helper()
	now := time.Now()
	exp := c.exp
	if exp.IsZero() {
		exp = now.Add(time.Hour)
	}
	b := jwt.NewBuilder().
		Issuer(c.iss).
		Audience([]string{c.aud}).
		Subject(c.sub).
		IssuedAt(now).
		Expiration(exp)
	if c.email != "" {
		b = b.Claim("email", c.email)
	}
	if c.name != "" {
		b = b.Claim("name", c.name)
	}
	if c.nonce != "" {
		b = b.Claim("nonce", c.nonce)
	}
	if c.emailVerified != nil {
		b = b.Claim("email_verified", c.emailVerified)
	}
	tok, err := b.Build()
	if err != nil {
		t.Fatalf("build token: %v", err)
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, key))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return string(signed)
}

func TestGoogleVerifierValid(t *testing.T) {
	set, key := newTestKeySet(t)
	v := NewGoogleVerifier(set, []string{"ios-client.apps.googleusercontent.com"})

	tok := signIDToken(t, key, claims{
		iss:           "https://accounts.google.com",
		aud:           "ios-client.apps.googleusercontent.com",
		sub:           "1029384756",
		email:         "user@example.com",
		name:          "User Example",
		emailVerified: true,
	})

	id, err := v.Verify(context.Background(), tok, "")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if id.Subject != "1029384756" || id.Email != "user@example.com" || id.Name != "User Example" {
		t.Fatalf("unexpected identity: %+v", id)
	}
	if !id.EmailVerified {
		t.Fatal("expected EmailVerified true")
	}
}

func TestGoogleVerifierRejectsWrongAudience(t *testing.T) {
	set, key := newTestKeySet(t)
	v := NewGoogleVerifier(set, []string{"expected-client"})
	tok := signIDToken(t, key, claims{
		iss: "https://accounts.google.com", aud: "attacker-client",
		sub: "1", email: "u@example.com", emailVerified: true,
	})
	if _, err := v.Verify(context.Background(), tok, ""); err == nil {
		t.Fatal("expected wrong-audience rejection")
	}
}

func TestGoogleVerifierRejectsWrongIssuer(t *testing.T) {
	set, key := newTestKeySet(t)
	v := NewGoogleVerifier(set, []string{"client"})
	tok := signIDToken(t, key, claims{
		iss: "https://evil.example.com", aud: "client",
		sub: "1", email: "u@example.com", emailVerified: true,
	})
	if _, err := v.Verify(context.Background(), tok, ""); err == nil {
		t.Fatal("expected wrong-issuer rejection")
	}
}

func TestGoogleVerifierRejectsExpired(t *testing.T) {
	set, key := newTestKeySet(t)
	v := NewGoogleVerifier(set, []string{"client"})
	tok := signIDToken(t, key, claims{
		iss: "https://accounts.google.com", aud: "client",
		sub: "1", email: "u@example.com", emailVerified: true,
		exp: time.Now().Add(-2 * time.Hour),
	})
	if _, err := v.Verify(context.Background(), tok, ""); err == nil {
		t.Fatal("expected expired rejection")
	}
}

func TestGoogleVerifierRejectsBadSignature(t *testing.T) {
	set, _ := newTestKeySet(t)
	// Sign with a key that is NOT in the verifier's key set.
	_, otherKey := newTestKeySet(t)
	v := NewGoogleVerifier(set, []string{"client"})
	tok := signIDToken(t, otherKey, claims{
		iss: "https://accounts.google.com", aud: "client",
		sub: "1", email: "u@example.com", emailVerified: true,
	})
	if _, err := v.Verify(context.Background(), tok, ""); err == nil {
		t.Fatal("expected bad-signature rejection")
	}
}

func TestGoogleVerifierRejectsUnverifiedEmail(t *testing.T) {
	set, key := newTestKeySet(t)
	v := NewGoogleVerifier(set, []string{"client"})
	tok := signIDToken(t, key, claims{
		iss: "https://accounts.google.com", aud: "client",
		sub: "1", email: "u@example.com", emailVerified: false,
	})
	if _, err := v.Verify(context.Background(), tok, ""); err == nil {
		t.Fatal("expected unverified-email rejection")
	}
}

func TestAppleVerifierNonce(t *testing.T) {
	set, key := newTestKeySet(t)
	v := NewAppleVerifier(set, []string{"org.wikisubmission.app"})

	// Raw nonce form.
	tok := signIDToken(t, key, claims{
		iss: "https://appleid.apple.com", aud: "org.wikisubmission.app",
		sub: "apple-sub-1", email: "u@privaterelay.appleid.com", emailVerified: true, nonce: "rawnonce123",
	})
	if _, err := v.Verify(context.Background(), tok, "rawnonce123"); err != nil {
		t.Fatalf("raw nonce should verify: %v", err)
	}

	// Hashed nonce form: token carries sha256hex(nonce).
	sum := sha256.Sum256([]byte("rawnonce123"))
	hashed := signIDToken(t, key, claims{
		iss: "https://appleid.apple.com", aud: "org.wikisubmission.app",
		sub: "apple-sub-1", nonce: hex.EncodeToString(sum[:]),
	})
	if _, err := v.Verify(context.Background(), hashed, "rawnonce123"); err != nil {
		t.Fatalf("hashed nonce should verify: %v", err)
	}

	// Mismatch.
	if _, err := v.Verify(context.Background(), tok, "wrong-nonce"); err == nil {
		t.Fatal("expected nonce mismatch rejection")
	}

	// Missing nonce in token.
	missing := signIDToken(t, key, claims{
		iss: "https://appleid.apple.com", aud: "org.wikisubmission.app", sub: "apple-sub-1",
	})
	if _, err := v.Verify(context.Background(), missing, "rawnonce123"); err == nil {
		t.Fatal("expected rejection when token has no nonce")
	}
}

// Apple must reject a token that asserts an email it has not verified, so an
// unverified email can never be bound onto an account.
func TestAppleVerifierRejectsUnverifiedEmail(t *testing.T) {
	set, key := newTestKeySet(t)
	v := NewAppleVerifier(set, []string{"org.wikisubmission.app"})

	tok := signIDToken(t, key, claims{
		iss: "https://appleid.apple.com", aud: "org.wikisubmission.app",
		sub: "apple-sub-1", email: "victim@example.com", emailVerified: false, nonce: "n1",
	})
	if _, err := v.Verify(context.Background(), tok, "n1"); err == nil {
		t.Fatal("expected rejection of unverified email")
	}
}

// A repeat Apple login omits the email entirely; that token must still verify
// (the account is later resolved by subject), so email_verified enforcement
// must not fire when there is no email.
func TestAppleVerifierAllowsMissingEmail(t *testing.T) {
	set, key := newTestKeySet(t)
	v := NewAppleVerifier(set, []string{"org.wikisubmission.app"})

	tok := signIDToken(t, key, claims{
		iss: "https://appleid.apple.com", aud: "org.wikisubmission.app",
		sub: "apple-sub-1", nonce: "n1",
	})
	id, err := v.Verify(context.Background(), tok, "n1")
	if err != nil {
		t.Fatalf("repeat login without email should verify: %v", err)
	}
	if id.Subject != "apple-sub-1" || id.Email != "" {
		t.Fatalf("unexpected identity: %+v", id)
	}
}

func TestVerifierNotConfigured(t *testing.T) {
	var v *OIDCVerifier
	if _, err := v.Verify(context.Background(), "x", ""); err == nil {
		t.Fatal("nil verifier should error")
	}
	empty := &OIDCVerifier{}
	if _, err := empty.Verify(context.Background(), "x", ""); err == nil {
		t.Fatal("unconfigured verifier should error")
	}
}
