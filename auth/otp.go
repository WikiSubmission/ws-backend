package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

const (
	// otpPurposeClaim marks the pending token as an email-OTP token so it cannot
	// be confused with a session token even if it ever reached the access path.
	otpPurposeClaim = "purpose"
	otpPurpose      = "email_otp"
	// otpCodeHashClaim carries the HMAC of the emailed code (never the code).
	otpCodeHashClaim = "ch"

	// DefaultOTPTTL bounds how long an emailed code stays valid, matching the
	// web app's 10-minute pending-code window.
	DefaultOTPTTL = 10 * time.Minute

	// otpCodeDigits is the length of the numeric code emailed to the user.
	otpCodeDigits = 6
)

// ErrOTPNotConfigured is returned by a disabled manager (empty secret).
var ErrOTPNotConfigured = errors.New("auth: otp manager not configured")

// ErrOTPInvalid is returned when a code fails to verify (bad code, expired or
// already-used token). It is deliberately generic so the caller does not leak
// which of those conditions failed.
var ErrOTPInvalid = errors.New("auth: invalid or expired code")

// OTPManager issues and verifies short-lived one-time email codes for passwordless
// sign-in. It is stateless with respect to the code itself: the plaintext code is
// emailed to the user and a signed "pending" token carrying only an HMAC of the
// code is handed back to the client, mirroring the web app's stateless flow (which
// stores the same payload in an httpOnly cookie). Single-use is enforced by
// tracking spent token IDs in memory.
//
// The pending token is signed with a key derived from the shared backend secret
// via a domain separator, so it is provably distinct from the session-signing key
// and can never be replayed as an access token against AuthJSVerifier.
type OTPManager struct {
	signKey jwk.Key       // HS256 key for the pending token (derived, session-distinct)
	hmacKey []byte        // key for hashing the emailed code
	ttl     time.Duration // pending-token lifetime
	now     func() time.Time

	mu   sync.Mutex
	used map[string]time.Time // jti -> expiry, for single-use enforcement
}

// NewOTPManager builds a manager from the shared backend secret. An empty secret
// yields a disabled manager (Configured() == false) whose methods return
// ErrOTPNotConfigured, matching NewMinter / NewAuthJSVerifier boot behaviour.
func NewOTPManager(secret string, ttl time.Duration) (*OTPManager, error) {
	if secret == "" {
		return &OTPManager{}, nil
	}
	if ttl <= 0 {
		ttl = DefaultOTPTTL
	}
	// Domain-separated derivation: the OTP signing key must differ from the raw
	// session key so a pending token is never a valid session token.
	signRaw := deriveKey(secret, "ws-otp-pending-v1")
	key, err := jwk.FromRaw(signRaw)
	if err != nil {
		return nil, err
	}
	if err := key.Set(jwk.AlgorithmKey, jwa.HS256); err != nil {
		return nil, err
	}
	return &OTPManager{
		signKey: key,
		hmacKey: deriveKey(secret, "ws-otp-code-v1"),
		ttl:     ttl,
		now:     time.Now,
		used:    make(map[string]time.Time),
	}, nil
}

// Configured reports whether the manager can issue and verify codes.
func (m *OTPManager) Configured() bool { return m != nil && m.signKey != nil }

func (m *OTPManager) clock() time.Time {
	if m.now != nil {
		return m.now()
	}
	return time.Now()
}

// GenerateCode returns a fresh cryptographically-random numeric code, with
// leading zeros preserved (e.g. "007421").
func (m *OTPManager) GenerateCode() (string, error) {
	max := big.NewInt(1)
	for i := 0; i < otpCodeDigits; i++ {
		max.Mul(max, big.NewInt(10))
	}
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", err
	}
	s := n.String()
	if len(s) < otpCodeDigits {
		s = strings.Repeat("0", otpCodeDigits-len(s)) + s
	}
	return s, nil
}

// IssueToken signs a pending token binding the given email to the HMAC of code.
// The returned token is handed to the client and presented back to Verify.
func (m *OTPManager) IssueToken(email, code string) (string, error) {
	if !m.Configured() {
		return "", ErrOTPNotConfigured
	}
	jti, err := randomID()
	if err != nil {
		return "", err
	}
	now := m.clock()
	tok, err := jwt.NewBuilder().
		Subject(strings.ToLower(strings.TrimSpace(email))).
		JwtID(jti).
		IssuedAt(now).
		Expiration(now.Add(m.ttl)).
		Claim(otpPurposeClaim, otpPurpose).
		Claim(otpCodeHashClaim, m.hashCode(code)).
		Build()
	if err != nil {
		return "", err
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.HS256, m.signKey))
	if err != nil {
		return "", err
	}
	return string(signed), nil
}

// Verify validates the pending token against the submitted code and, on success,
// returns the bound email. The token is single-use: a second Verify with the same
// token fails even if the code is correct. All failure modes collapse to
// ErrOTPInvalid so the caller cannot distinguish wrong-code from expired/replayed.
func (m *OTPManager) Verify(token, code string) (string, error) {
	if !m.Configured() {
		return "", ErrOTPNotConfigured
	}
	tok, err := jwt.Parse(
		[]byte(token),
		jwt.WithKey(jwa.HS256, m.signKey),
		jwt.WithValidate(true),
		jwt.WithAcceptableSkew(30*time.Second),
		jwt.WithClock(jwt.ClockFunc(m.clock)),
	)
	if err != nil {
		return "", ErrOTPInvalid
	}
	if p, _ := stringClaim(tok, otpPurposeClaim); p != otpPurpose {
		return "", ErrOTPInvalid
	}
	wantHash, _ := stringClaim(tok, otpCodeHashClaim)
	gotHash := m.hashCode(code)
	if subtle.ConstantTimeCompare([]byte(wantHash), []byte(gotHash)) != 1 {
		return "", ErrOTPInvalid
	}
	// Single-use: consume the jti only after the code matches, so a wrong guess
	// does not burn the (still-valid) token.
	if !m.consume(tok.JwtID(), tok.Expiration()) {
		return "", ErrOTPInvalid
	}
	return tok.Subject(), nil
}

func (m *OTPManager) hashCode(code string) string {
	mac := hmac.New(sha256.New, m.hmacKey)
	mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}

// consume records the jti as spent, returning false if it was already spent.
// Expired entries are swept opportunistically to bound the map's growth.
func (m *OTPManager) consume(jti string, exp time.Time) bool {
	if jti == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock()
	for k, v := range m.used {
		if now.After(v) {
			delete(m.used, k)
		}
	}
	if _, seen := m.used[jti]; seen {
		return false
	}
	m.used[jti] = exp
	return true
}

// deriveKey produces a 32-byte key bound to secret and a label, so multiple
// independent keys can be derived from one shared secret without collision.
func deriveKey(secret, label string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(label))
	return mac.Sum(nil)
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
