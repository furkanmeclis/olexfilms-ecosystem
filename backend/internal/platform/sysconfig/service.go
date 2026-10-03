package sysconfig

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrUnknownKey is returned for a key outside the catalog.
var ErrUnknownKey = errors.New("sysconfig: unknown key")

// Service reads settings through the cache and writes them to the
// database, dropping the cache on every write.
type Service struct {
	q     *db.Queries
	cache Cache
}

// New creates a Service; cache may be NoCache{}.
func New(q *db.Queries, cache Cache) *Service {
	if cache == nil {
		cache = NoCache{}
	}
	return &Service{q: q, cache: cache}
}

// Entry is one setting with its effective value.
type Entry struct {
	Definition
	// Value is the effective value: the stored override or the default.
	Value json.RawMessage `json:"value"`
	// IsDefault is true when no row overrides the default.
	IsDefault     bool       `json:"is_default"`
	SchemaVersion int32      `json:"schema_version"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
}

func (s *Service) snapshot(ctx context.Context) (Snapshot, error) {
	if snap, ok := s.cache.Get(ctx); ok {
		return snap, nil
	}
	rows, err := s.q.ListSystemSettings(ctx)
	if err != nil {
		return nil, err
	}
	snap := make(Snapshot, len(rows))
	for _, r := range rows {
		snap[r.Key] = json.RawMessage(r.Value)
	}
	s.cache.Set(ctx, snap)
	return snap, nil
}

// Raw returns the effective JSON value of key. Stored values that no
// longer pass the catalog schema fall back to the default, so a stale row
// can never leak an invalid value into a caller.
func (s *Service) Raw(ctx context.Context, key string) (json.RawMessage, error) {
	d, ok := Lookup(key)
	if !ok {
		return nil, ErrUnknownKey
	}
	snap, err := s.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	return effective(d, snap), nil
}

func effective(d Definition, snap Snapshot) json.RawMessage {
	if raw, ok := snap[d.Key]; ok {
		if v, err := d.Validate(raw); err == nil {
			return v
		}
	}
	return mustJSON(d.Default)
}

// Int returns an integer setting. A database error yields the default, so
// callers on the hot path never fail on configuration.
func (s *Service) Int(ctx context.Context, key string) int64 {
	var n int64
	s.decode(ctx, key, &n)
	return n
}

// Bool returns a boolean setting (default on error).
func (s *Service) Bool(ctx context.Context, key string) bool {
	var b bool
	s.decode(ctx, key, &b)
	return b
}

// String returns a string setting (default on error).
func (s *Service) String(ctx context.Context, key string) string {
	var str string
	s.decode(ctx, key, &str)
	return str
}

func (s *Service) decode(ctx context.Context, key string, dst any) {
	raw, err := s.Raw(ctx, key)
	if err != nil {
		if d, ok := Lookup(key); ok {
			raw = mustJSON(d.Default)
		} else {
			return
		}
	}
	_ = json.Unmarshal(raw, dst)
}

// ContractGraceDays is the typed accessor for KeyContractGraceDays.
func (s *Service) ContractGraceDays(ctx context.Context) int {
	return int(s.Int(ctx, KeyContractGraceDays))
}

// ForecastMinDays is the typed accessor for KeyForecastMinDays.
func (s *Service) ForecastMinDays(ctx context.Context) int {
	return int(s.Int(ctx, KeyForecastMinDays))
}

// BulkUndoWindowHours is the typed accessor for KeyBulkUndoWindowHours.
func (s *Service) BulkUndoWindowHours(ctx context.Context) int {
	return int(s.Int(ctx, KeyBulkUndoWindowHours))
}

// PhotoStandardEnabled is the typed accessor for KeyPhotoStandardEnabled.
func (s *Service) PhotoStandardEnabled(ctx context.Context) bool {
	return s.Bool(ctx, KeyPhotoStandardEnabled)
}

// SMTP groups the smtp.* keys. Empty Host / zero Port mean "use the
// environment configuration".
type SMTP struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	FromName string
}

// SMTP returns the stored SMTP overrides.
func (s *Service) SMTP(ctx context.Context) SMTP {
	return SMTP{
		Host:     s.String(ctx, KeySMTPHost),
		Port:     int(s.Int(ctx, KeySMTPPort)),
		Username: s.String(ctx, KeySMTPUsername),
		Password: s.String(ctx, KeySMTPPassword),
		From:     s.String(ctx, KeySMTPFrom),
		FromName: s.String(ctx, KeySMTPFromName),
	}
}

// MobileApp is the mobile app version gate (TEC-236). An empty MinVersion
// means no gate.
type MobileApp struct {
	MinVersion      string
	StoreURLIOS     string
	StoreURLAndroid string
	VersionRequired bool
}

// WithFallback fills the empty string fields from env (the environment
// defaults), so a stored setting wins and an unset one keeps the env value.
func (m MobileApp) WithFallback(env MobileApp) MobileApp {
	if m.MinVersion == "" {
		m.MinVersion = env.MinVersion
	}
	if m.StoreURLIOS == "" {
		m.StoreURLIOS = env.StoreURLIOS
	}
	if m.StoreURLAndroid == "" {
		m.StoreURLAndroid = env.StoreURLAndroid
	}
	m.VersionRequired = m.VersionRequired || env.VersionRequired
	return m
}

// MobileApp returns the stored mobile.* overrides (empty = not set).
func (s *Service) MobileApp(ctx context.Context) MobileApp {
	return MobileApp{
		MinVersion:      s.String(ctx, KeyMobileAppMinVersion),
		StoreURLIOS:     s.String(ctx, KeyMobileAppStoreURLIOS),
		StoreURLAndroid: s.String(ctx, KeyMobileAppStoreURLAndroid),
		VersionRequired: s.Bool(ctx, KeyMobileAppVersionRequired),
	}
}

// List returns every catalog entry with its effective value; secrets are
// masked.
func (s *Service) List(ctx context.Context) ([]Entry, error) {
	rows, err := s.q.ListSystemSettings(ctx)
	if err != nil {
		return nil, err
	}
	stored := make(map[string]db.SystemSetting, len(rows))
	snap := make(Snapshot, len(rows))
	for _, r := range rows {
		stored[r.Key] = r
		snap[r.Key] = json.RawMessage(r.Value)
	}
	defs := Catalog()
	out := make([]Entry, 0, len(defs))
	for _, d := range defs {
		out = append(out, buildEntry(d, snap, stored))
	}
	return out, nil
}

// Get returns one entry (ErrUnknownKey if the key is not in the catalog).
func (s *Service) Get(ctx context.Context, key string) (Entry, error) {
	d, ok := Lookup(key)
	if !ok {
		return Entry{}, ErrUnknownKey
	}
	row, err := s.q.GetSystemSetting(ctx, key)
	snap, stored := Snapshot{}, map[string]db.SystemSetting{}
	switch {
	case err == nil:
		snap[key] = json.RawMessage(row.Value)
		stored[key] = row
	case errors.Is(err, pgx.ErrNoRows):
	default:
		return Entry{}, err
	}
	return buildEntry(d, snap, stored), nil
}

func buildEntry(d Definition, snap Snapshot, stored map[string]db.SystemSetting) Entry {
	e := Entry{Definition: d, Value: effective(d, snap), IsDefault: true, SchemaVersion: SchemaVersion}
	if row, ok := stored[d.Key]; ok {
		if _, err := d.Validate(json.RawMessage(row.Value)); err == nil {
			e.IsDefault = false
		}
		e.SchemaVersion = row.SchemaVersion
		if row.UpdatedAt.Valid {
			t := row.UpdatedAt.Time
			e.UpdatedAt = &t
		}
	}
	if d.Secret {
		var str string
		if json.Unmarshal(e.Value, &str) == nil && str != "" {
			e.Value = mustJSON(SecretMask)
		}
	}
	return e
}

// Set validates raw against the catalog, stores it and drops the cache.
// Writing SecretMask to a secret key keeps the stored value. userID may be
// 0 (system).
func (s *Service) Set(ctx context.Context, key string, raw json.RawMessage, userID int64) (Entry, error) {
	d, ok := Lookup(key)
	if !ok {
		return Entry{}, ErrUnknownKey
	}
	canonical, err := d.Validate(raw)
	if err != nil {
		return Entry{}, err
	}
	if d.Secret {
		var str string
		if json.Unmarshal(canonical, &str) == nil && str == SecretMask {
			return s.Get(ctx, key)
		}
	}
	var by pgtype.Int8
	if userID > 0 {
		by = pgtype.Int8{Int64: userID, Valid: true}
	}
	if _, err := s.q.UpsertSystemSetting(ctx, db.UpsertSystemSettingParams{
		Key: key, Value: []byte(canonical), SchemaVersion: SchemaVersion, UpdatedBy: by,
	}); err != nil {
		return Entry{}, err
	}
	s.cache.Invalidate(ctx)
	return s.Get(ctx, key)
}

// Reset deletes the override so the key resolves to its default.
func (s *Service) Reset(ctx context.Context, key string) (Entry, error) {
	if _, ok := Lookup(key); !ok {
		return Entry{}, ErrUnknownKey
	}
	if _, err := s.q.DeleteSystemSetting(ctx, key); err != nil {
		return Entry{}, err
	}
	s.cache.Invalidate(ctx)
	return s.Get(ctx, key)
}
