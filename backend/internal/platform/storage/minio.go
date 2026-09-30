package storage

import (
	"context"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
)

// MinIODriver stores objects on MinIO (S3-compatible, used in development).
type MinIODriver struct {
	*S3Driver
}

// NewMinIO builds a MinIO-backed driver.
func NewMinIO(ctx context.Context, cfg config.ObjectStoreConfig) (*MinIODriver, error) {
	base, err := newS3Compatible("minio", cfg)
	if err != nil {
		return nil, err
	}
	_ = ctx
	return &MinIODriver{S3Driver: base}, nil
}
