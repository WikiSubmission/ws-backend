package auth

import (
	"testing"
	"time"
)

const otpTestSecret = "otp-secret-otp-secret-otp-secret-123456"

func TestOTPRoundTrip(t *testing.T) {
	m, err := NewOTPManager(otpTestSecret, time.Minute)
	if err != nil {
		t.Fatalf("NewOTPManager: %v", err)
	}
	if !m.Configured() {
		t.Fatal("manager should be configured")
	}

	code, err := m.GenerateCode()
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	if len(code) != otpCodeDigits {
		t.Fatalf("code %q length = %d, want %d", code, len(code), otpCodeDigits)
	}

	token, err := m.IssueToken("User@Example.com", code)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}

	email, err := m.Verify(token, code)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if email != "user@example.com" {
		t.Fatalf("email = %q, want normalized lowercase", email)
	}
}

func TestOTPWrongCode(t *testing.T) {
	m, _ := NewOTPManager(otpTestSecret, time.Minute)
	token, _ := m.IssueToken("a@b.c", "123456")

	if _, err := m.Verify(token, "000000"); err != ErrOTPInvalid {
		t.Fatalf("wrong code err = %v, want ErrOTPInvalid", err)
	}
	// A wrong guess must not burn the token: the correct code still works.
	if _, err := m.Verify(token, "123456"); err != nil {
		t.Fatalf("correct code after wrong guess: %v", err)
	}
}

func TestOTPBruteForceBurnsToken(t *testing.T) {
	m, _ := NewOTPManager(otpTestSecret, time.Minute)
	token, _ := m.IssueToken("a@b.c", "314159")

	// Exhaust the wrong-guess allowance.
	for i := 0; i < otpMaxAttempts; i++ {
		if _, err := m.Verify(token, "000000"); err != ErrOTPInvalid {
			t.Fatalf("wrong guess %d err = %v, want ErrOTPInvalid", i, err)
		}
	}
	// The token is now burned: even the correct code must be rejected.
	if _, err := m.Verify(token, "314159"); err != ErrOTPInvalid {
		t.Fatalf("correct code after cap err = %v, want ErrOTPInvalid (token burned)", err)
	}
}

func TestOTPSingleUse(t *testing.T) {
	m, _ := NewOTPManager(otpTestSecret, time.Minute)
	token, _ := m.IssueToken("a@b.c", "424242")

	if _, err := m.Verify(token, "424242"); err != nil {
		t.Fatalf("first verify: %v", err)
	}
	if _, err := m.Verify(token, "424242"); err != ErrOTPInvalid {
		t.Fatalf("replay err = %v, want ErrOTPInvalid", err)
	}
}

func TestOTPExpired(t *testing.T) {
	m, _ := NewOTPManager(otpTestSecret, time.Minute)
	base := time.Now()
	m.now = func() time.Time { return base }
	token, _ := m.IssueToken("a@b.c", "111111")

	// Advance beyond ttl + skew.
	m.now = func() time.Time { return base.Add(2 * time.Minute) }
	if _, err := m.Verify(token, "111111"); err != ErrOTPInvalid {
		t.Fatalf("expired err = %v, want ErrOTPInvalid", err)
	}
}

// A pending OTP token must never be accepted as a session/access token: it is
// signed with a domain-separated key, not the raw session secret.
func TestOTPTokenNotAcceptedAsSession(t *testing.T) {
	m, _ := NewOTPManager(otpTestSecret, time.Minute)
	token, _ := m.IssueToken("a@b.c", "999999")

	v, _ := NewAuthJSVerifier(otpTestSecret)
	if _, _, _, err := v.VerifyToken(token); err == nil {
		t.Fatal("OTP pending token must not verify as a session token")
	}
}

// A token from a different secret must not verify.
func TestOTPForeignSecret(t *testing.T) {
	m1, _ := NewOTPManager(otpTestSecret, time.Minute)
	m2, _ := NewOTPManager("another-secret-another-secret-abcdef", time.Minute)
	token, _ := m1.IssueToken("a@b.c", "555555")

	if _, err := m2.Verify(token, "555555"); err != ErrOTPInvalid {
		t.Fatalf("foreign-secret err = %v, want ErrOTPInvalid", err)
	}
}

func TestOTPDisabled(t *testing.T) {
	m, err := NewOTPManager("", time.Minute)
	if err != nil {
		t.Fatalf("NewOTPManager empty: %v", err)
	}
	if m.Configured() {
		t.Fatal("empty-secret manager should be disabled")
	}
	if _, err := m.IssueToken("a@b.c", "1"); err != ErrOTPNotConfigured {
		t.Fatalf("IssueToken err = %v, want ErrOTPNotConfigured", err)
	}
	if _, err := m.Verify("t", "1"); err != ErrOTPNotConfigured {
		t.Fatalf("Verify err = %v, want ErrOTPNotConfigured", err)
	}
}
