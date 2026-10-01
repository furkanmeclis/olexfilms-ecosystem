package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
)

// S3Driver stores objects on S3-compatible backends (SeaweedFS, AWS S3).
type S3Driver struct {
	client *s3.Client
	bucket string
}

// NewS3 builds an S3 driver from production S3 settings.
func NewS3(_ context.Context, cfg config.ObjectStoreConfig) (*S3Driver, error) {
	return newS3Compatible("s3", cfg)
}

func newS3Compatible(name string, cfg config.ObjectStoreConfig) (*S3Driver, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("storage: %s bucket is required", name)
	}
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("storage: %s endpoint is required", name)
	}
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	client := s3.New(s3.Options{
		Region:       region,
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		BaseEndpoint: aws.String(strings.TrimRight(cfg.Endpoint, "/")),
		UsePathStyle: cfg.UsePathStyle,
	})
	return &S3Driver{
		client: client,
		bucket: cfg.Bucket,
	}, nil
}

// Ping verifies bucket reachability via HeadBucket.
func (d *S3Driver) Ping(ctx context.Context) error {
	if d == nil || d.client == nil {
		return fmt.Errorf("storage: s3 driver is nil")
	}
	_, err := d.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(d.bucket),
	})
	if err != nil {
		return fmt.Errorf("storage: s3 ping: %w", err)
	}
	return nil
}

// Upload implements Driver.
func (d *S3Driver) Upload(ctx context.Context, file File, objectPath string) error {
	key, err := sanitizePath(objectPath)
	if err != nil {
		return err
	}
	body, size, err := seekableBody(file.Body, file.Size)
	if err != nil {
		return err
	}
	input := &s3.PutObjectInput{
		Bucket:        aws.String(d.bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentType:   aws.String(file.ContentType),
		ContentLength: aws.Int64(size),
	}
	if len(file.Metadata) > 0 {
		input.Metadata = file.Metadata
	}
	_, err = d.client.PutObject(ctx, input)
	if err != nil {
		return fmt.Errorf("storage: s3 upload: %w", err)
	}
	return nil
}

// Download implements Driver.
func (d *S3Driver) Download(ctx context.Context, objectPath string) (io.ReadCloser, int64, error) {
	return d.DownloadVersion(ctx, objectPath, "")
}

// DownloadVersion implements Driver.
func (d *S3Driver) DownloadVersion(ctx context.Context, objectPath, versionID string) (io.ReadCloser, int64, error) {
	key, err := sanitizePath(objectPath)
	if err != nil {
		return nil, 0, err
	}
	input := &s3.GetObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	}
	if versionID != "" {
		input.VersionId = aws.String(versionID)
	}
	out, err := d.client.GetObject(ctx, input)
	if err != nil {
		return nil, 0, fmt.Errorf("storage: s3 get: %w", err)
	}
	var size int64
	if out.ContentLength != nil {
		size = *out.ContentLength
	}
	return out.Body, size, nil
}

// Delete implements Driver.
func (d *S3Driver) Delete(ctx context.Context, objectPath string) error {
	key, err := sanitizePath(objectPath)
	if err != nil {
		return err
	}
	return d.deleteObject(ctx, key, "")
}

// DeleteVersion implements Driver.
func (d *S3Driver) DeleteVersion(ctx context.Context, objectPath, versionID string) error {
	key, err := sanitizePath(objectPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(versionID) == "" {
		return fmt.Errorf("storage: version id is required")
	}
	return d.deleteObject(ctx, key, versionID)
}

func (d *S3Driver) deleteObject(ctx context.Context, key, versionID string) error {
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	}
	if versionID != "" {
		input.VersionId = aws.String(versionID)
	}
	_, err := d.client.DeleteObject(ctx, input)
	if err != nil {
		return fmt.Errorf("storage: s3 delete: %w", err)
	}
	return nil
}

// Exists implements Driver.
func (d *S3Driver) Exists(ctx context.Context, objectPath string) (bool, error) {
	key, err := sanitizePath(objectPath)
	if err != nil {
		return false, err
	}
	_, err = d.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return false, nil
	}
	return true, nil
}

// Bucket implements Driver.
func (d *S3Driver) Bucket() string {
	if d == nil {
		return ""
	}
	return d.bucket
}

// Head implements Driver.
func (d *S3Driver) Head(ctx context.Context, objectPath string) (ObjectInfo, error) {
	key, err := sanitizePath(objectPath)
	if err != nil {
		return ObjectInfo{}, err
	}
	out, err := d.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("storage: s3 head: %w", err)
	}
	info := ObjectInfo{
		Key:      key,
		Metadata: map[string]string{},
	}
	if out.ContentLength != nil {
		info.Size = *out.ContentLength
	}
	if out.ETag != nil {
		info.ETag = strings.Trim(*out.ETag, `"`)
	}
	if out.LastModified != nil {
		info.LastModified = *out.LastModified
	}
	if out.ContentType != nil {
		info.ContentType = *out.ContentType
	}
	if out.ContentDisposition != nil {
		info.ContentDisposition = *out.ContentDisposition
	}
	if out.CacheControl != nil {
		info.CacheControl = *out.CacheControl
	}
	if out.StorageClass != "" {
		info.StorageClass = string(out.StorageClass)
	}
	if out.VersionId != nil {
		info.VersionID = *out.VersionId
	}
	for k, v := range out.Metadata {
		info.Metadata[k] = v
	}
	return info, nil
}

// List implements Driver.
func (d *S3Driver) List(ctx context.Context, input ListObjectsInput) (ListObjectsResult, error) {
	prefix, err := sanitizePrefix(input.Prefix)
	if err != nil {
		return ListObjectsResult{}, err
	}
	in := &s3.ListObjectsV2Input{
		Bucket: aws.String(d.bucket),
	}
	if prefix != "" {
		in.Prefix = aws.String(prefix)
	}
	if strings.TrimSpace(input.Delimiter) != "" {
		in.Delimiter = aws.String(input.Delimiter)
	}
	if strings.TrimSpace(input.ContinuationToken) != "" {
		in.ContinuationToken = aws.String(input.ContinuationToken)
	}
	if input.MaxKeys > 0 {
		in.MaxKeys = aws.Int32(input.MaxKeys)
	}
	out, err := d.client.ListObjectsV2(ctx, in)
	if err != nil {
		return ListObjectsResult{}, fmt.Errorf("storage: s3 list: %w", err)
	}
	result := ListObjectsResult{
		Objects:     make([]ObjectInfo, 0, len(out.Contents)),
		Prefixes:    make([]PrefixInfo, 0, len(out.CommonPrefixes)),
		IsTruncated: aws.ToBool(out.IsTruncated),
	}
	if out.NextContinuationToken != nil {
		result.NextContinuationToken = *out.NextContinuationToken
	}
	for _, obj := range out.Contents {
		if obj.Key == nil {
			continue
		}
		info := ObjectInfo{
			Key:      *obj.Key,
			Metadata: map[string]string{},
		}
		if obj.Size != nil {
			info.Size = *obj.Size
		}
		if obj.ETag != nil {
			info.ETag = strings.Trim(*obj.ETag, `"`)
		}
		if obj.LastModified != nil {
			info.LastModified = *obj.LastModified
		}
		if obj.StorageClass != "" {
			info.StorageClass = string(obj.StorageClass)
		}
		result.Objects = append(result.Objects, info)
	}
	for _, p := range out.CommonPrefixes {
		if p.Prefix == nil {
			continue
		}
		result.Prefixes = append(result.Prefixes, PrefixInfo{Prefix: *p.Prefix})
	}
	return result, nil
}

// Copy implements Driver.
func (d *S3Driver) Copy(ctx context.Context, sourcePath, destPath string) error {
	return d.CopyVersion(ctx, sourcePath, destPath, "")
}

// CopyVersion implements Driver.
func (d *S3Driver) CopyVersion(ctx context.Context, sourcePath, destPath, versionID string) error {
	src, err := sanitizePath(sourcePath)
	if err != nil {
		return err
	}
	dst, err := sanitizePath(destPath)
	if err != nil {
		return err
	}
	source := copySource(d.bucket, src)
	if versionID != "" {
		source += "?versionId=" + url.QueryEscape(versionID)
	}
	_, err = d.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(d.bucket),
		Key:        aws.String(dst),
		CopySource: aws.String(source),
	})
	if err != nil {
		return fmt.Errorf("storage: s3 copy: %w", err)
	}
	return nil
}

// ListVersions implements Driver.
func (d *S3Driver) ListVersions(ctx context.Context, objectPath string) ([]VersionInfo, error) {
	key, err := sanitizePath(objectPath)
	if err != nil {
		return nil, err
	}
	out, err := d.client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{
		Bucket: aws.String(d.bucket),
		Prefix: aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("storage: s3 versions: %w", err)
	}
	versions := make([]VersionInfo, 0, len(out.Versions))
	for _, v := range out.Versions {
		if v.Key == nil || *v.Key != key {
			continue
		}
		info := VersionInfo{IsLatest: aws.ToBool(v.IsLatest)}
		if v.VersionId != nil {
			info.VersionID = *v.VersionId
		}
		if v.Size != nil {
			info.Size = *v.Size
		}
		if v.ETag != nil {
			info.ETag = strings.Trim(*v.ETag, `"`)
		}
		if v.LastModified != nil {
			info.LastModified = *v.LastModified
		}
		versions = append(versions, info)
	}
	return versions, nil
}

// PresignGet implements Driver.
func (d *S3Driver) PresignGet(ctx context.Context, objectPath string, expiry time.Duration) (string, error) {
	key, err := sanitizePath(objectPath)
	if err != nil {
		return "", err
	}
	if expiry <= 0 {
		expiry = 15 * time.Minute
	}
	presign := s3.NewPresignClient(d.client)
	out, err := presign.PresignGetObject(
		ctx,
		&s3.GetObjectInput{
			Bucket: aws.String(d.bucket),
			Key:    aws.String(key),
		},
		s3.WithPresignExpires(expiry),
	)
	if err != nil {
		return "", fmt.Errorf("storage: s3 presign get: %w", err)
	}
	return out.URL, nil
}

// PresignPut implements Driver.
func (d *S3Driver) PresignPut(ctx context.Context, objectPath, contentType string, expiry time.Duration) (string, error) {
	key, err := sanitizePath(objectPath)
	if err != nil {
		return "", err
	}
	if expiry <= 0 {
		expiry = 15 * time.Minute
	}
	input := &s3.PutObjectInput{
		Bucket: aws.String(d.bucket),
		Key:    aws.String(key),
	}
	if strings.TrimSpace(contentType) != "" {
		input.ContentType = aws.String(contentType)
	}
	presign := s3.NewPresignClient(d.client)
	out, err := presign.PresignPutObject(ctx, input, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", fmt.Errorf("storage: s3 presign put: %w", err)
	}
	return out.URL, nil
}

func copySource(bucket, key string) string {
	parts := strings.Split(key, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return url.PathEscape(bucket) + "/" + strings.Join(parts, "/")
}

func seekableBody(r io.Reader, size int64) (io.ReadSeeker, int64, error) {
	if r == nil {
		return nil, 0, fmt.Errorf("storage: file body is required")
	}
	if size > 0 {
		r = io.LimitReader(r, size)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, 0, fmt.Errorf("storage: read body: %w", err)
	}
	if size > 0 && int64(len(data)) != size {
		return nil, 0, fmt.Errorf("storage: body size mismatch: got %d want %d", len(data), size)
	}
	return bytes.NewReader(data), int64(len(data)), nil
}
