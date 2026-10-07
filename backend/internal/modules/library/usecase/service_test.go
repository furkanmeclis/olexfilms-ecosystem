package usecase

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	platstorage "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestService_ListAndDownload_HidesCenterOnlyFromDealer(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	storage := &fakeStorage{}
	svc := New(store, storage)
	centerOnly := store.seedItem(1, 10, 100, AccessCenterOnly, nil)
	version := store.seedVersion(centerOnly.ID, "tr", 1, "library/center.pdf")

	dealer := Actor{UserID: 55, OrganizationID: 300, BrandID: 10, OrgType: OrgDealer, Roles: []string{"dealer_owner"}}
	items, _, err := svc.ListItems(ctx, dealer, ListInput{Locale: "tr"})
	if err != nil {
		t.Fatalf("ListItems() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("ListItems() returned %d items, want 0", len(items))
	}
	_, err = svc.Download(ctx, dealer, version.Uuid)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Download() error = %v, want ErrNotFound", err)
	}
}

func TestService_AddVersion_ListReturnsLatestAndKeepsHistory(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	storage := &fakeStorage{}
	svc := New(store, storage)
	item := store.seedItem(1, 10, 100, AccessAllNetwork, []string{"guide"})
	center := Actor{UserID: 9, OrganizationID: 100, BrandID: 10, OrgType: OrgCenter, Roles: []string{"center_staff"}}

	v1, err := svc.AddVersion(ctx, center, item.Uuid, UploadInput{
		Locale: "tr", Filename: "guide.pdf", Body: strings.NewReader("%PDF-1.7 first"),
	})
	if err != nil {
		t.Fatalf("AddVersion(v1) error = %v", err)
	}
	v2, err := svc.AddVersion(ctx, center, item.Uuid, UploadInput{
		Locale: "tr", Filename: "guide.pdf", Body: strings.NewReader("%PDF-1.7 second"),
	})
	if err != nil {
		t.Fatalf("AddVersion(v2) error = %v", err)
	}
	if v1.VersionNo != 1 || v2.VersionNo != 2 {
		t.Fatalf("versions = %d, %d; want 1, 2", v1.VersionNo, v2.VersionNo)
	}

	items, _, err := svc.ListItems(ctx, center, ListInput{Locale: "tr"})
	if err != nil {
		t.Fatalf("ListItems() error = %v", err)
	}
	if len(items) != 1 || items[0].LatestVersion == nil || items[0].LatestVersion.UUID != v2.UUID {
		t.Fatalf("latest version = %+v, want v2 %s", items, v2.UUID)
	}
	history, err := svc.ListVersions(ctx, center, item.Uuid)
	if err != nil {
		t.Fatalf("ListVersions() error = %v", err)
	}
	if len(history) != 2 || history[0].UUID != v2.UUID || history[1].UUID != v1.UUID {
		t.Fatalf("history = %+v, want v2 then v1", history)
	}
}

func TestService_ListItems_FallsBackToDefaultLocale(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	svc := New(store, &fakeStorage{})
	item := store.seedItem(1, 10, 100, AccessAllNetwork, []string{"price-list"})
	tr := store.seedVersion(item.ID, "tr", 1, "library/price.pdf")
	viewer := Actor{UserID: 7, OrganizationID: 200, BrandID: 10, OrgType: OrgDistributor, Roles: []string{"distributor_owner"}}

	items, _, err := svc.ListItems(ctx, viewer, ListInput{Locale: "de"})
	if err != nil {
		t.Fatalf("ListItems() error = %v", err)
	}
	if len(items) != 1 || items[0].LatestVersion == nil || items[0].LatestVersion.UUID != tr.Uuid {
		t.Fatalf("latest version = %+v, want tr fallback %s", items, tr.Uuid)
	}
}

func TestService_AddVersion_RejectsPDFExtensionWithZIPContent(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	svc := New(store, &fakeStorage{})
	item := store.seedItem(1, 10, 100, AccessAllNetwork, nil)
	center := Actor{UserID: 9, OrganizationID: 100, BrandID: 10, OrgType: OrgCenter, Roles: []string{"center_staff"}}

	_, err := svc.AddVersion(ctx, center, item.Uuid, UploadInput{
		Locale: "tr", Filename: "fake.pdf", Body: bytes.NewReader([]byte{'P', 'K', 3, 4, 'z', 'i', 'p'}),
	})
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Field != "file" {
		t.Fatalf("AddVersion() error = %v, want file validation error", err)
	}
}

func TestService_DeleteFolder_RejectsNonEmptyFolder(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	svc := New(store, &fakeStorage{})
	folder := store.seedFolder(10, 100)
	item := store.seedItem(1, 10, 100, AccessAllNetwork, nil)
	item.FolderID = pgtype.Int8{Int64: folder.ID, Valid: true}
	store.items[item.Uuid] = item
	center := Actor{UserID: 9, OrganizationID: 100, BrandID: 10, OrgType: OrgCenter, Roles: []string{"center_staff"}}

	err := svc.DeleteFolder(ctx, center, folder.Uuid)
	if !errors.Is(err, ErrFolderNotEmpty) {
		t.Fatalf("DeleteFolder() error = %v, want ErrFolderNotEmpty", err)
	}
}

func TestService_ListItems_DealerBrowsesCenterFolders(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	svc := New(store, &fakeStorage{})
	folder := store.seedFolder(10, 100)
	inFolder := store.seedItem(0, 10, 100, AccessAllNetwork, []string{"guide"})
	inFolder.FolderID = pgtype.Int8{Int64: folder.ID, Valid: true}
	store.items[inFolder.Uuid] = inFolder
	store.seedItem(0, 10, 100, AccessAllNetwork, []string{"price"})
	dealer := Actor{UserID: 55, OrganizationID: 300, BrandID: 10, OrgType: OrgDealer, Roles: []string{"dealer_owner"}}

	folders, err := svc.ListFolders(ctx, dealer)
	if err != nil || len(folders) != 1 || folders[0].UUID != folder.Uuid {
		t.Fatalf("ListFolders() = %+v, %v; want the center folder", folders, err)
	}
	items, total, err := svc.ListItems(ctx, dealer, ListInput{FolderUUID: &folder.Uuid, Tags: []string{"guide", "x"}})
	if err != nil {
		t.Fatalf("ListItems() error = %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].UUID != inFolder.Uuid {
		t.Fatalf("ListItems() = %+v (total %d), want the folder item", items, total)
	}
	if items[0].FolderUUID == nil || *items[0].FolderUUID != folder.Uuid {
		t.Fatalf("folder_uuid = %v, want %s", items[0].FolderUUID, folder.Uuid)
	}
	other := uuid.New()
	if _, _, err := svc.ListItems(ctx, dealer, ListInput{FolderUUID: &other}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ListItems(unknown folder) error = %v, want ErrNotFound", err)
	}
}

type fakeStore struct {
	nextID  int64
	folders map[uuid.UUID]db.LibraryFolder
	items   map[uuid.UUID]db.LibraryItem
	vers    map[uuid.UUID]db.LibraryItemVersion
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		nextID:  1,
		folders: map[uuid.UUID]db.LibraryFolder{},
		items:   map[uuid.UUID]db.LibraryItem{},
		vers:    map[uuid.UUID]db.LibraryItemVersion{},
	}
}

func (f *fakeStore) seedFolder(brandID, orgID int64) db.LibraryFolder {
	row := db.LibraryFolder{ID: f.next(), Uuid: uuid.New(), BrandID: brandID, OrganizationID: orgID, Name: "Guides", CreatedAt: ts(), UpdatedAt: ts()}
	f.folders[row.Uuid] = row
	return row
}

func (f *fakeStore) seedItem(id, brandID, orgID int64, access string, tags []string) db.LibraryItem {
	row := db.LibraryItem{ID: id, Uuid: uuid.New(), BrandID: brandID, OrganizationID: orgID, Name: "Guide", Tags: tags, AccessLevel: access, CreatedAt: ts(), UpdatedAt: ts()}
	if row.ID == 0 {
		row.ID = f.next()
	}
	f.items[row.Uuid] = row
	return row
}

func (f *fakeStore) seedVersion(itemID int64, locale string, no int32, key string) db.LibraryItemVersion {
	row := db.LibraryItemVersion{ID: f.next(), Uuid: uuid.New(), ItemID: itemID, Locale: locale, VersionNo: no, StorageKey: key, Mime: "application/pdf", SizeBytes: 12, Sha256: strings.Repeat("a", 64), CreatedAt: ts()}
	f.vers[row.Uuid] = row
	return row
}

func (f *fakeStore) next() int64 {
	id := f.nextID
	f.nextID++
	return id
}

func (f *fakeStore) CreateLibraryFolder(context.Context, db.CreateLibraryFolderParams) (db.LibraryFolder, error) {
	return db.LibraryFolder{}, nil
}
func (f *fakeStore) GetLibraryFolderByUUID(_ context.Context, id uuid.UUID) (db.LibraryFolder, error) {
	row, ok := f.folders[id]
	if !ok {
		return db.LibraryFolder{}, pgx.ErrNoRows
	}
	return row, nil
}
func (f *fakeStore) ListLibraryFolders(context.Context, int64) ([]db.LibraryFolder, error) {
	return nil, nil
}
func (f *fakeStore) ListLibraryFoldersByBrand(_ context.Context, brandID int64) ([]db.LibraryFolder, error) {
	var out []db.LibraryFolder
	for _, row := range f.folders {
		if row.BrandID == brandID {
			out = append(out, row)
		}
	}
	return out, nil
}
func (f *fakeStore) ListVisibleLibraryFolders(_ context.Context, arg db.ListVisibleLibraryFoldersParams) ([]db.LibraryFolder, error) {
	byID := map[int64]db.LibraryFolder{}
	for _, row := range f.folders {
		if row.BrandID == arg.BrandID && !row.DeletedAt.Valid {
			byID[row.ID] = row
		}
	}
	keep := map[int64]bool{}
	mark := func(id int64) {
		for row, ok := byID[id]; ok && !keep[row.ID]; row, ok = byID[row.ParentID.Int64] {
			keep[row.ID] = true
			if !row.ParentID.Valid {
				break
			}
		}
	}
	for _, row := range byID {
		if row.OrganizationID == arg.OrganizationID {
			mark(row.ID)
		}
	}
	for _, item := range f.items {
		if item.BrandID == arg.BrandID && item.FolderID.Valid && !item.DeletedAt.Valid &&
			contains(arg.AccessLevels, item.AccessLevel) &&
			(!item.RoleSlug.Valid || contains(arg.ViewerRoleSlugs, item.RoleSlug.String)) {
			mark(item.FolderID.Int64)
		}
	}
	var out []db.LibraryFolder
	for _, row := range f.folders {
		if keep[row.ID] {
			out = append(out, row)
		}
	}
	return out, nil
}
func (f *fakeStore) CountLibraryItems(ctx context.Context, arg db.CountLibraryItemsParams) (int64, error) {
	rows, err := f.ListLibraryItems(ctx, db.ListLibraryItemsParams{
		BrandID: arg.BrandID, AccessLevels: arg.AccessLevels, ViewerRoleSlugs: arg.ViewerRoleSlugs,
		FolderID: arg.FolderID, Tags: arg.Tags,
	})
	return int64(len(rows)), err
}

func anyContains(values, needles []string) bool {
	for _, n := range needles {
		if contains(values, n) {
			return true
		}
	}
	return false
}
func (f *fakeStore) UpdateLibraryFolder(context.Context, db.UpdateLibraryFolderParams) (db.LibraryFolder, error) {
	return db.LibraryFolder{}, nil
}
func (f *fakeStore) SoftDeleteLibraryFolder(_ context.Context, arg db.SoftDeleteLibraryFolderParams) (int64, error) {
	for _, item := range f.items {
		if item.OrganizationID == arg.OrganizationID && item.FolderID.Valid && item.FolderID.Int64 == arg.ID {
			return 0, nil
		}
	}
	return 1, nil
}
func (f *fakeStore) CreateLibraryItem(context.Context, db.CreateLibraryItemParams) (db.LibraryItem, error) {
	return db.LibraryItem{}, nil
}
func (f *fakeStore) GetLibraryItemByID(_ context.Context, id int64) (db.LibraryItem, error) {
	for _, row := range f.items {
		if row.ID == id {
			return row, nil
		}
	}
	return db.LibraryItem{}, pgx.ErrNoRows
}
func (f *fakeStore) GetLibraryItemByUUID(_ context.Context, id uuid.UUID) (db.LibraryItem, error) {
	row, ok := f.items[id]
	if !ok {
		return db.LibraryItem{}, pgx.ErrNoRows
	}
	return row, nil
}
func (f *fakeStore) UpdateLibraryItem(context.Context, db.UpdateLibraryItemParams) (db.LibraryItem, error) {
	return db.LibraryItem{}, nil
}
func (f *fakeStore) SoftDeleteLibraryItem(context.Context, db.SoftDeleteLibraryItemParams) (int64, error) {
	return 1, nil
}
func (f *fakeStore) ListLibraryItems(_ context.Context, arg db.ListLibraryItemsParams) ([]db.LibraryItem, error) {
	var out []db.LibraryItem
	for _, item := range f.items {
		if item.BrandID != arg.BrandID || !contains(arg.AccessLevels, item.AccessLevel) {
			continue
		}
		if item.RoleSlug.Valid && !contains(arg.ViewerRoleSlugs, item.RoleSlug.String) {
			continue
		}
		if arg.FolderID.Valid && (!item.FolderID.Valid || item.FolderID.Int64 != arg.FolderID.Int64) {
			continue
		}
		if len(arg.Tags) > 0 && !anyContains(item.Tags, arg.Tags) {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}
func (f *fakeStore) NextLibraryItemVersionNo(_ context.Context, arg db.NextLibraryItemVersionNoParams) (int32, error) {
	var max int32
	for _, row := range f.vers {
		if row.ItemID == arg.ItemID && row.Locale == arg.Locale && row.VersionNo > max {
			max = row.VersionNo
		}
	}
	return max + 1, nil
}
func (f *fakeStore) CreateLibraryItemVersion(_ context.Context, arg db.CreateLibraryItemVersionParams) (db.LibraryItemVersion, error) {
	row := db.LibraryItemVersion{
		ID: f.next(), Uuid: uuid.New(), ItemID: arg.ItemID, Locale: arg.Locale, VersionNo: arg.VersionNo,
		StorageKey: arg.StorageKey, Mime: arg.Mime, SizeBytes: arg.SizeBytes, Sha256: arg.Sha256,
		UploadedByUserID: arg.UploadedByUserID, CreatedAt: ts(),
	}
	f.vers[row.Uuid] = row
	return row, nil
}
func (f *fakeStore) ListLibraryItemVersions(_ context.Context, itemID int64) ([]db.LibraryItemVersion, error) {
	var out []db.LibraryItemVersion
	for _, row := range f.vers {
		if row.ItemID == itemID {
			out = append(out, row)
		}
	}
	sortVersions(out)
	return out, nil
}
func (f *fakeStore) ListLatestLibraryItemVersions(_ context.Context, itemID int64) ([]db.LibraryItemVersion, error) {
	latest := map[string]db.LibraryItemVersion{}
	for _, row := range f.vers {
		if row.ItemID != itemID {
			continue
		}
		cur, ok := latest[row.Locale]
		if !ok || row.VersionNo > cur.VersionNo {
			latest[row.Locale] = row
		}
	}
	out := make([]db.LibraryItemVersion, 0, len(latest))
	for _, row := range latest {
		out = append(out, row)
	}
	sortVersions(out)
	return out, nil
}
func (f *fakeStore) GetLibraryItemVersionByUUID(_ context.Context, id uuid.UUID) (db.LibraryItemVersion, error) {
	row, ok := f.vers[id]
	if !ok {
		return db.LibraryItemVersion{}, pgx.ErrNoRows
	}
	return row, nil
}

type fakeStorage struct {
	uploaded map[string][]byte
}

func (f *fakeStorage) Upload(_ context.Context, file platstorage.File, path string) error {
	if f.uploaded == nil {
		f.uploaded = map[string][]byte{}
	}
	body, err := io.ReadAll(file.Body)
	if err != nil {
		return err
	}
	f.uploaded[path] = body
	return nil
}

func (f *fakeStorage) PresignGet(_ context.Context, path string, _ time.Duration) (string, error) {
	return "https://example.test/" + path, nil
}

func ts() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Valid: true}
}

func sortVersions(rows []db.LibraryItemVersion) {
	for i := 0; i < len(rows)-1; i++ {
		for j := i + 1; j < len(rows); j++ {
			if rows[i].Locale > rows[j].Locale || (rows[i].Locale == rows[j].Locale && rows[i].VersionNo < rows[j].VersionNo) {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}
}
