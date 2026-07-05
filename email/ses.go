// Package email delivers transactional email. The only current use is the mobile
// email-OTP sign-in code. Delivery goes through Amazon SES's SMTP endpoint using
// the standard library's net/smtp, so no AWS SDK dependency is pulled in and the
// same AWS_SES_* credentials the web app already uses are reused as-is (the SMTP
// password is derived from the SES secret access key).
package email

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"strings"
	"time"
)

// Sender delivers a plain-text email. Implementations are safe for concurrent use.
type Sender interface {
	Send(ctx context.Context, to, subject, body string) error
}

const (
	defaultRegion  = "us-east-1"
	defaultFrom    = "noreply@wikisubmission.org"
	smtpSubmission = 587
	dialTimeout    = 10 * time.Second
)

// SESSMTPSender sends mail through the SES SMTP submission endpoint.
type SESSMTPSender struct {
	host     string
	addr     string
	username string
	password string
	from     string
}

// NewSESSenderFromEnv builds a sender from the AWS_SES_* environment (the same
// variables the web app reads). It returns (sender, true) when SES is configured,
// or (nil, false) when credentials are absent so the caller can fall back to a
// dev-only log path, matching the web /api/auth/send-otp behaviour.
func NewSESSenderFromEnv() (*SESSMTPSender, bool) {
	accessKey := strings.TrimSpace(os.Getenv("AWS_SES_ACCESS_KEY_ID"))
	secretKey := strings.TrimSpace(os.Getenv("AWS_SES_SECRET_ACCESS_KEY"))
	if accessKey == "" || secretKey == "" {
		return nil, false
	}
	region := strings.TrimSpace(os.Getenv("AWS_SES_REGION"))
	if region == "" {
		region = defaultRegion
	}
	from := strings.TrimSpace(os.Getenv("AWS_SES_FROM_ADDRESS"))
	if from == "" {
		from = defaultFrom
	}
	host := fmt.Sprintf("email-smtp.%s.amazonaws.com", region)
	return &SESSMTPSender{
		host:     host,
		addr:     fmt.Sprintf("%s:%d", host, smtpSubmission),
		username: accessKey,
		password: sesSMTPPassword(secretKey, region),
		from:     from,
	}, true
}

// Send delivers the message. It dials with a timeout, upgrades to TLS via
// STARTTLS, authenticates with the derived SMTP credentials, and sends.
func (s *SESSMTPSender) Send(ctx context.Context, to, subject, body string) error {
	conn, err := (&net.Dialer{Timeout: dialTimeout}).DialContext(ctx, "tcp", s.addr)
	if err != nil {
		return fmt.Errorf("email: dial %s: %w", s.addr, err)
	}

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("email: smtp client: %w", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.StartTLS(&tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}); err != nil {
		return fmt.Errorf("email: starttls: %w", err)
	}
	if err := client.Auth(smtp.PlainAuth("", s.username, s.password, s.host)); err != nil {
		return fmt.Errorf("email: auth: %w", err)
	}
	if err := client.Mail(s.from); err != nil {
		return fmt.Errorf("email: mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("email: rcpt to: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("email: data: %w", err)
	}
	if _, err := w.Write(buildMessage(s.from, to, subject, body)); err != nil {
		_ = w.Close()
		return fmt.Errorf("email: write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("email: close data: %w", err)
	}
	return client.Quit()
}

// buildMessage assembles an RFC 5322 plain-text message with UTF-8 headers.
func buildMessage(from, to, subject, body string) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
	return []byte(b.String())
}

// sesSMTPPassword derives the SES SMTP password from the IAM secret access key
// and region, per AWS's documented SigV4-based algorithm. The SMTP username is
// the IAM access key id; only the password needs derivation.
func sesSMTPPassword(secretKey, region string) string {
	const (
		date     = "11111111"
		service  = "ses"
		terminal = "aws4_request"
		message  = "SendRawEmail"
	)
	sign := func(key []byte, msg string) []byte {
		h := hmac.New(sha256.New, key)
		h.Write([]byte(msg))
		return h.Sum(nil)
	}
	sig := sign([]byte("AWS4"+secretKey), date)
	sig = sign(sig, region)
	sig = sign(sig, service)
	sig = sign(sig, terminal)
	sig = sign(sig, message)
	// Version 0x04 prefix, then the signature bytes, base64-encoded.
	return base64.StdEncoding.EncodeToString(append([]byte{0x04}, sig...))
}
