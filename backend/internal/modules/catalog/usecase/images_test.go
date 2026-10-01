package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// imageStore keeps one product in memory for the image use cases.
type imageStore struct {
	fakeStore
	row db.Product
}

func (f *imageStore) GetProductByUUID(_ context.Context, arg db.GetProductByUUIDParams) (db.Product, error) {
	if arg.Uuid != f.row.Uuid || arg.BrandID != f.row.BrandID {
		return db.Product{}, pgx.ErrNoRows
	}
	return f.row, nil
}

func (f *imageStore) GetProductCategory(_ context.Context, arg db.GetProductCategoryParams) (db.ProductCategory, error) {
	return db.ProductCategory{ID: arg.ID, BrandID: arg.BrandID}, nil
}

func (f *imageStore) AppendProductImage(_ context.Context, arg db.AppendProductImageParams) (db.Product, error) {
	cur := decodeImages(f.row.Images)
	if len(cur) >= int(arg.MaxImages) {
		return db.Product{}, pgx.ErrNoRows
	}
	var img model.Image
	_ = json.Unmarshal(arg.Image, &img)
	f.row.Images, _ = json.Marshal(append(cur, img))
	return f.row, nil
}

func (f *imageStore) ReplaceProductImages(_ context.Context, arg db.ReplaceProductImagesParams) (db.Product, error) {
	if !bytes.Equal(arg.Expected, f.row.Images) {
		return db.Product{}, pgx.ErrNoRows
	}
	f.row.Images = arg.Images
	return f.row, nil
}

func newImageStore(keys ...string) *imageStore {
	imgs := make([]model.Image, 0, len(keys))
	for i, k := range keys {
		imgs = append(imgs, model.Image{Key: k, Sort: i})
	}
	raw, _ := json.Marshal(imgs)
	return &imageStore{row: db.Product{ID: 7, Uuid: uuid.New(), BrandID: 1, Images: raw}}
}

func keysOf(p model.Product) []string {
	out := make([]string, 0, len(p.Images))
	for _, img := range p.Images {
		out = append(out, img.Key)
	}
	return out
}

func TestProductImageWritesAreCenterOnly(t *testing.T) {
	store := newImageStore()
	svc := New(store, nil)
	ctx := context.Background()
	id := store.row.Uuid
	for name, org := range map[string]orgctx.Scope{"distributor": distributor, "dealer": dealer, "none": {}} {
		calls := map[string]error{}
		calls["check slot"] = svc.CheckImageSlot(ctx, org, id)
		_, calls["add"] = svc.AddProductImage(ctx, org, id, NewImageKey("png"))
		_, calls["remove"] = svc.RemoveProductImage(ctx, org, id, "x")
		_, calls["reorder"] = svc.ReorderProductImages(ctx, org, id, nil)
		for op, err := range calls {
			if !errors.Is(err, ErrCenterOnly) {
				t.Errorf("%s %s: err = %v, want ErrCenterOnly", name, op, err)
			}
		}
	}
}

func TestProductImagesAddRemoveReorder(t *testing.T) {
	store := newImageStore()
	idx := &fakeIndexer{}
	svc := New(store, idx)
	ctx := context.Background()
	id := store.row.Uuid

	var keys []string
	for i := 0; i < MaxProductImages; i++ {
		k := NewImageKey("webp")
		if !IsUploadedImageKey(k) {
			t.Fatalf("NewImageKey %q is not an uploaded key", k)
		}
		p, err := svc.AddProductImage(ctx, center, id, k)
		if err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
		if got := p.Images[len(p.Images)-1]; got.Key != k || got.Sort != i {
			t.Fatalf("image %d = %+v", i, got)
		}
		keys = append(keys, k)
	}
	var verr *ValidationError
	if err := svc.CheckImageSlot(ctx, center, id); !errors.As(err, &verr) {
		t.Fatalf("check slot when full: %v, want validation", err)
	}
	if _, err := svc.AddProductImage(ctx, center, id, NewImageKey("png")); !errors.As(err, &verr) || verr.Fields[0].Code != "too_many" {
		t.Fatalf("11th image: %v, want too_many", err)
	}

	// Reorder: reverse; a partial or duplicated list is refused.
	rev := make([]string, len(keys))
	for i, k := range keys {
		rev[len(keys)-1-i] = k
	}
	p, err := svc.ReorderProductImages(ctx, center, id, rev)
	if err != nil {
		t.Fatalf("reorder: %v", err)
	}
	if got := keysOf(p); got[0] != keys[len(keys)-1] || p.Images[0].Sort != 0 {
		t.Fatalf("reordered = %v", p.Images)
	}
	if _, err := svc.ReorderProductImages(ctx, center, id, rev[1:]); !errors.As(err, &verr) {
		t.Fatalf("partial reorder: %v", err)
	}
	dup := append([]string{rev[0]}, rev[:len(rev)-1]...)
	if _, err := svc.ReorderProductImages(ctx, center, id, dup); !errors.As(err, &verr) {
		t.Fatalf("duplicate reorder: %v", err)
	}

	// Remove renumbers; an unknown key is ErrNotFound.
	p, err = svc.RemoveProductImage(ctx, center, id, rev[0])
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(p.Images) != MaxProductImages-1 || p.Images[0].Key != rev[1] || p.Images[0].Sort != 0 {
		t.Fatalf("after remove = %v", p.Images)
	}
	if _, err := svc.RemoveProductImage(ctx, center, id, rev[0]); !errors.Is(err, ErrNotFound) {
		t.Fatalf("remove twice: %v", err)
	}
	if len(idx.upserts) == 0 {
		t.Fatal("image changes were not reindexed")
	}
}

func TestReplaceImagesDetectsConcurrentChange(t *testing.T) {
	store := newImageStore("a", "b")
	svc := New(store, nil)
	cur := store.row
	store.row.Images = []byte(`[{"key":"b","sort":0}]`)
	if _, err := svc.replaceImages(context.Background(), center, cur, []model.Image{{Key: "a"}}); !errors.Is(err, ErrImagesChanged) {
		t.Fatalf("err = %v, want ErrImagesChanged", err)
	}
}

func TestPatchCannotClaimUploadedKeys(t *testing.T) {
	held := NewImageKey("png")
	foreign := NewImageKey("png")
	prev := []model.Image{{Key: held}}
	if err := checkUploadedKeys(prev, &[]model.Image{{Key: held}, {Key: "legacy/key.jpg"}}); err != nil {
		t.Fatalf("held + legacy keys: %v", err)
	}
	var verr *ValidationError
	if err := checkUploadedKeys(prev, &[]model.Image{{Key: foreign}}); !errors.As(err, &verr) {
		t.Fatalf("foreign uploaded key: %v", err)
	}
	if err := checkUploadedKeys(prev, nil); err != nil {
		t.Fatalf("no images in patch: %v", err)
	}
}

func TestPublicImageProductRejectsNonUploadedKeys(t *testing.T) {
	svc := New(&fakeStore{}, nil)
	for _, key := range []string{"", "../x", "products/a/b.png", "abc.svg", "0123456789abcdef0123456789abcdef.svg"} {
		if _, err := svc.PublicImageProduct(context.Background(), key); !errors.Is(err, ErrNotFound) {
			t.Errorf("key %q: %v, want ErrNotFound", key, err)
		}
	}
}
