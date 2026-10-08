package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5"
)

const (
	EventCertificatesExpiring = events.CertificatesExpiring
	EventCertificateExpired   = events.CertificateExpired

	DefaultBatchSize = 200
	dateLayout       = "2006-01-02"
)

// CronService runs certificate expiry notices and expiry transitions.
type CronService struct {
	pool     TxBeginner
	q        *db.Queries
	out      outbox.Enqueuer
	features FeatureChecker
	settings Settings
	log      *slog.Logger
	now      func() time.Time
}

func NewCron(pool TxBeginner, q *db.Queries, out outbox.Enqueuer, featureChecker FeatureChecker, settings Settings, log *slog.Logger) *CronService {
	if log == nil {
		log = slog.Default()
	}
	return &CronService{
		pool: pool, q: q, out: out, features: featureChecker, settings: settings, log: log,
		now: time.Now,
	}
}

func (s *CronService) WithClock(now func() time.Time) *CronService {
	if now != nil {
		s.now = now
	}
	return s
}

func (s *CronService) ExpiryScanTask(ctx context.Context) error {
	now := s.now()
	if _, err := s.NotifyExpiring(ctx, now); err != nil {
		return err
	}
	_, err := s.ExpireDue(ctx, now)
	return err
}

func (s *CronService) NotifyExpiring(ctx context.Context, now time.Time) (int, error) {
	days := 30
	if s.settings != nil {
		if configured := s.settings.CertificatesExpiryNoticeDays(ctx); configured > 0 {
			days = configured
		}
	}
	total := 0
	for {
		written, listed, err := s.notifyBatch(ctx, now, days)
		if err != nil {
			return total, err
		}
		total += written
		if listed < DefaultBatchSize {
			return total, nil
		}
	}
}

func (s *CronService) notifyBatch(ctx context.Context, now time.Time, days int) (written, listed int, err error) {
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		rows, err := q.ListCertificatesDueForExpiryNotice(ctx, db.ListCertificatesDueForExpiryNoticeParams{
			Now: ts(now), Days: int32(days), RowLimit: DefaultBatchSize,
		})
		if err != nil {
			return fmt.Errorf("certificates: list expiry notices: %w", err)
		}
		listed = len(rows)
		for _, cert := range rows {
			on, err := s.featureOn(ctx, cert.OrganizationID)
			if err != nil {
				return err
			}
			if !on {
				continue
			}
			marked, err := q.MarkCertificateExpiryNoticeSent(ctx, db.MarkCertificateExpiryNoticeSentParams{Now: ts(now), ID: cert.ID})
			if err != nil {
				return fmt.Errorf("certificates: mark expiry notice: %w", err)
			}
			if marked != 1 {
				continue
			}
			ev, err := s.event(ctx, q, EventCertificatesExpiring, cert, days)
			if err != nil {
				return err
			}
			if len(userIDsFromPayload(ev.Payload, "notify_user_ids")) == 0 {
				continue
			}
			if err := s.out.Enqueue(ctx, tx, ev); err != nil {
				return fmt.Errorf("certificates: expiring event: %w", err)
			}
			written++
		}
		return nil
	})
	if err != nil {
		return 0, listed, err
	}
	return written, listed, nil
}

func (s *CronService) ExpireDue(ctx context.Context, now time.Time) (int, error) {
	total := 0
	for {
		written, listed, err := s.expireBatch(ctx, now)
		if err != nil {
			return total, err
		}
		total += written
		if listed < DefaultBatchSize {
			return total, nil
		}
	}
}

func (s *CronService) expireBatch(ctx context.Context, now time.Time) (written, listed int, err error) {
	err = s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		rows, err := q.ListCertificatesDueForExpiry(ctx, db.ListCertificatesDueForExpiryParams{Now: ts(now), RowLimit: DefaultBatchSize})
		if err != nil {
			return fmt.Errorf("certificates: list due expiry: %w", err)
		}
		listed = len(rows)
		requireApproval := s.settings != nil && s.settings.CertificatesRequireAdminApproval(ctx)
		for _, row := range rows {
			on, err := s.featureOn(ctx, row.OrganizationID)
			if err != nil {
				return err
			}
			if !on {
				continue
			}
			cert, err := q.MarkCertificateExpired(ctx, db.MarkCertificateExpiredParams{ID: row.ID, Now: ts(now)})
			if err != nil {
				if err == pgx.ErrNoRows {
					continue
				}
				return fmt.Errorf("certificates: mark expired: %w", err)
			}
			if err := s.recheckOpenServices(ctx, q, cert, requireApproval); err != nil {
				return err
			}
			ev, err := s.event(ctx, q, EventCertificateExpired, cert, 0)
			if err != nil {
				return err
			}
			if err := s.out.Enqueue(ctx, tx, ev); err != nil {
				return fmt.Errorf("certificates: expired event: %w", err)
			}
			written++
		}
		return nil
	})
	if err != nil {
		return 0, listed, err
	}
	return written, listed, nil
}

func (s *CronService) recheckOpenServices(ctx context.Context, q *db.Queries, cert db.Certificate, requireApproval bool) error {
	services, err := q.ListOpenServicesRequiringCertificate(ctx, db.ListOpenServicesRequiringCertificateParams{
		OrganizationID: cert.OrganizationID, BrandID: cert.BrandID, UserID: cert.UserID, TypeID: cert.TypeID,
	})
	if err != nil {
		return fmt.Errorf("certificates: list open services: %w", err)
	}
	decision := "none"
	if requireApproval {
		decision = "pending_approval"
	}
	for _, svc := range services {
		if _, err := q.UpsertServiceCertificateWarning(ctx, db.UpsertServiceCertificateWarningParams{
			ServiceID: svc.ID, OrganizationID: svc.OrganizationID, BrandID: svc.BrandID,
			UserID: cert.UserID, TypeID: cert.TypeID, Reason: "expired", Decision: decision,
			Note: "certificate expired",
		}); err != nil {
			return fmt.Errorf("certificates: upsert service warning: %w", err)
		}
	}
	return nil
}

func (s *CronService) event(ctx context.Context, q *db.Queries, name string, cert db.Certificate, days int) (events.Event, error) {
	org, err := q.GetOrganizationByID(ctx, cert.OrganizationID)
	if err != nil {
		return events.Event{}, fmt.Errorf("certificates: organization: %w", err)
	}
	typ, err := q.GetCertificateType(ctx, db.GetCertificateTypeParams{ID: cert.TypeID, BrandID: cert.BrandID})
	if err != nil {
		return events.Event{}, fmt.Errorf("certificates: type: %w", err)
	}
	recipients, err := s.recipients(ctx, q, cert)
	if err != nil {
		return events.Event{}, err
	}
	payload := map[string]any{
		"certificate_uuid":  cert.Uuid.String(),
		"user_id":           cert.UserID,
		"organization_id":   cert.OrganizationID,
		"brand_id":          cert.BrandID,
		"type_id":           cert.TypeID,
		"type_name":         localizedName(typ.Name),
		"organization_name": org.Name,
		"expires_at":        "",
		"expires_date":      "",
		"notify_user_ids":   recipients,
	}
	if cert.ExpiresAt.Valid {
		payload["expires_at"] = cert.ExpiresAt.Time.UTC().Format(time.RFC3339)
		payload["expires_date"] = localDate(cert.ExpiresAt.Time, org.Timezone)
	}
	if days > 0 {
		payload["days"] = days
	}
	id, u := cert.ID, cert.Uuid
	return events.New(name).WithTenant(cert.OrganizationID).WithEntity("certificate", &id, &u).WithPayload(payload), nil
}

func (s *CronService) recipients(ctx context.Context, q *db.Queries, cert db.Certificate) ([]int64, error) {
	ids := []int64{cert.UserID}
	dealerOwners, err := q.ListOrganizationOwnerUserIDs(ctx, cert.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("certificates: organization owners: %w", err)
	}
	ids = append(ids, dealerOwners...)
	if cert.VerifiedByOrgID.Valid {
		verifierOwners, err := q.ListOrganizationOwnerUserIDs(ctx, cert.VerifiedByOrgID.Int64)
		if err != nil {
			return nil, fmt.Errorf("certificates: verifier owners: %w", err)
		}
		ids = append(ids, verifierOwners...)
	}
	slices.Sort(ids)
	return slices.Compact(ids), nil
}

func (s *CronService) featureOn(ctx context.Context, organizationID int64) (bool, error) {
	if s.features == nil {
		return true, nil
	}
	on, err := s.features.Enabled(ctx, organizationID, features.ModuleCertificates)
	if err != nil {
		return false, fmt.Errorf("certificates: feature check: %w", err)
	}
	return on, nil
}

func (s *CronService) inTx(ctx context.Context, fn func(q *db.Queries, tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("certificates: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx), tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("certificates: commit: %w", err)
	}
	return nil
}

func localDate(v time.Time, zone string) string {
	loc, err := time.LoadLocation(zone)
	if err != nil || zone == "" {
		loc = time.UTC
	}
	return v.In(loc).Format(dateLayout)
}

func localizedName(raw []byte) string {
	var names map[string]string
	if err := json.Unmarshal(raw, &names); err != nil {
		return string(raw)
	}
	for _, lang := range []string{"tr", "en"} {
		if names[lang] != "" {
			return names[lang]
		}
	}
	for _, v := range names {
		if v != "" {
			return v
		}
	}
	return ""
}

func userIDsFromPayload(payload map[string]any, key string) []int64 {
	raw, _ := payload[key].([]int64)
	return raw
}
