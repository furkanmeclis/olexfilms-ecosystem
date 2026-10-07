package main

import (
	"log/slog"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	accountingposting "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	accountingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/usecase"
	airepo "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/repository"
	aitools "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	aiusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/usecase"
	appointmentsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/appointments/usecase"
	authrepo "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/repository"
	authusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/usecase"
	catalogusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/usecase"
	customersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	leadsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/leads/usecase"
	legalusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/legal/usecase"
	ordersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	portalvehiclesusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/portalvehicles/usecase"
	servicesusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	shorturlsmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/shorturls"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	tasksusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/tasks/usecase"
	warrantyusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	warrantyclaimsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty_claims/usecase"
	wapipeline "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/pipeline"
	whatsappusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/llm/anthropic"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// whatsAppAIDeps are the worker services the WhatsApp AI pipeline needs.
type whatsAppAIDeps struct {
	cfg        config.Config
	pool       *pgxpool.Pool
	queries    *db.Queries
	features   *features.Service
	activity   *activity.Recorder
	notifier   whatsappusecase.Notifier
	messaging  *whatsappusecase.Messaging
	downloader whatsapp.MediaDownloader
	store      storage.Driver
	rdb        *redis.Client
	log        *slog.Logger
}

// newWhatsAppAIPipeline builds the AI chat core for the worker (TEC-396):
// the same tool registry as the API server (panel read and write tools,
// customer and visitor tools) over worker-side use cases, the
// confirmation flow and the chat agent, then the WhatsApp pipeline.
// Keep the registrations in step with internal/httpserver/server.go.
func newWhatsAppAIPipeline(d whatsAppAIDeps) *wapipeline.Pipeline {
	cfg, pool, q, log := d.cfg, d.pool, d.queries, d.log
	box := outbox.NewStore(pool, q)
	sys := sysconfig.New(q, sysconfig.NoCache{})
	rates := fxrates.New(q, nil, log)
	poster := accountingposting.New(q, box, rates)
	geoSvc := geo.New(pool, q)

	orgSvc := orgusecase.New(pool, q)
	orgSvc.SetGeo(geoSvc)
	orgSvc.SetOutbox(box)
	customerPII, err := crypto.NewPIIBox(cfg.Encryption.CustomerPIIKey)
	if err != nil {
		customerPII = nil
	}
	customersSvc := customersusecase.New(pool, q, customerPII, geoSvc)
	customersSvc.SetPortalURL(cfg.Auth.FrontendURL)
	customersSvc.SetOutbox(box)
	accountingSvc := accountingusecase.New(pool, q, poster, d.features)
	ordersSvc := ordersusecase.New(pool, q, box, rates).WithReceiptHook(ordersusecase.NewAccountingBridge(poster))
	servicesSvc := servicesusecase.New(pool, q, box).
		WithContractRequirement(sys, d.features).
		WithCompletedCancelAccounting(poster)
	warrantyReader := warrantyusecase.NewReader(pool, q, box, cfg.Auth.FrontendURL)
	catalogSvc := catalogusecase.New(q, nil)
	stockSvc := stockusecase.New(q)
	tasksSvc := tasksusecase.New(pool, q, box)
	leadsSvc := leadsusecase.New(pool, q, tasksSvc)
	appointmentsSvc := appointmentsusecase.New(pool, q, box, servicesSvc)
	appointmentsSvc.SetFeatureChecker(d.features)
	authUC := authusecase.New(authrepo.NewPostgres(pool, q), nil)
	links := shorturlsmodule.NewLinker(q, cfg.Auth.FrontendURL)

	reg := aitools.NewRegistry(d.features).WithToggles(aitools.SettingsToggles{Q: q}).WithLogger(log)
	aitools.RegisterPanel(reg, aitools.Deps{
		Tree: q, Services: servicesSvc, Warranties: warrantyReader, Customers: customersSvc,
		Stock: stockSvc, Orders: ordersSvc, Accounting: accountingSvc, Appointments: appointmentsSvc,
		Leads: leadsSvc, Tasks: tasksSvc, Catalog: catalogSvc, Organizations: orgSvc, Links: links,
	})
	aitools.RegisterPanelWrite(reg, aitools.WriteDeps{
		Tree: q, Tasks: tasksSvc, Leads: leadsSvc, Appointments: appointmentsSvc,
		Customers: customersSvc, Orders: ordersSvc, Products: catalogSvc, Services: servicesSvc,
	})
	aitools.RegisterCustomer(reg, aitools.CustomerDeps{
		Portal: portalvehiclesusecase.New(q), Services: servicesSvc, Warranties: warrantyReader,
		Appointments: appointmentsSvc, Claims: warrantyclaimsusecase.New(pool, q, d.store, box),
		Profile: authUC, Links: links,
	})
	aitools.RegisterVisitor(reg, aitools.VisitorDeps{
		Catalog: catalogSvc, Dealers: orgSvc, Settings: q,
		Warranties: warrantyusecase.NewPublicLookup(q), FrontendURL: cfg.Auth.FrontendURL,
	})

	store := airepo.New(pool)
	actions := aiusecase.NewActions(store, reg, d.activity, log)
	provider := anthropic.NewFromConfig(cfg.AI)
	models := llm.ModelsFromConfig(cfg.AI)
	chat := aiusecase.NewChat(aiusecase.ChatDeps{
		Store: store, Tools: reg, Actions: actions, Provider: provider, Models: models,
		Features: d.features, Consents: legalusecase.New(q), Outbox: box, Log: log,
	})
	var media llm.ObjectReader
	if d.store != nil {
		media = d.store
	}
	return wapipeline.Wire(wapipeline.WireDeps{
		Queries: q, AIStore: store, Chat: chat, Actions: actions, Messaging: d.messaging,
		Provider: provider, Models: models, Access: authUC, Features: d.features, Settings: sys,
		Redis: d.rdb, Env: cfg.App.Env, Notifier: d.notifier, Media: media, Downloader: d.downloader,
		DefaultBrandSlug: cfg.App.DefaultBrandSlug, Log: log,
	})
}
