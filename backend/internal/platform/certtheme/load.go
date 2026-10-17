package certtheme

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

// maxLogoBytes caps the car make logo read from storage.
const maxLogoBytes = 2 << 20

// Load gathers the certificate pages of a service: ok is false for a brand
// without them. vehicle carries the make, model and year the caller already
// resolved; Load adds the make logo (best effort), the applied parts and one
// services row per item with its category, unit barcode and the end of its
// warranty printed by date. keep (nil keeps all) selects the items by unit
// barcode, so a holder's certificate lists only that holder's warranties.
func Load(
	ctx context.Context,
	q *db.Queries,
	store storage.Driver,
	svc db.Service,
	vehicle Vehicle,
	date func(time.Time) string,
	keep func(barcode string) bool,
) (Input, bool, error) {
	brand, err := q.GetBrandByID(ctx, svc.BrandID)
	if err != nil {
		return Input{}, false, fmt.Errorf("certtheme: brand: %w", err)
	}
	if !ForBrand(brand.Slug) {
		return Input{}, false, nil
	}
	in := Input{Vehicle: vehicle}
	if cb, err := q.GetCarBrandByID(ctx, svc.CarBrandID); err == nil && cb.LogoObjectKey.Valid && store != nil {
		in.Vehicle.Logo, in.Vehicle.LogoMIME = readLogo(ctx, store, cb.LogoObjectKey.String)
	}
	items, err := q.ListServiceItems(ctx, svc.ID)
	if err != nil {
		return Input{}, false, fmt.Errorf("certtheme: items: %w", err)
	}
	warranties, err := q.ListWarrantiesByService(ctx, db.ListWarrantiesByServiceParams{ServiceID: svc.ID, BrandID: svc.BrandID})
	if err != nil {
		return Input{}, false, fmt.Errorf("certtheme: warranties: %w", err)
	}
	ends := map[int64]time.Time{}
	for _, w := range warranties {
		if w.Status == "voided" || !w.EndAt.Valid {
			continue
		}
		ends[w.ServiceItemID] = w.EndAt.Time
	}
	seen := map[string]bool{}
	categories := map[int64]string{}
	for _, it := range items {
		code := ""
		if u, err := q.GetUnit(ctx, it.UnitID); err == nil {
			code = u.Barcode
		}
		if keep != nil && !keep(code) {
			continue
		}
		var parts []string
		_ = json.Unmarshal(it.AppliedParts, &parts)
		for _, p := range parts {
			if !seen[p] {
				seen[p] = true
				in.Parts = append(in.Parts, p)
			}
		}
		p, err := q.GetProduct(ctx, db.GetProductParams{ID: it.ProductID, BrandID: svc.BrandID})
		if err != nil {
			return Input{}, false, fmt.Errorf("certtheme: product: %w", err)
		}
		cat, ok := categories[p.CategoryID]
		if !ok {
			c, err := q.GetProductCategory(ctx, db.GetProductCategoryParams{ID: p.CategoryID, BrandID: svc.BrandID})
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return Input{}, false, fmt.Errorf("certtheme: category: %w", err)
			}
			cat = c.Name
			categories[p.CategoryID] = cat
		}
		row := Service{Category: cat, Code: code, Product: p.Name}
		if end, ok := ends[it.ID]; ok && date != nil {
			row.Warranty = date(end)
		}
		in.Services = append(in.Services, row)
	}
	return in, true, nil
}

// readLogo downloads a catalog logo; a failure only drops the logo.
func readLogo(ctx context.Context, store storage.Driver, key string) ([]byte, string) {
	rc, _, err := store.Download(ctx, key)
	if err != nil {
		return nil, ""
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, maxLogoBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxLogoBytes {
		return nil, ""
	}
	mime := http.DetectContentType(data)
	if strings.HasPrefix(mime, "text/") && bytes.Contains(data, []byte("<svg")) {
		// SVG sniffs as text; an <img> SVG runs no script.
		mime = "image/svg+xml"
	}
	return data, mime
}
