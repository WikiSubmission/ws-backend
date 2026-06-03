// Package aws provides a minimal, self-contained AWS helper surface for
// ws-backend: uploading objects to S3 and signing CloudFront URLs for private
// content. It is intentionally a small port of the shared ws-lib/aws helpers so
// the service stays buildable inside its isolated Docker context with no
// cross-repo module dependency.
package aws

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/cloudfront/sign"
)

// CFSigner generates signed, time-limited URLs for files served from a private
// CloudFront distribution. It supports RSA and ECDSA signing keys.
type CFSigner struct {
	// KeyID is the CloudFront public key ID (from the trusted key group).
	KeyID string
	// PrivateKey signs the URL (RSA or ECDSA).
	PrivateKey crypto.Signer
	// BaseURL is the CloudFront distribution domain, e.g. https://d111.cloudfront.net.
	BaseURL string
}

// LoadSigner builds a CFSigner from environment variables:
//   - CLOUDFRONT_PUBLIC_KEY_ID  (required)
//   - CLOUDFRONT_BASE_URL       (required)
//   - CLOUDFRONT_PRIVATE_KEY    (PEM content, preferred) or
//   - CLOUDFRONT_PRIVATE_KEY_PATH (path to a PEM file, fallback)
//
// The PEM content is read directly from the environment when present, avoiding
// writing the private key to disk inside the container.
func LoadSigner() (*CFSigner, error) {
	keyID := os.Getenv("CLOUDFRONT_PUBLIC_KEY_ID")
	baseURL := os.Getenv("CLOUDFRONT_BASE_URL")
	if keyID == "" || baseURL == "" {
		return nil, fmt.Errorf("missing required CloudFront environment variables")
	}

	if content := os.Getenv("CLOUDFRONT_PRIVATE_KEY"); strings.TrimSpace(content) != "" {
		return NewCFSigner(keyID, baseURL, []byte(content))
	}

	path := os.Getenv("CLOUDFRONT_PRIVATE_KEY_PATH")
	if path == "" {
		path = "private_key.pem"
	}
	privKeyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file %q: %w", path, err)
	}
	return NewCFSigner(keyID, baseURL, privKeyBytes)
}

// NewCFSigner parses a PEM-encoded private key (PKCS#8, PKCS#1, or SEC1 EC) and
// returns a ready-to-use CFSigner.
func NewCFSigner(keyID, baseURL string, privKeyPEM []byte) (*CFSigner, error) {
	block, _ := pem.Decode(privKeyPEM)
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block: invalid format")
	}

	var key any
	var err error
	if key, err = x509.ParsePKCS8PrivateKey(block.Bytes); err != nil {
		if key, err = x509.ParsePKCS1PrivateKey(block.Bytes); err != nil {
			if key, err = x509.ParseECPrivateKey(block.Bytes); err != nil {
				return nil, fmt.Errorf("failed to parse private key: %w", err)
			}
		}
	}

	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("key type %T does not implement crypto.Signer", key)
	}

	return &CFSigner{
		KeyID:      keyID,
		PrivateKey: signer,
		BaseURL:    strings.TrimSuffix(baseURL, "/"),
	}, nil
}

// publicURL builds a standard, unsigned CloudFront URL for an object key.
func (s *CFSigner) publicURL(fileKey string) string {
	return fmt.Sprintf("%s/%s", s.BaseURL, strings.TrimPrefix(fileKey, "/"))
}

// GetSignedURL returns a time-limited URL valid for the given duration. Path
// segments are escaped so keys containing spaces or special characters sign
// and resolve correctly.
func (s *CFSigner) GetSignedURL(fileKey string, duration time.Duration) (string, error) {
	parts := strings.Split(fileKey, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	escapedKey := strings.Join(parts, "/")

	resource := s.publicURL(escapedKey)
	urlSigner := sign.NewURLSigner(s.KeyID, s.PrivateKey)
	signedURL, err := urlSigner.Sign(resource, time.Now().Add(duration))
	if err != nil {
		slog.Error("CloudFront signing failed", slog.String("key", fileKey), slog.Any("error", err))
		return "", err
	}
	return signedURL, nil
}
