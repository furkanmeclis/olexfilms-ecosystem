package repository

// TEC-389 (F4-01g): platform AI settings, the organization quota table, the
// usage report lookups and the quota threshold recipients.

import (
	"context"
	"errors"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// OrgQuotaSort is the sort contract of the platform quota table. usage is
// the org-pool tokens of the period; quota the effective quota (unlimited
// sorts as the largest).
var OrgQuotaSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{"usage": "usage", "quota": "quota", "name": "name"},
	Default: apiquery.SortField{Field: "usage", Desc: true},
}

// OrgQuotaFilter selects rows of the quota table. Nil slices mean no filter.
type OrgQuotaFilter struct {
	// DefaultQuota is ai_settings.default_monthly_token_quota (the quota of
	// organizations without an override).
	DefaultQuota    int64
	Period          string
	OrgTypes        []string
	OrganizationIDs []int64
	Q               string
	Sort            []apiquery.SortField
	Limit, Offset   int32
}

// ListOrgQuotas returns a page of the quota table and the total. An unknown
// sort field is an *apiquery.ValidationError.
func (s *Store) ListOrgQuotas(ctx context.Context, f OrgQuotaFilter) ([]db.ListAIOrgQuotasRow, int64, error) {
	sort, err := apiquery.ResolveSort(f.Sort, OrgQuotaSort)
	if err != nil {
		return nil, 0, err
	}
	q := textNarg(f.Q)
	rows, err := s.q.ListAIOrgQuotas(ctx, db.ListAIOrgQuotasParams{
		DefaultQuota: f.DefaultQuota, Period: f.Period, OrgTypes: f.OrgTypes,
		OrganizationIds: f.OrganizationIDs, Q: q,
		SortKey: sort.Key, SortDesc: sort.Desc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.q.CountAIOrgQuotas(ctx, db.CountAIOrgQuotasParams{
		OrgTypes: f.OrgTypes, OrganizationIds: f.OrganizationIDs, Q: q,
	})
	return rows, total, err
}

// UpdateSettings replaces the platform settings row.
func (s *Store) UpdateSettings(ctx context.Context, p db.UpdateAISettingsParams) (db.AiSetting, error) {
	return s.q.UpdateAISettings(ctx, p)
}

// UpsertOrgSettings writes the organization override.
func (s *Store) UpsertOrgSettings(ctx context.Context, p db.UpsertAIOrgSettingsParams) (db.AiOrgSetting, error) {
	return s.q.UpsertAIOrgSettings(ctx, p)
}

// OrganizationByUUID returns a not deleted organization; ok is false when
// there is none.
func (s *Store) OrganizationByUUID(ctx context.Context, id uuid.UUID) (db.Organization, bool, error) {
	row, err := s.q.GetAIOrganizationByUUID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Organization{}, false, nil
	}
	if err != nil {
		return db.Organization{}, false, err
	}
	return row, true, nil
}

// Descendants lists the organizations below one organization.
func (s *Store) Descendants(ctx context.Context, id int64) ([]db.Organization, error) {
	return s.q.Descendants(ctx, id)
}

// OrganizationsByIDs returns the display fields of organizations by id.
func (s *Store) OrganizationsByIDs(ctx context.Context, ids []int64) (map[int64]db.ListAIOrganizationsByIDsRow, error) {
	out := map[int64]db.ListAIOrganizationsByIDsRow{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q.ListAIOrganizationsByIDs(ctx, ids)
	for _, r := range rows {
		out[r.ID] = r
	}
	return out, err
}

// UsersByIDs returns the display fields of users by id.
func (s *Store) UsersByIDs(ctx context.Context, ids []int64) (map[int64]db.ListAIUsersByIDsRow, error) {
	out := map[int64]db.ListAIUsersByIDsRow{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q.ListAIUsersByIDs(ctx, ids)
	for _, r := range rows {
		out[r.ID] = r
	}
	return out, err
}

// UserIDsByUUIDs resolves user uuids; unknown ones are dropped (the result
// is never nil, so an unknown-only filter matches nothing).
func (s *Store) UserIDsByUUIDs(ctx context.Context, ids []uuid.UUID) ([]int64, error) {
	out, err := s.q.ListAIUserIDsByUUIDs(ctx, ids)
	if out == nil {
		out = []int64{}
	}
	return out, err
}

// OrganizationIDsByUUIDs resolves organization uuids like UserIDsByUUIDs.
func (s *Store) OrganizationIDsByUUIDs(ctx context.Context, ids []uuid.UUID) ([]int64, error) {
	out, err := s.q.ListAIOrganizationIDsByUUIDs(ctx, ids)
	if out == nil {
		out = []int64{}
	}
	return out, err
}

// UsageByUser sums the usage of an organization in [from, before) per user.
func (s *Store) UsageByUser(ctx context.Context, orgID int64, from, before time.Time) ([]db.SummarizeAIUsageByUserRow, error) {
	return s.q.SummarizeAIUsageByUser(ctx, db.SummarizeAIUsageByUserParams{
		OrganizationID: orgID, CreatedFrom: ts(from), CreatedBefore: ts(before),
	})
}

// UsageByChannel sums the usage of an organization in [from, before) per
// channel and pool.
func (s *Store) UsageByChannel(ctx context.Context, orgID int64, from, before time.Time) ([]db.SummarizeAIUsageByChannelRow, error) {
	return s.q.SummarizeAIUsageByChannel(ctx, db.SummarizeAIUsageByChannelParams{
		OrganizationID: orgID, CreatedFrom: ts(from), CreatedBefore: ts(before),
	})
}

// QuotaNotifyUserIDs returns the recipients of a quota threshold of a pool.
func (s *Store) QuotaNotifyUserIDs(ctx context.Context, orgID int64, pool string) ([]int64, error) {
	return s.q.ListAIQuotaNotifyUserIDs(ctx, db.ListAIQuotaNotifyUserIDsParams{Pool: pool, OrganizationID: orgID})
}

func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}
