package storage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
)

const (
	// SystemPrefix is reserved for internal object-store bookkeeping.
	SystemPrefix = "__system/"
	// TrashPrefix holds soft-deleted objects.
	TrashPrefix = "__system/trash/"
)

// File is an upload payload stub for future use.
type File struct {
	Body        io.Reader
	Size        int64
	ContentType string
	Filename    string
	Metadata    map[string]string
}

// ObjectInfo is a listed or headed object.
type ObjectInfo struct {
	Key                string
	Size               int64
	ETag               string
	LastModified       time.Time
	ContentType        string
	ContentDisposition string
	CacheControl       string
	StorageClass       string
	VersionID          string
	Metadata           map[string]string
}

// PrefixInfo is a common prefix (folder) from a delimited list.
type PrefixInfo struct {
	Prefix string
}

// ListObjectsInput lists objects under a prefix.
type ListObjectsInput struct {
	Prefix            string
	Delimiter         string
	ContinuationToken string
	MaxKeys           int32
}

// ListObjectsResult is one page of an S3-style list.
type ListObjectsResult struct {
	Objects               []ObjectInfo
	Prefixes              []PrefixInfo
	IsTruncated           bool
	NextContinuationToken string
}

// VersionInfo is one object version.
type VersionInfo struct {
	VersionID    string
	Size         int64
	ETag         string
	LastModified time.Time
	IsLatest     bool
}

// Driver is the low-level object store backend (SeaweedFS / S3).
type Driver interface {
	Ping(ctx context.Context) error
	Upload(ctx context.Context, file File, path string) error
	Download(ctx context.Context, path string) (io.ReadCloser, int64, error)
	DownloadVersion(ctx context.Context, path, versionID string) (io.ReadCloser, int64, error)
	Delete(ctx context.Context, path string) error
	DeleteVersion(ctx context.Context, path, versionID string) error
	Exists(ctx context.Context, path string) (bool, error)
	List(ctx context.Context, input ListObjectsInput) (ListObjectsResult, error)
	Head(ctx context.Context, path string) (ObjectInfo, error)
	Copy(ctx context.Context, sourcePath, destPath string) error
	CopyVersion(ctx context.Context, sourcePath, destPath, versionID string) error
	ListVersions(ctx context.Context, path string) ([]VersionInfo, error)
	PresignGet(ctx context.Context, path string, expiry time.Duration) (string, error)
	PresignPut(ctx context.Context, path, contentType string, expiry time.Duration) (string, error)
	Bucket() string
}

// NewFromConfig builds a storage driver from configuration.
func NewFromConfig(ctx context.Context, cfg config.StorageConfig) (Driver, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Driver)) {
	case "minio":
		return nil, fmt.Errorf("storage: STORAGE_DRIVER=minio was removed; use STORAGE_DRIVER=s3 with the S3_* settings")
	case "s3":
		return NewS3(ctx, cfg.S3)
	default:
		return nil, fmt.Errorf("storage: unsupported STORAGE_DRIVER %q", cfg.Driver)
	}
}

func sanitizePath(path string) (string, error) {
	path, err := sanitizePrefix(path)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", fmt.Errorf("storage: path is required")
	}
	return path, nil
}

func sanitizePrefix(path string) (string, error) {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "/")
	if strings.Contains(path, "..") {
		return "", fmt.Errorf("storage: invalid path")
	}
	return path, nil
}

// IsSystemKey reports whether key is reserved for internal bookkeeping.
func IsSystemKey(key string) bool {
	return strings.HasPrefix(strings.TrimPrefix(key, "/"), SystemPrefix)
}
