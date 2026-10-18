package instance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Config belongs to private state and encrypted instance backups, never status output.
type S3Config struct {
	Endpoint  string `json:"endpoint"`
	Bucket    string `json:"bucket"`
	Region    string `json:"region,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
}

// S3Destination uploads and rereads the complete object to verify exact bytes.
type S3Destination struct {
	client *minio.Client
	config S3Config
}

func NewS3Destination(config S3Config) (*S3Destination, error) {
	u, err := url.Parse(config.Endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("S3 endpoint must be an HTTP(S) origin without credentials or a path")
	}
	if config.Bucket == "" || config.AccessKey == "" || config.SecretKey == "" {
		return nil, fmt.Errorf("S3 bucket and credentials are required")
	}
	client, err := minio.New(u.Host, &minio.Options{Creds: credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""), Secure: u.Scheme == "https", Region: config.Region, MaxRetries: 3})
	if err != nil {
		return nil, fmt.Errorf("invalid S3 destination")
	}
	return &S3Destination{client, config}, nil
}

func (s *S3Destination) key(object string) string {
	return strings.Trim(strings.Trim(s.config.Prefix, "/")+"/"+object, "/")
}

// PutVerified succeeds only when the uploaded object decrypts to the same verified
// archive bytes as the local recovery point. ETags are not treated as SHA-256 hashes.
func (s *S3Destination) PutVerified(ctx context.Context, object, file, expectedSHA256 string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	upload, err := s.client.PutObject(ctx, s.config.Bucket, s.key(object), f, info.Size(), minio.PutObjectOptions{ContentType: "application/octet-stream"})
	if err != nil {
		return s3Error(ctx, "upload", err)
	}
	options := minio.GetObjectOptions{}
	options.VersionID = upload.VersionID
	remote, err := s.client.GetObject(ctx, s.config.Bucket, s.key(object), options)
	if err != nil {
		return s3Error(ctx, "verification download", err)
	}
	defer remote.Close()
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(remote, info.Size()+1))
	if err != nil {
		return s3Error(ctx, "verification download", err)
	}
	if n != info.Size() || hex.EncodeToString(hash.Sum(nil)) != expectedSHA256 {
		return fmt.Errorf("external backup bytes differ from the verified local archive")
	}
	return nil
}

func (s *S3Destination) Delete(ctx context.Context, object string) error {
	if err := s.client.RemoveObject(ctx, s.config.Bucket, s.key(object), minio.RemoveObjectOptions{}); err != nil {
		return s3Error(ctx, "delete", err)
	}
	return nil
}

func s3Error(ctx context.Context, action string, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	code := minio.ToErrorResponse(err).Code
	if code == "" {
		code = "destination unavailable"
	}
	return fmt.Errorf("S3 %s failed: %s; check the endpoint, bucket and access permissions", action, code)
}
