package usecase

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"golang.org/x/crypto/argon2"
)

// Connection package (TEC-1265): the ecosystem issues a new hub API key and
// hands it to Glorian in a password-encrypted JSON file. The format is
// shared with Glorian (glorian docs/inventory-api.md §2 "Bağlantı paketi",
// docs/design.md "Glorian bağlantı paketi"); both repos test the same
// fixture (testdata/connection-package/v1).
const (
	PackageFormat  = "olexfilms.glorian-connection"
	PackageVersion = 1
	PackageAAD     = "olexfilms.glorian-connection.v1"

	packageMemoryKiB  = 64 * 1024
	packageIterations = 3
	packageSaltBytes  = 16
	packageNonceBytes = 12
	apiKeyBytes       = 32

	// Limits of PackageInput.
	MinPackagePassword   = 12
	MaxPackagePassword   = 256
	DefaultPackageTTL    = 24
	MaxPackageTTL        = 72
	DefaultRateLimit     = 300
	MaxRateLimit         = 10000
	MaxIPAllowlist       = 50
	packageIssuerName    = "Olexfilms"
	packageFilenameStamp = "20060102-1504"
)

// PackageInput is POST /connection-package. Password is never stored or
// logged; it only derives the package key.
type PackageInput struct {
	BaseURL            string   `json:"base_url"`
	Password           string   `json:"password"`
	DealerWrite        *bool    `json:"dealer_write"`
	OrderStatusActions *bool    `json:"order_status_actions"`
	RateLimitPerMinute *int     `json:"rate_limit_per_minute"`
	IPAllowlist        []string `json:"ip_allowlist"`
	TTLHours           *int     `json:"ttl_hours"`
}

// PackageResult is the file to download plus the connection after the key
// change. Package is the encrypted envelope; the plain key is not in it.
type PackageResult struct {
	Filename       string          `json:"filename"`
	Package        json.RawMessage `json:"package"`
	KeyFingerprint string          `json:"key_fingerprint"`
	Rotated        bool            `json:"rotated"`
	ExpiresAt      time.Time       `json:"expires_at"`
	Connection     ConnectionView  `json:"connection"`
}

// packagePayload is the plaintext inside the envelope.
type packagePayload struct {
	Version            int                `json:"version"`
	ConnectionID       string             `json:"connection_id"`
	Issuer             packageIssuer      `json:"issuer"`
	HubURL             string             `json:"hub_url"`
	APIKey             string             `json:"api_key"`
	Permissions        packagePermissions `json:"permissions"`
	RateLimitPerMinute int                `json:"rate_limit_per_minute"`
	IPAllowlist        []string           `json:"ip_allowlist"`
	CreatedAt          string             `json:"created_at"`
	ExpiresAt          string             `json:"expires_at"`
}

type packageIssuer struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type packagePermissions struct {
	DealerWrite        bool `json:"dealer_write"`
	OrderStatusActions bool `json:"order_status_actions"`
}

type packageEnvelope struct {
	Format     string        `json:"format"`
	Version    int           `json:"version"`
	KDF        packageKDF    `json:"kdf"`
	Cipher     packageCipher `json:"cipher"`
	Ciphertext string        `json:"ciphertext"`
}

type packageKDF struct {
	Algorithm   string `json:"algorithm"`
	Version     int    `json:"version"`
	MemoryKiB   uint32 `json:"memory_kib"`
	Iterations  uint32 `json:"iterations"`
	Parallelism uint8  `json:"parallelism"`
	Salt        string `json:"salt"`
}

type packageCipher struct {
	Algorithm string `json:"algorithm"`
	Nonce     string `json:"nonce"`
}

// KeyFingerprint is the key's short id shown on both sides:
// first 12 hex characters of SHA-256 (Glorian InventoryConnectionConfig).
func KeyFingerprint(apiKey string) string {
	sum := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(sum[:])[:12]
}

// sealPackage encrypts plaintext with a key derived from password
// (Argon2id, p=1) under AES-256-GCM. salt and nonce are random unless given
// (fixture tests).
func sealPackage(plaintext []byte, password string, salt, nonce []byte) ([]byte, error) {
	if salt == nil {
		salt = make([]byte, packageSaltBytes)
		if _, err := rand.Read(salt); err != nil {
			return nil, err
		}
	}
	if nonce == nil {
		nonce = make([]byte, packageNonceBytes)
		if _, err := rand.Read(nonce); err != nil {
			return nil, err
		}
	}
	key := argon2.IDKey([]byte(password), salt, packageIterations, packageMemoryKiB, 1, 32)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, []byte(PackageAAD))
	return json.MarshalIndent(packageEnvelope{
		Format:  PackageFormat,
		Version: PackageVersion,
		KDF: packageKDF{
			Algorithm: "argon2id", Version: 19, MemoryKiB: packageMemoryKiB,
			Iterations: packageIterations, Parallelism: 1,
			Salt: base64.StdEncoding.EncodeToString(salt),
		},
		Cipher:     packageCipher{Algorithm: "aes-256-gcm", Nonce: base64.StdEncoding.EncodeToString(nonce)},
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	}, "", "    ")
}

// errPackageOpen: wrong password or a tampered package.
var errPackageOpen = errors.New("glorian admin: connection package cannot be opened")

// openPackage is the reverse of sealPackage (tests and the fixture check).
func openPackage(raw []byte, password string) ([]byte, error) {
	var env packageEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	if env.Format != PackageFormat || env.Version != PackageVersion || env.KDF.Algorithm != "argon2id" || env.KDF.Parallelism != 1 {
		return nil, fmt.Errorf("glorian admin: unsupported package")
	}
	salt, err := base64.StdEncoding.DecodeString(env.KDF.Salt)
	if err != nil {
		return nil, err
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Cipher.Nonce)
	if err != nil {
		return nil, err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err != nil {
		return nil, err
	}
	key := argon2.IDKey([]byte(password), salt, env.KDF.Iterations, env.KDF.MemoryKiB, 1, 32)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, []byte(PackageAAD))
	if err != nil {
		return nil, errPackageOpen
	}
	return plain, nil
}

// newAPIKey is 32 random bytes, base64url without padding (43 characters).
func newAPIKey() (string, error) {
	b := make([]byte, apiKeyBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

type packageSettings struct {
	baseURL     string
	permissions packagePermissions
	rateLimit   int
	ipAllowlist []string
	ttl         time.Duration
}

func (in PackageInput) validate() (packageSettings, error) {
	baseURL, _, _, err := PutInput{BaseURL: in.BaseURL}.validate()
	if err != nil {
		return packageSettings{}, err
	}
	n := utf8.RuneCountInString(in.Password)
	switch {
	case strings.TrimSpace(in.Password) == "":
		return packageSettings{}, invalid("password", "is required")
	case n < MinPackagePassword:
		return packageSettings{}, invalid("password", fmt.Sprintf("must be at least %d characters", MinPackagePassword))
	case n > MaxPackagePassword:
		return packageSettings{}, invalid("password", fmt.Sprintf("must be at most %d characters", MaxPackagePassword))
	}
	out := packageSettings{
		baseURL:     strings.TrimRight(baseURL, "/"),
		permissions: packagePermissions{DealerWrite: false, OrderStatusActions: true},
		rateLimit:   DefaultRateLimit,
		ipAllowlist: []string{},
		ttl:         DefaultPackageTTL * time.Hour,
	}
	if in.DealerWrite != nil {
		out.permissions.DealerWrite = *in.DealerWrite
	}
	if in.OrderStatusActions != nil {
		out.permissions.OrderStatusActions = *in.OrderStatusActions
	}
	if in.RateLimitPerMinute != nil {
		if *in.RateLimitPerMinute < 1 || *in.RateLimitPerMinute > MaxRateLimit {
			return packageSettings{}, invalid("rate_limit_per_minute", fmt.Sprintf("must be between 1 and %d", MaxRateLimit))
		}
		out.rateLimit = *in.RateLimitPerMinute
	}
	if len(in.IPAllowlist) > MaxIPAllowlist {
		return packageSettings{}, invalid("ip_allowlist", fmt.Sprintf("must have at most %d addresses", MaxIPAllowlist))
	}
	seen := map[string]bool{}
	for _, raw := range in.IPAllowlist {
		ip := net.ParseIP(strings.TrimSpace(raw))
		if ip == nil {
			return packageSettings{}, invalid("ip_allowlist", fmt.Sprintf("%q is not an IP address", raw))
		}
		s := ip.String()
		if !seen[s] {
			seen[s] = true
			out.ipAllowlist = append(out.ipAllowlist, s)
		}
	}
	if in.TTLHours != nil {
		if *in.TTLHours < 1 || *in.TTLHours > MaxPackageTTL {
			return packageSettings{}, invalid("ttl_hours", fmt.Sprintf("must be between 1 and %d", MaxPackageTTL))
		}
		out.ttl = time.Duration(*in.TTLHours) * time.Hour
	}
	return out, nil
}

// CreatePackage issues a new hub API key and returns it sealed in a
// connection package. A connection that already has a key keeps using it
// until the hub accepts the new one (the client's 401 fallback promotes
// it); a new or keyless connection gets the key as its active key. The
// connection is created inactive when missing. issuerURL is the
// ecosystem's public address shown in Glorian's preview.
func (s *Service) CreatePackage(ctx context.Context, brand db.Brand, in PackageInput, issuerURL string) (PackageResult, error) {
	set, err := in.validate()
	if err != nil {
		return PackageResult{}, err
	}
	conn, err := s.connection(ctx, brand.ID)
	if errors.Is(err, ErrNotConfigured) {
		active := false
		if _, _, err = s.Put(ctx, brand, PutInput{BaseURL: set.baseURL, Active: &active}); err != nil {
			return PackageResult{}, err
		}
		conn, err = s.connection(ctx, brand.ID)
	}
	if err != nil {
		return PackageResult{}, err
	}

	apiKey, err := newAPIKey()
	if err != nil {
		return PackageResult{}, err
	}
	issuedAt := s.now().UTC().Truncate(time.Second)
	expiresAt := issuedAt.Add(set.ttl)
	rotate := conn.ApiKeyEnc != ""

	plain, err := json.Marshal(packagePayload{
		Version:            PackageVersion,
		ConnectionID:       conn.Uuid.String(),
		Issuer:             packageIssuer{Name: packageIssuerName, URL: strings.TrimRight(issuerURL, "/")},
		HubURL:             set.baseURL,
		APIKey:             apiKey,
		Permissions:        set.permissions,
		RateLimitPerMinute: set.rateLimit,
		IPAllowlist:        set.ipAllowlist,
		CreatedAt:          issuedAt.Format(time.RFC3339),
		ExpiresAt:          expiresAt.Format(time.RFC3339),
	})
	if err != nil {
		return PackageResult{}, err
	}
	sealed, err := sealPackage(plain, in.Password, nil, nil)
	if err != nil {
		return PackageResult{}, fmt.Errorf("glorian admin: seal package: %w", err)
	}

	row, err := s.store.PackageKey(ctx, conn.ID, set.baseURL, apiKey, rotate, issuedAt, expiresAt)
	if err != nil {
		return PackageResult{}, err
	}
	view, err := s.view(ctx, row)
	if err != nil {
		return PackageResult{}, err
	}
	return PackageResult{
		Filename:       "glorian-baglanti-" + issuedAt.Format(packageFilenameStamp) + ".json",
		Package:        sealed,
		KeyFingerprint: KeyFingerprint(apiKey),
		Rotated:        rotate,
		ExpiresAt:      expiresAt,
		Connection:     view,
	}, nil
}
