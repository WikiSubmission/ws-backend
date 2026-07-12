package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// Provider JWKS endpoints and issuers. The key sets are fetched (and cached) at
// startup; the issuer lists are validated against the parsed token.
const (
	GoogleJWKSURL = "https://www.googleapis.com/oauth2/v3/certs"
	AppleJWKSURL  = "https://appleid.apple.com/auth/keys"
)

var (
	googleIssuers = []string{"https://accounts.google.com", "accounts.google.com"}
	appleIssuers  = []string{"https://appleid.apple.com"}
)

// VerifiedIdentity is the trusted subset of claims extracted from a provider ID
// token after signature, expiry, issuer, audience and nonce validation pass.
type VerifiedIdentity struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

// IDTokenVerifier validates a provider-issued OIDC ID token. Implementations
// are safe for concurrent use.
type IDTokenVerifier interface {
	Verify(ctx context.Context, idToken, nonce string) (VerifiedIdentity, error)
}

// OIDCVerifier verifies an ID token against a JWKS key set. It is provider
// agnostic: Google and Apple differ only in issuer, audience and whether a
// nonce is required. The key set is injected so tests can supply an in-memory
// JWKS instead of fetching one over the network.
type OIDCVerifier struct {
	keySet               jwk.Set
	issuers              []string
	audiences            []string
	requireNonce         bool
	requireEmailVerified bool
}

// NewGoogleVerifier verifies Google ID tokens. audiences must list the OAuth
// client IDs (iOS, Android, web) the mobile app authenticates with.
func NewGoogleVerifier(keySet jwk.Set, audiences []string) *OIDCVerifier {
	return &OIDCVerifier{
		keySet:               keySet,
		issuers:              googleIssuers,
		audiences:            audiences,
		requireEmailVerified: true,
	}
}

// NewAppleVerifier verifies Sign in with Apple identity tokens. audiences must
// list the app bundle/services IDs. A nonce is always required: native Sign in
// with Apple supplies one and it defends against token replay. Like Google, an
// asserted email must be verified before it can key an account (Apple sends
// email_verified alongside the email on first sign-in); repeat logins that omit
// the email entirely are unaffected.
func NewAppleVerifier(keySet jwk.Set, audiences []string) *OIDCVerifier {
	return &OIDCVerifier{
		keySet:               keySet,
		issuers:              appleIssuers,
		audiences:            audiences,
		requireNonce:         true,
		requireEmailVerified: true,
	}
}

// Verify validates the token and returns the trusted identity. The ctx is
// accepted for cancellation symmetry with future remote work; parsing itself is
// synchronous against the already-cached key set.
func (v *OIDCVerifier) Verify(_ context.Context, idToken, nonce string) (VerifiedIdentity, error) {
	if v == nil || v.keySet == nil {
		return VerifiedIdentity{}, errors.New("auth: id token verifier not configured")
	}

	tok, err := jwt.Parse(
		[]byte(idToken),
		jwt.WithKeySet(v.keySet),
		jwt.WithValidate(true),
		jwt.WithAcceptableSkew(60*time.Second),
	)
	if err != nil {
		return VerifiedIdentity{}, fmt.Errorf("auth: id token invalid: %w", err)
	}

	if !containsString(v.issuers, tok.Issuer()) {
		return VerifiedIdentity{}, fmt.Errorf("auth: unexpected issuer %q", tok.Issuer())
	}
	if !audienceAllowed(tok.Audience(), v.audiences) {
		return VerifiedIdentity{}, errors.New("auth: token audience not allowed")
	}
	if v.requireNonce {
		claimNonce, _ := stringClaim(tok, "nonce")
		if !nonceMatches(claimNonce, nonce) {
			return VerifiedIdentity{}, errors.New("auth: nonce mismatch")
		}
	}

	id := VerifiedIdentity{Subject: tok.Subject()}
	if id.Subject == "" {
		return VerifiedIdentity{}, errors.New("auth: token missing subject")
	}
	id.Email, _ = stringClaim(tok, "email")
	id.Name, _ = stringClaim(tok, "name")
	id.EmailVerified = boolClaim(tok, "email_verified")
	// Only enforce email_verified when the token actually asserts an email:
	// binding an unverified email onto an account is the risk. Apple omits the
	// email on repeat logins, and those tokens must still verify (the account is
	// then resolved by subject, not email).
	if v.requireEmailVerified && id.Email != "" && !id.EmailVerified {
		return VerifiedIdentity{}, errors.New("auth: email not verified")
	}

	return id, nil
}

// NewCachedKeySet registers a JWKS URL with a background-refreshing cache and
// returns a key set that always reflects the latest fetched keys. It refreshes
// once eagerly so a bad URL or unreachable endpoint fails at startup rather
// than on the first sign-in. The cache lives for the lifetime of ctx.
func NewCachedKeySet(ctx context.Context, url string) (jwk.Set, error) {
	cache := jwk.NewCache(ctx)
	if err := cache.Register(url); err != nil {
		return nil, err
	}
	if _, err := cache.Refresh(ctx, url); err != nil {
		return nil, fmt.Errorf("auth: warming JWKS %s: %w", url, err)
	}
	return jwk.NewCachedSet(cache, url), nil
}

// nonceMatches accepts either the raw nonce or its SHA-256 hex digest. Sign in
// with Apple returns the hashed nonce in some flows and the raw value in others,
// so both forms are honoured.
func nonceMatches(claim, provided string) bool {
	if claim == "" || provided == "" {
		return false
	}
	if claim == provided {
		return true
	}
	sum := sha256.Sum256([]byte(provided))
	return claim == hex.EncodeToString(sum[:])
}

func audienceAllowed(tokenAuds, allowed []string) bool {
	for _, a := range tokenAuds {
		if containsString(allowed, a) {
			return true
		}
	}
	return false
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func stringClaim(tok jwt.Token, name string) (string, bool) {
	v, ok := tok.Get(name)
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func boolClaim(tok jwt.Token, name string) bool {
	v, ok := tok.Get(name)
	if !ok {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true"
	default:
		return false
	}
}
