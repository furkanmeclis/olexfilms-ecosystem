package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/storage/model"
	platstorage "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	maxListScan    = 5000
	maxUploadBytes = 512 << 20
	trashRetention = 30 * 24 * time.Hour
	folderMIME     = "application/x-directory"
	uploadURLPath  = "/v1/platform/storage/uploads"
	publicPathFmt  = "/v1/public/storage/%s"
	signedPathFmt  = "/v1/public/storage/s/%s"
)

// Service implements platform object-storage browsing.
type Service struct {
	store storageDriver
	q     *db.Queries
	log   *slog.Logger

	usageMu sync.Mutex
	usageAt time.Time
	usage   model.Usage
}

type storageDriver = platstorage.Driver

// New constructs a storage service.
func New(store platstorage.Driver, q *db.Queries, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: store, q: q, log: log}
}

func (s *Service) bucket() string {
	if s.store == nil {
		return ""
	}
	return s.store.Bucket()
}

func (s *Service) Get(ctx context.Context, actor model.Actor, key string) (model.Object, error) {
	key, err := normalizeKey(key)
	if err != nil {
		return model.Object{}, err
	}
	if skipSystem(key) && !strings.HasPrefix(key, platstorage.TrashPrefix) {
		return model.Object{}, ErrNotFound
	}
	info, err := s.store.Head(ctx, key)
	if err != nil {
		return model.Object{}, ErrNotFound
	}
	obj := s.objectFromInfo(info, isFolderKey(key))
	s.enrich(ctx, actor, []*model.Object{&obj})
	return obj, nil
}

func (s *Service) CreateFolder(ctx context.Context, actor model.Actor, in model.CreateFolderInput) (model.Object, error) {
	key, err := joinKey(in.Prefix, in.Name)
	if err != nil {
		return model.Object{}, err
	}
	key = folderKey(key)
	if skipSystem(key) {
		return model.Object{}, ErrInvalidKey
	}
	exists, err := s.store.Exists(ctx, key)
	if err != nil {
		return model.Object{}, fmt.Errorf("storage: exists: %w", err)
	}
	if exists {
		return model.Object{}, ErrConflict
	}
	err = s.store.Upload(ctx, platstorage.File{
		Body:        bytes.NewReader(nil),
		Size:        0,
		ContentType: folderMIME,
		Filename:    in.Name,
		Metadata:    ownerMeta(actor),
	}, key)
	if err != nil {
		return model.Object{}, fmt.Errorf("storage: create folder: %w", err)
	}
	s.record(ctx, actor, key, "folder.created", map[string]any{"name": in.Name})
	return s.Get(ctx, actor, key)
}

func (s *Service) Upload(ctx context.Context, actor model.Actor, meta model.UploadMeta, body io.Reader) (model.Object, error) {
	key := strings.TrimSpace(meta.Key)
	var err error
	if key == "" {
		key, err = joinKey(meta.Prefix, meta.Filename)
		if err != nil {
			return model.Object{}, err
		}
	} else {
		key, err = normalizeKey(key)
		if err != nil {
			return model.Object{}, err
		}
	}
	if skipSystem(key) || isFolderKey(key) {
		return model.Object{}, ErrInvalidKey
	}
	if meta.Size > maxUploadBytes {
		return model.Object{}, ErrInvalidKey
	}
	mime := mimeFromName(baseName(key), meta.ContentType)
	err = s.store.Upload(ctx, platstorage.File{
		Body:        body,
		Size:        meta.Size,
		ContentType: mime,
		Filename:    meta.Filename,
		Metadata:    ownerMeta(actor),
	}, key)
	if err != nil {
		return model.Object{}, fmt.Errorf("storage: upload: %w", err)
	}
	s.invalidateUsage()
	s.record(ctx, actor, key, "file.uploaded", map[string]any{"name": baseName(key), "size": meta.Size})
	return s.Get(ctx, actor, key)
}

func (s *Service) CreateUploadSession(ctx context.Context, actor model.Actor, meta model.UploadMeta) (model.UploadSession, error) {
	key := strings.TrimSpace(meta.Key)
	var err error
	if key == "" {
		key, err = joinKey(meta.Prefix, meta.Filename)
		if err != nil {
			return model.UploadSession{}, err
		}
	} else {
		key, err = normalizeKey(key)
		if err != nil {
			return model.UploadSession{}, err
		}
	}
	if skipSystem(key) {
		return model.UploadSession{}, ErrInvalidKey
	}
	sessionID := uuid.NewString()
	out := model.UploadSession{
		SessionID: sessionID,
		Key:       key,
		Method:    "post",
		UploadURL: uploadURLPath,
		ExpiresIn: 3600,
	}
	if url, err := s.store.PresignPut(ctx, key, meta.ContentType, time.Hour); err == nil {
		out.PresignedURL = url
	}
	_ = actor
	return out, nil
}

func (s *Service) CompleteUpload(ctx context.Context, actor model.Actor, key string) (model.Object, error) {
	obj, err := s.Get(ctx, actor, key)
	if err != nil {
		return model.Object{}, err
	}
	s.invalidateUsage()
	s.record(ctx, actor, key, "file.uploaded", map[string]any{"name": obj.Name, "size": obj.Size})
	return obj, nil
}

func (s *Service) Open(ctx context.Context, key, versionID string, download bool) (model.OpenObject, error) {
	key, err := normalizeKey(key)
	if err != nil {
		return model.OpenObject{}, err
	}
	info, err := s.store.Head(ctx, key)
	if err != nil {
		return model.OpenObject{}, ErrNotFound
	}
	var body io.ReadCloser
	var size int64
	if versionID != "" {
		body, size, err = s.store.DownloadVersion(ctx, key, versionID)
	} else {
		body, size, err = s.store.Download(ctx, key)
	}
	if err != nil {
		return model.OpenObject{}, ErrNotFound
	}
	obj := s.objectFromInfo(info, isFolderKey(key))
	return model.OpenObject{Object: obj, Body: body, Size: size, Download: download}, nil
}

func (s *Service) objectFromInfo(info platstorage.ObjectInfo, folder bool) model.Object {
	name := baseName(info.Key)
	if folder {
		name = strings.TrimSuffix(name, "/")
		if name == "" {
			name = baseName(strings.TrimSuffix(info.Key, "/"))
		}
	}
	mime := info.ContentType
	if folder {
		mime = folderMIME
	} else {
		mime = mimeFromName(name, mime)
	}
	updated := info.LastModified.UTC().Format(time.RFC3339)
	kind := "file"
	fileKind := classifyFileKind(name, mime)
	if folder {
		kind = "folder"
		fileKind = "folder"
	}
	meta := info.Metadata
	if meta == nil {
		meta = map[string]string{}
	}
	owner := meta["owner-name"]
	return model.Object{
		ID:                 info.Key,
		Name:               name,
		Key:                info.Key,
		Prefix:             parentPrefix(info.Key),
		Bucket:             s.bucket(),
		Kind:               kind,
		FileKind:           fileKind,
		Size:               info.Size,
		MimeType:           mime,
		ETag:               info.ETag,
		VersionID:          info.VersionID,
		Version:            "current",
		Access:             "private",
		Owner:              owner,
		StorageClass:       info.StorageClass,
		ContentDisposition: info.ContentDisposition,
		CacheControl:       info.CacheControl,
		Metadata:           meta,
		CreatedAt:          updated,
		UpdatedAt:          updated,
	}
}

func (s *Service) objectFromPrefix(prefix string) model.Object {
	name := baseName(strings.TrimSuffix(prefix, "/"))
	now := time.Now().UTC().Format(time.RFC3339)
	return model.Object{
		ID:        prefix,
		Name:      name,
		Key:       prefix,
		Prefix:    parentPrefix(prefix),
		Bucket:    s.bucket(),
		Kind:      "folder",
		FileKind:  "folder",
		MimeType:  folderMIME,
		Access:    "private",
		Metadata:  map[string]string{},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (s *Service) enrich(ctx context.Context, actor model.Actor, objects []*model.Object) {
	if len(objects) == 0 {
		return
	}
	keys := make([]string, 0, len(objects))
	for _, o := range objects {
		keys = append(keys, o.Key)
	}
	starred := map[string]struct{}{}
	if actor.UserID > 0 && s.q != nil {
		rows, err := s.q.ListStorageStarsByKeys(ctx, db.ListStorageStarsByKeysParams{
			UserID: actor.UserID, Keys: keys,
		})
		if err == nil {
			for _, row := range rows {
				starred[row.ObjectKey] = struct{}{}
			}
		}
	}
	public := map[string]struct{}{}
	shared := map[string]struct{}{}
	if s.q != nil {
		if rows, err := s.q.ListActivePublicKeys(ctx, keys); err == nil {
			for _, k := range rows {
				public[k] = struct{}{}
			}
		}
		if rows, err := s.q.ListSharedKeys(ctx, keys); err == nil {
			for _, k := range rows {
				shared[k] = struct{}{}
			}
		}
	}
	for _, o := range objects {
		if _, ok := starred[o.Key]; ok {
			o.IsStarred = true
		}
		if _, ok := public[o.Key]; ok {
			o.IsPublic = true
			o.Access = "public"
		}
		if _, ok := shared[o.Key]; ok {
			o.IsShared = true
			if o.Access == "private" {
				o.Access = "shared"
			}
		}
	}
}

func (s *Service) record(ctx context.Context, actor model.Actor, key, action string, payload map[string]any) {
	if s.q == nil {
		return
	}
	body, err := json.Marshal(payload)
	if err != nil || payload == nil {
		body = []byte("{}")
	}
	_, err = s.q.InsertStorageActivity(ctx, db.InsertStorageActivityParams{
		ObjectKey:   key,
		ActorUserID: pgInt8(actor.UserID),
		Action:      action,
		Payload:     body,
	})
	if err != nil {
		s.log.Warn("storage_activity_failed", "action", action, "error", err)
	}
}

func (s *Service) listKeysUnder(ctx context.Context, key string) ([]string, error) {
	key, err := normalizeKey(key)
	if err != nil {
		return nil, err
	}
	if !isFolderKey(key) {
		exists, err := s.store.Exists(ctx, key)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, ErrNotFound
		}
		return []string{key}, nil
	}
	objects, _, err := s.scan(ctx, key, "", maxListScan)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(objects)+1)
	out = append(out, key)
	for _, obj := range objects {
		if obj.Key != key {
			out = append(out, obj.Key)
		}
	}
	return out, nil
}

func (s *Service) scan(ctx context.Context, prefix, delimiter string, max int) ([]platstorage.ObjectInfo, []platstorage.PrefixInfo, error) {
	var objects []platstorage.ObjectInfo
	var prefixes []platstorage.PrefixInfo
	token := ""
	for {
		page, err := s.store.List(ctx, platstorage.ListObjectsInput{
			Prefix:            prefix,
			Delimiter:         delimiter,
			ContinuationToken: token,
			MaxKeys:           1000,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("storage: list: %w", err)
		}
		for _, obj := range page.Objects {
			if skipSystem(obj.Key) && !strings.HasPrefix(prefix, platstorage.TrashPrefix) {
				continue
			}
			objects = append(objects, obj)
		}
		prefixes = append(prefixes, page.Prefixes...)
		if !page.IsTruncated || page.NextContinuationToken == "" || len(objects)+len(prefixes) >= max {
			break
		}
		token = page.NextContinuationToken
	}
	return objects, prefixes, nil
}

func ownerMeta(actor model.Actor) map[string]string {
	meta := map[string]string{}
	if actor.UserID > 0 {
		meta["owner-id"] = fmt.Sprintf("%d", actor.UserID)
	}
	if actor.Name != "" {
		meta["owner-name"] = actor.Name
	}
	return meta
}

func pgInt8(id int64) pgtype.Int8 {
	if id == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: id, Valid: true}
}

func pgText(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func formatTS(v pgtype.Timestamptz) string {
	if !v.Valid {
		return ""
	}
	return v.Time.UTC().Format(time.RFC3339)
}

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

func sortObjects(items []model.Object, sortSpec string) {
	field, desc := "name", false
	if sortSpec != "" {
		desc = strings.HasPrefix(sortSpec, "-")
		field = strings.TrimPrefix(sortSpec, "-")
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Kind != b.Kind {
			if a.Kind == "folder" {
				return !desc
			}
			if b.Kind == "folder" {
				return desc
			}
		}
		var less bool
		switch field {
		case "size":
			less = a.Size < b.Size
		case "updated_at":
			less = a.UpdatedAt < b.UpdatedAt
		case "type":
			less = a.FileKind < b.FileKind
		default:
			less = strings.ToLower(a.Name) < strings.ToLower(b.Name)
		}
		if desc {
			return !less
		}
		return less
	})
}
