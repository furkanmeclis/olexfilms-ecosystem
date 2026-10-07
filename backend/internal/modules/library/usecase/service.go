// Package usecase implements the document library API.
package usecase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	platstorage "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	MaxUploadBytes     = 50 << 20
	defaultLocale      = "tr"
	downloadURLTTL     = 10 * time.Minute
	AccessAllNetwork   = "all_network"
	AccessDistributors = "distributors"
	AccessDealers      = "dealers"
	AccessCenterOnly   = "center_only"
	OrgCenter          = "center"
	OrgDistributor     = "distributor"
	OrgDealer          = "dealer"
)

var (
	ErrForbidden      = errors.New("library: forbidden")
	ErrNotFound       = errors.New("library: not found")
	ErrFolderNotEmpty = errors.New("library: folder is not empty")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func invalid(field, msg string) error { return &ValidationError{Field: field, Message: msg} }

type Store interface {
	CreateLibraryFolder(ctx context.Context, arg db.CreateLibraryFolderParams) (db.LibraryFolder, error)
	GetLibraryFolderByUUID(ctx context.Context, argUuid uuid.UUID) (db.LibraryFolder, error)
	ListLibraryFolders(ctx context.Context, organizationID int64) ([]db.LibraryFolder, error)
	ListLibraryFoldersByBrand(ctx context.Context, brandID int64) ([]db.LibraryFolder, error)
	ListVisibleLibraryFolders(ctx context.Context, arg db.ListVisibleLibraryFoldersParams) ([]db.LibraryFolder, error)
	UpdateLibraryFolder(ctx context.Context, arg db.UpdateLibraryFolderParams) (db.LibraryFolder, error)
	SoftDeleteLibraryFolder(ctx context.Context, arg db.SoftDeleteLibraryFolderParams) (int64, error)
	CreateLibraryItem(ctx context.Context, arg db.CreateLibraryItemParams) (db.LibraryItem, error)
	GetLibraryItemByID(ctx context.Context, id int64) (db.LibraryItem, error)
	GetLibraryItemByUUID(ctx context.Context, argUuid uuid.UUID) (db.LibraryItem, error)
	UpdateLibraryItem(ctx context.Context, arg db.UpdateLibraryItemParams) (db.LibraryItem, error)
	SoftDeleteLibraryItem(ctx context.Context, arg db.SoftDeleteLibraryItemParams) (int64, error)
	ListLibraryItems(ctx context.Context, arg db.ListLibraryItemsParams) ([]db.LibraryItem, error)
	CountLibraryItems(ctx context.Context, arg db.CountLibraryItemsParams) (int64, error)
	NextLibraryItemVersionNo(ctx context.Context, arg db.NextLibraryItemVersionNoParams) (int32, error)
	CreateLibraryItemVersion(ctx context.Context, arg db.CreateLibraryItemVersionParams) (db.LibraryItemVersion, error)
	ListLibraryItemVersions(ctx context.Context, itemID int64) ([]db.LibraryItemVersion, error)
	ListLatestLibraryItemVersions(ctx context.Context, itemID int64) ([]db.LibraryItemVersion, error)
	GetLibraryItemVersionByUUID(ctx context.Context, argUuid uuid.UUID) (db.LibraryItemVersion, error)
}

type Storage interface {
	Upload(ctx context.Context, file platstorage.File, path string) error
	PresignGet(ctx context.Context, path string, expiry time.Duration) (string, error)
}

type Service struct {
	q     Store
	store Storage
	now   func() time.Time
}

func New(q Store, store Storage) *Service {
	return &Service{q: q, store: store, now: time.Now}
}

type FolderInput struct {
	ParentUUID *uuid.UUID `json:"parent_uuid"`
	Name       *string    `json:"name"`
	SortOrder  *int32     `json:"sort_order"`
}

type ItemInput struct {
	FolderUUID  *uuid.UUID `json:"folder_uuid"`
	Name        *string    `json:"name"`
	Description *string    `json:"description"`
	Tags        []string   `json:"tags"`
	AccessLevel *string    `json:"access_level"`
	RoleSlug    *string    `json:"role_slug"`
}

type UploadInput struct {
	Locale   string
	Filename string
	Size     int64
	Body     io.Reader
}

type ListInput struct {
	FolderUUID *uuid.UUID
	// Tags matches items carrying any of the tags (CSV `tag`).
	Tags []string
	// AccessLevels narrows the levels visible to the viewer (CSV `access_level`).
	AccessLevels []string
	Updated      apiquery.TimeRange
	Query        string
	Locale       string
	SortKey      string
	SortDesc     bool
	Limit        int32
	Offset       int32
}

// ItemsSortSpec is the sort whitelist of GET /v1/library (docs/list-contract.md).
var ItemsSortSpec = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"name": "name", "access_level": "access_level", "created_at": "created_at", "updated_at": "updated_at",
	},
	Default: apiquery.SortField{Field: "name"},
}

// AccessLevelValues are the valid access_level values.
var AccessLevelValues = []string{AccessAllNetwork, AccessDistributors, AccessDealers, AccessCenterOnly}

type Actor struct {
	UserID         int64
	OrganizationID int64
	BrandID        int64
	OrgType        string
	Roles          []string
}

type FolderView struct {
	UUID       uuid.UUID  `json:"uuid"`
	ParentUUID *uuid.UUID `json:"parent_uuid,omitempty"`
	Name       string     `json:"name"`
	SortOrder  int32      `json:"sort_order"`
	CreatedAt  string     `json:"created_at"`
	UpdatedAt  string     `json:"updated_at"`
}

type ItemView struct {
	UUID          uuid.UUID    `json:"uuid"`
	FolderUUID    *uuid.UUID   `json:"folder_uuid,omitempty"`
	Name          string       `json:"name"`
	Description   string       `json:"description,omitempty"`
	Tags          []string     `json:"tags"`
	AccessLevel   string       `json:"access_level"`
	RoleSlug      string       `json:"role_slug,omitempty"`
	LatestVersion *VersionView `json:"latest_version,omitempty"`
	CreatedAt     string       `json:"created_at"`
	UpdatedAt     string       `json:"updated_at"`
}

type VersionView struct {
	UUID      uuid.UUID `json:"uuid"`
	Locale    string    `json:"locale"`
	VersionNo int32     `json:"version_no"`
	MIME      string    `json:"mime"`
	SizeBytes int64     `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
	CreatedAt string    `json:"created_at"`
}

type DownloadView struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

func ActorFrom(pr authctx.Principal, org orgctx.Scope) Actor {
	return Actor{UserID: pr.UserInternal, OrganizationID: org.InternalID, BrandID: org.BrandID, OrgType: org.OrgType, Roles: pr.Roles}
}

// ListFolders returns the folder tree readable by the viewer: the whole
// brand tree for the center; for other organizations only folders holding
// (directly or below) an item they may see, their ancestors and the
// organization's own folders.
func (s *Service) ListFolders(ctx context.Context, actor Actor) ([]FolderView, error) {
	rows, err := s.readableFolders(ctx, actor)
	if err != nil {
		return nil, err
	}
	byID := map[int64]uuid.UUID{}
	for _, row := range rows {
		byID[row.ID] = row.Uuid
	}
	out := make([]FolderView, 0, len(rows))
	for _, row := range rows {
		out = append(out, folderView(row, byID))
	}
	return out, nil
}

func (s *Service) CreateFolder(ctx context.Context, actor Actor, in FolderInput) (FolderView, error) {
	if actor.OrgType != OrgCenter {
		return FolderView{}, ErrForbidden
	}
	name, err := requiredString("name", in.Name)
	if err != nil {
		return FolderView{}, err
	}
	parentID, err := s.folderID(ctx, actor, in.ParentUUID)
	if err != nil {
		return FolderView{}, err
	}
	sortOrder := int32(0)
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	}
	row, err := s.q.CreateLibraryFolder(ctx, db.CreateLibraryFolderParams{
		OrganizationID:  actor.OrganizationID,
		BrandID:         actor.BrandID,
		ParentID:        int8Arg(parentID),
		Name:            name,
		SortOrder:       sortOrder,
		CreatedByUserID: int8Value(actor.UserID),
	})
	if err != nil {
		return FolderView{}, mapDBError(err)
	}
	return folderView(row, nil), nil
}

func (s *Service) UpdateFolder(ctx context.Context, actor Actor, id uuid.UUID, in FolderInput) (FolderView, error) {
	if actor.OrgType != OrgCenter {
		return FolderView{}, ErrForbidden
	}
	cur, err := s.folder(ctx, actor, id)
	if err != nil {
		return FolderView{}, err
	}
	name := cur.Name
	if in.Name != nil {
		name, err = requiredString("name", in.Name)
		if err != nil {
			return FolderView{}, err
		}
	}
	parentID := int8Ptr(cur.ParentID)
	if in.ParentUUID != nil {
		parentID, err = s.folderID(ctx, actor, in.ParentUUID)
		if err != nil {
			return FolderView{}, err
		}
	}
	sortOrder := cur.SortOrder
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	}
	row, err := s.q.UpdateLibraryFolder(ctx, db.UpdateLibraryFolderParams{
		ID: cur.ID, OrganizationID: actor.OrganizationID, Name: name, ParentID: int8Arg(parentID), SortOrder: sortOrder,
	})
	if err != nil {
		return FolderView{}, mapDBError(err)
	}
	return folderView(row, nil), nil
}

func (s *Service) DeleteFolder(ctx context.Context, actor Actor, id uuid.UUID) error {
	if actor.OrgType != OrgCenter {
		return ErrForbidden
	}
	cur, err := s.folder(ctx, actor, id)
	if err != nil {
		return err
	}
	n, err := s.q.SoftDeleteLibraryFolder(ctx, db.SoftDeleteLibraryFolderParams{ID: cur.ID, OrganizationID: actor.OrganizationID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrFolderNotEmpty
	}
	return nil
}

func (s *Service) CreateItem(ctx context.Context, actor Actor, in ItemInput) (ItemView, error) {
	if actor.OrgType != OrgCenter {
		return ItemView{}, ErrForbidden
	}
	name, err := requiredString("name", in.Name)
	if err != nil {
		return ItemView{}, err
	}
	folderID, err := s.folderID(ctx, actor, in.FolderUUID)
	if err != nil {
		return ItemView{}, err
	}
	access := AccessCenterOnly
	if in.AccessLevel != nil {
		access = strings.TrimSpace(*in.AccessLevel)
	}
	if err := validateAccess(access); err != nil {
		return ItemView{}, err
	}
	row, err := s.q.CreateLibraryItem(ctx, db.CreateLibraryItemParams{
		OrganizationID:  actor.OrganizationID,
		BrandID:         actor.BrandID,
		FolderID:        int8Arg(folderID),
		Name:            name,
		Description:     textPtr(in.Description),
		Tags:            normalizeTags(in.Tags),
		AccessLevel:     access,
		RoleSlug:        textPtr(in.RoleSlug),
		CreatedByUserID: int8Value(actor.UserID),
	})
	if err != nil {
		return ItemView{}, mapDBError(err)
	}
	return s.itemView(ctx, row, nil, ""), nil
}

func (s *Service) UpdateItem(ctx context.Context, actor Actor, id uuid.UUID, in ItemInput) (ItemView, error) {
	if actor.OrgType != OrgCenter {
		return ItemView{}, ErrForbidden
	}
	cur, err := s.itemForManage(ctx, actor, id)
	if err != nil {
		return ItemView{}, err
	}
	name := cur.Name
	if in.Name != nil {
		name, err = requiredString("name", in.Name)
		if err != nil {
			return ItemView{}, err
		}
	}
	folderID := int8Ptr(cur.FolderID)
	if in.FolderUUID != nil {
		folderID, err = s.folderID(ctx, actor, in.FolderUUID)
		if err != nil {
			return ItemView{}, err
		}
	}
	access := cur.AccessLevel
	if in.AccessLevel != nil {
		access = strings.TrimSpace(*in.AccessLevel)
	}
	if err := validateAccess(access); err != nil {
		return ItemView{}, err
	}
	desc := cur.Description
	if in.Description != nil {
		desc = textPtr(in.Description)
	}
	role := cur.RoleSlug
	if in.RoleSlug != nil {
		role = textPtr(in.RoleSlug)
	}
	tags := cur.Tags
	if in.Tags != nil {
		tags = normalizeTags(in.Tags)
	}
	row, err := s.q.UpdateLibraryItem(ctx, db.UpdateLibraryItemParams{
		ID: cur.ID, OrganizationID: actor.OrganizationID, FolderID: int8Arg(folderID), Name: name,
		Description: desc, Tags: tags, AccessLevel: access, RoleSlug: role,
	})
	if err != nil {
		return ItemView{}, mapDBError(err)
	}
	return s.itemView(ctx, row, nil, ""), nil
}

func (s *Service) ArchiveItem(ctx context.Context, actor Actor, id uuid.UUID) error {
	if actor.OrgType != OrgCenter {
		return ErrForbidden
	}
	cur, err := s.itemForManage(ctx, actor, id)
	if err != nil {
		return err
	}
	n, err := s.q.SoftDeleteLibraryItem(ctx, db.SoftDeleteLibraryItemParams{ID: cur.ID, OrganizationID: actor.OrganizationID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) ListItems(ctx context.Context, actor Actor, in ListInput) ([]ItemView, int64, error) {
	var folderID *int64
	if in.FolderUUID != nil {
		row, err := s.readableFolder(ctx, actor, *in.FolderUUID)
		if err != nil {
			return nil, 0, err
		}
		folderID = &row.ID
	}
	limit := in.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	sortKey := in.SortKey
	if sortKey == "" {
		sortKey = ItemsSortSpec.Default.Field
	}
	tags := normalizeTags(in.Tags)
	if len(tags) == 0 {
		tags = nil
	}
	filter := db.CountLibraryItemsParams{
		BrandID: actor.BrandID, AccessLevels: accessLevels(actor.OrgType), ViewerRoleSlugs: actor.Roles,
		FolderID: int8Arg(folderID), Tags: tags, AccessFilter: in.AccessLevels,
		UpdatedFrom: tsArg(in.Updated.From), UpdatedBefore: tsArg(in.Updated.Before),
		Q: textValue(strings.TrimSpace(in.Query)),
	}
	items, err := s.q.ListLibraryItems(ctx, db.ListLibraryItemsParams{
		BrandID: filter.BrandID, AccessLevels: filter.AccessLevels, ViewerRoleSlugs: filter.ViewerRoleSlugs,
		FolderID: filter.FolderID, Tags: filter.Tags, AccessFilter: filter.AccessFilter,
		UpdatedFrom: filter.UpdatedFrom, UpdatedBefore: filter.UpdatedBefore, Q: filter.Q,
		SortKey: sortKey, SortDesc: in.SortDesc,
		PageOffset: maxInt32(in.Offset, 0), PageLimit: limit,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountLibraryItems(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	folders, err := s.q.ListLibraryFoldersByBrand(ctx, actor.BrandID)
	if err != nil {
		return nil, 0, err
	}
	folderUUIDs := make(map[int64]uuid.UUID, len(folders))
	for _, f := range folders {
		folderUUIDs[f.ID] = f.Uuid
	}
	locale := normalizeLocale(in.Locale)
	out := make([]ItemView, 0, len(items))
	for _, item := range items {
		var folderUUID *uuid.UUID
		if item.FolderID.Valid {
			if id, ok := folderUUIDs[item.FolderID.Int64]; ok {
				folderUUID = &id
			}
		}
		out = append(out, s.itemView(ctx, item, folderUUID, locale))
	}
	return out, total, nil
}

func (s *Service) AddVersion(ctx context.Context, actor Actor, itemID uuid.UUID, in UploadInput) (VersionView, error) {
	if actor.OrgType != OrgCenter {
		return VersionView{}, ErrForbidden
	}
	if s.store == nil {
		return VersionView{}, errors.New("library: storage is not configured")
	}
	item, err := s.itemForManage(ctx, actor, itemID)
	if err != nil {
		return VersionView{}, err
	}
	payload, mime, sum, err := readAndValidateUpload(in)
	if err != nil {
		return VersionView{}, err
	}
	locale := normalizeLocale(in.Locale)
	next, err := s.q.NextLibraryItemVersionNo(ctx, db.NextLibraryItemVersionNoParams{ItemID: item.ID, Locale: locale})
	if err != nil {
		return VersionView{}, err
	}
	key := storageKey(item.Uuid, locale, next, in.Filename)
	if err := s.store.Upload(ctx, platstorage.File{
		Body: bytes.NewReader(payload), Size: int64(len(payload)), ContentType: mime, Filename: in.Filename,
	}, key); err != nil {
		return VersionView{}, err
	}
	row, err := s.q.CreateLibraryItemVersion(ctx, db.CreateLibraryItemVersionParams{
		ItemID: item.ID, Locale: locale, VersionNo: next, StorageKey: key, Mime: mime,
		SizeBytes: int64(len(payload)), Sha256: sum, UploadedByUserID: int8Value(actor.UserID),
	})
	if err != nil {
		return VersionView{}, mapDBError(err)
	}
	return versionView(row), nil
}

func (s *Service) ListVersions(ctx context.Context, actor Actor, itemID uuid.UUID) ([]VersionView, error) {
	item, err := s.visibleItem(ctx, actor, itemID)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListLibraryItemVersions(ctx, item.ID)
	if err != nil {
		return nil, err
	}
	out := make([]VersionView, 0, len(rows))
	for _, row := range rows {
		out = append(out, versionView(row))
	}
	return out, nil
}

func (s *Service) Download(ctx context.Context, actor Actor, versionID uuid.UUID) (DownloadView, error) {
	if s.store == nil {
		return DownloadView{}, errors.New("library: storage is not configured")
	}
	version, err := s.q.GetLibraryItemVersionByUUID(ctx, versionID)
	if err != nil {
		return DownloadView{}, ErrNotFound
	}
	item, err := s.visibleItemByInternalID(ctx, actor, version.ItemID)
	if err != nil {
		return DownloadView{}, err
	}
	_ = item
	url, err := s.store.PresignGet(ctx, version.StorageKey, downloadURLTTL)
	if err != nil {
		return DownloadView{}, err
	}
	return DownloadView{URL: url, ExpiresAt: s.now().Add(downloadURLTTL).UTC()}, nil
}

func (s *Service) folder(ctx context.Context, actor Actor, id uuid.UUID) (db.LibraryFolder, error) {
	row, err := s.q.GetLibraryFolderByUUID(ctx, id)
	if err != nil {
		return db.LibraryFolder{}, ErrNotFound
	}
	if row.OrganizationID != actor.OrganizationID || !row.DeletedAt.Time.IsZero() && row.DeletedAt.Valid {
		return db.LibraryFolder{}, ErrNotFound
	}
	return row, nil
}

// readableFolders lists the folders the viewer may see (see ListFolders).
func (s *Service) readableFolders(ctx context.Context, actor Actor) ([]db.LibraryFolder, error) {
	if actor.OrgType == OrgCenter {
		return s.q.ListLibraryFoldersByBrand(ctx, actor.BrandID)
	}
	return s.q.ListVisibleLibraryFolders(ctx, db.ListVisibleLibraryFoldersParams{
		BrandID: actor.BrandID, OrganizationID: actor.OrganizationID,
		AccessLevels: accessLevels(actor.OrgType), ViewerRoleSlugs: actor.Roles,
	})
}

// readableFolder resolves a folder readable by the viewer (list filter); a
// folder hidden from the viewer is 404.
func (s *Service) readableFolder(ctx context.Context, actor Actor, id uuid.UUID) (db.LibraryFolder, error) {
	row, err := s.q.GetLibraryFolderByUUID(ctx, id)
	if err != nil || row.BrandID != actor.BrandID || row.DeletedAt.Valid {
		return db.LibraryFolder{}, ErrNotFound
	}
	if actor.OrgType == OrgCenter {
		return row, nil
	}
	rows, err := s.readableFolders(ctx, actor)
	if err != nil {
		return db.LibraryFolder{}, err
	}
	for _, f := range rows {
		if f.ID == row.ID {
			return row, nil
		}
	}
	return db.LibraryFolder{}, ErrNotFound
}

func (s *Service) folderID(ctx context.Context, actor Actor, id *uuid.UUID) (*int64, error) {
	if id == nil {
		return nil, nil
	}
	row, err := s.folder(ctx, actor, *id)
	if err != nil {
		return nil, err
	}
	return &row.ID, nil
}

func (s *Service) itemForManage(ctx context.Context, actor Actor, id uuid.UUID) (db.LibraryItem, error) {
	row, err := s.q.GetLibraryItemByUUID(ctx, id)
	if err != nil {
		return db.LibraryItem{}, ErrNotFound
	}
	if row.OrganizationID != actor.OrganizationID || row.BrandID != actor.BrandID {
		return db.LibraryItem{}, ErrNotFound
	}
	return row, nil
}

func (s *Service) visibleItem(ctx context.Context, actor Actor, id uuid.UUID) (db.LibraryItem, error) {
	row, err := s.q.GetLibraryItemByUUID(ctx, id)
	if err != nil {
		return db.LibraryItem{}, ErrNotFound
	}
	return s.visibleItemByRow(actor, row)
}

func (s *Service) visibleItemByInternalID(ctx context.Context, actor Actor, id int64) (db.LibraryItem, error) {
	row, err := s.q.GetLibraryItemByID(ctx, id)
	if err != nil {
		return db.LibraryItem{}, ErrNotFound
	}
	return s.visibleItemByRow(actor, row)
}

func (s *Service) visibleItemByRow(actor Actor, row db.LibraryItem) (db.LibraryItem, error) {
	if row.BrandID != actor.BrandID || !contains(accessLevels(actor.OrgType), row.AccessLevel) {
		return db.LibraryItem{}, ErrNotFound
	}
	if row.RoleSlug.Valid && !contains(actor.Roles, row.RoleSlug.String) {
		return db.LibraryItem{}, ErrNotFound
	}
	return row, nil
}

func (s *Service) itemView(ctx context.Context, item db.LibraryItem, folderUUID *uuid.UUID, locale string) ItemView {
	var latest *VersionView
	if versions, err := s.q.ListLatestLibraryItemVersions(ctx, item.ID); err == nil {
		if picked, ok := pickVersion(versions, locale); ok {
			v := versionView(picked)
			latest = &v
		}
	}
	desc := ""
	if item.Description.Valid {
		desc = item.Description.String
	}
	role := ""
	if item.RoleSlug.Valid {
		role = item.RoleSlug.String
	}
	return ItemView{
		UUID: item.Uuid, FolderUUID: folderUUID, Name: item.Name, Description: desc, Tags: item.Tags,
		AccessLevel: item.AccessLevel, RoleSlug: role, LatestVersion: latest,
		CreatedAt: timeString(item.CreatedAt), UpdatedAt: timeString(item.UpdatedAt),
	}
}

func readAndValidateUpload(in UploadInput) ([]byte, string, string, error) {
	filename := strings.TrimSpace(in.Filename)
	if filename == "" {
		return nil, "", "", invalid("file", "filename is required")
	}
	if in.Size > MaxUploadBytes {
		return nil, "", "", invalid("file", "file must be 50 MB or smaller")
	}
	limit := MaxUploadBytes + 1
	body, err := io.ReadAll(io.LimitReader(in.Body, int64(limit)))
	if err != nil {
		return nil, "", "", invalid("file", "could not read file")
	}
	if len(body) == 0 {
		return nil, "", "", invalid("file", "file cannot be empty")
	}
	if len(body) > MaxUploadBytes {
		return nil, "", "", invalid("file", "file must be 50 MB or smaller")
	}
	mime, err := sniffAllowed(filename, body)
	if err != nil {
		return nil, "", "", err
	}
	sum := sha256.Sum256(body)
	return body, mime, hex.EncodeToString(sum[:]), nil
}

func sniffAllowed(filename string, body []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	sniff := http.DetectContentType(body)
	switch ext {
	case ".pdf":
		if bytes.HasPrefix(body, []byte("%PDF-")) {
			return "application/pdf", nil
		}
	case ".zip":
		if isZip(body) {
			return "application/zip", nil
		}
	case ".docx":
		if isZip(body) {
			return "application/vnd.openxmlformats-officedocument.wordprocessingml.document", nil
		}
	case ".xlsx":
		if isZip(body) {
			return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", nil
		}
	case ".png":
		if strings.HasPrefix(sniff, "image/png") {
			return "image/png", nil
		}
	case ".jpg", ".jpeg":
		if strings.HasPrefix(sniff, "image/jpeg") {
			return "image/jpeg", nil
		}
	case ".gif":
		if strings.HasPrefix(sniff, "image/gif") {
			return "image/gif", nil
		}
	case ".webp":
		if len(body) >= 12 && string(body[:4]) == "RIFF" && string(body[8:12]) == "WEBP" {
			return "image/webp", nil
		}
	}
	return "", invalid("file", "file content does not match an allowed type")
}

func isZip(body []byte) bool {
	return len(body) >= 4 && body[0] == 'P' && body[1] == 'K' && (body[2] == 3 || body[2] == 5 || body[2] == 7) && (body[3] == 4 || body[3] == 6 || body[3] == 8)
}

func pickVersion(rows []db.LibraryItemVersion, locale string) (db.LibraryItemVersion, bool) {
	if len(rows) == 0 {
		return db.LibraryItemVersion{}, false
	}
	locale = normalizeLocale(locale)
	var fallback db.LibraryItemVersion
	hasFallback := false
	var any db.LibraryItemVersion
	for i, row := range rows {
		if i == 0 {
			any = row
		}
		if row.Locale == locale {
			return row, true
		}
		if row.Locale == defaultLocale {
			fallback, hasFallback = row, true
		}
	}
	if hasFallback {
		return fallback, true
	}
	return any, true
}

func accessLevels(orgType string) []string {
	switch orgType {
	case OrgCenter:
		return []string{AccessAllNetwork, AccessDistributors, AccessDealers, AccessCenterOnly}
	case OrgDistributor:
		return []string{AccessAllNetwork, AccessDistributors}
	case OrgDealer:
		return []string{AccessAllNetwork, AccessDealers}
	default:
		return []string{AccessAllNetwork}
	}
}

func storageKey(item uuid.UUID, locale string, version int32, filename string) string {
	name := strings.Trim(strings.ReplaceAll(filepath.Base(filename), "\\", "/"), "/")
	return fmt.Sprintf("library/%s/%s/v%d/%s", item.String(), locale, version, name)
}

func folderView(row db.LibraryFolder, byID map[int64]uuid.UUID) FolderView {
	var parent *uuid.UUID
	if row.ParentID.Valid && byID != nil {
		if id, ok := byID[row.ParentID.Int64]; ok {
			parent = &id
		}
	}
	return FolderView{UUID: row.Uuid, ParentUUID: parent, Name: row.Name, SortOrder: row.SortOrder, CreatedAt: timeString(row.CreatedAt), UpdatedAt: timeString(row.UpdatedAt)}
}

func versionView(row db.LibraryItemVersion) VersionView {
	return VersionView{UUID: row.Uuid, Locale: row.Locale, VersionNo: row.VersionNo, MIME: row.Mime, SizeBytes: row.SizeBytes, SHA256: row.Sha256, CreatedAt: timeString(row.CreatedAt)}
}

func validateAccess(access string) error {
	switch access {
	case AccessAllNetwork, AccessDistributors, AccessDealers, AccessCenterOnly:
		return nil
	default:
		return invalid("access_level", "must be all_network, distributors, dealers or center_only")
	}
}

func requiredString(field string, value *string) (string, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "", invalid(field, "is required")
	}
	return strings.TrimSpace(*value), nil
}

func normalizeTags(tags []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(tags))
	for _, raw := range tags {
		tag := strings.TrimSpace(raw)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func normalizeLocale(locale string) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return defaultLocale
	}
	if locale == "zh_CN" {
		return "zh-CN"
	}
	return locale
}

func int8Value(v int64) pgtype.Int8 {
	if v == 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: v, Valid: true}
}

func int8Arg(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

func int8Ptr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

func textPtr(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func textValue(v string) pgtype.Text {
	if strings.TrimSpace(v) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: strings.TrimSpace(v), Valid: true}
}

func tsArg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func timeString(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}

func maxInt32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func mapDBError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505", "23514", "23503":
			return invalid("body", "violates library constraints")
		}
	}
	return err
}
