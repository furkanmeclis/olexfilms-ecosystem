package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

// Memory is an in-process object store for tests.
type Memory struct {
	mu   sync.RWMutex
	data map[string]memoryObject
}

type memoryObject struct {
	Body        []byte
	ContentType string
	Filename    string
}

// NewMemory returns an empty memory driver.
func NewMemory() *Memory {
	return &Memory{data: map[string]memoryObject{}}
}

// Ping always succeeds.
func (m *Memory) Ping(context.Context) error { return nil }

// Upload stores the object in memory.
func (m *Memory) Upload(_ context.Context, file File, objectPath string) error {
	key, err := sanitizePath(objectPath)
	if err != nil {
		return err
	}
	if file.Body == nil {
		return fmt.Errorf("storage: file body is required")
	}
	body, err := io.ReadAll(file.Body)
	if err != nil {
		return fmt.Errorf("storage: read body: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = memoryObject{
		Body:        body,
		ContentType: file.ContentType,
		Filename:    file.Filename,
	}
	return nil
}

// Download returns a reader over the stored bytes.
func (m *Memory) Download(_ context.Context, objectPath string) (io.ReadCloser, int64, error) {
	key, err := sanitizePath(objectPath)
	if err != nil {
		return nil, 0, err
	}
	m.mu.RLock()
	obj, ok := m.data[key]
	m.mu.RUnlock()
	if !ok {
		return nil, 0, fmt.Errorf("storage: object not found")
	}
	return io.NopCloser(bytes.NewReader(obj.Body)), int64(len(obj.Body)), nil
}

// Delete removes an object.
func (m *Memory) Delete(_ context.Context, objectPath string) error {
	key, err := sanitizePath(objectPath)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

// Exists reports whether the key is present.
func (m *Memory) Exists(_ context.Context, objectPath string) (bool, error) {
	key, err := sanitizePath(objectPath)
	if err != nil {
		return false, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.data[key]
	return ok, nil
}

// GetMeta returns content-type/filename for a stored object (tests / media serve).
func (m *Memory) GetMeta(objectPath string) (contentType, filename string, ok bool) {
	key, err := sanitizePath(objectPath)
	if err != nil {
		return "", "", false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	obj, ok := m.data[key]
	if !ok {
		return "", "", false
	}
	return obj.ContentType, obj.Filename, true
}

// The methods below complete the Driver interface for tests; versioning and
// presigning are not modelled (single version, no URLs).

var _ Driver = (*Memory)(nil)

// DownloadVersion ignores the version and returns the current object.
func (m *Memory) DownloadVersion(ctx context.Context, objectPath, _ string) (io.ReadCloser, int64, error) {
	return m.Download(ctx, objectPath)
}

// DeleteVersion ignores the version and deletes the object.
func (m *Memory) DeleteVersion(ctx context.Context, objectPath, _ string) error {
	return m.Delete(ctx, objectPath)
}

// List returns every object under the prefix (no delimiter or paging).
func (m *Memory) List(_ context.Context, input ListObjectsInput) (ListObjectsResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out ListObjectsResult
	for key, obj := range m.data {
		if strings.HasPrefix(key, input.Prefix) {
			out.Objects = append(out.Objects, ObjectInfo{Key: key, Size: int64(len(obj.Body)), ContentType: obj.ContentType})
		}
	}
	sort.Slice(out.Objects, func(i, j int) bool { return out.Objects[i].Key < out.Objects[j].Key })
	return out, nil
}

// Head returns size and content type of an object.
func (m *Memory) Head(_ context.Context, objectPath string) (ObjectInfo, error) {
	key, err := sanitizePath(objectPath)
	if err != nil {
		return ObjectInfo{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	obj, ok := m.data[key]
	if !ok {
		return ObjectInfo{}, fmt.Errorf("storage: object not found")
	}
	return ObjectInfo{Key: key, Size: int64(len(obj.Body)), ContentType: obj.ContentType}, nil
}

// Copy duplicates an object.
func (m *Memory) Copy(_ context.Context, sourcePath, destPath string) error {
	src, err := sanitizePath(sourcePath)
	if err != nil {
		return err
	}
	dst, err := sanitizePath(destPath)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	obj, ok := m.data[src]
	if !ok {
		return fmt.Errorf("storage: object not found")
	}
	m.data[dst] = obj
	return nil
}

// CopyVersion ignores the version and copies the object.
func (m *Memory) CopyVersion(ctx context.Context, sourcePath, destPath, _ string) error {
	return m.Copy(ctx, sourcePath, destPath)
}

// ListVersions reports the single current version.
func (m *Memory) ListVersions(ctx context.Context, objectPath string) ([]VersionInfo, error) {
	info, err := m.Head(ctx, objectPath)
	if err != nil {
		return nil, err
	}
	return []VersionInfo{{VersionID: "memory", Size: info.Size, IsLatest: true}}, nil
}

// PresignGet is not supported by the memory driver.
func (m *Memory) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "", fmt.Errorf("storage: memory driver cannot presign")
}

// PresignPut is not supported by the memory driver.
func (m *Memory) PresignPut(context.Context, string, string, time.Duration) (string, error) {
	return "", fmt.Errorf("storage: memory driver cannot presign")
}

// Bucket names the in-memory store.
func (m *Memory) Bucket() string { return "memory" }
