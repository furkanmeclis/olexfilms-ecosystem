package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

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
	Search     SearchConfig
	Gotenberg  GotenbergConfig
	Wuzapi     WuzapiConfig
}

// GotenbergConfig controls HTML→PDF rendering via Gotenberg Chromium.
type GotenbergConfig struct {
	URL string
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

// JWTConfig holds access JWT and opaque refresh token settings.
type JWTConfig struct {
	AccessSecret  string
	RefreshSecret string
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
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
			Key: getEnv("APP_ENCRYPTION_KEY", defaultEncryptionKey),
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
			AccessSecret:  getEnv("JWT_ACCESS_SECRET", "app-dev-access-secret-change-me-32b"),
			RefreshSecret: getEnv("JWT_REFRESH_SECRET", "app-dev-refresh-secret-change-me-32b"),
			AccessTTL:     getDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTTL:    getDuration("JWT_REFRESH_TTL", 168*time.Hour),
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
		Search: SearchConfig{
			Enabled:     getBool("SEARCH_ENABLED", true),
			Driver:      strings.ToLower(getEnv("SEARCH_DRIVER", "meilisearch")),
			MeiliHost:   getEnv("MEILI_HOST", "http://127.0.0.1:7700"),
			MeiliKey:    getEnv("MEILI_MASTER_KEY", "app-dev-meili-master-key-change-me"),
			IndexPrefix: getEnv("MEILI_INDEX_PREFIX", "app"),
		},
		Gotenberg: GotenbergConfig{
			URL: getEnv("GOTENBERG_URL", "http://127.0.0.1:3001"),
		},
		Wuzapi: WuzapiConfig{
			URL:           getEnv("WUZAPI_URL", ""),
			AdminToken:    getEnv("WUZAPI_ADMIN_TOKEN", ""),
			WebhookSecret: getEnv("WUZAPI_WEBHOOK_SECRET", ""),
			WebhookURL:    getEnv("WUZAPI_WEBHOOK_URL", ""),
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
