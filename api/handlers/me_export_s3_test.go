package handlers

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	wsaws "github.com/wikiSubmission/ws-backend/aws"
)

type fakeS3 struct {
	in  *s3.PutObjectInput
	err error
}

func (f *fakeS3) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.in = params
	if f.err != nil {
		return nil, f.err
	}
	return &s3.PutObjectOutput{}, nil
}

func testSigner(t *testing.T) *wsaws.CFSigner {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalECPrivateKey(key)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	s, err := wsaws.NewCFSigner("KEYID", "https://cdn.example.com", pemBytes)
	if err != nil {
		t.Fatalf("NewCFSigner: %v", err)
	}
	return s
}

func TestS3UploaderUploadUserExport(t *testing.T) {
	fake := &fakeS3{}
	u := S3Uploader{Client: fake, Signer: testSigner(t), Bucket: "ws-user-exports"}

	key, url, err := u.UploadUserExport(context.Background(), 42, []byte(`{"profile":{}}`))
	if err != nil {
		t.Fatalf("UploadUserExport: %v", err)
	}

	// Deterministic per-user key so a new export overwrites the old data.
	if key != "exports/user-42/export.json" {
		t.Errorf("object key = %q", key)
	}
	if awssdk.ToString(fake.in.Bucket) != "ws-user-exports" {
		t.Errorf("bucket = %q", awssdk.ToString(fake.in.Bucket))
	}
	if awssdk.ToString(fake.in.Key) != key {
		t.Errorf("uploaded key %q != returned key %q", awssdk.ToString(fake.in.Key), key)
	}
	if awssdk.ToString(fake.in.ContentType) != "application/json" {
		t.Errorf("content type = %q", awssdk.ToString(fake.in.ContentType))
	}
	if !strings.HasPrefix(awssdk.ToString(fake.in.ContentDisposition), "attachment;") {
		t.Errorf("content disposition = %q", awssdk.ToString(fake.in.ContentDisposition))
	}
	if !strings.HasPrefix(url, "https://cdn.example.com/exports/user-42/export.json?") {
		t.Errorf("download url = %q", url)
	}
	if !strings.Contains(url, "Signature=") {
		t.Errorf("download url not signed: %q", url)
	}
}

func TestS3UploaderUploadFails(t *testing.T) {
	fake := &fakeS3{err: errors.New("access denied")}
	u := S3Uploader{Client: fake, Signer: testSigner(t), Bucket: "b"}

	if _, _, err := u.UploadUserExport(context.Background(), 1, []byte("{}")); err == nil {
		t.Fatal("expected error when S3 upload fails")
	}
}
