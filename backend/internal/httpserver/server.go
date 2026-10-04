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
	contractsmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts"
	contractshandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/handler"
	contractsrepo "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/repository"
	contractsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/contracts/usecase"
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
	glorianadminmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/glorianadmin"
	glorianadminhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/glorianadmin/handler"
	glorianadminusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/glorianadmin/usecase"
	oauthprovidermodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/oauthprovider"
	oauthproviderhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/oauthprovider/handler"
	oauthproviderusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/integrations/oauthprovider/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/legacymobile"
	legalmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/legal"
	legalhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/legal/handler"
	legalusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/legal/usecase"
	logsmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/logs"
	logshandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/logs/handler"
	logsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/logs/usecase"
	measurementsmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements"
	measurementshandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements/handler"
	measurementsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/measurements/usecase"
	notifmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications"
	notifhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/providers"
	notifusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/notifications/usecase"
	ordersmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders"
	ordershandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/handler"
	ordersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/orders/usecase"
	orgmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	portalvehiclesmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/portalvehicles"
	portalvehicleshandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/portalvehicles/handler"
	portalvehiclesusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/portalvehicles/usecase"
	pricingmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing"
	pricinghandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/handler"
	pricingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/pricing/usecase"
	ratesmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/rates"
	rateshandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/rates/handler"
	searchmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search"
	searchgroups "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search/groups"
	searchhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search/indexsync"
	searchusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/search/usecase"
	servicecatalogmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/servicecatalog"
	servicecataloghandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/servicecatalog/handler"
	servicecatalogusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/servicecatalog/usecase"
	servicesmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services"
	serviceshandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/handler"
	servicereview "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/review"
	servicesusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	settingsmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/settings"
	settingshandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/settings/handler"
	settingsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/settings/usecase"
	shorturlsmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/shorturls"
	stockmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock"
	stockhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/handler"
	stockrebuild "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/rebuild"
	stockusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/usecase"
	storagemodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/storage"
	storagehandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/storage/handler"
	storageusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/storage/usecase"
	tasksmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/tasks"
	taskshandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/tasks/handler"
	tasksusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/tasks/usecase"
	transfersmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/transfers"
	transfershandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/transfers/handler"
	transfersusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/transfers/usecase"
	vehiclecatalogmodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/vehiclecatalog"
	vehiclecataloghandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/vehiclecatalog/handler"
	vehiclecatalogusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/vehiclecatalog/usecase"
	warehousemodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/glorian"
	warehousehandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/handler"
	warehouseusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warehouse/usecase"
	warrantymodule "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty"
	warrantyhandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/handler"
	warrantyusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
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
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
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
	// SearchFinder replaces the Meilisearch client in the module lists
	// (TEC-209 tests: an in-memory index). Nil: the configured client.
	SearchFinder searchengine.ListFinder
	// Clock replaces the access token clock (issue and expiry check), so a
	// test can move time forward (TEC-284 legacy token). Nil: time.Now.
	Clock func() time.Time
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
	// sysconfig is the global system settings store (TEC-215).
	sysconfig *sysconfig.Service
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
	if deps.Clock != nil {
		tokens.SetClock(deps.Clock)
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
		customersusecase.NewSearchAdapter(deps.Queries), // TEC-164
		// TEC-209: services, warranties, vehicles (plate / VIN).
		servicesusecase.NewSearchAdapter(deps.Queries),
		warrantyusecase.NewSearchAdapter(deps.Queries),
		customersusecase.NewVehicleSearchAdapter(deps.Queries),
		// TEC-210: organizations (dealer code), orders, stock units (barcode).
		orgusecase.NewSearchAdapter(deps.Queries),
		ordersusecase.NewSearchAdapter(deps.Queries),
		stockusecase.NewSearchAdapter(deps.Queries),
	)
	searchClient := searchengine.NewClient(cfg.Search, log)
	searchIndexer := searchengine.NewIndexer(searchClient, searchReg, deps.Queue, log)
	// TEC-209: module lists answer q from the index when it is up.
	var listFinder searchengine.ListFinder
	if searchClient != nil {
		listFinder = searchClient
	}
	if deps.SearchFinder != nil {
		listFinder = deps.SearchFinder
	}
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
	// TEC-210: organization.* events; q searches the organizations index.
	orgSvc.SetOutbox(outbox.NewStore(deps.DB, deps.Queries))
	if listFinder != nil {
		orgSvc.SetFinder(listFinder)
	}
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
	mobileH := authhandler.NewMobile(uc, notifSvc, ratelimit.New(deps.Redis, cfg.App.Env))
	authmodule.RegisterMobileRoutes(mux, mobileH,
		tokens, loader, cfg.Mobile.MinAPIVersion, cfg.Mobile.MaxAPIVersion)
	// TEC-233: minimal measurement storage (K28), 202 {uuid, status}.
	measurementsH := measurementshandler.New(measurementsusecase.New(deps.Queries))
	measurementsmodule.RegisterMobileRoutes(mux, measurementsH,
		tokens, loader, deps.Queries, cfg.Mobile.MinAPIVersion, cfg.Mobile.MaxAPIVersion)
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
	customersSvc := customersusecase.New(deps.DB, deps.Queries, customerPII, geoSvc)
	// TEC-161: anonymization cuts live sessions and refreshes the users index.
	if deps.Redis != nil {
		customersSvc.SetRevoker(authrevoke.New(deps.Redis, cfg.App.Env, cfg.JWT.AccessTTL))
	}
	customersSvc.SetSearchIndexer(searchIndexer)
	// TEC-164: q searches the customers index; customer.created links /portal.
	if listFinder != nil {
		customersSvc.SetFinder(listFinder)
	}
	customersSvc.SetPortalURL(cfg.Auth.FrontendURL)
	customersSvc.SetOutbox(outbox.NewStore(deps.DB, deps.Queries)) // TEC-193: customer.merged
	// TEC-190: vehicle transfer codes go out like the phone OTP (WhatsApp,
	// SMS fallback), synchronously and never through the outbox.
	customersSvc.SetTransfers(
		&whatsapp.Sender{WhatsApp: waSvc.Provider(), SMS: sms.Noop{Log: log}, SMSFallback: waSvc.SMSFallbackEnabled},
		customersusecase.TransferConfig{Key: otp.DeriveKey(cfg.Encryption.Key), AppName: cfg.App.Name},
	)
	customersH := customershandler.New(customersSvc, activityRec).WithLimiter(ratelimit.New(deps.Redis, cfg.App.Env))
	customershandler.RegisterRoutes(mux, customersH, tokens, loader, deps.Queries, featureSvc, stepUpSvc)
	ratesmodule.RegisterRoutes(mux, rateshandler.New(ratesSvc, activityRec), tokens, loader)
	// TEC-146: price list and effective price views (K8).
	pricingSvc := pricingusecase.New(deps.Queries)
	pricingmodule.RegisterRoutes(mux, pricinghandler.New(pricingSvc, activityRec),
		tokens, loader, deps.Queries, stepUpSvc, featureSvc)
	// TEC-306: service catalog, distributor overrides and effective service prices.
	serviceCatalogSvc := servicecatalogusecase.New(deps.Queries)
	servicecatalogmodule.RegisterRoutes(mux, servicecataloghandler.New(serviceCatalogSvc, activityRec),
		tokens, loader, deps.Queries, featureSvc)
	// TEC-172: accounting accounts, cari, manual entries and settlements.
	accountingPoster := accountingposting.New(deps.Queries, outbox.NewStore(deps.DB, deps.Queries), ratesSvc)
	accountingSvc := accountingusecase.New(deps.DB, deps.Queries, accountingPoster, featureSvc).
		WithOutbox(outbox.NewStore(deps.DB, deps.Queries)) // TEC-174: dispute events
	// TEC-198: re-parenting carries the open cari over (K25).
	orgSvc.SetParentChangeHook(reparentHook(accountingSvc))
	// TEC-207 (K4): the distributor preset opens its warehouse on create.
	orgSvc.SetWarehousePresetHook(warehouseusecase.New(deps.DB, deps.Queries))
	accountingH := accountinghandler.New(accountingSvc)
	accountinghandler.RegisterRoutes(mux, accountingH, tokens, loader, deps.Queries, stepUpSvc, featureSvc)
	// TEC-166: orders (draft, server-side prices, rate frozen at approval).
	// TEC-169: a received order books seller income / buyer purchase.
	ordersSvc := ordersusecase.New(deps.DB, deps.Queries, outbox.NewStore(deps.DB, deps.Queries), ratesSvc).
		WithReceiptHook(ordersusecase.NewAccountingBridge(accountingPoster))
	if listFinder != nil {
		ordersSvc.SetFinder(listFinder) // TEC-210
	}
	ordersmodule.RegisterRoutes(mux, ordershandler.New(ordersSvc), tokens, loader, deps.Queries, featureSvc)
	// TEC-197: stock transfer requests between siblings (K13).
	// TEC-200: a received transfer books A alacak / B borç.
	transfersSvc := transfersusecase.New(deps.DB, deps.Queries, outbox.NewStore(deps.DB, deps.Queries)).
		WithAccounting(accountingPoster)
	transfersmodule.RegisterRoutes(mux, transfershandler.New(transfersSvc), tokens, loader, deps.Queries, featureSvc)
	// TEC-179: services (draft, items from stock, stock-free transitions, images).
	servicesSvc := servicesusecase.New(deps.DB, deps.Queries, outbox.NewStore(deps.DB, deps.Queries))
	if listFinder != nil {
		servicesSvc.SetFinder(listFinder) // TEC-209
	}
	servicesH := serviceshandler.New(servicesSvc, deps.Storage)
	servicesmodule.RegisterRoutes(mux, servicesH, tokens, loader, deps.Queries, featureSvc)
	// TEC-234: old hub mobile app aliases, /v1/mobile/legacy/* (MOBILE_LEGACY_ALIASES; F5'te kaldırılır).
	legacymobile.RegisterRoutes(mux, cfg.Mobile.LegacyAliases, legacymobile.Handlers{
		Login: mobileH.Login, Me: mobileH.Me, SwitchOrganization: mobileH.SwitchOrganization,
		ListServices: servicesH.List, GetService: servicesH.Get,
		CreateMeasurement: measurementsH.Create,
		PutPushToken:      mobileH.PutPushToken, DeletePushToken: mobileH.DeletePushToken,
		Logout: mobileH.Logout,
		// TEC-284 (F2-FIX-3): long-lived legacy token bound to the device session.
		Sessions: uc,
	}, tokens, loader, deps.Queries, featureSvc)
	pdfClient := pdfrender.NewWithOptions(cfg.Gotenberg.URL, pdfrender.Options{MaxConnsPerHost: cfg.Queue.Concurrency})
	realtime.RegisterRoutes(mux, realtime.NewHandler(rtIssuer, uc), tokens, loader)

	nh := notifhandler.New(notifSvc)
	notifmodule.RegisterRoutes(mux, nh, tokens, loader)
	notifmodule.RegisterEventHandlers(eventBus, notifSvc, log)
	// TEC-186: service.completed opens one warranty per service item.
	warrantymodule.RegisterEventHandlers(eventBus, deps.DB, deps.Queries, cfg.Auth.FrontendURL, log)
	// TEC-192: service.completed schedules the delayed review request.
	var reviewQueue servicereview.Enqueuer
	if deps.Queue != nil {
		reviewQueue = deps.Queue
	}
	servicereview.RegisterEventHandlers(eventBus, reviewQueue, cfg.Services.ReviewRequestDelay, log)
	// TEC-270: glorian stock entries/placements and exits schedule the push.
	var glorianQueue glorian.Enqueuer
	if deps.Queue != nil {
		glorianQueue = deps.Queue
	}
	glorian.RegisterEventHandlers(eventBus, deps.Queries, glorianQueue, log)
	// TEC-209: service / warranty / vehicle outbox events refresh the indexes.
	indexsync.Register(eventBus, deps.Queries, searchIndexer, log)
	// TEC-189: public warranty lookup behind /garanti/{public_code}.
	warrantymodule.RegisterPublicRoutes(mux, deps.Queries, ratelimit.New(deps.Redis, cfg.App.Env),
		cfg.Warranty.PublicRateLimit, cfg.Warranty.PublicRateWindow,
		pdfClient, cfg.Auth.FrontendURL, cfg.Warranty.PublicPDFRateLimit)
	// TEC-249: public short URL resolver behind the frontend /s/{token}.
	shorturlsmodule.RegisterPublicRoutes(mux, shorturlsmodule.New(deps.Queries), ratelimit.New(deps.Redis, cfg.App.Env),
		cfg.ShortURLs.PublicRateLimit, cfg.ShortURLs.PublicRateWindow)
	// TEC-191: panel / portal warranty list and detail, center void.
	warrantyReader := warrantymodule.RegisterListRoutes(mux, deps.DB, deps.Queries, cfg.Auth.FrontendURL,
		tokens, loader, featureSvc, stepUpSvc, listFinder)

	// TEC-145: product catalog (brand scoped, center writes).
	catalogSvc := catalogusecase.New(deps.Queries, searchIndexer)
	warrantyCert := warrantymodule.NewCertificate(deps.Queries, deps.Storage, cfg.Auth.FrontendURL, log)
	servicePDF := servicesusecase.NewPDF(servicesSvc, warrantyCert, deps.Storage, log)
	// TEC-207: end-of-day warehouse reports and their PDF.
	eodSvc := warehouseusecase.NewEOD(deps.DB, deps.Queries)
	eodPDF := warehouseusecase.NewEODPDF(eodSvc, deps.Storage, log)
	ioReg := ioengine.NewRegistry(
		// TEC-211: price columns behind pricing.* grants.
		catalogusecase.NewIOAdapter(catalogSvc, deps.Queries).WithPrices(pricingSvc),
		ioadapters.NewUsers(deps.Queries),
		ioadapters.NewRoles(deps.Queries),
		ioadapters.NewNotifications(deps.Queries),
		ioadapters.NewActivity(deps.Queries),
		// TEC-175: cari statement and balance report exports.
		accountingusecase.NewStatementAdapter(accountingSvc),
		accountingusecase.NewBalancesAdapter(accountingSvc),
		// TEC-161: personal data export (center and portal).
		customersusecase.NewDataExportAdapter(customersSvc),
		customersusecase.NewPortalDataExportAdapter(customersSvc),
		// TEC-164: customer list export.
		customersusecase.NewListExportAdapter(customersSvc),
		// TEC-158: staged stock import (writes through ledger.Post).
		stockusecase.NewImporter(deps.DB, deps.Queries, outbox.NewStore(deps.DB, deps.Queries)),
		// TEC-188: warranty certificate PDF (panel and portal).
		warrantyusecase.NewCertificateAdapter(warrantyCert),
		warrantyusecase.NewPortalCertificateAdapter(warrantyCert),
		// TEC-196: service PDF.
		servicesusecase.NewPDFAdapter(servicePDF),
		// TEC-207: end-of-day report PDF.
		warehouseusecase.NewEODPDFAdapter(eodPDF),
		// TEC-239: service PDF requested from the portal.
		servicesusecase.NewPortalPDFAdapter(servicePDF),
	)
	exportSvc := exportusecase.New(deps.Queries, deps.Storage, ioReg, deps.Queue, notifSvc, activityRec, log)
	exportSvc.SetDocumentPDF(pdfClient)
	accountingH.WithExports(exportSvc)
	customersH.WithExports(exportSvc)
	warrantymodule.RegisterCertificateRoutes(mux, warrantyhandler.NewCertificate(warrantyCert, exportSvc), tokens, loader, deps.Queries, featureSvc)
	servicesmodule.RegisterPDFRoutes(mux, serviceshandler.NewPDF(servicePDF, exportSvc), tokens, loader, deps.Queries, featureSvc)
	// TEC-239: portal service detail and PDF.
	servicesmodule.RegisterPortalDetailRoutes(mux, serviceshandler.NewPortalDetail(servicesSvc, servicePDF, exportSvc), tokens, loader)
	// A nil *queue.Client must reach the import service as a nil Enqueuer
	// (sync mode); a typed nil would fail every confirm.
	var importQueue importusecase.Enqueuer
	if deps.Queue != nil {
		importQueue = deps.Queue
	}
	importSvc := importusecase.New(deps.Queries, deps.Storage, ioReg, importQueue, notifSvc, activityRec, log)
	bulkReg := bulkengine.NewRegistry(
		bulkadapters.NewUsers(deps.Queries),
		bulkadapters.NewRoles(deps.Queries),
		// TEC-212: tenant resources with undo.
		bulkadapters.NewCatalogProducts(deps.Queries),
		bulkadapters.NewTasks(deps.Queries),
	)
	bulkSvc := bulkusecase.New(deps.Queries, bulkReg, deps.Queue, notifSvc, activityRec, cfg.Bulk, log).WithPool(deps.DB)
	logsSvc := logsusecase.New(deps.Queries)
	if s.worker != nil {
		warrantyCron := warrantymodule.NewCron(deps.DB, deps.Queries, cfg.Auth.FrontendURL)
		s.worker.WithExport(exportSvc.ProcessExport).
			WithImport(importSvc.ProcessImport).
			WithBulk(bulkSvc.ProcessBulk).
			WithLogPurge(logsSvc.ApplyDueRules).
			WithRatesFetch(ratesSvc.FetchTask).
			WithWarrantyCron(warrantyCron.ExpireTask, warrantyCron.ExpiringScanTask).
			WithWarrantyRepairScan(warrantymodule.NewRepairScanner(deps.DB, deps.Queries, cfg.Auth.FrontendURL, cfg.Warranty.RepairScanDays, log).Task).
			WithVehicleTransferExpire(customersSvc.ExpireTransfersTask).
			WithServiceReviewRequest(servicereview.NewTaskSender(deps.DB, deps.Queries, log).Task).
			WithNotificationPurge(notifSvc.PurgeExpired).
			WithWhatsAppPoll(waSvc.PollStatus).
			WithTasksDueScan(tasksusecase.NewCron(deps.DB, deps.Queries, outbox.NewStore(deps.DB, deps.Queries)).DueScanTask)
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
	contractsSvc := contractsusecase.New(contractsrepo.New(deps.DB, deps.Queries))
	contractsmodule.RegisterRoutes(mux, contractshandler.New(contractsSvc), tokens, loader, deps.Queries, featureSvc)
	exportmodule.RegisterRoutes(mux, exporthandler.New(exportSvc), tokens, loader, stepUpSvc, deps.Queries)
	importmodule.RegisterRoutes(mux, importhandler.New(importSvc), tokens, loader, deps.Queries)
	bulkmodule.RegisterRoutes(mux, bulkhandler.New(bulkSvc), tokens, loader)
	bulkmodule.RegisterTenantRoutes(mux, bulkhandler.New(bulkSvc), featureSvc, tokens, loader, deps.Queries)
	catalogmodule.RegisterRoutes(mux, cataloghandler.New(catalogSvc, exportSvc, importSvc, deps.Storage, activityRec), featureSvc, tokens, loader, deps.Queries)
	// TEC-155: stock read API (barcode history, organization/bin stock).
	// TEC-157: reclassification (request, approval applies it via the ledger).
	stockSvc := stockusecase.New(deps.Queries)
	if listFinder != nil {
		stockSvc.SetFinder(listFinder) // TEC-210: unit list q
	}
	stockmodule.RegisterRoutes(mux, stockhandler.New(stockSvc),
		stockhandler.NewReclassify(stockusecase.NewReclassifications(deps.DB, deps.Queries, outbox.NewStore(deps.DB, deps.Queries))),
		featureSvc, tokens, loader, deps.Queries, stepUpSvc)
	// TEC-184: roll split (meters cut off a roll as a new unit).
	stockmodule.RegisterSplitRoutes(mux, stockhandler.NewSplit(stockusecase.NewSplits(deps.DB, deps.Queries, outbox.NewStore(deps.DB, deps.Queries))),
		featureSvc, tokens, loader, deps.Queries)
	// TEC-202: barcode batches (center), label templates and label PDFs.
	stockmodule.RegisterLabelRoutes(mux, stockhandler.NewLabels(
		stockusecase.NewBarcodes(deps.DB, deps.Queries), stockusecase.NewLabelTemplates(deps.Queries),
		stockusecase.NewLabels(deps.Queries, pdfClient)),
		featureSvc, tokens, loader, deps.Queries)
	// TEC-158: stock import upload (preview/confirm/undo on /v1/tenant/imports).
	stockmodule.RegisterImportRoutes(mux, stockhandler.NewImport(importSvc), featureSvc, tokens, loader, deps.Queries)
	// TEC-156: super_admin projection drift check (dry run).
	stockmodule.RegisterPlatformRoutes(mux, stockhandler.NewRebuild(stockrebuild.New(deps.DB, deps.Queries), deps.Queries), tokens, loader)
	// TEC-201: warehouse tree (warehouses, rooms, aisle/shelf/bin locations).
	warehousemodule.RegisterRoutes(mux, warehousehandler.New(warehouseusecase.New(deps.DB, deps.Queries)),
		featureSvc, tokens, loader, deps.Queries)

	// TEC-214: center tasks (center roles only; brand scoped).
	tasksmodule.RegisterRoutes(mux, taskshandler.New(tasksusecase.New(deps.DB, deps.Queries,
		outbox.NewStore(deps.DB, deps.Queries))), tokens, loader, deps.Queries)
	// TEC-149: vehicle catalog (global car brands/models, super_admin writes).
	vehiclecatalogmodule.RegisterRoutes(mux, vehiclecataloghandler.New(
		vehiclecatalogusecase.New(deps.Queries), deps.Storage, activityRec), tokens, loader)
	// TEC-215: system settings store with a 30 s Redis cache.
	var sysCache sysconfig.Cache = sysconfig.NoCache{}
	if deps.Redis != nil {
		sysCache = sysconfig.NewRedisCache(deps.Redis, cfg.App.Env, func(op string, err error) {
			log.Warn("sysconfig_cache_error", "op", op, "error", err)
		})
	}
	sysSvc := sysconfig.New(deps.Queries, sysCache)
	s.sysconfig = sysSvc
	bulkSvc.WithUndoWindow(sysSvc.BulkUndoWindowHours)
	// TEC-206: stock counts (scans through the TEC-203 resolver, approval via the ledger).
	warehousemodule.RegisterCountRoutes(mux, warehousehandler.NewCounts(warehouseusecase.NewCounts(deps.DB, deps.Queries,
		outbox.NewStore(deps.DB, deps.Queries), warehouseusecase.NewScanner(deps.Queries, sysSvc))),
		featureSvc, tokens, loader, deps.Queries)

	// TEC-203: universal scan resolver (scan.* settings from sysconfig).
	warehousemodule.RegisterScanRoutes(mux, warehousehandler.NewScan(warehouseusecase.NewScanner(deps.Queries, sysSvc)),
		featureSvc, tokens, loader, deps.Queries)
	// TEC-204: stock entry documents (confirm posts entry + placement via the ledger).
	warehousemodule.RegisterEntryRoutes(mux, warehousehandler.NewEntries(warehouseusecase.NewStockEntries(deps.DB, deps.Queries,
		outbox.NewStore(deps.DB, deps.Queries))), featureSvc, tokens, loader, deps.Queries)
	// TEC-205: bin <-> bin moves, warehouse transfer documents, order receipt placement.
	warehousemodule.RegisterTransferRoutes(mux, warehousehandler.NewTransfers(warehouseusecase.NewWarehouseTransfers(deps.DB, deps.Queries,
		outbox.NewStore(deps.DB, deps.Queries))), featureSvc, tokens, loader, deps.Queries)
	// TEC-207: end-of-day reports (manual run + PDF export job on worker-docs).
	warehousemodule.RegisterEODRoutes(mux, warehousehandler.NewEOD(eodSvc, eodPDF, exportSvc),
		featureSvc, tokens, loader, deps.Queries)
	// TEC-235: mobile mirrors (/v1/mobile/warehouse/*) of the scan, count and
	// transfer routes for distributor staff; same use cases and scope.
	warehousemodule.RegisterMobileRoutes(mux, warehousemodule.MobileHandlers{
		Tree: warehousehandler.New(warehouseusecase.New(deps.DB, deps.Queries)),
		Scan: warehousehandler.NewScan(warehouseusecase.NewScanner(deps.Queries, sysSvc)),
		Counts: warehousehandler.NewCounts(warehouseusecase.NewCounts(deps.DB, deps.Queries,
			outbox.NewStore(deps.DB, deps.Queries), warehouseusecase.NewScanner(deps.Queries, sysSvc))),
		Transfers: warehousehandler.NewTransfers(warehouseusecase.NewWarehouseTransfers(deps.DB, deps.Queries,
			outbox.NewStore(deps.DB, deps.Queries))),
	}, featureSvc, tokens, loader, deps.Queries, cfg.Mobile.MinAPIVersion, cfg.Mobile.MaxAPIVersion)
	settingsmodule.RegisterRoutes(mux, settingshandler.New(settingsusecase.New(deps.Queries), deps.Storage),
		settingshandler.NewSystem(sysSvc), tokens, loader)
	accessmodule.RegisterRoutes(mux, accesshandler.New(stepUpSvc, activityRec), tokens, loader)
	authsettingsmodule.RegisterRoutes(mux, authsettingshandler.New(authSettingsSvc, activityRec), tokens, loader)
	githubmodule.RegisterRoutes(mux, githubhandler.New(githubSvc, activityRec), tokens, loader)
	whatsappmodule.RegisterRoutes(mux, whatsapphandler.New(waSvc, activityRec, log), tokens, loader)
	legalmodule.RegisterRoutes(mux, legalhandler.New(legalusecase.New(deps.Queries), activityRec), tokens, loader)
	oauthprovidermodule.RegisterRoutes(mux, oauthproviderhandler.New(oauthProvSvc, activityRec), tokens, loader)
	activitymodule.RegisterRoutes(mux, activityhandler.New(activityusecase.New(deps.Queries)), tokens, loader)
	logsmodule.RegisterRoutes(mux, logshandler.New(logsSvc), tokens, loader)
	searchSvc := searchusecase.New(searchClient, searchReg, deps.Queries, log)
	// TEC-213: Cmd+K global search through the module lists.
	searchSvc.SetGroups(listFinder, deps.Queries, featureSvc, searchgroups.Build(searchgroups.Lists{
		Customers: customersSvc, Services: servicesSvc, Warranties: warrantyReader,
		Orders: ordersSvc, Organizations: orgSvc, Stock: stockSvc,
	})...)
	searchmodule.RegisterRoutes(mux, searchhandler.New(searchSvc), tokens, loader, deps.Queries)
	storageSvc := storageusecase.New(deps.Storage, deps.Queries, log)
	storagemodule.RegisterRoutes(
		mux,
		storagehandler.New(storageSvc),
		tokens,
		loader,
	)
	// TEC-238: customer portal vehicles, vehicle detail and service history.
	portalvehiclesmodule.RegisterRoutes(mux, portalvehicleshandler.New(portalvehiclesusecase.New(deps.Queries)), tokens, loader)
	// TEC-273: Glorian admin API (connection settings, sync runs, outbound
	// replay, reconcile); glorian.Store is wired here.
	glorianadminmodule.RegisterRoutes(mux, glorianadminhandler.New(glorianadminusecase.New(deps.Queries, secretBox,
		glorian.HTTPClientFactory(glorian.OptionsFromConfig(cfg.Glorian)), glorianQueue, log), activityRec), tokens, loader)

	var brandResolver *brandctx.Resolver
	if deps.Queries != nil {
		brandResolver = brandctx.NewResolver(brandctx.DBLoader(deps.Queries), cfg.App.DefaultBrandSlug, time.Minute)
	}

	outboxStore := outbox.NewStore(deps.DB, deps.Queries)
	outboxPub := outbox.NewPublisher(outboxStore, eventBus, log)
	s.outboxPub = outboxPub

	// TEC-236: app version gate on /v1/mobile/* (legacy aliases included);
	// system settings mobile.* override the MOBILE_APP_* environment.
	gateSettings := sysSvc
	if deps.Queries == nil {
		gateSettings = nil
	}
	appGate := middleware.MobileAppVersion(mobileAppPolicy(gateSettings, cfg.Mobile), cfg.Mobile.AppUserAgentProducts)

	s.http = &http.Server{
		Addr:         cfg.HTTP.Addr,
		Handler:      middleware.ServerErrors(log)(middleware.RequestID(errtrack.Middleware(errtrack.Recover(log)(middleware.ResolveBrand(brandResolver)(middleware.ResolveLocale(appGate(mux))))))),
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
