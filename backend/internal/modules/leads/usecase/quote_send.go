package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	shorturlsuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/shorturls/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	CodeQuoteNoRecipient = "QUOTE_NO_RECIPIENT"

	quoteReminderDelay = 72 * time.Hour
)

var ErrQuoteNoRecipient = errors.New("quotes: no recipient")

type QuoteSendResult struct {
	Quote     Quote  `json:"quote"`
	PublicURL string `json:"public_url"`
}

type PublicQuote struct {
	UUID             uuid.UUID         `json:"uuid"`
	DisplayNo        string            `json:"display_no"`
	OrganizationName string            `json:"organization_name"`
	Currency         string            `json:"currency"`
	Subtotal         string            `json:"subtotal"`
	DiscountTotal    string            `json:"discount_total"`
	TaxTotal         string            `json:"tax_total"`
	GrandTotal       string            `json:"grand_total"`
	ValidUntil       *time.Time        `json:"valid_until,omitempty"`
	Lines            []QuoteLine       `json:"lines"`
	PDF              PublicQuotePDFRef `json:"pdf"`
}

type PublicQuotePDFRef struct {
	URL string `json:"url"`
}

func (s *Service) SendQuote(ctx context.Context, c Caller, id uuid.UUID) (QuoteSendResult, error) {
	cur, err := s.quoteRow(ctx, c, id)
	if err != nil {
		return QuoteSendResult{}, err
	}
	if cur.Status != QuoteStatusDraft && cur.Status != QuoteStatusSent {
		return QuoteSendResult{}, ErrQuoteConflict
	}
	var row db.Quote
	var publicURL string
	var reminder *db.QuoteReminder
	err = s.withTx(ctx, func(tx pgx.Tx, q *db.Queries) error {
		locked, err := q.LockQuoteByID(ctx, cur.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrQuoteNotFound
		}
		if err != nil {
			return fmt.Errorf("quotes: lock: %w", err)
		}
		if locked.Status != QuoteStatusDraft && locked.Status != QuoteStatusSent {
			return ErrQuoteConflict
		}
		recipient, err := q.GetQuoteRecipient(ctx, db.GetQuoteRecipientParams{QuoteID: locked.ID, OrganizationID: locked.OrganizationID})
		if err != nil {
			return fmt.Errorf("quotes: recipient: %w", err)
		}
		phone := textValue(recipient.CandidatePhoneE164)
		if phone == "" {
			phone = textValue(recipient.CustomerPhoneE164)
		}
		if phone == "" {
			return ErrQuoteNoRecipient
		}
		target := "/portal/quotes/" + locked.PublicToken.String()
		publicURL = target
		if s.quoteLinks != nil {
			link, err := s.quoteLinks.LinkWithStore(ctx, q, shorturlsuc.CreateInput{
				BrandID: locked.BrandID, OrganizationID: &locked.OrganizationID, CreatedBy: actorPtr(c.actor()), Target: target,
			})
			if err != nil {
				return fmt.Errorf("quotes: short url: %w", err)
			}
			publicURL = link
		}
		firstSend := locked.Status != QuoteStatusSent
		row, err = q.EnsureQuoteSent(ctx, db.EnsureQuoteSentParams{ID: locked.ID, OrganizationID: locked.OrganizationID})
		if err != nil {
			return fmt.Errorf("quotes: send status: %w", err)
		}
		if _, err := q.CreateQuoteDelivery(ctx, db.CreateQuoteDeliveryParams{
			QuoteID: locked.ID, OrganizationID: locked.OrganizationID, BrandID: locked.BrandID,
			Channel: "whatsapp", Status: "pending",
		}); err != nil {
			return fmt.Errorf("quotes: delivery: %w", err)
		}
		lead, err := q.GetLeadByID(ctx, db.GetLeadByIDParams{ID: locked.LeadID, BrandID: locked.BrandID})
		if err != nil {
			return fmt.Errorf("quotes: lead: %w", err)
		}
		if firstSend {
			if lead.Status != StatusQuoted {
				if _, err := q.SetLeadStatus(ctx, db.SetLeadStatusParams{
					ID: lead.ID, OrganizationID: lead.OrganizationID, Status: StatusQuoted,
				}); err != nil {
					return fmt.Errorf("quotes: lead quoted: %w", err)
				}
			}
			if err := addEvent(ctx, q, lead, "quote_sent", map[string]any{
				"kind": "quote_sent", "quote_uuid": locked.Uuid.String(), "quote_no": locked.QuoteNo,
			}, c.actor()); err != nil {
				return fmt.Errorf("quotes: lead event: %w", err)
			}
			scheduled := s.nowFunc().Add(quoteReminderDelay).UTC()
			r, err := q.CreateQuoteReminderIfMissing(ctx, db.CreateQuoteReminderIfMissingParams{
				QuoteID: locked.ID, OrganizationID: locked.OrganizationID, BrandID: locked.BrandID,
				ScheduledAt: pgtype.Timestamptz{Time: scheduled, Valid: true},
			})
			if err == nil {
				reminder = &r
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("quotes: reminder: %w", err)
			}
		}
		return s.enqueueQuoteSent(ctx, tx, row, recipient, phone, publicURL, false, c.actor())
	})
	if err != nil {
		return QuoteSendResult{}, err
	}
	if reminder != nil {
		if err := s.enqueueReminder(*reminder); err != nil {
			return QuoteSendResult{}, err
		}
	}
	q, err := s.quoteOf(ctx, row)
	if err != nil {
		return QuoteSendResult{}, err
	}
	return QuoteSendResult{Quote: q, PublicURL: publicURL}, nil
}

func (s *Service) RemindQuote(ctx context.Context, c Caller, id uuid.UUID) (Quote, error) {
	cur, err := s.quoteRow(ctx, c, id)
	if err != nil {
		return Quote{}, err
	}
	if cur.Status != QuoteStatusSent {
		return Quote{}, ErrQuoteConflict
	}
	err = s.withTx(ctx, func(tx pgx.Tx, q *db.Queries) error {
		locked, err := q.LockQuoteByID(ctx, cur.ID)
		if err != nil {
			return err
		}
		if locked.Status != QuoteStatusSent {
			return ErrQuoteConflict
		}
		recipient, err := q.GetQuoteRecipient(ctx, db.GetQuoteRecipientParams{QuoteID: locked.ID, OrganizationID: locked.OrganizationID})
		if err != nil {
			return err
		}
		phone := textValue(recipient.CandidatePhoneE164)
		if phone == "" {
			phone = textValue(recipient.CustomerPhoneE164)
		}
		if phone == "" {
			return ErrQuoteNoRecipient
		}
		link := "/portal/quotes/" + locked.PublicToken.String()
		if s.quoteLinks != nil {
			v, err := s.quoteLinks.LinkWithStore(ctx, q, shorturlsuc.CreateInput{
				BrandID: locked.BrandID, OrganizationID: &locked.OrganizationID, CreatedBy: actorPtr(c.actor()), Target: link,
			})
			if err != nil {
				return err
			}
			link = v
		}
		if _, err := q.CreateQuoteDelivery(ctx, db.CreateQuoteDeliveryParams{
			QuoteID: locked.ID, OrganizationID: locked.OrganizationID, BrandID: locked.BrandID,
			Channel: "whatsapp", Status: "pending",
		}); err != nil {
			return err
		}
		return s.enqueueQuoteSent(ctx, tx, locked, recipient, phone, link, true, c.actor())
	})
	if err != nil {
		return Quote{}, err
	}
	return s.quoteOf(ctx, cur)
}

func (s *Service) QuoteReminderTask(ctx context.Context, reminderID int64) error {
	return s.withTx(ctx, func(tx pgx.Tx, q *db.Queries) error {
		rem, err := q.GetQuoteReminderByID(ctx, reminderID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if rem.SentAt.Valid {
			return nil
		}
		quote, err := q.LockQuoteByID(ctx, rem.QuoteID)
		if errors.Is(err, pgx.ErrNoRows) {
			_, _ = q.MarkQuoteReminderSent(ctx, reminderID)
			return nil
		}
		if err != nil {
			return err
		}
		if quote.Status != QuoteStatusSent {
			_, err = q.MarkQuoteReminderSent(ctx, reminderID)
			return err
		}
		recipient, err := q.GetQuoteRecipient(ctx, db.GetQuoteRecipientParams{QuoteID: quote.ID, OrganizationID: quote.OrganizationID})
		if err != nil {
			return err
		}
		phone := textValue(recipient.CandidatePhoneE164)
		if phone == "" {
			phone = textValue(recipient.CustomerPhoneE164)
		}
		if phone == "" {
			_, err = q.MarkQuoteReminderSent(ctx, reminderID)
			return err
		}
		link := "/portal/quotes/" + quote.PublicToken.String()
		if s.quoteLinks != nil {
			v, err := s.quoteLinks.LinkWithStore(ctx, q, shorturlsuc.CreateInput{
				BrandID: quote.BrandID, OrganizationID: &quote.OrganizationID, Target: link,
			})
			if err != nil {
				return err
			}
			link = v
		}
		if _, err := q.CreateQuoteDelivery(ctx, db.CreateQuoteDeliveryParams{
			QuoteID: quote.ID, OrganizationID: quote.OrganizationID, BrandID: quote.BrandID,
			Channel: "whatsapp", Status: "pending",
		}); err != nil {
			return err
		}
		if err := s.enqueueQuoteSent(ctx, tx, quote, recipient, phone, link, true, pgtype.Int8{}); err != nil {
			return err
		}
		_, err = q.MarkQuoteReminderSent(ctx, reminderID)
		return err
	})
}

func (s *Service) PublicQuote(ctx context.Context, brandID int64, token uuid.UUID, pdfURL string) (PublicQuote, error) {
	row, err := s.q.GetQuotePublicViewByToken(ctx, db.GetQuotePublicViewByTokenParams{
		PublicToken: token, BrandID: brandID, Today: pgtype.Date{Time: s.nowFunc().UTC(), Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicQuote{}, ErrQuoteNotFound
	}
	if err != nil {
		return PublicQuote{}, fmt.Errorf("quotes: public: %w", err)
	}
	lines, err := s.q.ListQuoteLines(ctx, row.ID)
	if err != nil {
		return PublicQuote{}, err
	}
	_ = s.recordQuoteViewed(ctx, row)
	out := PublicQuote{
		UUID: row.Uuid, DisplayNo: quoteDisplayNo(row.QuoteNo), OrganizationName: row.OrganizationName,
		Currency: row.Currency, Subtotal: moneyText(row.Subtotal), DiscountTotal: moneyText(row.DiscountTotal),
		TaxTotal: moneyText(row.TaxTotal), GrandTotal: moneyText(row.GrandTotal), ValidUntil: datePtr(row.ValidUntil),
		PDF: PublicQuotePDFRef{URL: pdfURL},
	}
	out.Lines = make([]QuoteLine, 0, len(lines))
	for _, l := range lines {
		out.Lines = append(out.Lines, QuoteLine{
			LineType: l.LineType, Description: l.DescriptionSnapshot, Quantity: qtyText(l.Quantity),
			UnitPrice: moneyText(l.UnitPrice), DiscountAmount: moneyText(l.DiscountAmount),
			LineTotal: moneyText(l.LineTotal), SortOrder: l.SortOrder,
		})
	}
	return out, nil
}

func (s *Service) PublicQuoteOwner(ctx context.Context, brandID int64, token uuid.UUID) (uuid.UUID, int64, int64, error) {
	row, err := s.q.GetQuotePublicViewByToken(ctx, db.GetQuotePublicViewByTokenParams{
		PublicToken: token, BrandID: brandID, Today: pgtype.Date{Time: s.nowFunc().UTC(), Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, 0, 0, ErrQuoteNotFound
	}
	if err != nil {
		return uuid.Nil, 0, 0, err
	}
	return row.Uuid, row.OrganizationID, row.BrandID, nil
}

func (s *Service) recordQuoteViewed(ctx context.Context, row db.GetQuotePublicViewByTokenRow) error {
	payload := []byte(fmt.Sprintf(`{"kind":"quote_viewed","quote_uuid":%q,"quote_no":%d}`, row.Uuid.String(), row.QuoteNo))
	_, err := s.q.AddQuoteViewedEventIfMissing(ctx, db.AddQuoteViewedEventIfMissingParams{
		LeadID: row.LeadID, OrganizationID: row.OrganizationID, BrandID: row.BrandID,
		Payload: payload, QuoteUuid: row.Uuid.String(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

func (s *Service) enqueueQuoteSent(ctx context.Context, tx pgx.Tx, row db.Quote, r db.GetQuoteRecipientRow, phone, link string, reminder bool, actor pgtype.Int8) error {
	if s.out == nil {
		return nil
	}
	payload := map[string]any{
		"quote_uuid":        row.Uuid.String(),
		"quote_no":          row.QuoteNo,
		"display_no":        quoteDisplayNo(row.QuoteNo),
		"brand_id":          row.BrandID,
		"recipient_phone":   phone,
		"recipient_user_id": int64(0),
		"recipient_name":    r.RecipientName,
		"organization_name": r.OrganizationName,
		"quote_url":         link,
		"total_amount":      moneyText(row.GrandTotal) + " " + row.Currency,
		"reminder":          reminder,
		"language":          r.Language,
	}
	if r.CustomerUserID.Valid {
		payload["recipient_user_id"] = r.CustomerUserID.Int64
	}
	ev := events.New(events.QuoteSent).
		WithTenant(row.OrganizationID).
		WithEntity("quote", &row.ID, &row.Uuid).
		WithPayload(payload)
	if actor.Valid {
		ev = ev.WithActor(actor.Int64)
	}
	return s.out.Enqueue(ctx, tx, ev)
}

func (s *Service) enqueueReminder(rem db.QuoteReminder) error {
	if s.quoteQueue == nil {
		return nil
	}
	task, err := queue.NewQuoteReminderTask(rem.ID)
	if err != nil {
		return err
	}
	delay := time.Until(rem.ScheduledAt.Time)
	_, err = s.quoteQueue.Enqueue(task, queue.QuoteReminderOpts(rem.QuoteID, delay)...)
	if errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func (s *Service) withTx(ctx context.Context, fn func(pgx.Tx, *db.Queries) error) error {
	if s.pool == nil {
		return fn(nil, s.q)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := fn(tx, s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func textValue(v pgtype.Text) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

func actorPtr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}
