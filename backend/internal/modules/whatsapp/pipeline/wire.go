package pipeline

import (
	"context"
	"log/slog"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	aimodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/model"
	airepo "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/repository"
	aiusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/usecase"
	legal "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/legal/usecase"
	wausecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
	"github.com/redis/go-redis/v9"
)

// WireDeps are the services the server and the worker hand to Wire.
// Redis, Notifier, Media and Downloader may be nil.
type WireDeps struct {
	Queries          *db.Queries
	AIStore          *airepo.Store
	Chat             *aiusecase.Chat
	Actions          *aiusecase.Actions
	Messaging        *wausecase.Messaging
	Provider         llm.Provider
	Models           llm.Models
	Access           AccessResolver
	Features         FeatureChecker
	Settings         Settings
	Redis            *redis.Client
	Env              string
	Notifier         wausecase.Notifier
	Media            llm.ObjectReader
	Downloader       whatsapp.MediaDownloader
	DefaultBrandSlug string
	Log              *slog.Logger
}

// Wire builds the pipeline with the identity resolver (Redis cache,
// language detection booked on the system pool) and the Redis run lock.
func Wire(d WireDeps) *Pipeline {
	var (
		cache  wausecase.IdentityCache = wausecase.NoIdentityCache{}
		locker Locker                  = &MemoryLocker{}
	)
	if d.Redis != nil {
		cache = wausecase.NewRedisIdentityCache(d.Redis, d.Env, d.Log)
		locker = NewRedisLocker(d.Redis, d.Env)
	}
	identity := wausecase.NewIdentityResolver(d.Queries, cache, d.Provider, d.Models, d.Log).
		WithUsageRecorder(LocaleUsage{Store: d.AIStore, Queries: d.Queries, DefaultBrandSlug: d.DefaultBrandSlug})
	return New(Deps{
		Queries: d.Queries, Agent: d.Chat, Actions: d.Actions, Identity: identity, Sender: d.Messaging,
		Consents: legal.New(d.Queries), Access: d.Access, Features: d.Features, Settings: d.Settings,
		Locker: locker, Notifier: d.Notifier, Media: d.Media, Downloader: d.Downloader,
		DefaultBrandSlug: d.DefaultBrandSlug, Log: d.Log,
	})
}

// LocaleUsage books a language detection call on the brand center's
// system pool (channel whatsapp, purpose locale).
type LocaleUsage struct {
	Store            *airepo.Store
	Queries          *db.Queries
	DefaultBrandSlug string
}

// RecordLocaleUsage implements wausecase.LocaleUsageRecorder.
func (u LocaleUsage) RecordLocaleUsage(ctx context.Context, conv db.Conversation, model string, usage llm.Usage) error {
	if u.Store == nil || usage == (llm.Usage{}) {
		return nil
	}
	var (
		brand db.Brand
		err   error
	)
	if conv.BrandID.Valid {
		brand, err = u.Queries.GetBrandByID(ctx, conv.BrandID.Int64)
	} else {
		brand, err = u.Queries.GetBrandBySlug(ctx, u.DefaultBrandSlug)
	}
	if err != nil {
		return err
	}
	center, err := u.Queries.GetBrandCenter(ctx, brand.ID)
	if err != nil {
		return err
	}
	if model == "" {
		model = "unknown"
	}
	_, _, err = u.Store.RecordUsage(ctx, airepo.Usage{
		OrganizationID: center.ID, BrandID: brand.ID, Pool: aimodel.PoolSystem,
		Channel: aimodel.UsageChannelWhatsApp, Purpose: aimodel.PurposeLocale, Model: truncate(model, 128),
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		CacheReadTokens: usage.CacheReadTokens, CacheWriteTokens: usage.CacheWriteTokens,
	})
	return err
}
