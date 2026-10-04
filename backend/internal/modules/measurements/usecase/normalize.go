package usecase

// TEC-294 (F3-02b): normalization of raw records into measurement_values,
// measurement_tires and the device registry, on upload and as a backfill.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TxBeginner starts a transaction (a pool) or a savepoint (a transaction).
type TxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// NormalizeStore is what normalizing one result writes.
type NormalizeStore interface {
	GetMeasurementDeviceBySerial(ctx context.Context, arg db.GetMeasurementDeviceBySerialParams) (db.MeasurementDevice, error)
	InsertMeasurementDeviceIfAbsent(ctx context.Context, arg db.InsertMeasurementDeviceIfAbsentParams) (db.MeasurementDevice, error)
	DeleteMeasurementValues(ctx context.Context, arg db.DeleteMeasurementValuesParams) error
	DeleteMeasurementTires(ctx context.Context, arg db.DeleteMeasurementTiresParams) error
	InsertMeasurementValue(ctx context.Context, arg db.InsertMeasurementValueParams) (db.MeasurementValue, error)
	InsertMeasurementTire(ctx context.Context, arg db.InsertMeasurementTireParams) (db.MeasurementTire, error)
	MarkMeasurementResultParsed(ctx context.Context, arg db.MarkMeasurementResultParsedParams) error
}

// WithNormalizer makes uploads normalize in their insert transaction. tx is
// the pool; log receives the records that cannot be parsed.
func (s *Service) WithNormalizer(tx TxBeginner, log *slog.Logger) *Service {
	s.tx = tx
	s.log = log
	return s
}

func (s *Service) logger() *slog.Logger {
	if s.log != nil {
		return s.log
	}
	return slog.Default()
}

// Create stores one upload (see create). With a normalizer the row is
// normalized in the same transaction; a raw record that cannot be parsed is
// still accepted (202) and keeps parsed_at NULL (K28: the flow never breaks).
func (s *Service) Create(ctx context.Context, c Caller, in Input) (Result, error) {
	if s.tx == nil {
		return s.create(ctx, c, in)
	}
	tx, err := s.tx.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(tx)
	res, err := (&Service{store: q}).create(ctx, c, in)
	if err != nil {
		return Result{}, err
	}
	if !res.Replayed {
		row, err := q.GetMeasurementResultByUUID(ctx, db.GetMeasurementResultByUUIDParams{
			Uuid: res.UUID, OrganizationID: c.OrganizationID,
		})
		if err != nil {
			return Result{}, err
		}
		if _, err := normalizeIsolated(ctx, tx, row, s.logger()); err != nil {
			return Result{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return res, nil
}

// NormalizeOutcome is what normalizing one result wrote.
type NormalizeOutcome struct {
	Parsed        bool
	Values, Tires int
	DeviceCreated bool
}

// normalizeIsolated normalizes row inside a savepoint of tx: a record that
// cannot be parsed, or whose rows the database refuses, is logged and rolled
// back to the savepoint, leaving parsed_at NULL. Only a failure to open or
// release the savepoint is returned.
func normalizeIsolated(ctx context.Context, tx TxBeginner, row db.MeasurementResult, log *slog.Logger) (NormalizeOutcome, error) {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return NormalizeOutcome{}, err
	}
	out, err := Normalize(ctx, db.New(sp), row)
	if err != nil {
		_ = sp.Rollback(ctx)
		log.Error("measurement_parse_failed", "measurement_uuid", row.Uuid.String(), "source", row.Source, "error", err)
		return NormalizeOutcome{}, nil
	}
	if err := sp.Commit(ctx); err != nil {
		return NormalizeOutcome{}, err
	}
	return out, nil
}

// Normalize parses row.raw and replaces the row's readings and tires,
// registers the device serial in the organization when it is new and marks
// the row parsed. It is idempotent: a second call rewrites the same rows.
func Normalize(ctx context.Context, q NormalizeStore, row db.MeasurementResult) (NormalizeOutcome, error) {
	var out NormalizeOutcome
	rep, err := ParseRaw(row.Raw)
	if err != nil {
		return out, err
	}
	org := row.OrganizationID

	var deviceID pgtype.Int8
	serial := rep.DeviceSerial
	if row.DeviceSerial.Valid && row.DeviceSerial.String != "" {
		serial = row.DeviceSerial.String
	}
	if serial != "" {
		dev, created, err := ensureDevice(ctx, q, row, serial, rep.DeviceModel)
		if err != nil {
			return out, err
		}
		deviceID = pgtype.Int8{Int64: dev.ID, Valid: true}
		out.DeviceCreated = created
	}

	if err := q.DeleteMeasurementValues(ctx, db.DeleteMeasurementValuesParams{ResultID: row.ID, OrganizationID: org}); err != nil {
		return out, err
	}
	if err := q.DeleteMeasurementTires(ctx, db.DeleteMeasurementTiresParams{ResultID: row.ID, OrganizationID: org}); err != nil {
		return out, err
	}
	for _, v := range rep.Values {
		arg := db.InsertMeasurementValueParams{
			OrganizationID: org, BrandID: row.BrandID, ResultID: row.ID,
			PlaceID: v.PlaceID, PartType: v.PartType, IsInside: v.IsInside,
			SubstrateType: textPtr(v.SubstrateType), MeasuredAt: timePtr(v.MeasuredAt),
		}
		if v.Position != nil {
			arg.Position = pgtype.Int4{Int32: *v.Position, Valid: true}
		}
		if v.Interpretation != nil {
			arg.Interpretation = pgtype.Int2{Int16: *v.Interpretation, Valid: true}
		}
		if arg.ValueUm, err = numericPtr(v.ValueUM); err != nil {
			return out, err
		}
		if _, err := q.InsertMeasurementValue(ctx, arg); err != nil {
			return out, fmt.Errorf("insert reading: %w", err)
		}
		out.Values++
	}
	for _, t := range rep.Tires {
		arg := db.InsertMeasurementTireParams{
			OrganizationID: org, BrandID: row.BrandID, ResultID: row.ID,
			Section: textPtr(t.Section), Width: textPtr(t.Width), Profile: textPtr(t.Profile),
			Diameter: textPtr(t.Diameter), Maker: textPtr(t.Maker), Season: textPtr(t.Season),
		}
		if arg.TreadDepth1Mm, err = numericPtr(t.TreadDepth1MM); err != nil {
			return out, err
		}
		if arg.TreadDepth2Mm, err = numericPtr(t.TreadDepth2MM); err != nil {
			return out, err
		}
		if _, err := q.InsertMeasurementTire(ctx, arg); err != nil {
			return out, fmt.Errorf("insert tire: %w", err)
		}
		out.Tires++
	}
	if err := q.MarkMeasurementResultParsed(ctx, db.MarkMeasurementResultParsedParams{
		MeasuredAt: pgtype.Timestamptz{Time: rep.MeasuredAt, Valid: true}, DeviceID: deviceID,
		BodyType: text(rep.BodyType), ID: row.ID, OrganizationID: org,
	}); err != nil {
		return out, err
	}
	out.Parsed = true
	return out, nil
}

// ensureDevice finds the organization's device of serial or registers it
// (model from raw); a concurrent registration of the same serial is reused.
func ensureDevice(ctx context.Context, q NormalizeStore, row db.MeasurementResult, serial, model string) (db.MeasurementDevice, bool, error) {
	key := db.GetMeasurementDeviceBySerialParams{OrganizationID: row.OrganizationID, Serial: serial}
	dev, err := q.GetMeasurementDeviceBySerial(ctx, key)
	if err == nil {
		return dev, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return dev, false, err
	}
	dev, err = q.InsertMeasurementDeviceIfAbsent(ctx, db.InsertMeasurementDeviceIfAbsentParams{
		OrganizationID: row.OrganizationID, BrandID: row.BrandID, Serial: serial, Model: text(model),
	})
	if err == nil {
		return dev, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return dev, false, err
	}
	dev, err = q.GetMeasurementDeviceBySerial(ctx, key)
	return dev, false, err
}

func numericPtr(s *string) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if s == nil {
		return n, nil
	}
	if err := n.Scan(*s); err != nil {
		return n, fmt.Errorf("numeric %q: %w", *s, err)
	}
	return n, nil
}

// ReparseReport is the outcome of one backfill run.
type ReparseReport struct {
	Scanned        int `json:"scanned"`
	Parsed         int `json:"parsed"`
	Unparseable    int `json:"unparseable"`
	Values         int `json:"values"`
	Tires          int `json:"tires"`
	DevicesCreated int `json:"devices_created"`
}

// Reparse normalizes every result whose parsed_at is NULL (mobile uploads
// from before TEC-294 and the migrator's legacy_import rows), each in its
// own transaction. Parsed rows are never touched again, so a second run
// writes nothing; records that still cannot be parsed stay NULL and are
// counted as unparseable.
func Reparse(ctx context.Context, pool TxBeginner, q *db.Queries, batch int32, log *slog.Logger) (ReparseReport, error) {
	var rep ReparseReport
	if batch <= 0 {
		batch = 200
	}
	if log == nil {
		log = slog.Default()
	}
	var after int64
	for {
		ids, err := q.ListUnparsedMeasurementResultIDs(ctx, db.ListUnparsedMeasurementResultIDsParams{AfterID: after, LimitCount: batch})
		if err != nil {
			return rep, err
		}
		if len(ids) == 0 {
			return rep, nil
		}
		for _, id := range ids {
			after = id
			out, done, err := reparseOne(ctx, pool, id, log)
			if err != nil {
				return rep, fmt.Errorf("measurement %d: %w", id, err)
			}
			if !done {
				continue
			}
			rep.Scanned++
			if !out.Parsed {
				rep.Unparseable++
				continue
			}
			rep.Parsed++
			rep.Values += out.Values
			rep.Tires += out.Tires
			if out.DeviceCreated {
				rep.DevicesCreated++
			}
		}
	}
}

// reparseOne locks one unparsed result and normalizes it; done is false
// when another run parsed it meanwhile.
func reparseOne(ctx context.Context, pool TxBeginner, id int64, log *slog.Logger) (NormalizeOutcome, bool, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return NormalizeOutcome{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row, err := db.New(tx).LockUnparsedMeasurementResult(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return NormalizeOutcome{}, false, nil
	}
	if err != nil {
		return NormalizeOutcome{}, false, err
	}
	out, err := normalizeIsolated(ctx, tx, row, log)
	if err != nil {
		return out, false, err
	}
	return out, true, tx.Commit(ctx)
}
