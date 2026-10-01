package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
)

// ErrMissingPIIKey is returned when CUSTOMER_PII_KEY is empty. There is no
// built-in fallback key: customer national ids and tax numbers are never
// encrypted with a value that is public in the repository.
var ErrMissingPIIKey = errors.New("crypto: CUSTOMER_PII_KEY is not set (generate one with: openssl rand -base64 32, or make dev-pii-key)")

// piiVersion prefixes every sealed value so the key can be rotated later.
const piiVersion byte = 1

// PIIBox encrypts customer identity numbers (TC kimlik no, tax number) with
// AES-256-GCM for BYTEA columns (TEC-159, TEC-100 decision 3). The sealed
// form is version(1) | nonce(12) | ciphertext+tag. The additional data binds
// a ciphertext to its column and row, so a value copied to another customer
// does not decrypt.
type PIIBox struct {
	gcm cipher.AEAD
}

// NewPIIBox builds a box from a base64-encoded 32-byte key (CUSTOMER_PII_KEY).
func NewPIIBox(keyBase64 string) (*PIIBox, error) {
	keyBase64 = strings.TrimSpace(keyBase64)
	if keyBase64 == "" {
		return nil, ErrMissingPIIKey
	}
	key, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("%w: CUSTOMER_PII_KEY must be base64 of 32 bytes", ErrInvalidKey)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("piibox: cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("piibox: gcm: %w", err)
	}
	return &PIIBox{gcm: gcm}, nil
}

// PIIAAD is the additional data for a customer field, e.g.
// PIIAAD("customer_profiles.national_id", userID).
func PIIAAD(field string, userID int64) []byte {
	return []byte(field + ":" + strconv.FormatInt(userID, 10))
}

// Seal encrypts plaintext; an empty plaintext yields nil (column NULL).
func (b *PIIBox) Seal(plaintext string, aad []byte) ([]byte, error) {
	if plaintext == "" {
		return nil, nil
	}
	out := make([]byte, 1+b.gcm.NonceSize(), 1+b.gcm.NonceSize()+len(plaintext)+b.gcm.Overhead())
	out[0] = piiVersion
	if _, err := io.ReadFull(rand.Reader, out[1:]); err != nil {
		return nil, fmt.Errorf("piibox: nonce: %w", err)
	}
	return b.gcm.Seal(out, out[1:], []byte(plaintext), aad), nil
}

// Open reverses Seal; nil or empty input yields "".
func (b *PIIBox) Open(sealed, aad []byte) (string, error) {
	if len(sealed) == 0 {
		return "", nil
	}
	n := b.gcm.NonceSize()
	if sealed[0] != piiVersion || len(sealed) < 1+n+b.gcm.Overhead() {
		return "", ErrInvalidInput
	}
	plain, err := b.gcm.Open(nil, sealed[1:1+n], sealed[1+n:], aad)
	if err != nil {
		return "", ErrInvalidInput
	}
	return string(plain), nil
}

// Last4 is the mask stored next to a sealed identity number: the last four
// letters or digits, upper-cased ("" for an empty value).
func Last4(value string) string {
	var kept []rune
	for _, r := range strings.ToUpper(value) {
		if r < unicode.MaxASCII && (unicode.IsDigit(r) || unicode.IsLetter(r)) {
			kept = append(kept, r)
		}
	}
	if len(kept) > 4 {
		kept = kept[len(kept)-4:]
	}
	return string(kept)
}
