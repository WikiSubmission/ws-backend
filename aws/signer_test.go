package aws

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

// testECPEM generates a fresh SEC1-encoded EC private key in PEM form.
func testECPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
}

func TestNewCFSigner(t *testing.T) {
	t.Run("parses EC key and trims trailing slash", func(t *testing.T) {
		s, err := NewCFSigner("KEY123", "https://cdn.example.com/", testECPEM(t))
		if err != nil {
			t.Fatalf("NewCFSigner: %v", err)
		}
		if s.BaseURL != "https://cdn.example.com" {
			t.Errorf("BaseURL not trimmed, got %q", s.BaseURL)
		}
		if s.KeyID != "KEY123" || s.PrivateKey == nil {
			t.Errorf("signer not populated: %+v", s)
		}
	})

	t.Run("rejects invalid PEM", func(t *testing.T) {
		if _, err := NewCFSigner("k", "https://cdn.example.com", []byte("not a pem")); err == nil {
			t.Fatal("expected error for invalid PEM, got nil")
		}
	})
}

func TestGetSignedURL(t *testing.T) {
	s, err := NewCFSigner("KEY123", "https://cdn.example.com", testECPEM(t))
	if err != nil {
		t.Fatalf("NewCFSigner: %v", err)
	}

	signed, err := s.GetSignedURL("exports/user-7/export.json", 30*24*time.Hour)
	if err != nil {
		t.Fatalf("GetSignedURL: %v", err)
	}

	if !strings.HasPrefix(signed, "https://cdn.example.com/exports/user-7/export.json?") {
		t.Errorf("unexpected URL prefix: %s", signed)
	}
	for _, param := range []string{"Expires=", "Signature=", "Key-Pair-Id=KEY123"} {
		if !strings.Contains(signed, param) {
			t.Errorf("signed URL missing %q: %s", param, signed)
		}
	}
}

func TestGetSignedURLEscapesKey(t *testing.T) {
	s, err := NewCFSigner("KEY123", "https://cdn.example.com", testECPEM(t))
	if err != nil {
		t.Fatalf("NewCFSigner: %v", err)
	}
	signed, err := s.GetSignedURL("exports/grr cat.json", time.Hour)
	if err != nil {
		t.Fatalf("GetSignedURL: %v", err)
	}
	if strings.Contains(signed, "grr cat") {
		t.Errorf("space not escaped in path: %s", signed)
	}
	if !strings.Contains(signed, "grr%20cat") {
		t.Errorf("expected percent-escaped space: %s", signed)
	}
}
