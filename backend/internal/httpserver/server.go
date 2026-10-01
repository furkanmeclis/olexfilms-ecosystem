package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/cache"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/errtrack"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	accessmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/access"
	accesshandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/access/handler"
	accountinghandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/handler"
	accountingposting "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	accountingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/usecase"
	activitymodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/activity"
	activityhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/activity/handler"
	activityusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/activity/usecase"
	authmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth"
	authhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/identity"
	authrepo "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/repository"
	authusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/auth/usecase"
	authsettingsmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/authsettings"
	authsettingshandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/authsettings/handler"
	authsettingsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/authsettings/usecase"
	bulkmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/bulk"
	bulkhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/bulk/handler"
	bulkusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/bulk/usecase"
	catalogmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog"
	cataloghandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/handler"
	catalogusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/catalog/usecase"
	customershandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/handler"
	customersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	documentsmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents"
	dochandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/handler"
	docusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/documents/usecase"
	exportmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports"
	exporthandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/handler"
	exportusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/exports/usecase"
	featuremodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/features"
	featurehandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/features/handler"
	geomodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/geo"
	geohandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/geo/handler"
	importmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/imports"
	importhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/imports/handler"
	importusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/imports/usecase"
	githubmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/github"
	githubhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/github/handler"
	githubusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/github/usecase"
	oauthprovidermodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/oauthprovider"
	oauthproviderhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/oauthprovider/handler"
	oauthproviderusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/oauthprovider/usecase"
	legalmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/legal"
	legalhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/legal/handler"
	legalusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/legal/usecase"
	logsmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/logs"
	logshandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/logs/handler"
	logsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/logs/usecase"
	notifmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications"
	notifhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/providers"
	notifusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/usecase"
	orgmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	pricingmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing"
	pricinghandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/handler"
	pricingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	ratesmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/rates"
	rateshandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/rates/handler"
	searchmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search"
	searchhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search/handler"
	searchusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search/usecase"
	settingsmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/settings"
	settingshandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/settings/handler"
	settingsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/settings/usecase"
	stockmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock"
	stockhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/handler"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	storagemodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/storage"
	storagehandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/storage/handler"
	storageusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/storage/usecase"
	vehiclecatalogmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/vehiclecatalog"
	vehiclecataloghandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/vehiclecatalog/handler"
	vehiclecatalogusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/vehiclecatalog/usecase"
	whatsappmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp"
	whatsapphandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/whatsapp/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/activity"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authrevoke"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine"
	bulkadapters "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/bulkengine/adapters"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/geo"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	ioadapters "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine/adapters"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/mail"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/otp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/pdfrender"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ratelimit"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine"
	searchadapters "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/searchengine/adapters"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sms"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/stepup"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/queue"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/realtime"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Deps holds infrastructure clients wired into the HTTP server.
type Deps struct {
	DB       *pgxpool.Pool
	Queries  *db.Queries
	Redis    *redis.Client
	Queue    *queue.Client
	Storage  storage.Driver
	Realtime realtime.Publisher
	Worker   *queue.Worker
	Events   events.Bus
}

// Server is the HTTP composition root for infrastructure routes.
type Server struct {
	cfg           config.Config
	log           *slog.Logger
	db            *pgxpool.Pool
	queries       *db.Queries
	redis         *redis.Client
	queueClient   *queue.Client
	storage       storage.Driver
	realtime      realtime.Publisher
	worker        *queue.Worker
	events        events.Bus
	outboxPub     *outbox.Publisher
	outboxStop    func()
	searchClient  *searchengine.Client
	searchIndexer *searchengine.Indexer
	githubSvc     *githubusecase.Service
	oauthProvSvc  *oauthproviderusecase.Service
	authSettings  *authsettingsusecase.Service
	documents     *docusecase.Service
	http          *http.Server
	// Kept for integration tests that mount placeholder routes behind the
	// real auth chain.
	mux    *http.ServeMux
	tokens *jwt.Manager
	loader middleware.IdentityLoader
	stepUp *stepup.Service
	// features resolves module flags (TEC-86).
	features *features.Service
}

// New wires router and middleware for the API skeleton.
func New(cfg config.Config, log *slog.Logger, deps Deps) (*Server, error) {
	mux := http.NewServeMux()
	eventBus := deps.Events
	if eventBus == nil {
		eventBus = events.NewBus(log)
	}

	s := &Server{
		cfg:         cfg,
		log:         log,
		db:          deps.DB,
		queries:     deps.Queries,
		redis:       deps.Redis,
		queueClient: deps.Queue,
		storage:     deps.Storage,
		realtime:    deps.Realtime,
		worker:      deps.Worker,
		events:      eventBus,
	}

	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.HandleFunc("GET /v1/app/config", s.handleAppConfig)
	s.mountDocs(mux)

	tokens, err := jwt.NewManager(cfg.JWT.AccessSecret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	if err != nil {
		return nil, fmt.Errorf("httpserver: jwt: %w", err)
	}

	// Pass a nil interface (not a typed-nil *queue.Client) when the queue is
	// disabled so notifications are delivered inline.
	var notifQueue notifusecase.Enqueuer
	if deps.Queue != nil {
		notifQueue = deps.Queue
	}
	notifSvc := notifmodule.NewService(notifmodule.Deps{
		Config: cfg, Queries: deps.Queries, Queue: notifQueue, Realtime: deps.Realtime,
		Mail: mail.NewSMTPSender(cfg.SMTP), SMS: sms.Noop{Log: log}, Log: log,
	})
	notifSvc.WithActionSigner(cfg.JWT.AccessSecret, 0)

	repo := authrepo.NewPostgres(deps.DB, deps.Queries)
	uc := authusecase.New(repo, tokens)
	uc.SetNotifier(notifSvc)
	uc.SetLogger(log)
	if deps.Redis != nil {
		uc.SetRevocations(authrevoke.New(deps.Redis, cfg.App.Env, cfg.JWT.AccessTTL))
	}

	searchReg := searchengine.NewRegistry(
		searchadapters.NewUsers(deps.Queries),
		searchadapters.NewRoles(deps.Queries),
		catalogusecase.NewSearchAdapter(deps.Queries),
	)
	searchClient := searchengine.NewClient(cfg.Search, log)
	searchIndexer := searchengine.NewIndexer(searchClient, searchReg, deps.Queue, log)
	s.searchClient = searchClient
	s.searchIndexer = searchIndexer
	uc.SetSearchIndexer(searchIndexer)

	rtIssuer, err := realtime.NewTokenIssuer(cfg.Centrifugo)
	if err != nil {
		return nil, fmt.Errorf("httpserver: realtime tokens: %w", err)
	}
	uc.SetRealtimeHints(authusecase.RealtimeHints{
		Enabled: rtIssuer.Enabled(),
		WSURL:   cfg.Centrifugo.WSURL,
	})

	activityRec := activity.NewRecorder(deps.Queries, log)

	stepUpStore := stepup.NewStore(deps.Redis, cfg.App.Env)
	var stepUpWebAuthn *stepup.WebAuthn
	stepUpOrigins := cfg.CORS.AllowedOrigins
	if len(stepUpOrigins) == 0 {
		stepUpOrigins = []string{strings.TrimRight(cfg.Auth.FrontendURL, "/")}
	}
	if webauthn, err := stepup.NewWebAuthn(stepup.WebAuthnConfig{
		RPID:          cfg.Auth.WebAuthnRPID,
		RPDisplayName: cfg.App.Name,
		RPOrigins:     stepUpOrigins,
	}); err == nil {
		stepUpWebAuthn = webauthn
	} else {
		log.Warn("stepup_webauthn_disabled", "error", err)
	}
	secretBox, err := crypto.NewSecretBox(cfg.Encryption.Key)
	if err != nil {
		return nil, fmt.Errorf("httpserver: encryption: %w", err)
	}
	stepUpSvc := stepup.NewService(deps.Queries, stepUpStore, stepUpWebAuthn, repo)
	stepUpSvc.SetSecretBox(secretBox)

	githubSvc := githubusecase.New(deps.Queries, secretBox)
	oauthProvSvc := oauthproviderusecase.New(deps.Queries, secretBox)
	authSettingsSvc := authsettingsusecase.New(deps.Queries)
	uc.SetAuthSettings(authSettingsSvc)
	uc.SetAccessPolicy(stepUpSvc)
	uc.SetSecretBox(secretBox, cfg.App.Name)
	oauthUC := authusecase.NewOAuth(repo, secretBox)
	s.githubSvc = githubSvc
	s.oauthProvSvc = oauthProvSvc
	s.authSettings = authSettingsSvc

	h := authhandler.New(uc, oauthUC, githubSvc, oauthProvSvc, authSettingsSvc, cfg.Auth.AdapterSecret, stepUpSvc, activityRec)
	h.SetRateLimiter(ratelimit.New(deps.Redis, cfg.App.Env))

	// WhatsApp gateway (wuzapi) + phone OTP (TEC-92). The OTP message is
	// critical: it is sent synchronously through the provider, not queued.
	waSvc := whatsappmodule.NewService(cfg.Wuzapi, deps.DB, deps.Queries, secretBox, notifSvc, log)
	notifSvc.RegisterProvider(providers.WhatsAppProvider{WA: waSvc.Provider()})
	otpSvc := otp.New(
		otp.NewPGStore(deps.DB, deps.Queries),
		&whatsapp.Sender{WhatsApp: waSvc.Provider(), SMS: sms.Noop{Log: log}, SMSFallback: waSvc.SMSFallbackEnabled},
		ratelimit.New(deps.Redis, cfg.App.Env),
		otp.Config{Key: otp.DeriveKey(cfg.Encryption.Key)},
		log,
	)
	h.SetOTP(otpSvc)
	uc.SetPhoneRepository(repo)
	loader := identity.Loader{UC: uc}
	s.mux, s.tokens, s.loader, s.stepUp = mux, tokens, loader, stepUpSvc
	orgSvc := orgusecase.New(deps.DB, deps.Queries)
	geoSvc := geo.New(deps.DB, deps.Queries)
	orgSvc.SetGeo(geoSvc)
	ratesSvc := fxrates.New(deps.Queries, fxrates.NewFetcher(cfg.Rates.TCMBURL, cfg.Rates.ECBURL), log)
	uc.SetOrganizationResolver(orgSvc)
	authmodule.RegisterRoutes(mux, h, tokens, loader, stepUpSvc)
	// TEC-91: mobile API (Bearer, aud=mobile) and QR web sign-in.
	uc.SetMobileRefreshTTL(cfg.JWT.MobileRefreshTTL)
	if deps.Queries != nil {
		var qrPublisher authusecase.QRPublisher
		if deps.Realtime != nil {
			qrPublisher = deps.Realtime
		}
		uc.SetQRLogin(deps.Queries, qrPublisher, rtIssuer, cfg.Mobile.QRLoginTTL)
	}
	authmodule.RegisterMobileRoutes(mux,
		authhandler.NewMobile(uc, notifSvc, ratelimit.New(deps.Redis, cfg.App.Env)),
		tokens, loader, cfg.Mobile.MinAPIVersion, cfg.Mobile.MaxAPIVersion)
	var featureCache features.Cache = features.NoCache{}
	if deps.Redis != nil {
		featureCache = features.NewRedisCache(deps.Redis, cfg.App.Env, func(op string, err error) {
			log.Warn("feature_cache_error", "op", op, "error", err)
		})
	}
	featureSvc := features.New(deps.DB, deps.Queries, featureCache, log)
	s.features = featureSvc
	orgmodule.RegisterRoutes(mux, orgSvc, uc, deps.Storage, tokens, loader, deps.Queries, ratelimit.New(deps.Redis, cfg.App.Env), stepUpSvc, featureSvc)
	featuremodule.RegisterRoutes(mux, featurehandler.New(featureSvc, deps.Queries, notifSvc, activityRec, log), featureSvc, tokens, loader, deps.Queries)
	geomodule.RegisterRoutes(mux, geohandler.New(geoSvc, deps.Queries, activityRec), tokens, loader)
	// TEC-160: customers (one phone = one user), vehicles, upgrade to dealer.
	// Without CUSTOMER_PII_KEY (development only, config enforces it
	// elsewhere) identity numbers are refused instead of stored in clear.
	customerPII, piiErr := crypto.NewPIIBox(cfg.Encryption.CustomerPIIKey)
	if piiErr != nil {
		customerPII = nil
		log.Warn("customer_pii_disabled", "error", piiErr)
	}
	customershandler.RegisterRoutes(mux,
		customershandler.New(customersusecase.New(deps.DB, deps.Queries, customerPII, geoSvc), activityRec),
		tokens, loader, deps.Queries, featureSvc)
	ratesmodule.RegisterRoutes(mux, rateshandler.New(ratesSvc, activityRec), tokens, loader)
	// TEC-146: price list and effective price views (K8).
	pricingmodule.RegisterRoutes(mux, pricinghandler.New(pricingusecase.New(deps.Queries), activityRec),
		tokens, loader, deps.Queries, stepUpSvc, featureSvc)
	// TEC-172: accounting accounts, cari, manual entries and settlements.
	accountingPoster := accountingposting.New(deps.Queries, outbox.NewStore(deps.DB, deps.Queries), ratesSvc)
	accountingSvc := accountingusecase.New(deps.DB, deps.Queries, accountingPoster, featureSvc)
	accountingH := accountinghandler.New(accountingSvc)
	accountinghandler.RegisterRoutes(mux, accountingH, tokens, loader, deps.Queries, stepUpSvc, featureSvc)
	pdfClient := pdfrender.NewWithOptions(cfg.Gotenberg.URL, pdfrender.Options{MaxConnsPerHost: cfg.Queue.Concurrency})
	realtime.RegisterRoutes(mux, realtime.NewHandler(rtIssuer, uc), tokens, loader)

	nh := notifhandler.New(notifSvc)
	notifmodule.RegisterRoutes(mux, nh, tokens, loader)
	notifmodule.RegisterEventHandlers(eventBus, notifSvc, log)

	// TEC-145: product catalog (brand scoped, center writes).
	catalogSvc := catalogusecase.New(deps.Queries, searchIndexer)
	ioReg := ioengine.NewRegistry(
		catalogusecase.NewIOAdapter(catalogSvc, deps.Queries),
		ioadapters.NewUsers(deps.Queries),
		ioadapters.NewRoles(deps.Queries),
		ioadapters.NewNotifications(deps.Queries),
		ioadapters.NewActivity(deps.Queries),
		// TEC-175: cari statement and balance report exports.
		accountingusecase.NewStatementAdapter(accountingSvc),
		accountingusecase.NewBalancesAdapter(accountingSvc),
	)
	exportSvc := exportusecase.New(deps.Queries, deps.Storage, ioReg, deps.Queue, notifSvc, activityRec, log)
	exportSvc.SetDocumentPDF(pdfClient)
	accountingH.WithExports(exportSvc)
	importSvc := importusecase.New(deps.Queries, deps.Storage, ioReg, deps.Queue, notifSvc, activityRec, log)
	bulkReg := bulkengine.NewRegistry(
		bulkadapters.NewUsers(deps.Queries),
		bulkadapters.NewRoles(deps.Queries),
	)
	bulkSvc := bulkusecase.New(deps.Queries, bulkReg, deps.Queue, notifSvc, activityRec, cfg.Bulk, log)
	logsSvc := logsusecase.New(deps.Queries)
	if s.worker != nil {
		s.worker.WithExport(exportSvc.ProcessExport).
			WithImport(importSvc.ProcessImport).
			WithBulk(bulkSvc.ProcessBulk).
			WithLogPurge(logsSvc.ApplyDueRules).
			WithRatesFetch(ratesSvc.FetchTask).
			WithNotificationPurge(notifSvc.PurgeExpired).
			WithWhatsAppPoll(waSvc.PollStatus)
		if searchIndexer != nil {
			s.worker.WithSearch(
				searchIndexer.ProcessUpsert,
				searchIndexer.ProcessDelete,
				searchIndexer.ProcessReindex,
			)
		}
	}
	var docQueue docusecase.Enqueuer
	if deps.Queue != nil {
		docQueue = deps.Queue
	}
	docSvc := docusecase.New(deps.DB, deps.Queries, deps.Storage, pdfClient, docQueue, pdfrender.ParseFontMode(cfg.Gotenberg.Fonts), log)
	s.documents = docSvc
	if s.worker != nil {
		s.worker.WithDocsRender(docSvc.ProcessRender)
	}
	documentsmodule.RegisterRoutes(mux, dochandler.New(docSvc, ratelimit.New(deps.Redis, cfg.App.Env)), tokens, loader, deps.Queries)
	exportmodule.RegisterRoutes(mux, exporthandler.New(exportSvc), tokens, loader, stepUpSvc, deps.Queries)
	importmodule.RegisterRoutes(mux, importhandler.New(importSvc), tokens, loader, deps.Queries)
	bulkmodule.RegisterRoutes(mux, bulkhandler.New(bulkSvc), tokens, loader)
	catalogmodule.RegisterRoutes(mux, cataloghandler.New(catalogSvc, exportSvc, importSvc, deps.Storage, activityRec), featureSvc, tokens, loader, deps.Queries)
	// TEC-155: stock read API (barcode history, organization/bin stock).
	stockmodule.RegisterRoutes(mux, stockhandler.New(stockusecase.New(deps.Queries)), featureSvc, tokens, loader, deps.Queries)
	// TEC-149: vehicle catalog (global car brands/models, super_admin writes).
	vehiclecatalogmodule.RegisterRoutes(mux, vehiclecataloghandler.New(
		vehiclecatalogusecase.New(deps.Queries), deps.Storage, activityRec), tokens, loader)
	settingsmodule.RegisterRoutes(mux, settingshandler.New(settingsusecase.New(deps.Queries), deps.Storage), tokens, loader)
	accessmodule.RegisterRoutes(mux, accesshandler.New(stepUpSvc, activityRec), tokens, loader)
	authsettingsmodule.RegisterRoutes(mux, authsettingshandler.New(authSettingsSvc, activityRec), tokens, loader)
	githubmodule.RegisterRoutes(mux, githubhandler.New(githubSvc, activityRec), tokens, loader)
	whatsappmodule.RegisterRoutes(mux, whatsapphandler.New(waSvc, activityRec, log), tokens, loader)
	legalmodule.RegisterRoutes(mux, legalhandler.New(legalusecase.New(deps.Queries), activityRec), tokens, loader)
	oauthprovidermodule.RegisterRoutes(mux, oauthproviderhandler.New(oauthProvSvc, activityRec), tokens, loader)
	activitymodule.RegisterRoutes(mux, activityhandler.New(activityusecase.New(deps.Queries)), tokens, loader)
	logsmodule.RegisterRoutes(mux, logshandler.New(logsSvc), tokens, loader)
	searchSvc := searchusecase.New(searchClient, searchReg, deps.Queries, log)
	searchmodule.RegisterRoutes(mux, searchhandler.New(searchSvc), tokens, loader)
	storageSvc := storageusecase.New(deps.Storage, deps.Queries, log)
	storagemodule.RegisterRoutes(
		mux,
		storagehandler.New(storageSvc),
		tokens,
		loader,
	)

	var brandResolver *brandctx.Resolver
	if deps.Queries != nil {
		brandResolver = brandctx.NewResolver(brandctx.DBLoader(deps.Queries), cfg.App.DefaultBrandSlug, time.Minute)
	}

	outboxStore := outbox.NewStore(deps.DB, deps.Queries)
	outboxPub := outbox.NewPublisher(outboxStore, eventBus, log)
	s.outboxPub = outboxPub

	s.http = &http.Server{
		Addr:         cfg.HTTP.Addr,
		Handler:      middleware.ServerErrors(log)(middleware.RequestID(errtrack.Middleware(errtrack.Recover(log)(middleware.ResolveBrand(brandResolver)(middleware.ResolveLocale(mux)))))),
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
	}
	return s, nil
}

// Start listens until the server stops. Optionally starts an in-process queue worker.
func (s *Server) Start() error {
	if s.outboxPub != nil {
		s.outboxStop = s.outboxPub.StartRun(context.Background())
	}
	if s.worker != nil {
		go func() {
			if err := s.worker.Start(); err != nil {
				s.log.Error("queue_worker_failed", "error", err)
			}
		}()
	}
	if s.searchIndexer != nil {
		go s.searchIndexer.Bootstrap(context.Background())
	}
	if s.features != nil && s.queries != nil {
		// Catalog sync: modules added to the Go catalog after the last
		// migration get their rows (level/sort follow the catalog).
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := s.features.SyncCatalog(ctx); err != nil {
			s.log.Warn("feature_catalog_sync_failed", "error", err)
		}
		if err := notifusecase.SyncCatalog(ctx, s.queries); err != nil {
			s.log.Warn("notification_catalog_sync_failed", "error", err)
		}
		cancel()
	}
	s.log.Info("http_listen", "addr", s.cfg.HTTP.Addr, "env", s.cfg.App.Env)
	if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("httpserver: listen: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the HTTP server, queue worker, and queue client.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.outboxStop != nil {
		s.outboxStop()
	}
	if s.worker != nil {
		s.worker.Shutdown()
	}
	if s.queueClient != nil {
		_ = s.queueClient.Close()
	}
	return s.http.Shutdown(ctx)
}

func (s *Server) handleAppConfig(w http.ResponseWriter, r *http.Request) {
	settingsSvc := settingsusecase.New(s.queries)
	letterhead, _ := settingsSvc.PublicLetterhead(r.Context())

	registrationEnabled := false
	passwordLogin := true
	passwordRegister := false
	passkeyLogin := true
	if s.authSettings != nil {
		if p, err := s.authSettings.Policy(r.Context()); err == nil {
			registrationEnabled = p.RegistrationEnabled
			passwordLogin = p.PasswordLoginEnabled
			passwordRegister = p.RegistrationEnabled && p.PasswordRegisterEnabled
			passkeyLogin = p.PasskeyLoginEnabled
		}
	}

	githubLogin, githubRegister := false, false
	if s.githubSvc != nil {
		githubLogin, _ = s.githubSvc.IsAuthEnabled(r.Context())
		reg, _ := s.githubSvc.IsRegisterEnabled(r.Context())
		githubRegister = registrationEnabled && reg
	}

	method := func(login, register bool) map[string]bool {
		return map[string]bool{"login": login, "register": register}
	}
	oauthMethod := func(provider string) map[string]bool {
		login, register := false, false
		if s.oauthProvSvc != nil {
			login, _ = s.oauthProvSvc.IsLoginEnabled(r.Context(), provider)
			reg, _ := s.oauthProvSvc.IsRegisterEnabled(r.Context(), provider)
			register = registrationEnabled && reg
		}
		return method(login, register)
	}

	responseJSON := map[string]any{
		"mode":                 "platform",
		"name":                 s.cfg.App.Name,
		"vapid_configured":     s.cfg.VAPID.PublicKey != "" && s.cfg.VAPID.PrivateKey != "",
		"github_auth_enabled":  githubLogin, // legacy
		"registration_enabled": registrationEnabled,
		"auth_methods": map[string]any{
			"password": method(passwordLogin, passwordRegister),
			"passkey":  map[string]bool{"login": passkeyLogin},
			"github":   method(githubLogin, githubRegister),
			"google":   oauthMethod("google"),
			"facebook": oauthMethod("facebook"),
			"apple":    oauthMethod("apple"),
		},
		"letterhead": letterhead,
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"data":    responseJSON,
	})
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	checks := map[string]string{
		"postgres": "ok",
		"redis":    "ok",
	}
	ready := true

	if err := database.Ping(ctx, s.db); err != nil {
		checks["postgres"] = "down"
		ready = false
	}
	if err := cache.Ping(ctx, s.redis); err != nil {
		checks["redis"] = "down"
		ready = false
	}

	if s.storage != nil {
		if err := s.storage.Ping(ctx); err != nil {
			s.log.Warn("readyz_storage_soft_check", "error", err)
		}
	}
	if s.searchClient != nil && s.searchClient.Enabled() {
		if err := s.searchClient.Ping(ctx); err != nil {
			checks["meilisearch"] = "down"
			s.log.Warn("readyz_meilisearch_soft_check", "error", err)
		} else {
			checks["meilisearch"] = "ok"
		}
	}

	status := http.StatusOK
	state := "ready"
	if !ready {
		status = http.StatusServiceUnavailable
		state = "not_ready"
	}

	writeJSON(w, status, map[string]any{
		"status": state,
		"checks": checks,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// Documents exposes the documents use case so business modules can register
// their SourceLoader per document kind.
func (s *Server) Documents() *docusecase.Service { return s.documents }
