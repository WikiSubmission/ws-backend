package auth

import (
	"errors"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// AuthJSVerifier verifies HS256 JWTs minted by the Auth.js frontend layer.
type AuthJSVerifier struct {
	key jwk.Key
}

// NewAuthJSVerifier creates a verifier from the shared backend secret.
// Returns a disabled verifier (always errors) when secret is empty.
func NewAuthJSVerifier(secret string) (*AuthJSVerifier, error) {
	if secret == "" {
		return &AuthJSVerifier{}, nil
	}
	key, err := jwk.FromRaw([]byte(secret))
	if err != nil {
		return nil, err
	}
	if err := key.Set(jwk.AlgorithmKey, jwa.HS256); err != nil {
		return nil, err
	}
	return &AuthJSVerifier{key: key}, nil
}

// VerifyToken parses and validates the HS256 JWT, returning (authID, email, error).
// authID is the namespaced subject claim, e.g. "google:12345" or "email:user@x.com".
func (v *AuthJSVerifier) VerifyToken(tokenString string) (authID string, email string, err error) {
	if v.key == nil {
		return "", "", errors.New("auth: verifier not configured")
	}
	token, err := jwt.Parse(
		[]byte(tokenString),
		jwt.WithKey(jwa.HS256, v.key),
		jwt.WithValidate(true),
		jwt.WithAcceptableSkew(30*time.Second),
	)
	if err != nil {
		return "", "", err
	}
	authID = token.Subject()
	if e, ok := token.Get("email"); ok {
		email, _ = e.(string)
	}
	return authID, email, nil
}
