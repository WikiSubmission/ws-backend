package handlers

import (
	"context"
	"fmt"
	"time"

	wsaws "github.com/wikiSubmission/ws-backend/aws"
)

const (
	// exportSignedURLTTL is how long the emailed CloudFront download link stays
	// valid. It matches the S3 lifecycle expiration on the exports prefix.
	exportSignedURLTTL = 30 * 24 * time.Hour
	exportContentType  = "application/json"
	// exportDisposition forces browsers to download rather than render the JSON.
	exportDisposition = `attachment; filename="wikisubmission-export.json"`
)

// S3Uploader stores user data exports in a private S3 bucket and returns a
// time-limited CloudFront signed URL. The object key is deterministic per user,
// so each export overwrites the previous one.
type S3Uploader struct {
	Client wsaws.S3PutObjectAPI
	Signer *wsaws.CFSigner
	Bucket string
}

// exportObjectKey is the stable per-user key. Keeping it deterministic means a
// new export overwrites the old data, and an S3 lifecycle rule on the "exports/"
// prefix purges it after 30 days.
func exportObjectKey(userID int) string {
	return fmt.Sprintf("exports/user-%d/export.json", userID)
}

func (u S3Uploader) UploadUserExport(ctx context.Context, userID int, payload []byte) (string, string, error) {
	key := exportObjectKey(userID)
	if err := wsaws.PutObject(ctx, u.Client, u.Bucket, key, payload, exportContentType, exportDisposition); err != nil {
		return "", "", err
	}
	url, err := u.Signer.GetSignedURL(key, exportSignedURLTTL)
	if err != nil {
		return "", "", err
	}
	return key, url, nil
}
