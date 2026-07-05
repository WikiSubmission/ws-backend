package email

import (
	"strings"
	"testing"
)

func TestSESSMTPPasswordDeterministicAndRegionBound(t *testing.T) {
	p1 := sesSMTPPassword("secret", "us-east-1")
	p2 := sesSMTPPassword("secret", "us-east-1")
	if p1 != p2 {
		t.Fatal("derivation must be deterministic")
	}
	if p1 == sesSMTPPassword("secret", "eu-west-1") {
		t.Fatal("password must be bound to region")
	}
	if p1 == sesSMTPPassword("other", "us-east-1") {
		t.Fatal("password must be bound to secret")
	}
	// SES SMTP passwords are the base64 of a 33-byte value (0x04 + 32-byte HMAC),
	// which encodes to exactly 44 unpadded characters.
	if len(p1) != 44 {
		t.Fatalf("unexpected password length %d: %q", len(p1), p1)
	}
}

func TestNewSESSenderFromEnv(t *testing.T) {
	t.Setenv("AWS_SES_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SES_SECRET_ACCESS_KEY", "")
	if _, ok := NewSESSenderFromEnv(); ok {
		t.Fatal("expected disabled sender without credentials")
	}

	t.Setenv("AWS_SES_ACCESS_KEY_ID", "AKIA")
	t.Setenv("AWS_SES_SECRET_ACCESS_KEY", "shhh")
	t.Setenv("AWS_SES_REGION", "eu-west-2")
	s, ok := NewSESSenderFromEnv()
	if !ok {
		t.Fatal("expected configured sender")
	}
	if s.host != "email-smtp.eu-west-2.amazonaws.com" {
		t.Fatalf("host = %q", s.host)
	}
	if s.addr != "email-smtp.eu-west-2.amazonaws.com:587" {
		t.Fatalf("addr = %q", s.addr)
	}
	if s.from != defaultFrom {
		t.Fatalf("from = %q, want default", s.from)
	}
	if s.username != "AKIA" {
		t.Fatalf("username = %q", s.username)
	}
}

func TestBuildMessage(t *testing.T) {
	msg := string(buildMessage("from@x.com", "to@y.com", "Subj", "line1\nline2"))
	for _, want := range []string{
		"From: from@x.com\r\n",
		"To: to@y.com\r\n",
		"Subject: Subj\r\n",
		"Content-Type: text/plain; charset=UTF-8\r\n",
		"\r\nline1\r\nline2",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message missing %q\n---\n%s", want, msg)
		}
	}
}
