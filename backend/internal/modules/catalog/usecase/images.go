package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// MaxProductImages caps the images of one product (upload and PATCH).
const MaxProductImages = 10

// ErrImagesChanged: the image list changed between read and write (409).
var ErrImagesChanged = errors.New("catalog: product images changed concurrently")

// uploadedKey is the flat key of an uploaded image: 32 hex + raster ext.
var uploadedKey = regexp.MustCompile(`^[0-9a-f]{32}\.(jpg|png|webp)$`)

// IsUploadedImageKey reports whether key was issued by the upload route
// (and so has an object under storage.ProductImageObjectKey).
func IsUploadedImageKey(key string) bool { return uploadedKey.MatchString(key) }

// NewImageKey returns a fresh, unguessable image key with ext.
func NewImageKey(ext string) string {
	return strings.ReplaceAll(uuid.NewString(), "-", "") + "." + strings.TrimPrefix(ext, ".")
}

func tooManyImages() error {
	return &ValidationError{Fields: []FieldError{{
		Field: "images", Code: "too_many", Message: fmt.Sprintf("at most %d images", MaxProductImages),
	}}}
}

// checkUploadedKeys refuses a create/PATCH image list that names an
// uploaded key the product did not already hold: uploaded objects live under
// their own product, so copying a key elsewhere would only break the public
// URL (and let one product claim another's image).
func checkUploadedKeys(prev []model.Image, in *[]model.Image) error {
	if in == nil {
		return nil
	}
	held := make(map[string]struct{}, len(prev))
	for _, img := range prev {
		held[img.Key] = struct{}{}
	}
	for _, img := range *in {
		key := strings.TrimSpace(img.Key)
		if _, ok := held[key]; !ok && IsUploadedImageKey(key) {
			return &ValidationError{Fields: []FieldError{{
				Field: "images", Code: "invalid", Message: "uploaded images are added through the image upload route",
			}}}
		}
	}
	return nil
}

// CheckImageSlot is the pre-upload check: center only, product of the active
// brand, fewer than MaxProductImages images. It avoids storing an object the
// append would reject; AddProductImage re-checks atomically.
func (s *Service) CheckImageSlot(ctx context.Context, org orgctx.Scope, id uuid.UUID) error {
	if err := requireCenter(org); err != nil {
		return err
	}
	cur, err := s.product(ctx, org.BrandID, id)
	if err != nil {
		return err
	}
	if len(decodeImages(cur.Images)) >= MaxProductImages {
		return tooManyImages()
	}
	return nil
}

// AddProductImage appends an uploaded image key to the product (center only).
func (s *Service) AddProductImage(ctx context.Context, org orgctx.Scope, id uuid.UUID, key string) (model.Product, error) {
	if err := requireCenter(org); err != nil {
		return model.Product{}, err
	}
	cur, err := s.product(ctx, org.BrandID, id)
	if err != nil {
		return model.Product{}, err
	}
	img, _ := json.Marshal(model.Image{Key: key, Sort: len(decodeImages(cur.Images))})
	row, err := s.store.AppendProductImage(ctx, db.AppendProductImageParams{
		Image: img, ID: cur.ID, BrandID: org.BrandID, MaxImages: MaxProductImages,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Product{}, tooManyImages()
	}
	if err != nil {
		return model.Product{}, mapDBError(err)
	}
	s.reindex(ctx, row.Uuid)
	return s.productView(ctx, row)
}

// RemoveProductImage drops key from the product (center only) and renumbers
// the sort order. ErrNotFound when the product does not list the key.
func (s *Service) RemoveProductImage(ctx context.Context, org orgctx.Scope, id uuid.UUID, key string) (model.Product, error) {
	if err := requireCenter(org); err != nil {
		return model.Product{}, err
	}
	cur, err := s.product(ctx, org.BrandID, id)
	if err != nil {
		return model.Product{}, err
	}
	images := decodeImages(cur.Images)
	next := make([]model.Image, 0, len(images))
	for _, img := range images {
		if img.Key != key {
			next = append(next, img)
		}
	}
	if len(next) == len(images) {
		return model.Product{}, ErrNotFound
	}
	return s.replaceImages(ctx, org, cur, next)
}

// ReorderProductImages sets the image order (center only). keys must name
// every current image exactly once.
func (s *Service) ReorderProductImages(ctx context.Context, org orgctx.Scope, id uuid.UUID, keys []string) (model.Product, error) {
	if err := requireCenter(org); err != nil {
		return model.Product{}, err
	}
	cur, err := s.product(ctx, org.BrandID, id)
	if err != nil {
		return model.Product{}, err
	}
	images := decodeImages(cur.Images)
	byKey := make(map[string]model.Image, len(images))
	for _, img := range images {
		byKey[img.Key] = img
	}
	invalid := &ValidationError{Fields: []FieldError{{
		Field: "keys", Code: "invalid", Message: "keys must list every product image exactly once",
	}}}
	if len(keys) != len(images) {
		return model.Product{}, invalid
	}
	next := make([]model.Image, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		img, ok := byKey[k]
		if _, dup := seen[k]; !ok || dup {
			return model.Product{}, invalid
		}
		seen[k] = struct{}{}
		next = append(next, img)
	}
	return s.replaceImages(ctx, org, cur, next)
}

func (s *Service) replaceImages(ctx context.Context, org orgctx.Scope, cur db.Product, next []model.Image) (model.Product, error) {
	for i := range next {
		next[i].Sort = i
	}
	raw, _ := json.Marshal(next)
	row, err := s.store.ReplaceProductImages(ctx, db.ReplaceProductImagesParams{
		Images: raw, ID: cur.ID, BrandID: org.BrandID, Expected: cur.Images,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Product{}, ErrImagesChanged
	}
	if err != nil {
		return model.Product{}, mapDBError(err)
	}
	s.reindex(ctx, row.Uuid)
	return s.productView(ctx, row)
}

// PublicImageProduct returns the uuid of the active product listing an
// uploaded image key (public, unauthenticated route). Inactive products and
// unknown or non-uploaded keys are ErrNotFound.
func (s *Service) PublicImageProduct(ctx context.Context, key string) (uuid.UUID, error) {
	if !IsUploadedImageKey(key) {
		return uuid.Nil, ErrNotFound
	}
	id, err := s.store.GetActiveProductUUIDByImageKey(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	return id, err
}
