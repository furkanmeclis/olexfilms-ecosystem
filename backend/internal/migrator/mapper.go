package migrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

// Key names one legacy row and the table it lands in.
type Key struct {
	System      string // migration_map.source_system ("hub", "wh")
	Table       string // legacy table
	ID          string // legacy primary key, as text
	TargetTable string // table in this database
}

// MapResult is the outcome of Mapper.Upsert.
type MapResult struct {
	// UUID is the target row's uuid: new on first sight, stable afterwards.
	UUID uuid.UUID
	// Created is true when the key was not mapped before.
	Created bool
	// Changed is true when the key was mapped with a different checksum: the
	// legacy row changed since the last run and the target should be updated.
	Changed bool
}

// Mapper reads and writes migration_map inside the step transaction, so a
// failed step leaves no half-written mapping behind.
type Mapper struct {
	q db.Querier
}

// NewMapper binds a mapper to q (normally db.New(stepTx)).
func NewMapper(q db.Querier) *Mapper { return &Mapper{q: q} }

// Upsert maps key to a target uuid. The first call creates the mapping
// (Created); later calls return the same uuid with Created false, and
// Changed when checksum differs from the stored one (the stored checksum is
// then replaced).
func (m *Mapper) Upsert(ctx context.Context, key Key, checksum string) (MapResult, error) {
	if key.System == "" || key.Table == "" || key.ID == "" || key.TargetTable == "" {
		return MapResult{}, errors.New("migrator: mapper key is incomplete")
	}
	row, err := m.get(ctx, key)
	switch {
	case err == nil:
		if row.TargetTable != key.TargetTable {
			return MapResult{}, fmt.Errorf("migrator: %s.%s#%s is mapped to %s, not %s",
				key.System, key.Table, key.ID, row.TargetTable, key.TargetTable)
		}
		if row.Checksum == checksum {
			return MapResult{UUID: row.TargetUuid}, nil
		}
		if err := m.q.UpdateMigrationMapChecksum(ctx, db.UpdateMigrationMapChecksumParams{ID: row.ID, Checksum: checksum}); err != nil {
			return MapResult{}, fmt.Errorf("migrator: update map checksum: %w", err)
		}
		return MapResult{UUID: row.TargetUuid, Changed: true}, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return MapResult{}, fmt.Errorf("migrator: read map: %w", err)
	}

	row, err = m.q.InsertMigrationMap(ctx, db.InsertMigrationMapParams{
		SourceSystem: key.System,
		SourceTable:  key.Table,
		SourceID:     key.ID,
		TargetTable:  key.TargetTable,
		TargetUuid:   uuid.New(),
		Checksum:     checksum,
	})
	if err == nil {
		return MapResult{UUID: row.TargetUuid, Created: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return MapResult{}, fmt.Errorf("migrator: insert map: %w", err)
	}
	// Lost a race with a concurrent insert of the same key: use the winner.
	row, err = m.get(ctx, key)
	if err != nil {
		return MapResult{}, fmt.Errorf("migrator: read map after conflict: %w", err)
	}
	return MapResult{UUID: row.TargetUuid, Changed: row.Checksum != checksum}, nil
}

// Link maps a not yet mapped key to an existing target row (a legacy record
// that matches an account already in this database). It reports false when
// the key was mapped meanwhile; the caller then uses Lookup.
func (m *Mapper) Link(ctx context.Context, key Key, target uuid.UUID, checksum string) (bool, error) {
	if key.System == "" || key.Table == "" || key.ID == "" || key.TargetTable == "" || target == uuid.Nil {
		return false, errors.New("migrator: mapper link is incomplete")
	}
	_, err := m.q.InsertMigrationMap(ctx, db.InsertMigrationMapParams{
		SourceSystem: key.System,
		SourceTable:  key.Table,
		SourceID:     key.ID,
		TargetTable:  key.TargetTable,
		TargetUuid:   target,
		Checksum:     checksum,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("migrator: link map: %w", err)
	}
	return true, nil
}

// Lookup returns the target uuid of an already mapped key (for foreign keys
// to rows an earlier step migrated).
func (m *Mapper) Lookup(ctx context.Context, system, table, id string) (uuid.UUID, bool, error) {
	row, err := m.get(ctx, Key{System: system, Table: table, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("migrator: read map: %w", err)
	}
	return row.TargetUuid, true, nil
}

func (m *Mapper) get(ctx context.Context, key Key) (db.MigrationMap, error) {
	return m.q.GetMigrationMap(ctx, db.GetMigrationMapParams{
		SourceSystem: key.System, SourceTable: key.Table, SourceID: key.ID,
	})
}

// Checksum hashes the source values of a legacy row (sha256, hex). Values are
// formatted with %v and separated by a unit separator, so ("a","bc") and
// ("ab","c") differ.
func Checksum(values ...any) string {
	parts := make([]string, len(values))
	for i, v := range values {
		if v == nil {
			parts[i] = "\x00"
			continue
		}
		parts[i] = fmt.Sprintf("%v", v)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}
