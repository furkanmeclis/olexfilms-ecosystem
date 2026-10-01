package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/catalog"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/providers"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/msgtemplate"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// AdminStore is the persistence of the admin surface (template editor,
// delivery log, channel switches).
type AdminStore interface {
	ListNotificationTemplates(ctx context.Context, arg db.ListNotificationTemplatesParams) ([]db.NotificationTemplate, error)
	UpsertNotificationTemplate(ctx context.Context, arg db.UpsertNotificationTemplateParams) (db.NotificationTemplate, error)
	ListNotificationDeliveries(ctx context.Context, arg db.ListNotificationDeliveriesParams) ([]db.ListNotificationDeliveriesRow, error)
	CountNotificationDeliveries(ctx context.Context, arg db.CountNotificationDeliveriesParams) (int64, error)
	SetNotificationChannelEnabled(ctx context.Context, arg db.SetNotificationChannelEnabledParams) (db.NotificationChannelSetting, error)
}

func (s *Service) admin() (AdminStore, error) {
	q, ok := s.q.(AdminStore)
	if !ok {
		return nil, errors.New("notifications: admin store unavailable")
	}
	return q, nil
}

// Events returns the event catalog.
func (s *Service) Events() []catalog.Event { return catalog.All() }

// TemplateFilter narrows ListTemplates.
type TemplateFilter struct {
	Code, Channel, Language, Role string
}

// ListTemplates returns templates (global and brand rows).
func (s *Service) ListTemplates(ctx context.Context, f TemplateFilter) ([]model.Template, error) {
	q, err := s.admin()
	if err != nil {
		return nil, err
	}
	rows, err := q.ListNotificationTemplates(ctx, db.ListNotificationTemplatesParams{
		Code: optionalText(f.Code), Channel: optionalText(f.Channel),
		Language: optionalText(f.Language), Role: optionalText(f.Role),
	})
	if err != nil {
		return nil, err
	}
	out := make([]model.Template, 0, len(rows))
	for _, r := range rows {
		out = append(out, projectTemplate(r))
	}
	return out, nil
}

func projectTemplate(r db.NotificationTemplate) model.Template {
	t := model.Template{
		UUID: r.Uuid, Code: r.Code, Role: r.Role, Channel: r.Channel, Language: r.Language,
		Subject: r.Subject, Body: r.Body, Format: r.Format, Active: r.Active, UpdatedAt: r.UpdatedAt.Time,
	}
	if r.BrandID.Valid {
		id := r.BrandID.Int64
		t.BrandID = &id
	}
	return t
}

// ValidationError lists rejected template fields.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

func (s *Service) validateTemplate(in *model.TemplateInput) (catalog.Event, error) {
	in.Code = strings.TrimSpace(in.Code)
	in.Role = strings.TrimSpace(in.Role)
	if in.Role == "" {
		in.Role = catalog.RoleGeneric
	}
	if in.Format == "" {
		in.Format = "markdown"
	}
	ev, ok := catalog.Lookup(in.Code)
	switch {
	case !ok:
		return ev, &ValidationError{"code", "unknown event"}
	case !catalog.IsRole(in.Role):
		return ev, &ValidationError{"role", "unknown role"}
	case !catalog.IsChannel(in.Channel):
		return ev, &ValidationError{"channel", "unknown channel"}
	case !i18n.IsSupported(in.Language):
		return ev, &ValidationError{"language", "unsupported language"}
	case in.Format != "markdown" && in.Format != "text":
		return ev, &ValidationError{"format", "must be markdown or text"}
	case strings.TrimSpace(in.Body) == "":
		return ev, &ValidationError{"body", "required"}
	case len(in.Subject) > 512:
		return ev, &ValidationError{"subject", "too long"}
	}
	if unknown := msgtemplate.Unknown(ev.Spec(), in.Subject, in.Body); len(unknown) > 0 {
		return ev, &ValidationError{"body", "unknown placeholders: " + strings.Join(unknown, ", ")}
	}
	return ev, nil
}

// UpsertTemplate validates placeholders against the catalog and stores a
// global (brand-less) template.
func (s *Service) UpsertTemplate(ctx context.Context, actorID int64, in model.TemplateInput) (model.Template, error) {
	q, err := s.admin()
	if err != nil {
		return model.Template{}, err
	}
	if _, err := s.validateTemplate(&in); err != nil {
		return model.Template{}, err
	}
	active := true
	if in.Active != nil {
		active = *in.Active
	}
	row, err := q.UpsertNotificationTemplate(ctx, db.UpsertNotificationTemplateParams{
		Code: in.Code, Role: in.Role, Channel: in.Channel, Language: in.Language,
		Subject: in.Subject, Body: in.Body, Format: in.Format, Active: active,
		UpdatedByUserID: pgtype.Int8{Int64: actorID, Valid: actorID > 0},
	})
	if err != nil {
		return model.Template{}, err
	}
	return projectTemplate(row), nil
}

// PreviewTemplate renders a draft with the event's sample values.
func (s *Service) PreviewTemplate(ctx context.Context, in model.TemplateInput) (model.Rendered, error) {
	ev, err := s.validateTemplate(&in)
	if err != nil {
		return model.Rendered{}, err
	}
	lang := i18n.Normalize(in.Language)
	vars := ev.SampleVars(string(lang))
	out := model.Rendered{
		Subject: msgtemplate.Render(in.Subject, vars), Body: msgtemplate.Render(in.Body, vars),
		Language: string(lang), Dir: i18n.Dir(lang),
	}
	if in.Channel == model.ChannelEmail {
		html, err := providers.EmailProvider{}.HTML(ctx, db.Notification{
			Title: out.Subject, Body: out.Body, Language: pgtype.Text{String: string(lang), Valid: true},
		})
		if err != nil {
			return model.Rendered{}, err
		}
		out.HTML = html
	} else {
		out.Body = msgtemplate.MarkdownToText(out.Body)
	}
	return out, nil
}

// DeliveryFilter narrows ListDeliveries.
type DeliveryFilter struct {
	Status, Channel, EventCode string
	EventID                    *uuid.UUID
	UserUUID                   *uuid.UUID
}

// ListDeliveries returns the delivery log, newest first.
func (s *Service) ListDeliveries(ctx context.Context, q apiquery.Query, f DeliveryFilter) (apiquery.Page[model.Delivery], error) {
	store, err := s.admin()
	if err != nil {
		return apiquery.Page[model.Delivery]{}, err
	}
	var userID pgtype.Int8
	if f.UserUUID != nil {
		id, err := s.ResolveUserID(ctx, *f.UserUUID)
		if errors.Is(err, ErrNotFound) {
			return apiquery.NewPage([]model.Delivery{}, 0, q.Limit, q.Offset), nil
		}
		if err != nil {
			return apiquery.Page[model.Delivery]{}, err
		}
		userID = pgtype.Int8{Int64: id, Valid: true}
	}
	var eventID pgtype.UUID
	if f.EventID != nil {
		eventID = pgtype.UUID{Bytes: *f.EventID, Valid: true}
	}
	rows, err := store.ListNotificationDeliveries(ctx, db.ListNotificationDeliveriesParams{
		Status: optionalText(f.Status), Channel: optionalText(f.Channel), EventCode: optionalText(f.EventCode),
		EventID: eventID, UserID: userID, LimitCount: q.Limit, OffsetCount: q.Offset,
	})
	if err != nil {
		return apiquery.Page[model.Delivery]{}, err
	}
	total, err := store.CountNotificationDeliveries(ctx, db.CountNotificationDeliveriesParams{
		Status: optionalText(f.Status), Channel: optionalText(f.Channel), EventCode: optionalText(f.EventCode),
		EventID: eventID, UserID: userID,
	})
	if err != nil {
		return apiquery.Page[model.Delivery]{}, err
	}
	items := make([]model.Delivery, 0, len(rows))
	for _, r := range rows {
		items = append(items, model.Delivery{
			UUID: r.Uuid, EventID: r.EventID, EventCode: r.EventCode, UserUUID: r.UserUuid, UserEmail: r.UserEmail,
			Channel: r.Channel, Role: r.Role.String, Language: r.Language.String, Status: r.Status,
			Provider: r.Provider.String, ProviderRef: r.ProviderRef.String, Error: r.Error.String,
			Attempts: r.Attempts, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time,
		})
	}
	return apiquery.NewPage(items, total, q.Limit, q.Offset), nil
}

// ChannelSettings returns the admin channel switches.
func (s *Service) ChannelSettings(ctx context.Context) ([]model.ChannelSetting, error) {
	rows, err := s.q.ListNotificationChannelSettings(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.ChannelSetting, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.ChannelSetting{Channel: r.Channel, Enabled: r.Enabled, UpdatedAt: r.UpdatedAt.Time})
	}
	return out, nil
}

// SetChannelEnabled switches a channel on or off for the whole platform.
func (s *Service) SetChannelEnabled(ctx context.Context, actorID int64, channel string, enabled bool) (model.ChannelSetting, error) {
	q, err := s.admin()
	if err != nil {
		return model.ChannelSetting{}, err
	}
	if !catalog.IsChannel(channel) {
		return model.ChannelSetting{}, fmt.Errorf("%w: unknown channel %q", ErrInvalidRequest, channel)
	}
	row, err := q.SetNotificationChannelEnabled(ctx, db.SetNotificationChannelEnabledParams{
		Channel: channel, Enabled: enabled, UpdatedByUserID: pgtype.Int8{Int64: actorID, Valid: actorID > 0},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ChannelSetting{}, ErrNotFound
	}
	if err != nil {
		return model.ChannelSetting{}, err
	}
	return model.ChannelSetting{Channel: row.Channel, Enabled: row.Enabled, UpdatedAt: row.UpdatedAt.Time}, nil
}
