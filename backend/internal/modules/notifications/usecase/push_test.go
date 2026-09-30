package usecase

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type pushTestQuerier struct {
	prefs db.NotificationPreference
	subs  []db.PushSubscription
	err   error
	list  int
}

func (m *pushTestQuerier) GetNotificationPreferences(_ context.Context, userID int64) (db.NotificationPreference, error) {
	if m.err != nil {
		return db.NotificationPreference{}, m.err
	}
	p := m.prefs
	p.UserID = userID
	return p, nil
}

func (m *pushTestQuerier) ListPushSubscriptionsByUser(_ context.Context, _ int64) ([]db.PushSubscription, error) {
	m.list++
	if m.err != nil {
		return nil, m.err
	}
	return m.subs, nil
}

func (m *pushTestQuerier) CreateNotification(context.Context, db.CreateNotificationParams) (db.Notification, error) {
	return db.Notification{}, nil
}

func (m *pushTestQuerier) GetNotificationByUUID(context.Context, uuid.UUID) (db.Notification, error) {
	return db.Notification{}, nil
}

func (m *pushTestQuerier) GetNotificationByID(context.Context, int64) (db.Notification, error) {
	return db.Notification{}, nil
}

func (m *pushTestQuerier) ListNotificationsForUser(context.Context, db.ListNotificationsForUserParams) ([]db.Notification, error) {
	return nil, nil
}

func (m *pushTestQuerier) CountNotificationsForUser(context.Context, db.CountNotificationsForUserParams) (int64, error) {
	return 0, nil
}

func (m *pushTestQuerier) CountUnreadInappForUser(context.Context, pgtype.Int8) (int64, error) {
	return 0, nil
}

func (m *pushTestQuerier) ListPlatformNotifications(context.Context, db.ListPlatformNotificationsParams) ([]db.Notification, error) {
	return nil, nil
}

func (m *pushTestQuerier) CountPlatformNotifications(context.Context, db.CountPlatformNotificationsParams) (int64, error) {
	return 0, nil
}

func (m *pushTestQuerier) MarkNotificationProcessing(context.Context, int64) (db.Notification, error) {
	return db.Notification{}, nil
}

func (m *pushTestQuerier) ListStuckProcessingNotificationIDs(context.Context, int32) ([]int64, error) {
	return nil, nil
}

func (m *pushTestQuerier) MarkNotificationSent(context.Context, db.MarkNotificationSentParams) (db.Notification, error) {
	return db.Notification{}, nil
}

func (m *pushTestQuerier) MarkNotificationFailed(context.Context, db.MarkNotificationFailedParams) (db.Notification, error) {
	return db.Notification{}, nil
}

func (m *pushTestQuerier) MarkNotificationRead(context.Context, db.MarkNotificationReadParams) (db.Notification, error) {
	return db.Notification{}, nil
}

func (m *pushTestQuerier) MarkAllNotificationsReadForUser(context.Context, pgtype.Int8) (int64, error) {
	return 0, nil
}

func (m *pushTestQuerier) InsertNotificationHistory(context.Context, db.InsertNotificationHistoryParams) (db.NotificationHistory, error) {
	return db.NotificationHistory{}, nil
}

func (m *pushTestQuerier) GetTemplateByCodeChannelLang(context.Context, db.GetTemplateByCodeChannelLangParams) (db.NotificationTemplate, error) {
	return db.NotificationTemplate{}, nil
}

func (m *pushTestQuerier) UpsertNotificationPreferences(context.Context, db.UpsertNotificationPreferencesParams) (db.NotificationPreference, error) {
	return db.NotificationPreference{}, nil
}

func (m *pushTestQuerier) GetUserByID(context.Context, int64) (db.User, error) {
	return db.User{}, nil
}

func (m *pushTestQuerier) GetUserByUUID(context.Context, uuid.UUID) (db.User, error) {
	return db.User{}, nil
}

var _ Querier = (*pushTestQuerier)(nil)

func TestPushBestEffortRespectsPushEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		pushEnabled bool
		wantList    int
	}{
		{
			name:        "disabled skips subscription lookup",
			pushEnabled: false,
			wantList:    0,
		},
		{
			name:        "enabled attempts subscription lookup",
			pushEnabled: true,
			wantList:    1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := &pushTestQuerier{
				prefs: db.NotificationPreference{PushEnabled: tc.pushEnabled},
				subs:  nil,
			}
			svc := &Service{
				q:   q,
				log: slog.Default(),
				vapid: &VAPIDConfig{
					PublicKey:  "pub",
					PrivateKey: "priv",
					Subject:    "mailto:test@example.com",
				},
			}

			svc.pushBestEffort(context.Background(), 42, "Title", "Body", map[string]any{
				"notification_uuid": uuid.NewString(),
				"action_url":        "/platform/notifications",
			})

			if q.list != tc.wantList {
				t.Fatalf("ListPushSubscriptionsByUser calls = %d, want %d", q.list, tc.wantList)
			}
		})
	}
}

func TestPushBestEffortMissingPreferences(t *testing.T) {
	t.Parallel()

	q := &pushTestQuerier{err: pgx.ErrNoRows}
	svc := &Service{
		q: q,
		vapid: &VAPIDConfig{
			PublicKey:  "pub",
			PrivateKey: "priv",
			Subject:    "mailto:test@example.com",
		},
	}

	svc.pushBestEffort(context.Background(), 42, "Title", "Body", nil)

	if q.list != 0 {
		t.Fatalf("expected no subscription lookup when preferences missing, got %d", q.list)
	}
}

func TestPushBestEffortNoVAPID(t *testing.T) {
	t.Parallel()

	q := &pushTestQuerier{prefs: db.NotificationPreference{PushEnabled: true}}
	svc := &Service{q: q}

	svc.pushBestEffort(context.Background(), 42, "Title", "Body", nil)

	if q.list != 0 {
		t.Fatalf("expected no subscription lookup without VAPID, got %d", q.list)
	}
}

func TestPushBestEffortPreferencesError(t *testing.T) {
	t.Parallel()

	q := &pushTestQuerier{err: errors.New("db down")}
	svc := &Service{
		q: q,
		vapid: &VAPIDConfig{
			PublicKey:  "pub",
			PrivateKey: "priv",
			Subject:    "mailto:test@example.com",
		},
	}

	svc.pushBestEffort(context.Background(), 42, "Title", "Body", nil)

	if q.list != 0 {
		t.Fatalf("expected no subscription lookup on preferences error, got %d", q.list)
	}
}
