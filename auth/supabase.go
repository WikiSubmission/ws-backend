package auth

import (
	"context"
	"errors"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// JWKSCache holds a refreshing JWKS cache for verifying Supabase JWTs.
type JWKSCache struct {
	cache *jwk.Cache
	url   string
}

// NewJWKSCache initialises a JWKS cache that auto-refreshes.
// Call once at startup. If the URL is empty, returns a no-op that
// always returns an error (auth disabled).
func NewJWKSCache(ctx context.Context, jwksURL string) (*JWKSCache, error) {
	if jwksURL == "" {
		return &JWKSCache{}, nil
	}
	c := jwk.NewCache(ctx)
	if err := c.Register(jwksURL, jwk.WithRefreshInterval(15*time.Minute)); err != nil {
		return nil, err
	}
	// Force first fetch so startup fails fast on misconfiguration.
	if _, err := c.Refresh(ctx, jwksURL); err != nil {
		return nil, err
	}
	return &JWKSCache{cache: c, url: jwksURL}, nil
}

// VerifyToken verifies a Supabase JWT and returns (supabase_uid, email, error).
func (j *JWKSCache) VerifyToken(tokenString string) (uid string, email string, err error) {
	if j.cache == nil {
		return "", "", errors.New("auth: JWKS not configured")
	}
	set, err := j.cache.Get(context.Background(), j.url)
	if err != nil {
		return "", "", err
	}
	token, err := jwt.Parse([]byte(tokenString), jwt.WithKeySet(set), jwt.WithValidate(true))
	if err != nil {
		return "", "", err
	}
	uid = token.Subject()
	if e, ok := token.Get("email"); ok {
		email, _ = e.(string)
	}
	return uid, email, nil
}
