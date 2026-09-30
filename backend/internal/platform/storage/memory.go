package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
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
