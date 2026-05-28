package auth

import (
	"errors"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/wikiSubmission/ws-backend/api/observability"
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

// VerifyToken parses and validates the HS256 JWT, returning (authID, email, name, error).
// authID is the namespaced subject claim, e.g. "google:12345" or "email:user@x.com".
// name is the optional human-readable display name carried by the Auth.js layer.
func (v *AuthJSVerifier) VerifyToken(tokenString string) (authID string, email string, name string, err error) {
	if v.key == nil {
		observability.JWTValidationsTotal.WithLabelValues("not_configured").Inc()
		return "", "", "", errors.New("auth: verifier not configured")
	}
	token, err := jwt.Parse(
		[]byte(tokenString),
		jwt.WithKey(jwa.HS256, v.key),
		jwt.WithValidate(true),
		jwt.WithAcceptableSkew(30*time.Second),
	)
	if err != nil {
		observability.JWTValidationsTotal.WithLabelValues(classifyJWTError(err)).Inc()
		return "", "", "", err
	}
	observability.JWTValidationsTotal.WithLabelValues("valid").Inc()
	authID = token.Subject()
	if e, ok := token.Get("email"); ok {
		email, _ = e.(string)
	}
	if n, ok := token.Get("name"); ok {
		name, _ = n.(string)
	}
	return authID, email, name, nil
}

func classifyJWTError(err error) string {
	switch {
	case errors.Is(err, jwt.ErrTokenExpired()):
		return "expired"
	case errors.Is(err, jwt.ErrTokenNotYetValid()):
		return "not_yet_valid"
	default:
		return "invalid"
	}
}
