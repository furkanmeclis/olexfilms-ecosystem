package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	LevelDebug = "debug"
	LevelWarn  = "warn"
	LevelError = "error"
)

var PersistLevels = []string{LevelDebug, LevelWarn, LevelError}

var AllowedIntervals = []int{5, 15, 30, 60, 180, 360, 720, 1440, 10080}

// Log is the API projection of an app_logs row.
type Log struct {
	UUID      uuid.UUID      `json:"uuid"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Source    string         `json:"source"`
	Attrs     map[string]any `json:"attrs"`
	RequestID string         `json:"request_id,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// Stats is a per-level count summary.
type Stats struct {
	Debug int64 `json:"debug"`
	Warn  int64 `json:"warn"`
	Error int64 `json:"error"`
	Total int64 `json:"total"`
}

// PurgeRule is a scheduled deletion rule.
type PurgeRule struct {
	UUID             uuid.UUID  `json:"uuid"`
	Name             string     `json:"name"`
	Enabled          bool       `json:"enabled"`
	IsSystem         bool       `json:"is_system"`
	Levels           []string   `json:"levels"`
	Source           string     `json:"source,omitempty"`
	MessageContains  string     `json:"message_contains,omitempty"`
	OlderThanHours   int32      `json:"older_than_hours"`
	IntervalMinutes  int32      `json:"interval_minutes"`
	LastRunAt        *time.Time `json:"last_run_at,omitempty"`
	LastDeletedCount int64      `json:"last_deleted_count"`
	LastError        string     `json:"last_error,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// ListFilter selects stored logs.
type ListFilter struct {
	Levels      []string
	Source      string
	Q           string
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	OlderHours  *int32
}

// PurgeInput deletes matching logs (or an explicit uuid set).
type PurgeInput struct {
	UUIDs          []uuid.UUID `json:"uuids"`
	DryRun         bool        `json:"dry_run"`
	Levels         []string    `json:"levels"`
	Source         string      `json:"source"`
	Q              string      `json:"q"`
	OlderThanHours *int32      `json:"older_than_hours"`
}

// PurgeResult reports how many rows matched / were deleted.
type PurgeResult struct {
	Deleted int64 `json:"deleted"`
	DryRun  bool  `json:"dry_run"`
}

// RuleInput creates or updates a purge rule.
type RuleInput struct {
	Name            string   `json:"name"`
	Enabled         *bool    `json:"enabled"`
	Levels          []string `json:"levels"`
	Source          *string  `json:"source"`
	MessageContains *string  `json:"message_contains"`
	OlderThanHours  *int32   `json:"older_than_hours"`
	IntervalMinutes *int32   `json:"interval_minutes"`
}
