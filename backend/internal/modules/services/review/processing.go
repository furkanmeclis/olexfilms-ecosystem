package review

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/jackc/pgx/v5"
)

const (
	lowScoreSettingKey = "reviews.low_score_threshold"
	defaultLowScore    = 2
)

// Processor consumes service.reviewed once, opens a center task for low
// scores and notifies the dealer owner(s). The processed_at claim is the
// idempotency guard.
type Processor struct {
	pool TxBeginner
	q    *db.Queries
	out  outbox.Enqueuer
	log  *slog.Logger
}

// NewProcessor builds the review processing event consumer.
func NewProcessor(pool TxBeginner, q *db.Queries, out outbox.Enqueuer, log *slog.Logger) *Processor {
	if log == nil {
		log = slog.Default()
	}
	return &Processor{pool: pool, q: q, out: out, log: log}
}

// HandleServiceReviewed processes one review event. Missing/invalid payloads
// are skipped because the outbox can only redeliver the same event.
func (p *Processor) HandleServiceReviewed(ctx context.Context, ev events.Event) error {
	reviewID, ok := payloadInt64(ev.Payload, "review_id")
	if !ok && ev.EntityID != nil {
		reviewID, ok = *ev.EntityID, true
	}
	if !ok || reviewID <= 0 {
		p.log.Warn("service_review_processing_no_review", "event_id", ev.EventID.String())
		return nil
	}
	return p.Process(ctx, reviewID)
}

// Process handles one review by internal id.
func (p *Processor) Process(ctx context.Context, reviewID int64) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("service review processing: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := p.q.WithTx(tx)
	if _, err := q.MarkServiceReviewProcessed(ctx, reviewID); errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		return fmt.Errorf("service review processing: claim: %w", err)
	}
	d, err := q.GetServiceReviewProcessingDetails(ctx, reviewID)
	if err != nil {
		return fmt.Errorf("service review processing: details: %w", err)
	}
	threshold, err := p.lowScoreThreshold(ctx, q)
	if err != nil {
		return err
	}
	if int(d.MinRating) > threshold {
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("service review processing: commit: %w", err)
		}
		return nil
	}
	task, err := q.InsertTask(ctx, db.InsertTaskParams{
		OrganizationID: d.CenterOrganizationID,
		BrandID:        d.BrandID,
		SubjectOrgID:   d.OrganizationID,
		Title:          fmt.Sprintf("Low review score: %s", d.OrganizationName),
		Description:    lowScoreDescription(d),
		Priority:       "high",
		Source:         "auto",
	})
	if err != nil {
		return fmt.Errorf("service review processing: task: %w", err)
	}
	if p.out != nil {
		id, uid := task.ID, task.Uuid
		if err := p.out.Enqueue(ctx, tx, events.New(events.TasksCreated).
			WithTenant(d.CenterOrganizationID).
			WithEntity("task", &id, &uid).
			WithPayload(map[string]any{
				"task_id": task.ID, "task_uuid": task.Uuid.String(),
				"organization_id": task.OrganizationID, "brand_id": task.BrandID,
				"subject_org_id": task.SubjectOrgID, "subject_org_name": d.OrganizationName,
				"title": task.Title, "priority": task.Priority, "status": task.Status, "source": task.Source,
			})); err != nil {
			return fmt.Errorf("service review processing: task outbox: %w", err)
		}
		if len(d.DealerOwnerUserIds) > 0 {
			rid, ruid := d.ID, d.Uuid
			if err := p.out.Enqueue(ctx, tx, events.New(events.ServiceReviewLowScore).
				WithTenant(d.OrganizationID).
				WithEntity("service_review", &rid, &ruid).
				WithPayload(map[string]any{
					"review_uuid": d.Uuid.String(), "service_uuid": d.ServiceUuid.String(),
					"service_no": d.ServiceNo, "organization_name": d.OrganizationName,
					"brand_id": d.BrandID, "platform_rating": fmt.Sprint(d.PlatformRating),
					"product_rating": fmt.Sprint(d.ProductRating), "min_rating": fmt.Sprint(d.MinRating),
					"notify_user_ids": d.DealerOwnerUserIds,
				})); err != nil {
				return fmt.Errorf("service review processing: notification outbox: %w", err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("service review processing: commit: %w", err)
	}
	p.log.Info("service_review_processed", "review_id", reviewID, "min_rating", d.MinRating)
	return nil
}

func (p *Processor) lowScoreThreshold(ctx context.Context, q *db.Queries) (int, error) {
	row, err := q.GetSystemSetting(ctx, lowScoreSettingKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return defaultLowScore, nil
	}
	if err != nil {
		return 0, fmt.Errorf("service review processing: threshold: %w", err)
	}
	var n int
	if err := json.Unmarshal(row.Value, &n); err == nil && n >= 1 && n <= 5 {
		return n, nil
	}
	var obj struct {
		Value int `json:"value"`
	}
	if err := json.Unmarshal(row.Value, &obj); err == nil && obj.Value >= 1 && obj.Value <= 5 {
		return obj.Value, nil
	}
	return defaultLowScore, nil
}

func lowScoreDescription(d db.GetServiceReviewProcessingDetailsRow) string {
	lines := []string{
		fmt.Sprintf("Dealer: %s", d.OrganizationName),
		fmt.Sprintf("Service: %s", d.ServiceNo),
		fmt.Sprintf("Platform rating: %d", d.PlatformRating),
		fmt.Sprintf("Product rating: %d", d.ProductRating),
		fmt.Sprintf("Lowest rating: %d", d.MinRating),
	}
	if d.Plate.Valid && strings.TrimSpace(d.Plate.String) != "" {
		lines = append(lines, "Plate: "+strings.TrimSpace(d.Plate.String))
	}
	if !d.IsAnonymous {
		name := strings.TrimSpace(d.CustomerName + " " + d.CustomerSurname)
		if name != "" {
			lines = append(lines, "Customer: "+name)
		}
	}
	if d.Comment.Valid && strings.TrimSpace(d.Comment.String) != "" {
		lines = append(lines, "Comment: "+strings.TrimSpace(d.Comment.String))
	}
	return strings.Join(lines, "\n")
}

// RegisterProcessingHandlers attaches the service.reviewed processor.
func RegisterProcessingHandlers(bus events.Bus, processor *Processor) {
	if bus == nil || processor == nil {
		return
	}
	bus.Subscribe(events.ServiceReviewed, processor.HandleServiceReviewed)
}
