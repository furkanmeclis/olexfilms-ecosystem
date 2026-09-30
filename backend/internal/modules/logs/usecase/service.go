package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/logging"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/logs/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrSystemRule = errors.New("system purge rule cannot be deleted")
)

// Querier is the persistence surface used by the logs service.
type Querier interface {
	InsertAppLog(ctx context.Context, arg db.InsertAppLogParams) error
	GetAppLogByUUID(ctx context.Context, arg uuid.UUID) (db.AppLog, error)
	ListAppLogs(ctx context.Context, arg db.ListAppLogsParams) ([]db.AppLog, error)
	CountAppLogs(ctx context.Context, arg db.CountAppLogsParams) (int64, error)
	CountAppLogsByLevel(ctx context.Context) ([]db.CountAppLogsByLevelRow, error)
	ListAppLogSources(ctx context.Context) ([]string, error)
	DeleteAppLogByUUID(ctx context.Context, arg uuid.UUID) (int64, error)
	DeleteAppLogsByUUIDs(ctx context.Context, uuids []uuid.UUID) (int64, error)
	DeleteAppLogsMatching(ctx context.Context, arg db.DeleteAppLogsMatchingParams) (int64, error)
	ListLogPurgeRules(ctx context.Context) ([]db.LogPurgeRule, error)
	GetLogPurgeRuleByUUID(ctx context.Context, arg uuid.UUID) (db.LogPurgeRule, error)
	ListEnabledLogPurgeRules(ctx context.Context) ([]db.LogPurgeRule, error)
	CreateLogPurgeRule(ctx context.Context, arg db.CreateLogPurgeRuleParams) (db.LogPurgeRule, error)
	UpdateLogPurgeRule(ctx context.Context, arg db.UpdateLogPurgeRuleParams) (db.LogPurgeRule, error)
	DeleteLogPurgeRule(ctx context.Context, arg uuid.UUID) (int64, error)
	MarkLogPurgeRuleRun(ctx context.Context, arg db.MarkLogPurgeRuleRunParams) (db.LogPurgeRule, error)
}

// Service lists, deletes, and purges application logs.
type Service struct {
	q Querier
}

// New creates a logs service.
func New(q Querier) *Service {
	return &Service{q: q}
}

var _ logging.Writer = (*Service)(nil)

// WriteLog implements logging.Writer.
func (s *Service) WriteLog(ctx context.Context, entry logging.Entry) error {
	if s == nil || s.q == nil {
		return nil
	}
	if entry.Level == "" || entry.Level == "info" {
		return nil
	}
	attrs, err := json.Marshal(entry.Attrs)
	if err != nil || len(attrs) == 0 {
		attrs = []byte("{}")
	}
	created := entry.Time
	if created.IsZero() {
		created = time.Now().UTC()
	}
	return s.q.InsertAppLog(ctx, db.InsertAppLogParams{
		Level:     entry.Level,
		Message:   entry.Message,
		Source:    entry.Source,
		Attrs:     attrs,
		RequestID: textNarg(emptyToNil(entry.RequestID)),
		CreatedAt: timestamptz(created),
	})
}

// List returns paginated logs.
func (s *Service) List(ctx context.Context, q apiquery.Query, filter model.ListFilter) ([]model.Log, int64, error) {
	if err := apiquery.ValidateSort(q.Sort, apiquery.LogsSort); err != nil {
		return nil, 0, err
	}
	if err := validateLevels(filter.Levels); err != nil {
		return nil, 0, err
	}
	params := listParams(q.Limit, q.Offset, filter)
	rows, err := s.q.ListAppLogs(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountAppLogs(ctx, db.CountAppLogsParams{
		Levels:      params.Levels,
		Source:      params.Source,
		Q:           params.Q,
		CreatedFrom: params.CreatedFrom,
		CreatedTo:   params.CreatedTo,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Log, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapLog(row))
	}
	return out, total, nil
}

// Get returns a single log.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (model.Log, error) {
	row, err := s.q.GetAppLogByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Log{}, ErrNotFound
		}
		return model.Log{}, err
	}
	return mapLog(row), nil
}

// Stats returns counts by level.
func (s *Service) Stats(ctx context.Context) (model.Stats, error) {
	rows, err := s.q.CountAppLogsByLevel(ctx)
	if err != nil {
		return model.Stats{}, err
	}
	var st model.Stats
	for _, row := range rows {
		switch row.Level {
		case model.LevelDebug:
			st.Debug = row.Count
		case model.LevelWarn:
			st.Warn = row.Count
		case model.LevelError:
			st.Error = row.Count
		}
		st.Total += row.Count
	}
	return st, nil
}

// Sources returns distinct log sources.
func (s *Service) Sources(ctx context.Context) ([]string, error) {
	items, err := s.q.ListAppLogSources(ctx)
	if err != nil {
		return nil, err
	}
	if items == nil {
		return []string{}, nil
	}
	return items, nil
}

// Delete removes one log row.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	n, err := s.q.DeleteAppLogByUUID(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Purge deletes matching logs.
func (s *Service) Purge(ctx context.Context, in model.PurgeInput) (model.PurgeResult, error) {
	if err := validateLevels(in.Levels); err != nil {
		return model.PurgeResult{}, err
	}
	if len(in.UUIDs) > 0 {
		if in.DryRun {
			return model.PurgeResult{Deleted: int64(len(in.UUIDs)), DryRun: true}, nil
		}
		n, err := s.q.DeleteAppLogsByUUIDs(ctx, in.UUIDs)
		if err != nil {
			return model.PurgeResult{}, err
		}
		return model.PurgeResult{Deleted: n}, nil
	}
	filter := model.ListFilter{
		Levels:     in.Levels,
		Source:     strings.TrimSpace(in.Source),
		Q:          strings.TrimSpace(in.Q),
		OlderHours: in.OlderThanHours,
	}
	if in.DryRun {
		params := listParams(1, 0, filter)
		total, err := s.q.CountAppLogs(ctx, db.CountAppLogsParams{
			Levels:      params.Levels,
			Source:      params.Source,
			Q:           params.Q,
			CreatedFrom: params.CreatedFrom,
			CreatedTo:   params.CreatedTo,
		})
		if err != nil {
			return model.PurgeResult{}, err
		}
		return model.PurgeResult{Deleted: total, DryRun: true}, nil
	}
	n, err := s.q.DeleteAppLogsMatching(ctx, matchingParams(filter))
	if err != nil {
		return model.PurgeResult{}, err
	}
	return model.PurgeResult{Deleted: n}, nil
}

// ListRules returns all purge rules.
func (s *Service) ListRules(ctx context.Context) ([]model.PurgeRule, error) {
	rows, err := s.q.ListLogPurgeRules(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.PurgeRule, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapRule(row))
	}
	return out, nil
}

// GetRule returns one purge rule.
func (s *Service) GetRule(ctx context.Context, id uuid.UUID) (model.PurgeRule, error) {
	row, err := s.q.GetLogPurgeRuleByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.PurgeRule{}, ErrNotFound
		}
		return model.PurgeRule{}, err
	}
	return mapRule(row), nil
}

// CreateRule inserts a user-defined purge rule.
func (s *Service) CreateRule(ctx context.Context, actorID int64, in model.RuleInput) (model.PurgeRule, error) {
	rule, err := validateNewRule(in)
	if err != nil {
		return model.PurgeRule{}, err
	}
	row, err := s.q.CreateLogPurgeRule(ctx, db.CreateLogPurgeRuleParams{
		Name:            rule.Name,
		Enabled:         rule.Enabled,
		Levels:          rule.Levels,
		Source:          textNarg(emptyToNil(rule.Source)),
		MessageContains: textNarg(emptyToNil(rule.MessageContains)),
		OlderThanHours:  rule.OlderThanHours,
		IntervalMinutes: rule.IntervalMinutes,
		CreatedBy:       pgtype.Int8{Int64: actorID, Valid: actorID > 0},
	})
	if err != nil {
		return model.PurgeRule{}, err
	}
	return mapRule(row), nil
}

// PatchRule updates a purge rule.
func (s *Service) PatchRule(ctx context.Context, id uuid.UUID, in model.RuleInput) (model.PurgeRule, error) {
	current, err := s.q.GetLogPurgeRuleByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.PurgeRule{}, ErrNotFound
		}
		return model.PurgeRule{}, err
	}
	merged := mergeRule(mapRule(current), in)
	if err := validateRuleValues(merged); err != nil {
		return model.PurgeRule{}, err
	}
	row, err := s.q.UpdateLogPurgeRule(ctx, db.UpdateLogPurgeRuleParams{
		Uuid:            id,
		Name:            merged.Name,
		Enabled:         merged.Enabled,
		Levels:          merged.Levels,
		Source:          textNarg(emptyToNil(merged.Source)),
		MessageContains: textNarg(emptyToNil(merged.MessageContains)),
		OlderThanHours:  merged.OlderThanHours,
		IntervalMinutes: merged.IntervalMinutes,
	})
	if err != nil {
		return model.PurgeRule{}, err
	}
	return mapRule(row), nil
}

// DeleteRule removes a non-system purge rule.
func (s *Service) DeleteRule(ctx context.Context, id uuid.UUID) error {
	current, err := s.q.GetLogPurgeRuleByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if current.IsSystem {
		return ErrSystemRule
	}
	n, err := s.q.DeleteLogPurgeRule(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// RunRule applies a single rule immediately.
func (s *Service) RunRule(ctx context.Context, id uuid.UUID) (model.PurgeRule, int64, error) {
	row, err := s.q.GetLogPurgeRuleByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.PurgeRule{}, 0, ErrNotFound
		}
		return model.PurgeRule{}, 0, err
	}
	n, runErr := s.applyRule(ctx, row)
	marked, err := s.markRun(ctx, row.Uuid, n, runErr)
	if err != nil {
		return model.PurgeRule{}, n, err
	}
	if runErr != nil {
		return mapRule(marked), n, runErr
	}
	return mapRule(marked), n, nil
}

// ApplyDueRules runs enabled rules whose interval has elapsed.
func (s *Service) ApplyDueRules(ctx context.Context) error {
	rows, err := s.q.ListEnabledLogPurgeRules(ctx)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	var joined error
	for _, row := range rows {
		if !RuleIsDue(timestamptzPtr(row.LastRunAt), int(row.IntervalMinutes), now) {
			continue
		}
		n, runErr := s.applyRule(ctx, row)
		if _, err := s.markRun(ctx, row.Uuid, n, runErr); err != nil {
			joined = errors.Join(joined, err)
		}
		if runErr != nil {
			joined = errors.Join(joined, runErr)
		}
	}
	return joined
}

func (s *Service) applyRule(ctx context.Context, row db.LogPurgeRule) (int64, error) {
	filter := model.ListFilter{
		Levels:     row.Levels,
		Source:     textValue(row.Source),
		Q:          textValue(row.MessageContains),
		OlderHours: &row.OlderThanHours,
	}
	return s.q.DeleteAppLogsMatching(ctx, matchingParams(filter))
}

func (s *Service) markRun(ctx context.Context, id uuid.UUID, deleted int64, runErr error) (db.LogPurgeRule, error) {
	var lastErr pgtype.Text
	if runErr != nil {
		lastErr = pgtype.Text{String: truncate(runErr.Error(), 500), Valid: true}
	}
	return s.q.MarkLogPurgeRuleRun(ctx, db.MarkLogPurgeRuleRunParams{
		DeletedCount: deleted,
		LastError:    lastErr,
		Uuid:         id,
	})
}

// RuleIsDue reports whether a rule should run at now.
func RuleIsDue(lastRun *time.Time, intervalMinutes int, now time.Time) bool {
	if intervalMinutes <= 0 {
		return false
	}
	if lastRun == nil {
		return true
	}
	next := lastRun.Add(time.Duration(intervalMinutes) * time.Minute)
	return !next.After(now)
}

func listParams(limit, offset int32, filter model.ListFilter) db.ListAppLogsParams {
	params := db.ListAppLogsParams{
		Levels:      filter.Levels,
		Source:      textNarg(emptyToNil(strings.TrimSpace(filter.Source))),
		Q:           textNarg(emptyToNil(strings.TrimSpace(filter.Q))),
		CreatedFrom: timeNarg(filter.CreatedFrom),
		CreatedTo:   timeNarg(filter.CreatedTo),
		LimitCount:  limit,
		OffsetCount: offset,
	}
	if filter.OlderHours != nil && *filter.OlderHours > 0 {
		from := time.Now().UTC().Add(-time.Duration(*filter.OlderHours) * time.Hour)
		// "older than" means created_at < now - hours, so created_to exclusive.
		if !params.CreatedTo.Valid || from.Before(params.CreatedTo.Time) {
			params.CreatedTo = timestamptz(from)
		}
	}
	return params
}

func matchingParams(filter model.ListFilter) db.DeleteAppLogsMatchingParams {
	lp := listParams(0, 0, filter)
	return db.DeleteAppLogsMatchingParams{
		OlderThanHours: intNarg(filter.OlderHours),
		Levels:         lp.Levels,
		Source:         lp.Source,
		Q:              lp.Q,
		CreatedFrom:    lp.CreatedFrom,
		CreatedTo:      lp.CreatedTo,
	}
}

func validateLevels(levels []string) error {
	if len(levels) == 0 {
		return nil
	}
	allowed := map[string]struct{}{
		model.LevelDebug: {},
		model.LevelWarn:  {},
		model.LevelError: {},
	}
	for _, l := range levels {
		if _, ok := allowed[l]; !ok {
			return &apiquery.ValidationError{Details: []apiquery.Detail{{
				Field: "levels", Message: fmt.Sprintf("unknown level %q", l), Code: "unknown",
			}}}
		}
	}
	return nil
}

func validateNewRule(in model.RuleInput) (model.PurgeRule, error) {
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	hours := int32(168)
	if in.OlderThanHours != nil {
		hours = *in.OlderThanHours
	}
	interval := int32(1440)
	if in.IntervalMinutes != nil {
		interval = *in.IntervalMinutes
	}
	rule := model.PurgeRule{
		Name:            strings.TrimSpace(in.Name),
		Enabled:         enabled,
		Levels:          normalizeLevels(in.Levels),
		Source:          deref(in.Source),
		MessageContains: deref(in.MessageContains),
		OlderThanHours:  hours,
		IntervalMinutes: interval,
	}
	if err := validateRuleValues(rule); err != nil {
		return model.PurgeRule{}, err
	}
	return rule, nil
}

func mergeRule(current model.PurgeRule, in model.RuleInput) model.PurgeRule {
	if name := strings.TrimSpace(in.Name); name != "" {
		current.Name = name
	}
	if in.Enabled != nil {
		current.Enabled = *in.Enabled
	}
	if in.Levels != nil {
		current.Levels = normalizeLevels(in.Levels)
	}
	if in.Source != nil {
		current.Source = strings.TrimSpace(*in.Source)
	}
	if in.MessageContains != nil {
		current.MessageContains = strings.TrimSpace(*in.MessageContains)
	}
	if in.OlderThanHours != nil {
		current.OlderThanHours = *in.OlderThanHours
	}
	if in.IntervalMinutes != nil {
		current.IntervalMinutes = *in.IntervalMinutes
	}
	return current
}

func validateRuleValues(rule model.PurgeRule) error {
	var details []apiquery.Detail
	if rule.Name == "" || len(rule.Name) > 120 {
		details = append(details, apiquery.Detail{
			Field: "name", Message: "name must be 1-120 characters", Code: "invalid",
		})
	}
	if err := validateLevels(rule.Levels); err != nil {
		var ve *apiquery.ValidationError
		if errors.As(err, &ve) {
			details = append(details, ve.Details...)
		}
	}
	if rule.OlderThanHours < 1 || rule.OlderThanHours > 43800 {
		details = append(details, apiquery.Detail{
			Field: "older_than_hours", Message: "older_than_hours must be between 1 and 43800", Code: "invalid",
		})
	}
	if !allowedInterval(int(rule.IntervalMinutes)) {
		details = append(details, apiquery.Detail{
			Field: "interval_minutes", Message: "unsupported schedule interval", Code: "invalid",
		})
	}
	if len(rule.Source) > 200 {
		details = append(details, apiquery.Detail{
			Field: "source", Message: "source is too long", Code: "invalid",
		})
	}
	if len(rule.MessageContains) > 200 {
		details = append(details, apiquery.Detail{
			Field: "message_contains", Message: "message_contains is too long", Code: "invalid",
		})
	}
	if len(details) > 0 {
		return &apiquery.ValidationError{Details: details}
	}
	return nil
}

func allowedInterval(n int) bool {
	for _, v := range model.AllowedIntervals {
		if v == n {
			return true
		}
	}
	return false
}

func normalizeLevels(levels []string) []string {
	if levels == nil {
		return []string{}
	}
	out := make([]string, 0, len(levels))
	seen := map[string]struct{}{}
	for _, l := range levels {
		l = strings.TrimSpace(strings.ToLower(l))
		if l == "" {
			continue
		}
		if _, ok := seen[l]; ok {
			continue
		}
		seen[l] = struct{}{}
		out = append(out, l)
	}
	return out
}

func mapLog(row db.AppLog) model.Log {
	var attrs map[string]any
	_ = json.Unmarshal(row.Attrs, &attrs)
	if attrs == nil {
		attrs = map[string]any{}
	}
	return model.Log{
		UUID:      row.Uuid,
		Level:     row.Level,
		Message:   row.Message,
		Source:    row.Source,
		Attrs:     attrs,
		RequestID: textValue(row.RequestID),
		CreatedAt: row.CreatedAt.Time,
	}
}

func mapRule(row db.LogPurgeRule) model.PurgeRule {
	levels := row.Levels
	if levels == nil {
		levels = []string{}
	}
	return model.PurgeRule{
		UUID:             row.Uuid,
		Name:             row.Name,
		Enabled:          row.Enabled,
		IsSystem:         row.IsSystem,
		Levels:           levels,
		Source:           textValue(row.Source),
		MessageContains:  textValue(row.MessageContains),
		OlderThanHours:   row.OlderThanHours,
		IntervalMinutes:  row.IntervalMinutes,
		LastRunAt:        timestamptzPtr(row.LastRunAt),
		LastDeletedCount: row.LastDeletedCount,
		LastError:        textValue(row.LastError),
		CreatedAt:        row.CreatedAt.Time,
		UpdatedAt:        row.UpdatedAt.Time,
	}
}

func textNarg(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func textValue(v pgtype.Text) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

func emptyToNil(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

func timestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func timeNarg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return timestamptz(*t)
}

func timestamptzPtr(v pgtype.Timestamptz) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

func intNarg(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *v, Valid: true}
}

func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n]
}
