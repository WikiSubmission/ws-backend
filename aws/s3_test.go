package aws

import (
	"context"
	"errors"
	"io"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type fakePutObjectAPI struct {
	gotInput *s3.PutObjectInput
	gotBody  []byte
	err      error
}

func (f *fakePutObjectAPI) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.gotInput = params
	if params.Body != nil {
		f.gotBody, _ = io.ReadAll(params.Body)
	}
	if f.err != nil {
		return nil, f.err
	}
	return &s3.PutObjectOutput{}, nil
}

func TestPutObject(t *testing.T) {
	t.Run("sends bucket, key, body, and metadata", func(t *testing.T) {
		fake := &fakePutObjectAPI{}
		body := []byte(`{"hello":"world"}`)

		err := PutObject(context.Background(), fake, "ws-user-exports",
			"exports/user-7/export.json", body,
			"application/json", `attachment; filename="export.json"`)
		if err != nil {
			t.Fatalf("PutObject: %v", err)
		}

		in := fake.gotInput
		if in == nil {
			t.Fatal("PutObject never called")
		}
		if awssdk.ToString(in.Bucket) != "ws-user-exports" {
			t.Errorf("bucket = %q", awssdk.ToString(in.Bucket))
		}
		if awssdk.ToString(in.Key) != "exports/user-7/export.json" {
			t.Errorf("key = %q", awssdk.ToString(in.Key))
		}
		if awssdk.ToString(in.ContentType) != "application/json" {
			t.Errorf("contentType = %q", awssdk.ToString(in.ContentType))
		}
		if awssdk.ToString(in.ContentDisposition) != `attachment; filename="export.json"` {
			t.Errorf("contentDisposition = %q", awssdk.ToString(in.ContentDisposition))
		}
		if string(fake.gotBody) != string(body) {
			t.Errorf("body = %q", fake.gotBody)
		}
	})

	t.Run("omits empty metadata", func(t *testing.T) {
		fake := &fakePutObjectAPI{}
		if err := PutObject(context.Background(), fake, "b", "k", []byte("x"), "", ""); err != nil {
			t.Fatalf("PutObject: %v", err)
		}
		if fake.gotInput.ContentType != nil {
			t.Errorf("expected nil ContentType, got %q", awssdk.ToString(fake.gotInput.ContentType))
		}
		if fake.gotInput.ContentDisposition != nil {
			t.Errorf("expected nil ContentDisposition, got %q", awssdk.ToString(fake.gotInput.ContentDisposition))
		}
	})

	t.Run("wraps upload error", func(t *testing.T) {
		fake := &fakePutObjectAPI{err: errors.New("access denied")}
		err := PutObject(context.Background(), fake, "b", "k", []byte("x"), "", "")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, fake.err) {
			t.Errorf("error not wrapped: %v", err)
		}
	})
}
