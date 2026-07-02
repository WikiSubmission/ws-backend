package auth

import (
	"errors"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

const (
	// tokenTypeClaim names the JWT claim that distinguishes a refresh token from
	// an access token. Access tokens omit it (matching the Auth.js web tokens),
	// so the access-path verifier stays backward compatible while still being
	// able to reject a refresh token presented as an access token.
	tokenTypeClaim = "typ"

	// TokenTypeRefresh is the tokenTypeClaim value for mobile refresh tokens.
	TokenTypeRefresh = "refresh"

	// DefaultAccessTokenTTL keeps mobile access tokens as short-lived as the web
	// session token. The long-lived mobile session is carried by the refresh
	// token, exchanged at /auth/mobile/refresh — a stolen access token expires
	// quickly, matching the web exposure window.
	DefaultAccessTokenTTL = time.Hour

	// DefaultRefreshTokenTTL bounds how long a mobile client can stay signed in
	// without re-authenticating with the provider.
	DefaultRefreshTokenTTL = 60 * 24 * time.Hour
)

// ErrMinterNotConfigured is returned by a Minter built from an empty secret.
var ErrMinterNotConfigured = errors.New("auth: minter not configured")

// Identity is the verified subject a session token is minted for. AuthID is the
// namespaced subject claim, e.g. "google:12345" or "apple:001234".
type Identity struct {
	AuthID string
	Email  string
	Name   string
}

// Minter signs the HS256 session tokens the backend already trusts. It mirrors
// the frontend Auth.js mintBackendToken so a mobile client, which can never
// hold WS_BACKEND_JWT_SECRET, can obtain an equivalent token through the
// native-auth exchange endpoint where the secret does live.
type Minter struct {
	key jwk.Key
}

// NewMinter builds a minter from the shared backend secret. An empty secret
// yields a disabled minter whose Mint methods return ErrMinterNotConfigured,
// matching NewAuthJSVerifier's boot-without-config behaviour.
func NewMinter(secret string) (*Minter, error) {
	if secret == "" {
		return &Minter{}, nil
	}
	key, err := jwk.FromRaw([]byte(secret))
	if err != nil {
		return nil, err
	}
	if err := key.Set(jwk.AlgorithmKey, jwa.HS256); err != nil {
		return nil, err
	}
	return &Minter{key: key}, nil
}

// Configured reports whether the minter can sign tokens.
func (m *Minter) Configured() bool { return m != nil && m.key != nil }

// MintAccessToken signs a short-lived access token accepted by AuthJSVerifier.
func (m *Minter) MintAccessToken(id Identity, ttl time.Duration, now time.Time) (string, error) {
	return m.mint(id, ttl, now, "")
}

// MintRefreshToken signs a long-lived refresh token (typ=refresh). It is
// rejected on the access path and is only accepted by VerifyRefreshToken.
func (m *Minter) MintRefreshToken(id Identity, ttl time.Duration, now time.Time) (string, error) {
	return m.mint(id, ttl, now, TokenTypeRefresh)
}

func (m *Minter) mint(id Identity, ttl time.Duration, now time.Time, typ string) (string, error) {
	if !m.Configured() {
		return "", ErrMinterNotConfigured
	}
	builder := jwt.NewBuilder().
		Subject(id.AuthID).
		IssuedAt(now).
		Expiration(now.Add(ttl)).
		Claim("email", id.Email)
	if id.Name != "" {
		builder = builder.Claim("name", id.Name)
	}
	if typ != "" {
		builder = builder.Claim(tokenTypeClaim, typ)
	}
	tok, err := builder.Build()
	if err != nil {
		return "", err
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.HS256, m.key))
	if err != nil {
		return "", err
	}
	return string(signed), nil
}
