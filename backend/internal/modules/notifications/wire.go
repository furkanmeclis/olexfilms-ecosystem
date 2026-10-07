package notifications

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/providers"
	notifusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/mail"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sms"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/realtime"
)

// Deps are the drivers shared by the server and the worker.
type Deps struct {
	Config   config.Config
	Queries  *db.Queries
	Queue    notifusecase.Enqueuer // nil = deliver inline
	Realtime realtime.Publisher
	Mail     mail.Sender
	SMS      sms.Provider // nil = logging no-op driver
	Log      *slog.Logger
}

// NewService builds the notification center with its channel drivers:
// inapp (+ Centrifugo user:{uuid}), email (HTML), webpush (when VAPID keys
// are set), expo_push, sms (behind the admin switch) and a whatsapp
// placeholder until the WhatsApp provider is registered with
// RegisterProvider (it is built after this service).
func NewService(d Deps) *notifusecase.Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	smsDriver := d.SMS
	if smsDriver == nil {
		smsDriver = sms.Noop{Log: log}
	}
	notify := d.Config.Notify
	q := d.Queries
	provs := []providers.Provider{
		providers.InappProvider{Pub: d.Realtime, Log: log},
		providers.EmailProvider{Mail: d.Mail, Brand: emailBrand(q, notify)},
		providers.ExpoProvider{
			Store: q, URL: notify.ExpoPushURL, AccessToken: notify.ExpoAccessToken,
			HTTP: &http.Client{Timeout: 10 * time.Second},
		},
		providers.SMSProvider{SMS: smsDriver},
		providers.NoopProvider{Name: providers.ChannelWhatsApp, Log: log},
	}
	svc := notifusecase.New(q, d.Queue, provs, log)
	svc.WithVAPID(notifusecase.VAPIDConfig{
		PublicKey:  d.Config.VAPID.PublicKey,
		PrivateKey: d.Config.VAPID.PrivateKey,
		Subject:    d.Config.VAPID.Subject,
	})
	return svc
}

func emailBrand(q *db.Queries, cfg config.NotifyConfig) func(context.Context, int64) providers.EmailBrand {
	return func(ctx context.Context, id int64) providers.EmailBrand {
		out := providers.EmailBrand{Name: "Olex Films", LogoURL: cfg.EmailLogoURL, Color: cfg.EmailColor}
		if q == nil || id <= 0 {
			return out
		}
		if b, err := q.GetBrandByID(ctx, id); err == nil && b.Name != "" {
			out.Name = b.Name
		}
		return out
	}
}

// PushProviders returns the web push (nil without VAPID keys) and Expo
// providers configured like the notification center's, for senders that
// push outside a notification row (campaigns, TEC-407).
func PushProviders(cfg config.Config, q *db.Queries) (web, expo providers.Provider) {
	if cfg.VAPID.PublicKey != "" && cfg.VAPID.PrivateKey != "" {
		web = providers.WebPushProvider{
			Store: q,
			VAPID: providers.VAPID{PublicKey: cfg.VAPID.PublicKey, PrivateKey: cfg.VAPID.PrivateKey, Subject: cfg.VAPID.Subject},
		}
	}
	expo = providers.ExpoProvider{
		Store: q, URL: cfg.Notify.ExpoPushURL, AccessToken: cfg.Notify.ExpoAccessToken,
		HTTP: &http.Client{Timeout: 10 * time.Second},
	}
	return web, expo
}

// EmailBrandFunc is the e-mail frame lookup of the notification center.
func EmailBrandFunc(q *db.Queries, cfg config.Config) func(context.Context, int64) providers.EmailBrand {
	return emailBrand(q, cfg.Notify)
}
