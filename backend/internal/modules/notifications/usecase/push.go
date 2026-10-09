package usecase

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/providers"
	"github.com/jackc/pgx/v5/pgtype"
)

// VAPIDConfig holds Web Push VAPID keys.
type VAPIDConfig struct {
	PublicKey  string
	PrivateKey string
	Subject    string
}

// WithVAPID enables the webpush channel when both keys are set.
func (s *Service) WithVAPID(cfg VAPIDConfig) *Service {
	if cfg.PublicKey == "" || cfg.PrivateKey == "" {
		return s
	}
	s.vapid = &cfg
	if store, ok := s.q.(providers.WebPushStore); ok {
		s.RegisterProvider(providers.WebPushProvider{
			Store: store,
			VAPID: providers.VAPID{PublicKey: cfg.PublicKey, PrivateKey: cfg.PrivateKey, Subject: cfg.Subject},
		})
	}
	return s
}

// RegisterProvider adds or replaces the driver of a channel (used for
// drivers built after the service, such as the WhatsApp provider).
func (s *Service) RegisterProvider(p providers.Provider) *Service {
	if p != nil {
		s.providers[p.Channel()] = p
	}
	return s
}

// Provider returns the driver registered for a channel (nil when none).
func (s *Service) Provider(channel string) providers.Provider {
	if s == nil {
		return nil
	}
	return s.providers[channel]
}

// Channels lists the channels with a registered driver, sorted.
func (s *Service) Channels() []string {
	if s == nil {
		return nil
	}
	out := make([]string, 0, len(s.providers))
	for ch := range s.providers {
		out = append(out, ch)
	}
	sort.Strings(out)
	return out
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

// PushDeviceInput registers an Expo push token of a mobile device.
type PushDeviceInput struct {
	DeviceID   string `json:"device_id"`
	Platform   string `json:"platform"`
	ExpoToken  string `json:"expo_token"`
	AppVersion string `json:"app_version"`
}

type deviceStore interface {
	UpsertDevicePushToken(ctx context.Context, arg db.UpsertDevicePushTokenParams) (db.DevicePushToken, error)
	RevokeDevicePushToken(ctx context.Context, arg db.RevokeDevicePushTokenParams) (int64, error)
}

func validExpoToken(t string) bool {
	return (strings.HasPrefix(t, "ExponentPushToken[") || strings.HasPrefix(t, "ExpoPushToken[")) &&
		strings.HasSuffix(t, "]") && len(t) <= 255
}

// RegisterPushDevice upserts the caller's Expo token (a token moves to the
// latest user that registers it and is un-revoked).
func (s *Service) RegisterPushDevice(ctx context.Context, userID int64, in PushDeviceInput) (model.PushDevice, error) {
	q, ok := s.q.(deviceStore)
	if !ok {
		return model.PushDevice{}, ErrInvalidRequest
	}
	in.ExpoToken = strings.TrimSpace(in.ExpoToken)
	in.DeviceID = strings.TrimSpace(in.DeviceID)
	in.Platform = strings.ToLower(strings.TrimSpace(in.Platform))
	switch {
	case !validExpoToken(in.ExpoToken):
		return model.PushDevice{}, fmt.Errorf("%w: expo_token is invalid", ErrInvalidRequest)
	case in.DeviceID == "" || len(in.DeviceID) > 128:
		return model.PushDevice{}, fmt.Errorf("%w: device_id is required", ErrInvalidRequest)
	case in.Platform != "ios" && in.Platform != "android":
		return model.PushDevice{}, fmt.Errorf("%w: platform must be ios or android", ErrInvalidRequest)
	case len(in.AppVersion) > 32:
		return model.PushDevice{}, fmt.Errorf("%w: app_version is too long", ErrInvalidRequest)
	}
	row, err := q.UpsertDevicePushToken(ctx, db.UpsertDevicePushTokenParams{
		UserID: userID, DeviceID: in.DeviceID, Platform: in.Platform, ExpoToken: in.ExpoToken,
		AppVersion: optionalText(in.AppVersion),
	})
	if err != nil {
		return model.PushDevice{}, err
	}
	return model.PushDevice{
		UUID: row.Uuid, DeviceID: row.DeviceID, Platform: row.Platform,
		AppVersion: row.AppVersion.String, LastSeenAt: row.LastSeenAt.Time,
	}, nil
}

type deviceRevoker interface {
	RevokeDevicePushTokensForDevice(ctx context.Context, arg db.RevokeDevicePushTokensForDeviceParams) (int64, error)
	RevokeAllDevicePushTokensForUser(ctx context.Context, userID int64) (int64, error)
}

// RevokePushDevicesForDevice revokes every Expo token of one device of the
// user (mobile sign-out, TEC-91).
func (s *Service) RevokePushDevicesForDevice(ctx context.Context, userID int64, deviceID string) error {
	q, ok := s.q.(deviceRevoker)
	if !ok || strings.TrimSpace(deviceID) == "" {
		return nil
	}
	_, err := q.RevokeDevicePushTokensForDevice(ctx, db.RevokeDevicePushTokensForDeviceParams{
		UserID: userID, DeviceID: strings.TrimSpace(deviceID),
	})
	return err
}

// RevokeAllPushDevices revokes every Expo token of the user (sign out
// everywhere, TEC-91).
func (s *Service) RevokeAllPushDevices(ctx context.Context, userID int64) error {
	q, ok := s.q.(deviceRevoker)
	if !ok {
		return nil
	}
	_, err := q.RevokeAllDevicePushTokensForUser(ctx, userID)
	return err
}

// RevokePushDevice revokes the caller's Expo token.
func (s *Service) RevokePushDevice(ctx context.Context, userID int64, token string) error {
	q, ok := s.q.(deviceStore)
	if !ok {
		return ErrInvalidRequest
	}
	_, err := q.RevokeDevicePushToken(ctx, db.RevokeDevicePushTokenParams{
		ExpoToken: strings.TrimSpace(token), UserID: pgtype.Int8{Int64: userID, Valid: true},
	})
	return err
}
