package usecase

// TEC-389 (F4-01g): platform AI settings (models from the allow list,
// default and system pool quotas, tool switches, extra instructions and the
// knowledge text) and the organization quota table. Only the center sets
// quotas (ai.settings.manage, super_admin); a distributor never hands out
// quota to its dealers (QUESTIONS 8).

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/repository"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Limits of the settings form.
const (
	// MaxKnowledgeTextBytes bounds the Markdown knowledge text (20 KB).
	MaxKnowledgeTextBytes = 20 * 1024
	// MaxExtraInstructions bounds the extra instructions (characters, the
	// database CHECK).
	MaxExtraInstructions = 20000
	// MaxMonthlyQuota bounds a monthly token quota.
	MaxMonthlyQuota int64 = 1_000_000_000_000
)

// Organization types of the quota table filter.
var OrgTypes = []string{tools.OrgCenter, tools.OrgDistributor, tools.OrgDealer}

// ToolLister lists the registered assistant tools (*tools.Registry).
type ToolLister interface {
	All() []tools.Tool
}

// Admin is the settings, quota and usage report use case.
type Admin struct {
	Store  *repository.Store
	Models llm.Models
	// Tools validates tool_toggles keys and lists the switchable tools;
	// nil accepts any key.
	Tools ToolLister
	now   func() time.Time
}

// NewAdmin builds the use case.
func NewAdmin(store *repository.Store, models llm.Models, toolList ToolLister) *Admin {
	return &Admin{Store: store, Models: models, Tools: toolList, now: time.Now}
}

// SetClock replaces the clock (tests).
func (a *Admin) SetClock(now func() time.Time) { a.now = now }

// UserRef is a user shown next to a record.
type UserRef struct {
	UUID    uuid.UUID `json:"uuid"`
	Name    string    `json:"name"`
	Surname string    `json:"surname"`
}

// OrgRef is an organization shown next to a record.
type OrgRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	Type string    `json:"type"`
}

// ToolInfo is one switchable tool of the settings form.
type ToolInfo struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Realm   string `json:"realm"`
	Enabled bool   `json:"enabled"`
}

// Settings is the platform settings view.
type Settings struct {
	DefaultModel             string          `json:"default_model"`
	FastModel                string          `json:"fast_model"`
	AllowedModels            []string        `json:"allowed_models"`
	DefaultMonthlyTokenQuota int64           `json:"default_monthly_token_quota"`
	SystemPoolMonthlyQuota   int64           `json:"system_pool_monthly_quota"`
	ToolToggles              map[string]bool `json:"tool_toggles"`
	Tools                    []ToolInfo      `json:"tools"`
	ExtraInstructions        string          `json:"extra_instructions"`
	KnowledgeText            string          `json:"knowledge_text"`
	UpdatedAt                time.Time       `json:"updated_at"`
	UpdatedBy                *UserRef        `json:"updated_by"`
}

// SettingsInput changes the settings; nil fields keep their value.
type SettingsInput struct {
	DefaultModel             *string          `json:"default_model"`
	FastModel                *string          `json:"fast_model"`
	DefaultMonthlyTokenQuota *int64           `json:"default_monthly_token_quota"`
	SystemPoolMonthlyQuota   *int64           `json:"system_pool_monthly_quota"`
	ToolToggles              *map[string]bool `json:"tool_toggles"`
	ExtraInstructions        *string          `json:"extra_instructions"`
	KnowledgeText            *string          `json:"knowledge_text"`
}

// AllowedModels is the model allow list offered in the form: the
// AI_ALLOWED_MODELS list, else the env default and fast models.
func (a *Admin) AllowedModels() []string {
	if len(a.Models.Allowed) > 0 {
		return append([]string(nil), a.Models.Allowed...)
	}
	var out []string
	for _, m := range []string{a.Models.Default, a.Models.Fast} {
		if m = strings.TrimSpace(m); m != "" && !contains(out, m) {
			out = append(out, m)
		}
	}
	return out
}

// GetSettings returns the platform settings.
func (a *Admin) GetSettings(ctx context.Context) (Settings, error) {
	row, err := a.Store.Settings(ctx)
	if err != nil {
		return Settings{}, err
	}
	return a.settingsView(ctx, row)
}

// UpdateSettings validates and stores the settings.
func (a *Admin) UpdateSettings(ctx context.Context, actorID int64, in SettingsInput) (Settings, error) {
	row, err := a.Store.Settings(ctx)
	if err != nil {
		return Settings{}, err
	}
	toggles, err := decodeToggles(row.ToolToggles)
	if err != nil {
		return Settings{}, err
	}
	p := db.UpdateAISettingsParams{
		DefaultModel: row.DefaultModel, FastModel: row.FastModel,
		DefaultMonthlyTokenQuota: row.DefaultMonthlyTokenQuota, SystemPoolMonthlyQuota: row.SystemPoolMonthlyQuota,
		ExtraInstructions: row.ExtraInstructions, KnowledgeText: row.KnowledgeText,
		UpdatedByUserID: pgtype.Int8{Int64: actorID, Valid: actorID > 0},
	}
	for _, m := range []struct {
		field string
		in    *string
		dst   *string
	}{{"default_model", in.DefaultModel, &p.DefaultModel}, {"fast_model", in.FastModel, &p.FastModel}} {
		if m.in == nil {
			continue
		}
		v := strings.TrimSpace(*m.in)
		if !a.Models.IsAllowed(v) {
			return Settings{}, &ValidationError{Field: m.field, Message: "must be one of the allowed models: " + strings.Join(a.AllowedModels(), ", ")}
		}
		*m.dst = v
	}
	for _, q := range []struct {
		field string
		in    *int64
		dst   *int64
	}{
		{"default_monthly_token_quota", in.DefaultMonthlyTokenQuota, &p.DefaultMonthlyTokenQuota},
		{"system_pool_monthly_quota", in.SystemPoolMonthlyQuota, &p.SystemPoolMonthlyQuota},
	} {
		if q.in == nil {
			continue
		}
		if err := validQuota(q.field, *q.in); err != nil {
			return Settings{}, err
		}
		*q.dst = *q.in
	}
	if in.ToolToggles != nil {
		next := map[string]bool{}
		for name, on := range *in.ToolToggles {
			if a.Tools != nil && !a.toolKnown(name) {
				return Settings{}, &ValidationError{Field: "tool_toggles", Message: fmt.Sprintf("unknown tool %q", name)}
			}
			if !on {
				next[name] = false // missing keys mean enabled
			}
		}
		toggles = next
	}
	if in.ExtraInstructions != nil {
		v := strings.TrimSpace(*in.ExtraInstructions)
		if utf8.RuneCountInString(v) > MaxExtraInstructions {
			return Settings{}, &ValidationError{Field: "extra_instructions", Message: fmt.Sprintf("must be at most %d characters", MaxExtraInstructions)}
		}
		p.ExtraInstructions = v
	}
	if in.KnowledgeText != nil {
		v := strings.TrimSpace(*in.KnowledgeText)
		if len(v) > MaxKnowledgeTextBytes {
			return Settings{}, &ValidationError{Field: "knowledge_text", Message: "must be at most 20 KB"}
		}
		p.KnowledgeText = v
	}
	if p.ToolToggles, err = json.Marshal(toggles); err != nil {
		return Settings{}, err
	}
	updated, err := a.Store.UpdateSettings(ctx, p)
	if err != nil {
		return Settings{}, err
	}
	return a.settingsView(ctx, updated)
}

func validQuota(field string, v int64) error {
	if v < 0 || v > MaxMonthlyQuota {
		return &ValidationError{Field: field, Message: fmt.Sprintf("must be between 0 (unlimited) and %d", MaxMonthlyQuota)}
	}
	return nil
}

func (a *Admin) toolKnown(name string) bool {
	for _, t := range a.Tools.All() {
		if t.Spec().Name == name {
			return true
		}
	}
	return false
}

func decodeToggles(raw []byte) (map[string]bool, error) {
	out := map[string]bool{}
	if len(raw) == 0 {
		return out, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("ai settings: tool toggles: %w", err)
	}
	for k, v := range m {
		if b, ok := v.(bool); ok {
			out[k] = b
		}
	}
	return out, nil
}

func (a *Admin) settingsView(ctx context.Context, row db.AiSetting) (Settings, error) {
	toggles, err := decodeToggles(row.ToolToggles)
	if err != nil {
		return Settings{}, err
	}
	out := Settings{
		DefaultModel: row.DefaultModel, FastModel: row.FastModel, AllowedModels: a.AllowedModels(),
		DefaultMonthlyTokenQuota: row.DefaultMonthlyTokenQuota, SystemPoolMonthlyQuota: row.SystemPoolMonthlyQuota,
		ToolToggles: toggles, Tools: []ToolInfo{},
		ExtraInstructions: row.ExtraInstructions, KnowledgeText: row.KnowledgeText,
		UpdatedAt: row.UpdatedAt.Time,
	}
	if a.Tools != nil {
		for _, t := range a.Tools.All() {
			s := t.Spec()
			on, set := toggles[s.Name]
			out.Tools = append(out.Tools, ToolInfo{Name: s.Name, Kind: string(s.Kind), Realm: string(s.Realm), Enabled: !set || on})
		}
	}
	if row.UpdatedByUserID.Valid {
		users, err := a.Store.UsersByIDs(ctx, []int64{row.UpdatedByUserID.Int64})
		if err != nil {
			return Settings{}, err
		}
		if u, ok := users[row.UpdatedByUserID.Int64]; ok {
			out.UpdatedBy = &UserRef{UUID: u.Uuid, Name: u.Name, Surname: u.Surname}
		}
	}
	return out, nil
}

// --- quotas ------------------------------------------------------------------

// EffectiveQuota is the monthly org-pool quota of an organization: its
// override, else the platform default (0 = unlimited).
func EffectiveQuota(settings db.AiSetting, override pgtype.Int8) int64 {
	if override.Valid {
		return override.Int64
	}
	return settings.DefaultMonthlyTokenQuota
}

// Percent is used / limit in percent (two decimals); nil when unlimited.
func Percent(used, limit int64) *float64 {
	if limit <= 0 {
		return nil
	}
	p := math.Round(float64(used)*10000/float64(limit)) / 100
	return &p
}

// OrgQuota is one row of the platform quota table.
type OrgQuota struct {
	Organization OrgRef `json:"organization"`
	Status       string `json:"status"`
	Enabled      bool   `json:"enabled"`
	// QuotaOverride is the organization's own quota; nil inherits the
	// platform default.
	QuotaOverride *int64 `json:"quota_override"`
	// Quota is the effective monthly quota (0 = unlimited).
	Quota        int64    `json:"quota"`
	Period       string   `json:"period"`
	Used         int64    `json:"used"`
	Percent      *float64 `json:"percent"`
	RequestCount int64    `json:"request_count"`
	// SystemUsed is the brand center's system pool usage (customer,
	// visitor and triage calls); 0 for other organizations.
	SystemUsed int64 `json:"system_used"`
}

// OrgQuotaListFilter is the quota table query.
type OrgQuotaListFilter struct {
	// Period is YYYY-MM; empty = the current month (UTC).
	Period        string
	OrgTypes      []string
	Q             string
	Sort          []apiquery.SortField
	Limit, Offset int32
}

// ParsePeriod validates YYYY-MM; empty is the month of now (UTC).
func ParsePeriod(raw string, now time.Time) (string, time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = repository.Period(now)
	}
	t, err := time.Parse("2006-01", raw)
	if err != nil || t.Year() < 2000 {
		return "", time.Time{}, &ValidationError{Field: "period", Message: "must be YYYY-MM"}
	}
	return raw, t.UTC(), nil
}

// ListOrgQuotas returns a page of the quota table.
func (a *Admin) ListOrgQuotas(ctx context.Context, f OrgQuotaListFilter) ([]OrgQuota, int64, error) {
	period, _, err := ParsePeriod(f.Period, a.now())
	if err != nil {
		return nil, 0, err
	}
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		return nil, 0, err
	}
	rows, total, err := a.Store.ListOrgQuotas(ctx, repository.OrgQuotaFilter{
		DefaultQuota: settings.DefaultMonthlyTokenQuota, Period: period, OrgTypes: f.OrgTypes, Q: f.Q,
		Sort: f.Sort, Limit: f.Limit, Offset: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]OrgQuota, 0, len(rows))
	for _, r := range rows {
		out = append(out, orgQuotaOf(r, period))
	}
	return out, total, nil
}

func orgQuotaOf(r db.ListAIOrgQuotasRow, period string) OrgQuota {
	q := OrgQuota{
		Organization: OrgRef{UUID: r.Uuid, Name: r.Name, Type: r.Type}, Status: r.Status, Enabled: r.Enabled,
		Quota: r.Quota, Period: period, Used: r.Used, Percent: Percent(r.Used, r.Quota),
		RequestCount: r.RequestCount, SystemUsed: r.SystemUsed,
	}
	if r.QuotaOverride.Valid {
		v := r.QuotaOverride.Int64
		q.QuotaOverride = &v
	}
	return q
}

// OrgQuotaInput replaces an organization override. Enabled nil keeps the
// current switch (on without an override); MonthlyTokenQuota nil (or
// absent) returns the organization to the platform default; 0 = unlimited.
type OrgQuotaInput struct {
	Enabled           *bool  `json:"enabled"`
	MonthlyTokenQuota *int64 `json:"monthly_token_quota"`
}

// UpdateOrgQuota writes the override of an organization (404 when it does
// not exist) and returns its quota row of the current month.
func (a *Admin) UpdateOrgQuota(ctx context.Context, actorID int64, orgUUID uuid.UUID, in OrgQuotaInput) (OrgQuota, error) {
	org, ok, err := a.Store.OrganizationByUUID(ctx, orgUUID)
	if err != nil {
		return OrgQuota{}, err
	}
	if !ok {
		return OrgQuota{}, ErrNotFound
	}
	current, has, err := a.Store.OrgSettings(ctx, org.ID)
	if err != nil {
		return OrgQuota{}, err
	}
	enabled := !has || current.Enabled
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	p := db.UpsertAIOrgSettingsParams{
		OrganizationID: org.ID, BrandID: org.BrandID, Enabled: enabled,
		UpdatedByUserID: pgtype.Int8{Int64: actorID, Valid: actorID > 0},
	}
	if in.MonthlyTokenQuota != nil {
		if err := validQuota("monthly_token_quota", *in.MonthlyTokenQuota); err != nil {
			return OrgQuota{}, err
		}
		p.MonthlyTokenQuota = pgtype.Int8{Int64: *in.MonthlyTokenQuota, Valid: true}
	}
	if _, err := a.Store.UpsertOrgSettings(ctx, p); err != nil {
		return OrgQuota{}, err
	}
	period := repository.Period(a.now())
	settings, err := a.Store.Settings(ctx)
	if err != nil {
		return OrgQuota{}, err
	}
	rows, _, err := a.Store.ListOrgQuotas(ctx, repository.OrgQuotaFilter{
		DefaultQuota: settings.DefaultMonthlyTokenQuota, Period: period, OrganizationIDs: []int64{org.ID}, Limit: 1,
	})
	if err != nil {
		return OrgQuota{}, err
	}
	if len(rows) == 0 {
		return OrgQuota{}, ErrNotFound
	}
	return orgQuotaOf(rows[0], period), nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
