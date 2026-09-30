package usecase

import (
	"context"
	"encoding/json"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

// VAPIDConfig holds Web Push VAPID keys.
type VAPIDConfig struct {
	PublicKey  string
	PrivateKey string
	Subject    string
}

// WithVAPID enables web push when both keys are set.
func (s *Service) WithVAPID(cfg VAPIDConfig) *Service {
	if cfg.PublicKey != "" && cfg.PrivateKey != "" {
		s.vapid = &cfg
	}
	return s
}

// VAPIDPublicKey returns the configured public key (may be empty).
func (s *Service) VAPIDPublicKey() string {
	if s == nil || s.vapid == nil {
		return ""
	}
	return s.vapid.PublicKey
}

// UpsertPushSubscription stores a browser push subscription for the user.
func (s *Service) UpsertPushSubscription(ctx context.Context, userID int64, endpoint, p256dh, auth string) (db.PushSubscription, error) {
	q, ok := s.q.(interface {
		UpsertPushSubscription(context.Context, db.UpsertPushSubscriptionParams) (db.PushSubscription, error)
	})
	if !ok {
		return db.PushSubscription{}, ErrInvalidRequest
	}
	return q.UpsertPushSubscription(ctx, db.UpsertPushSubscriptionParams{
		UserID:    userID,
		Endpoint:  endpoint,
		KeyP256dh: p256dh,
		KeyAuth:   auth,
	})
}

// DeletePushSubscription removes a subscription endpoint for the user.
func (s *Service) DeletePushSubscription(ctx context.Context, userID int64, endpoint string) error {
	q, ok := s.q.(interface {
		DeletePushSubscription(context.Context, db.DeletePushSubscriptionParams) error
	})
	if !ok {
		return ErrInvalidRequest
	}
	return q.DeletePushSubscription(ctx, db.DeletePushSubscriptionParams{
		UserID:   userID,
		Endpoint: endpoint,
	})
}

func (s *Service) pushBestEffort(ctx context.Context, userID int64, title, body string, data map[string]any) {
	if s.vapid == nil || userID == 0 {
		return
	}
	q, ok := s.q.(interface {
		GetNotificationPreferences(context.Context, int64) (db.NotificationPreference, error)
		ListPushSubscriptionsByUser(context.Context, int64) ([]db.PushSubscription, error)
	})
	if !ok {
		return
	}
	prefs, err := q.GetNotificationPreferences(ctx, userID)
	if err != nil || !prefs.PushEnabled {
		return
	}
	subs, err := q.ListPushSubscriptionsByUser(ctx, userID)
	if err != nil || len(subs) == 0 {
		return
	}
	if data == nil {
		data = map[string]any{}
	}
	payload, _ := json.Marshal(map[string]any{
		"title": title,
		"body":  body,
		"data":  data,
	})
	for _, sub := range subs {
		resp, pushErr := webpush.SendNotification(payload, &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys: webpush.Keys{
				P256dh: sub.KeyP256dh,
				Auth:   sub.KeyAuth,
			},
		}, &webpush.Options{
			VAPIDPublicKey:  s.vapid.PublicKey,
			VAPIDPrivateKey: s.vapid.PrivateKey,
			Subscriber:      s.vapid.Subject,
			TTL:             60,
		})
		if pushErr != nil {
			s.log.Error("webpush_failed", "endpoint", sub.Endpoint, "error", pushErr)
			continue
		}
		_ = resp.Body.Close()
	}
}
