package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/crypto"
	"github.com/joho/godotenv"
)

// Config holds process configuration loaded from the environment.
type Config struct {
	App        AppConfig
	Encryption EncryptionConfig
	HTTP       HTTPConfig
	DB         DBConfig
	Redis      RedisConfig
	Storage    StorageConfig
	CORS       CORSConfig
	SMTP       SMTPConfig
	Queue      QueueConfig
	Bulk       BulkConfig
	Centrifugo CentrifugoConfig
	Auth       AuthConfig
	JWT        JWTConfig
	Log        LogConfig
	VAPID      VAPIDConfig
	Notify     NotifyConfig
	Search     SearchConfig
	Gotenberg  GotenbergConfig
	Wuzapi     WuzapiConfig
	Rates      RatesConfig
	Mobile     MobileConfig
	Warranty   WarrantyConfig
	Services   ServicesConfig
	Glorian    GlorianConfig
	ShortURLs  ShortURLsConfig
	Leads      LeadsConfig
	AI         AIConfig
}

// AIConfig is the LLM provider of the AI assistant (F4, platform/llm). The
// API key lives only in env; empty disables the provider (503
// AI_UNAVAILABLE). DefaultModel / FastModel back empty
// ai_settings.default_model / fast_model; AllowedModels is the allow list
// (empty = the two env models).
type AIConfig struct {
	APIKey         string
	BaseURL        string
	DefaultModel   string
	FastModel      string
	AllowedModels  []string
	MaxTokens      int
	RequestTimeout time.Duration
}

// LeadsConfig caps the public dealer application form
// POST /v1/public/dealer-applications (TEC-317): ApplicationIPLimit
// submissions per client IP and ApplicationPhoneLimit per E.164 phone in
// ApplicationRateWindow. Zero disables a limit.
type LeadsConfig struct {
	ApplicationIPLimit    int
	ApplicationPhoneLimit int
	ApplicationRateWindow time.Duration
}

// ShortURLsConfig caps the public short URL resolver
// GET /v1/public/short-urls/{token} per client IP (TEC-249).
type ShortURLsConfig struct {
	PublicRateLimit  int
	PublicRateWindow time.Duration
}

// GlorianConfig is the outbound Inventory API connection to the Glorian hub
// (TEC-267, design K2). Until integration_connections lands (TEC-266) the
// single connection comes from env; Enabled=false or an empty base URL/key
// makes the resolver answer ErrInactiveConnection. Timeouts bound one read
// or write attempt; RetryBaseDelay is the first GET backoff step and
// MaxRetryAfter the longest 429 Retry-After waited in place.
type GlorianConfig struct {
	Enabled        bool
	BaseURL        string
	APIKey         string
	Timeout        time.Duration
	WriteTimeout   time.Duration
	RetryBaseDelay time.Duration
	MaxRetryAfter  time.Duration
}

// MobileConfig is the mobile API contract (TEC-91): the supported range of
// the X-Mobile-Api-Version header (integer major versions) and the QR web
// sign-in challenge lifetime.
type MobileConfig struct {
	MinAPIVersion int
	MaxAPIVersion int
	QRLoginTTL    time.Duration
	// LegacyAliases mounts the old hub app's aliases under
	// /v1/mobile/legacy/* (TEC-234, MOBILE_LEGACY_ALIASES, default off;
	// removed in F5).
	LegacyAliases bool
	// App version gate (TEC-236): environment defaults of the mobile.*
	// system settings. AppMinVersion empty = no gate unless the setting is
	// stored. AppUserAgentProducts are the User-Agent product tokens read as
	// the app version when X-App-Version is missing.
	AppMinVersion        string
	AppStoreURLIOS       string
	AppStoreURLAndroid   string
	AppVersionRequired   bool
	AppUserAgentProducts []string
}

// WarrantyConfig tunes the periodic warranty tasks. RepairScanDays is the
// look-back window of warranty:repair_scan (TEC-194), by completed_at.
// PublicRateLimit / PublicRateWindow cap the public lookup
// GET /v1/public/warranties/{public_code} per client IP (TEC-189);
// PublicPDFRateLimit caps its anonymous PDF in the same window (TEC-248).
type WarrantyConfig struct {
	RepairScanDays     int
	PublicRateLimit    int
	PublicRateWindow   time.Duration
	PublicPDFRateLimit int
}

// ServicesConfig tunes the service follow-up tasks. ReviewRequestDelay is
// how long after service.completed the Google review request is sent
// (TEC-192, SERVICE_REVIEW_REQUEST_DELAY, default 24h).
type ServicesConfig struct {
	ReviewRequestDelay time.Duration
}

// RatesConfig points the daily exchange rate fetch (TEC-84) at TCMB and ECB.
type RatesConfig struct {
	TCMBURL string
	ECBURL  string
}

// GotenbergConfig controls HTML→PDF rendering via Gotenberg Chromium.
type GotenbergConfig struct {
	URL string
	// Fonts is PDF_FONTS: embedded (Noto subsets inlined, default) or
	// system (fonts installed in the Gotenberg image).
	Fonts string
}

// WuzapiConfig is the WhatsApp gateway (design K16/K21). An empty URL or
// admin token disables the gateway (OTP requests then fail closed).
type WuzapiConfig struct {
	URL        string
	AdminToken string
	// WebhookSecret verifies x-hmac-signature (wuzapi WUZAPI_GLOBAL_HMAC_KEY).
	WebhookSecret string
	// WebhookURL is registered on the instance user (backend /hooks/wuzapi).
	WebhookURL string
}

// AuthConfig holds NextAuth adapter integration settings.
type AuthConfig struct {
	AdapterSecret string
	WebAuthnRPID  string
	FrontendURL   string
}

// VAPIDConfig holds Web Push keys (empty = push disabled).
type VAPIDConfig struct {
	PublicKey  string
	PrivateKey string
	Subject    string
}

// NotifyConfig configures notification center drivers (TEC-87).
type NotifyConfig struct {
	// ExpoPushURL is the Expo push send endpoint; ExpoAccessToken is optional.
	ExpoPushURL     string
	ExpoAccessToken string
	// Email frame: logo URL and brand color of the HTML layout.
	EmailLogoURL string
	EmailColor   string
}

// JWTConfig holds access JWT and opaque refresh token settings.
type JWTConfig struct {
	AccessSecret  string
	RefreshSecret string
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
	// MobileRefreshTTL is the refresh lifetime of mobile app sessions.
	MobileRefreshTTL time.Duration
}

type AppConfig struct {
	Name string
	Env  string
	// DefaultBrandSlug is used when the request host matches no brand domain.
	DefaultBrandSlug string
}

// EncryptionConfig holds at-rest secret encryption material.
type EncryptionConfig struct {
	Key string
	// CustomerPIIKey (CUSTOMER_PII_KEY, base64 of 32 bytes) encrypts customer
	// national ids and tax numbers (TEC-159). No default: an empty key is an
	// error outside development and crypto.NewPIIBox refuses it everywhere.
	CustomerPIIKey string
}

type HTTPConfig struct {
	Addr         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

type DBConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	SSLMode  string
	MaxConns int32
}

func (c DBConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User,
		c.Password,
		c.Host,
		c.Port,
		c.Name,
		c.SSLMode,
	)
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type StorageConfig struct {
	Driver    string // s3 (SeaweedFS or any S3-compatible store)
	LocalPath string
	MaxBytes  int64
	S3        ObjectStoreConfig
}

// ObjectStoreConfig holds S3-compatible credentials (SeaweedFS or S3).
// Objects are never served from a public bucket URL; the API streams them.
type ObjectStoreConfig struct {
	Endpoint     string
	Region       string
	Bucket       string
	AccessKey    string
	SecretKey    string
	UsePathStyle bool
}

type CORSConfig struct {
	AllowedOrigins []string
}

// SMTPConfig configures outbound email (MailHog in development).
type SMTPConfig struct {
	Driver   string
	Host     string
	Port     int
	Username string
	Password string
	From     string
	FromName string
}

type LogConfig struct {
	Level  string
	Format string
}

// QueueConfig controls Asynq background jobs.
type QueueConfig struct {
	Enabled         bool
	Concurrency     int
	WorkerInProcess bool
}

// BulkConfig controls bulk action engine thresholds.
type BulkConfig struct {
	SyncMax       int
	RollbackHours int
}

// CentrifugoConfig controls realtime pub/sub via Centrifugo.
type CentrifugoConfig struct {
	Enabled   bool
	APIURL    string
	APIKey    string
	TokenHMAC string
	WSURL     string
	TokenTTL  time.Duration
}

// SearchConfig controls Meilisearch-backed command palette indexing.
type SearchConfig struct {
	Enabled     bool
	Driver      string
	MeiliHost   string
	MeiliKey    string
	IndexPrefix string
}

// Load reads optional .env then environment variables into Config.
func Load() (Config, error) {
	_ = godotenv.Load("../.env", ".env")

	frontendURL := getEnv("PUBLIC_FRONTEND_URL", "http://localhost:3000")

	cfg := Config{
		App: AppConfig{
			Name:             getEnv("APP_NAME", "api"),
			Env:              getEnv("APP_ENV", "development"),
			DefaultBrandSlug: getEnv("DEFAULT_BRAND_SLUG", "olex"),
		},
		Encryption: EncryptionConfig{
			Key:            getEnv("APP_ENCRYPTION_KEY", defaultEncryptionKey),
			CustomerPIIKey: getEnv("CUSTOMER_PII_KEY", ""),
		},
		HTTP: HTTPConfig{
			Addr:         getEnv("APP_HTTP_ADDR", ":8080"),
			ReadTimeout:  getDuration("HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout: getDuration("HTTP_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:  getDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
		},
		DB: DBConfig{
			Host:     getEnv("DB_HOST", "127.0.0.1"),
			Port:     getInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "app"),
			Password: getEnv("DB_PASSWORD", "change_me"),
			Name:     getEnv("DB_NAME", "app"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
			MaxConns: int32(getInt("DB_MAX_CONNS", 20)),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "127.0.0.1:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getInt("REDIS_DB", 0),
		},
		Storage: loadStorageConfig(),
		CORS: CORSConfig{
			AllowedOrigins: splitCSV(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000")),
		},
		SMTP: SMTPConfig{
			Driver:   strings.ToLower(getEnv("MAIL_DRIVER", "smtp")),
			Host:     getEnv("SMTP_HOST", "127.0.0.1"),
			Port:     getInt("SMTP_PORT", 1025),
			Username: getEnv("SMTP_USERNAME", ""),
			Password: getEnv("SMTP_PASSWORD", ""),
			From:     firstNonEmpty(getEnv("MAIL_FROM", ""), getEnv("SMTP_FROM", "noreply@localhost")),
			FromName: getEnv("MAIL_FROM_NAME", "App"),
		},
		Queue: QueueConfig{
			Enabled:         getBool("QUEUE_ENABLED", true),
			Concurrency:     getInt("QUEUE_CONCURRENCY", 10),
			WorkerInProcess: getBool("QUEUE_WORKER_INPROCESS", true),
		},
		Bulk: BulkConfig{
			SyncMax:       getInt("BULK_SYNC_MAX", 50),
			RollbackHours: getInt("BULK_ROLLBACK_HOURS", 24),
		},
		Centrifugo: CentrifugoConfig{
			Enabled:   getBool("CENTRIFUGO_ENABLED", true),
			APIURL:    getEnv("CENTRIFUGO_API_URL", "http://127.0.0.1:8000"),
			APIKey:    getEnv("CENTRIFUGO_API_KEY", defaultCentrifugoAPIKey),
			TokenHMAC: getEnv("CENTRIFUGO_TOKEN_HMAC_SECRET", defaultCentrifugoTokenHMAC),
			WSURL:     getEnv("CENTRIFUGO_WS_URL", "ws://127.0.0.1:8000/connection/websocket"),
			TokenTTL:  getDuration("CENTRIFUGO_TOKEN_TTL", time.Hour),
		},
		Auth: AuthConfig{
			AdapterSecret: getEnv("AUTH_ADAPTER_SECRET", defaultAdapterSecret),
			WebAuthnRPID:  getEnv("AUTH_WEBAUTHN_RP_ID", hostOf(frontendURL, "localhost")),
			FrontendURL:   frontendURL,
		},
		JWT: JWTConfig{
			AccessSecret:     getEnv("JWT_ACCESS_SECRET", "app-dev-access-secret-change-me-32b"),
			RefreshSecret:    getEnv("JWT_REFRESH_SECRET", "app-dev-refresh-secret-change-me-32b"),
			AccessTTL:        getDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTTL:       getDuration("JWT_REFRESH_TTL", 168*time.Hour),
			MobileRefreshTTL: getDuration("JWT_MOBILE_REFRESH_TTL", 720*time.Hour),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "debug"),
			Format: getEnv("LOG_FORMAT", "json"),
		},
		VAPID: VAPIDConfig{
			PublicKey:  getEnv("VAPID_PUBLIC_KEY", ""),
			PrivateKey: getEnv("VAPID_PRIVATE_KEY", ""),
			Subject:    getEnv("VAPID_SUBJECT", "mailto:noreply@example.com"),
		},
		Notify: NotifyConfig{
			ExpoPushURL:     getEnv("EXPO_PUSH_URL", "https://exp.host/--/api/v2/push/send"),
			ExpoAccessToken: getEnv("EXPO_ACCESS_TOKEN", ""),
			EmailLogoURL:    getEnv("NOTIFY_EMAIL_LOGO_URL", ""),
			EmailColor:      getEnv("NOTIFY_EMAIL_COLOR", "#111827"),
		},
		Search: SearchConfig{
			Enabled:     getBool("SEARCH_ENABLED", true),
			Driver:      strings.ToLower(getEnv("SEARCH_DRIVER", "meilisearch")),
			MeiliHost:   getEnv("MEILI_HOST", "http://127.0.0.1:7700"),
			MeiliKey:    getEnv("MEILI_MASTER_KEY", "app-dev-meili-master-key-change-me"),
			IndexPrefix: getEnv("MEILI_INDEX_PREFIX", "app"),
		},
		Gotenberg: GotenbergConfig{
			URL:   getEnv("GOTENBERG_URL", "http://127.0.0.1:3001"),
			Fonts: getEnv("PDF_FONTS", "embedded"),
		},
		Wuzapi: WuzapiConfig{
			URL:           getEnv("WUZAPI_URL", ""),
			AdminToken:    getEnv("WUZAPI_ADMIN_TOKEN", ""),
			WebhookSecret: getEnv("WUZAPI_WEBHOOK_SECRET", ""),
			WebhookURL:    getEnv("WUZAPI_WEBHOOK_URL", ""),
		},
		Rates: RatesConfig{
			TCMBURL: getEnv("RATES_TCMB_URL", "https://www.tcmb.gov.tr/kurlar/today.xml"),
			ECBURL:  getEnv("RATES_ECB_URL", "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml"),
		},
		Warranty: WarrantyConfig{
			RepairScanDays:     getInt("WARRANTY_REPAIR_SCAN_DAYS", 30),
			PublicRateLimit:    getInt("WARRANTY_PUBLIC_RATE_LIMIT", 30),
			PublicRateWindow:   getDuration("WARRANTY_PUBLIC_RATE_WINDOW", time.Minute),
			PublicPDFRateLimit: getInt("WARRANTY_PUBLIC_PDF_RATE_LIMIT", 5),
		},
		ShortURLs: ShortURLsConfig{
			PublicRateLimit:  getInt("SHORT_URL_PUBLIC_RATE_LIMIT", 60),
			PublicRateWindow: getDuration("SHORT_URL_PUBLIC_RATE_WINDOW", time.Minute),
		},
		Leads: LeadsConfig{
			ApplicationIPLimit:    getInt("DEALER_APPLICATION_IP_RATE_LIMIT", 5),
			ApplicationPhoneLimit: getInt("DEALER_APPLICATION_PHONE_RATE_LIMIT", 3),
			ApplicationRateWindow: getDuration("DEALER_APPLICATION_RATE_WINDOW", time.Hour),
		},
		Services: ServicesConfig{
			ReviewRequestDelay: getDuration("SERVICE_REVIEW_REQUEST_DELAY", 24*time.Hour),
		},
		Mobile: MobileConfig{
			MinAPIVersion: getInt("MOBILE_API_MIN_VERSION", 1),
			MaxAPIVersion: getInt("MOBILE_API_MAX_VERSION", 1),
			QRLoginTTL:    getDuration("QR_LOGIN_TTL", 120*time.Second),
			LegacyAliases: getBool("MOBILE_LEGACY_ALIASES", false),

			AppMinVersion:        getEnv("MOBILE_APP_MIN_VERSION", ""),
			AppStoreURLIOS:       getEnv("MOBILE_APP_STORE_URL_IOS", ""),
			AppStoreURLAndroid:   getEnv("MOBILE_APP_STORE_URL_ANDROID", ""),
			AppVersionRequired:   getBool("MOBILE_APP_VERSION_REQUIRED", false),
			AppUserAgentProducts: splitCSV(getEnv("MOBILE_APP_UA_PRODUCTS", "OlexFilms")),
		},
		Glorian: GlorianConfig{
			Enabled:        getBool("GLORIAN_INVENTORY_ENABLED", false),
			BaseURL:        getEnv("GLORIAN_INVENTORY_BASE_URL", ""),
			APIKey:         getEnv("GLORIAN_INVENTORY_API_KEY", ""),
			Timeout:        getDuration("GLORIAN_INVENTORY_TIMEOUT", 15*time.Second),
			WriteTimeout:   getDuration("GLORIAN_INVENTORY_WRITE_TIMEOUT", 30*time.Second),
			RetryBaseDelay: getDuration("GLORIAN_INVENTORY_RETRY_BASE_DELAY", 200*time.Millisecond),
			MaxRetryAfter:  getDuration("GLORIAN_INVENTORY_MAX_RETRY_AFTER", 60*time.Second),
		},
		AI: AIConfig{
			APIKey:         strings.TrimSpace(getEnv("ANTHROPIC_API_KEY", "")),
			BaseURL:        getEnv("ANTHROPIC_BASE_URL", ""),
			DefaultModel:   getEnv("AI_MODEL_DEFAULT", "claude-sonnet-5-5"),
			FastModel:      getEnv("AI_MODEL_FAST", "claude-haiku-4-5"),
			AllowedModels:  splitCSV(getEnv("AI_ALLOWED_MODELS", "")),
			MaxTokens:      getInt("AI_MAX_TOKENS", 8000),
			RequestTimeout: getDuration("AI_REQUEST_TIMEOUT", 120*time.Second),
		},
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Development-only fallbacks. validate() rejects them outside development.
const (
	defaultEncryptionKey       = "app-dev-encryption-key-32bytes!!"
	defaultAdapterSecret       = "app-dev-auth-adapter-secret-change-me"
	defaultCentrifugoAPIKey    = "app-centrifugo-api-key-change-me"
	defaultCentrifugoTokenHMAC = "app-centrifugo-token-hmac-change-me"
)

func (c Config) validate() error {
	if c.App.Name == "" {
		return fmt.Errorf("config: APP_NAME is required")
	}
	if c.HTTP.Addr == "" {
		return fmt.Errorf("config: APP_HTTP_ADDR is required")
	}
	if c.DB.Host == "" || c.DB.User == "" || c.DB.Name == "" {
		return fmt.Errorf("config: database settings are incomplete")
	}
	if c.Redis.Addr == "" {
		return fmt.Errorf("config: REDIS_ADDR is required")
	}
	if c.Queue.Concurrency <= 0 {
		return fmt.Errorf("config: QUEUE_CONCURRENCY must be positive")
	}
	if c.JWT.AccessTTL <= 0 || c.JWT.RefreshTTL <= 0 {
		return fmt.Errorf("config: JWT TTLs must be positive")
	}
	if c.App.Env != "development" && c.App.Env != "test" {
		if c.JWT.AccessSecret == "" || c.JWT.RefreshSecret == "" {
			return fmt.Errorf("config: JWT_ACCESS_SECRET and JWT_REFRESH_SECRET are required outside development")
		}
		if c.JWT.AccessSecret == "app-dev-access-secret-change-me-32b" ||
			c.JWT.RefreshSecret == "app-dev-refresh-secret-change-me-32b" {
			return fmt.Errorf("config: replace default JWT secrets outside development")
		}
		if len(c.JWT.AccessSecret) < 32 {
			return fmt.Errorf("config: JWT_ACCESS_SECRET must be at least 32 characters outside development")
		}
		// Fallbacks below are public (.env.example / source); refuse them so
		// production never encrypts secrets or signs tokens with known keys.
		if c.Encryption.Key == "" || c.Encryption.Key == defaultEncryptionKey {
			return fmt.Errorf("config: set a unique APP_ENCRYPTION_KEY outside development")
		}
		if _, err := crypto.NewPIIBox(c.Encryption.CustomerPIIKey); err != nil {
			return fmt.Errorf("config: CUSTOMER_PII_KEY: %w", err)
		}
		if strings.TrimSpace(c.Encryption.CustomerPIIKey) == strings.TrimSpace(c.Encryption.Key) {
			return fmt.Errorf("config: CUSTOMER_PII_KEY must differ from APP_ENCRYPTION_KEY")
		}
		if c.Auth.AdapterSecret == defaultAdapterSecret {
			return fmt.Errorf("config: replace default AUTH_ADAPTER_SECRET outside development")
		}
		if c.Centrifugo.Enabled && (c.Centrifugo.TokenHMAC == defaultCentrifugoTokenHMAC ||
			c.Centrifugo.APIKey == defaultCentrifugoAPIKey) {
			return fmt.Errorf("config: replace default CENTRIFUGO_TOKEN_HMAC_SECRET / CENTRIFUGO_API_KEY outside development")
		}
	}
	if c.JWT.AccessSecret == "" {
		return fmt.Errorf("config: JWT_ACCESS_SECRET is required")
	}
	if c.Search.Enabled {
		if c.Search.MeiliHost == "" {
			return fmt.Errorf("config: MEILI_HOST is required when SEARCH_ENABLED=true")
		}
		if c.Search.IndexPrefix == "" {
			return fmt.Errorf("config: MEILI_INDEX_PREFIX is required when SEARCH_ENABLED=true")
		}
		if c.App.Env != "development" && c.App.Env != "test" {
			if c.Search.MeiliKey == "" {
				return fmt.Errorf("config: MEILI_MASTER_KEY is required when SEARCH_ENABLED=true")
			}
			if c.Search.MeiliKey == "app-dev-meili-master-key-change-me" {
				return fmt.Errorf("config: replace default MEILI_MASTER_KEY outside development")
			}
		}
	}
	return nil
}

func loadStorageConfig() StorageConfig {
	return StorageConfig{
		Driver:    strings.ToLower(getEnv("STORAGE_DRIVER", "s3")),
		LocalPath: getEnv("STORAGE_LOCAL_PATH", "./storage/objects"),
		MaxBytes:  getInt64("STORAGE_MAX_BYTES", 10<<20),
		S3: ObjectStoreConfig{
			Endpoint:     getEnv("S3_ENDPOINT", "http://127.0.0.1:9000"),
			Region:       getEnv("S3_REGION", "us-east-1"),
			Bucket:       getEnv("S3_BUCKET", "app"),
			AccessKey:    getEnv("S3_ACCESS_KEY", "s3admin"),
			SecretKey:    getEnv("S3_SECRET_KEY", "s3admin-secret"),
			UsePathStyle: getBool("S3_USE_PATH_STYLE", true),
		},
	}
}

// hostOf returns the hostname of rawURL (no port), or fallback when it has none.
func hostOf(rawURL, fallback string) string {
	if u, err := url.Parse(strings.TrimSpace(rawURL)); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getInt64(key string, fallback int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func getBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
