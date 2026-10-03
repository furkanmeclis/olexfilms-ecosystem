package migrator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

// VehicleCatalogStep imports the hub car brands and models into the global
// vehicle catalog and copies the brand logos from the legacy hub storage to
// the object store (TEC-256).
//
// A legacy brand or model that matches a row already here (same external id,
// else same name, case insensitive) is linked to it, not duplicated, and that
// row is left as it is; only a missing logo is filled in.
type VehicleCatalogStep struct {
	// System is the migration_map source system; empty means SourceHub.
	System string
}

// Name implements Step.
func (VehicleCatalogStep) Name() string { return "vehicle_catalog" }

func (s VehicleCatalogStep) system() string {
	if s.System == "" {
		return SourceHub
	}
	return s.System
}

const carBrandsQuery = `SELECT id, name, COALESCE(external_id, ''), COALESCE(logo, ''), is_active, show_name,
	logo_height, deleted_at, created_at, updated_at
FROM car_brands`

const carModelsQuery = `SELECT id, brand_id, name, COALESCE(external_id, ''), COALESCE(powertrain, ''),
	yearstart, yearstop, COALESCE(coupe, ''), is_active, deleted_at, created_at, updated_at
FROM car_models`

// Column limits of car_brands / car_models (000044).
const (
	carExternalIDMax = 64
	carBrandNameMax  = 150
	carModelNameMax  = 200
	carAttrMax       = 64
)

// legacyLogoRoots are the directories under LEGACY_HUB_STORAGE_DIR a legacy
// logo path ("car-brands/x.png", Laravel public disk) is looked up in.
var legacyLogoRoots = []string{"app/public", "public", ""}

type legacyCarBrand struct {
	ID                     int64
	Name, ExternalID, Logo string
	Active, ShowName       bool
	LogoHeight             sql.NullInt64
	DeletedAt              sql.NullTime
	CreatedAt, UpdatedAt   sql.NullTime
}

type legacyCarModel struct {
	ID, BrandID                        int64
	Name, ExternalID, Powertrain, Body string
	YearStart, YearStop                sql.NullInt64
	Active                             bool
	DeletedAt                          sql.NullTime
	CreatedAt, UpdatedAt               sql.NullTime
}

// Run implements Step.
func (s VehicleCatalogStep) Run(ctx context.Context, src Sources, dst *Target, m *Mapper) (StepResult, error) {
	c := counts{}
	hub, err := src.Get(SourceHub)
	if err != nil {
		return StepResult{}, err
	}
	where, args := deltaFilter(dst)

	rows, err := hub.Query(ctx, carBrandsQuery+where+" ORDER BY id", args...)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	var brands []legacyCarBrand
	for rows.Next() {
		var b legacyCarBrand
		if err := rows.Scan(&b.ID, &b.Name, &b.ExternalID, &b.Logo, &b.Active, &b.ShowName, &b.LogoHeight,
			&b.DeletedAt, &b.CreatedAt, &b.UpdatedAt); err != nil {
			_ = rows.Close()
			return StepResult{Counts: c}, fmt.Errorf("scan car brand: %w", err)
		}
		brands = append(brands, b)
	}
	if err := rows.Close(); err != nil {
		return StepResult{Counts: c}, fmt.Errorf("read car brands: %w", err)
	}

	var watermark time.Time
	for _, b := range brands {
		c.inc("brands_read")
		if ts := latest(b.CreatedAt, b.UpdatedAt, b.DeletedAt); ts.After(watermark) {
			watermark = ts
		}
		if err := s.importBrand(ctx, dst, m, b, c); err != nil {
			return StepResult{Counts: c}, fmt.Errorf("car brand %d: %w", b.ID, err)
		}
	}

	rows, err = hub.Query(ctx, carModelsQuery+where+" ORDER BY id", args...)
	if err != nil {
		return StepResult{Counts: c}, err
	}
	var models []legacyCarModel
	for rows.Next() {
		var md legacyCarModel
		if err := rows.Scan(&md.ID, &md.BrandID, &md.Name, &md.ExternalID, &md.Powertrain, &md.YearStart,
			&md.YearStop, &md.Body, &md.Active, &md.DeletedAt, &md.CreatedAt, &md.UpdatedAt); err != nil {
			_ = rows.Close()
			return StepResult{Counts: c}, fmt.Errorf("scan car model: %w", err)
		}
		models = append(models, md)
	}
	if err := rows.Close(); err != nil {
		return StepResult{Counts: c}, fmt.Errorf("read car models: %w", err)
	}
	brandIDs := map[int64]int64{}
	for _, md := range models {
		c.inc("models_read")
		if ts := latest(md.CreatedAt, md.UpdatedAt, md.DeletedAt); ts.After(watermark) {
			watermark = ts
		}
		if err := s.importModel(ctx, dst.Q, m, brandIDs, md, c); err != nil {
			return StepResult{Counts: c}, fmt.Errorf("car model %d: %w", md.ID, err)
		}
	}
	return StepResult{Counts: c, Watermark: watermark}, nil
}

// deltaFilter is the delta-mode WHERE clause of the tables with Laravel
// timestamps (soft deletes bump updated_at).
func deltaFilter(dst *Target) (string, []any) { return deltaFilterOn(dst, "") }

// deltaFilterOn is deltaFilter with the timestamp columns qualified by
// alias (e.g. "p.").
func deltaFilterOn(dst *Target, alias string) (string, []any) {
	if dst.Mode == ModeDelta && !dst.Since.IsZero() {
		return " WHERE COALESCE(" + alias + "updated_at, " + alias + "created_at) > ?", []any{dst.Since}
	}
	return "", nil
}

func (s VehicleCatalogStep) importBrand(ctx context.Context, dst *Target, m *Mapper, b legacyCarBrand, c counts) error {
	q := dst.Q
	name := truncate(strings.TrimSpace(b.Name), carBrandNameMax)
	if name == "" {
		c.inc("brand_skipped_no_name:" + strconv.FormatInt(b.ID, 10))
		return nil
	}
	extID := legacyExternalID(b.ExternalID, c, "brand", b.ID)
	logoHeight := pgtype.Int2{}
	if b.LogoHeight.Valid && b.LogoHeight.Int64 >= 8 && b.LogoHeight.Int64 <= 512 {
		logoHeight = pgtype.Int2{Int16: int16(b.LogoHeight.Int64), Valid: true}
	}
	active := b.Active && !b.DeletedAt.Valid
	if b.DeletedAt.Valid {
		c.inc("brand_deleted_inactive")
	}

	key := Key{System: s.system(), Table: "car_brands", ID: strconv.FormatInt(b.ID, 10), TargetTable: "car_brands"}
	sum := Checksum(b.Name, b.ExternalID, b.Logo, b.Active, b.ShowName, b.LogoHeight.Int64, b.LogoHeight.Valid, b.DeletedAt.Valid)

	_, mapped, err := m.Lookup(ctx, key.System, key.Table, key.ID)
	if err != nil {
		return err
	}
	if !mapped {
		existing, err := q.MigratorFindCarBrand(ctx, db.MigratorFindCarBrandParams{ExternalID: extID, Name: name})
		switch {
		case err == nil:
			linked, err := m.Link(ctx, key, existing.Uuid, sum)
			if err != nil {
				return err
			}
			if linked {
				c.inc("brands_linked_existing")
				if !existing.LogoObjectKey.Valid {
					return s.copyLogo(ctx, dst, existing.ID, existing.Uuid, b, c)
				}
				return nil
			}
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("find car brand: %w", err)
		}
	}

	res, err := m.Upsert(ctx, key, sum)
	if err != nil {
		return err
	}
	current, err := q.MigratorCarBrandByUUID(ctx, res.UUID)
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read car brand: %w", err)
	}
	if !exists {
		id, err := q.MigratorInsertCarBrand(ctx, db.MigratorInsertCarBrandParams{
			Uuid: res.UUID, ExternalID: extID, Name: name, ShowName: b.ShowName, LogoHeight: logoHeight,
			Active: active, CreatedAt: pgTime(b.CreatedAt),
		})
		if err != nil {
			return fmt.Errorf("insert car brand: %w", err)
		}
		c.inc("brands_created")
		return s.copyLogo(ctx, dst, id, res.UUID, b, c)
	}
	if !res.Changed {
		c.inc("brands_unchanged")
		if !current.LogoObjectKey.Valid {
			return s.copyLogo(ctx, dst, current.ID, current.Uuid, b, c)
		}
		return nil
	}
	if err := q.MigratorUpdateCarBrand(ctx, db.MigratorUpdateCarBrandParams{
		ID: current.ID, Name: name, ShowName: b.ShowName, LogoHeight: logoHeight, Active: active,
	}); err != nil {
		return fmt.Errorf("update car brand: %w", err)
	}
	c.inc("brands_updated")
	return s.copyLogo(ctx, dst, current.ID, current.Uuid, b, c)
}

// copyLogo uploads the legacy logo file of b and points the brand at it.
// The object key carries a hash of the bytes, so a rerun writes the same key
// and an unchanged logo is not uploaded again. Missing or unsupported files
// are reported, not fatal.
func (s VehicleCatalogStep) copyLogo(ctx context.Context, dst *Target, brandID int64, brandUUID uuid.UUID, b legacyCarBrand, c counts) error {
	rel := strings.TrimSpace(b.Logo)
	if rel == "" {
		return nil
	}
	id := strconv.FormatInt(b.ID, 10)
	if dst.LegacyFiles == nil || dst.Storage == nil {
		c.inc("logo_skipped_no_storage")
		return nil
	}
	body, err := readLegacyFile(dst.LegacyFiles, rel, storage.MaxLogoBytes)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		c.inc("logo_missing")
		c.inc("logo_missing:brand:" + id)
		return nil
	case errors.Is(err, errFileTooLarge):
		c.inc("logo_too_large:brand:" + id)
		return nil
	case err != nil:
		return fmt.Errorf("read logo %q: %w", rel, err)
	}
	mime, err := storage.DetectLogoMIME("", body)
	if err != nil {
		c.inc("logo_unsupported:brand:" + id)
		return nil
	}
	ext, err := storage.LogoExtForMIME(mime)
	if err != nil {
		c.inc("logo_unsupported:brand:" + id)
		return nil
	}
	sum := sha256.Sum256(body)
	objectKey := storage.VehicleBrandLogoObjectKey(brandUUID, hex.EncodeToString(sum[:8]), ext)

	exists, err := dst.Storage.Exists(ctx, objectKey)
	if err != nil {
		return fmt.Errorf("logo exists: %w", err)
	}
	if !exists && !dst.DryRun {
		if err := dst.Storage.Upload(ctx, storage.File{
			Body: bytes.NewReader(body), Size: int64(len(body)), ContentType: mime, Filename: path.Base(rel),
		}, objectKey); err != nil {
			return fmt.Errorf("upload logo: %w", err)
		}
		c.inc("logos_uploaded")
	}
	if err := dst.Q.MigratorSetCarBrandLogo(ctx, db.MigratorSetCarBrandLogoParams{ID: brandID, LogoObjectKey: objectKey}); err != nil {
		return fmt.Errorf("set logo: %w", err)
	}
	c.inc("logos_set")
	return nil
}

var errFileTooLarge = errors.New("migrator: legacy file is too large")

// readLegacyFile reads rel from the legacy storage, trying the Laravel disk
// roots in order. It returns fs.ErrNotExist when no root has the file.
func readLegacyFile(fsys fs.FS, rel string, maxBytes int64) ([]byte, error) {
	rel = strings.TrimPrefix(path.Clean("/"+strings.ReplaceAll(rel, "\\", "/")), "/")
	if rel == "" || rel == "." || !fs.ValidPath(rel) {
		return nil, fs.ErrNotExist
	}
	for _, root := range legacyLogoRoots {
		p := rel
		if root != "" {
			p = root + "/" + rel
		}
		f, err := fsys.Open(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		if int64(len(body)) > maxBytes {
			return nil, errFileTooLarge
		}
		return body, nil
	}
	return nil, fs.ErrNotExist
}

func (s VehicleCatalogStep) importModel(ctx context.Context, q *db.Queries, m *Mapper, brandIDs map[int64]int64, md legacyCarModel, c counts) error {
	id := strconv.FormatInt(md.ID, 10)
	name := truncate(strings.TrimSpace(md.Name), carModelNameMax)
	if name == "" {
		c.inc("model_skipped_no_name:" + id)
		return nil
	}
	brandID, ok := brandIDs[md.BrandID]
	if !ok {
		target, found, err := m.Lookup(ctx, s.system(), "car_brands", strconv.FormatInt(md.BrandID, 10))
		if err != nil {
			return err
		}
		if !found {
			c.inc("model_skipped_brand_unmapped:" + id)
			return nil
		}
		row, err := q.MigratorCarBrandByUUID(ctx, target)
		if err != nil {
			return fmt.Errorf("read car brand: %w", err)
		}
		brandID = row.ID
		brandIDs[md.BrandID] = brandID
	}
	extID := legacyExternalID(md.ExternalID, c, "model", md.ID)
	yearStart, yearStop := legacyYear(md.YearStart), legacyYear(md.YearStop)
	if yearStart.Valid && yearStop.Valid && yearStop.Int16 < yearStart.Int16 {
		yearStop = pgtype.Int2{}
	}
	body := pgText(truncate(strings.TrimSpace(md.Body), carAttrMax))
	powertrain := pgText(truncate(strings.TrimSpace(md.Powertrain), carAttrMax))
	active := md.Active && !md.DeletedAt.Valid
	if md.DeletedAt.Valid {
		c.inc("model_deleted_inactive")
	}

	key := Key{System: s.system(), Table: "car_models", ID: id, TargetTable: "car_models"}
	sum := Checksum(md.BrandID, md.Name, md.ExternalID, md.Powertrain, md.YearStart.Int64, md.YearStart.Valid,
		md.YearStop.Int64, md.YearStop.Valid, md.Body, md.Active, md.DeletedAt.Valid)

	_, mapped, err := m.Lookup(ctx, key.System, key.Table, key.ID)
	if err != nil {
		return err
	}
	if !mapped {
		existing, err := q.MigratorFindCarModel(ctx, db.MigratorFindCarModelParams{ExternalID: extID, CarBrandID: brandID, Name: name})
		switch {
		case err == nil:
			linked, err := m.Link(ctx, key, existing, sum)
			if err != nil {
				return err
			}
			if linked {
				c.inc("models_linked_existing")
				return nil
			}
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("find car model: %w", err)
		}
	}

	res, err := m.Upsert(ctx, key, sum)
	if err != nil {
		return err
	}
	modelID, err := q.MigratorCarModelIDByUUID(ctx, res.UUID)
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read car model: %w", err)
	}
	if !exists {
		if _, err := q.MigratorInsertCarModel(ctx, db.MigratorInsertCarModelParams{
			Uuid: res.UUID, CarBrandID: brandID, ExternalID: extID, Name: name, BodyType: body,
			Powertrain: powertrain, YearStart: yearStart, YearStop: yearStop, Active: active,
			CreatedAt: pgTime(md.CreatedAt),
		}); err != nil {
			return fmt.Errorf("insert car model: %w", err)
		}
		c.inc("models_created")
		return nil
	}
	if !res.Changed {
		c.inc("models_unchanged")
		return nil
	}
	if err := q.MigratorUpdateCarModel(ctx, db.MigratorUpdateCarModelParams{
		ID: modelID, CarBrandID: brandID, Name: name, BodyType: body, Powertrain: powertrain,
		YearStart: yearStart, YearStop: yearStop, Active: active,
	}); err != nil {
		return fmt.Errorf("update car model: %w", err)
	}
	c.inc("models_updated")
	return nil
}

// legacyExternalID keeps a legacy external id that fits the column; a longer
// one is dropped (a truncated id could collide) and reported.
func legacyExternalID(raw string, c counts, kind string, id int64) pgtype.Text {
	v := strings.TrimSpace(raw)
	if v == "" {
		return pgtype.Text{}
	}
	if len([]rune(v)) > carExternalIDMax {
		c.inc("external_id_too_long:" + kind + ":" + strconv.FormatInt(id, 10))
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

// legacyYear keeps years the car_models checks accept (1900-2100).
func legacyYear(v sql.NullInt64) pgtype.Int2 {
	if !v.Valid || v.Int64 < 1900 || v.Int64 > 2100 {
		return pgtype.Int2{}
	}
	return pgtype.Int2{Int16: int16(v.Int64), Valid: true}
}
