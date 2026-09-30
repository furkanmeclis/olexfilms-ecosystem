package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

var (
	ErrInvalidKey   = errors.New("encryption key must be 32 bytes")
	ErrInvalidInput = errors.New("invalid encrypted payload")
)

// SecretBox encrypts and decrypts strings with AES-256-GCM.
type SecretBox struct {
	gcm cipher.AEAD
}

// NewSecretBox creates a box from a 32-byte key (raw or base64-encoded).
func NewSecretBox(keyMaterial string) (*SecretBox, error) {
	key, err := decodeKey(keyMaterial)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secretbox: cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secretbox: gcm: %w", err)
	}
	return &SecretBox{gcm: gcm}, nil
}

func decodeKey(keyMaterial string) ([]byte, error) {
	if keyMaterial == "" {
		return nil, ErrInvalidKey
	}
	if raw, err := base64.StdEncoding.DecodeString(keyMaterial); err == nil && len(raw) == 32 {
		return raw, nil
	}
	if len(keyMaterial) == 32 {
		return []byte(keyMaterial), nil
	}
	return nil, ErrInvalidKey
}

// Encrypt returns base64(nonce|ciphertext).
func (s *SecretBox) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	nonce := make([]byte, s.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("secretbox: nonce: %w", err)
	}
	sealed := s.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt.
func (s *SecretBox) Decrypt(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	sealed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrInvalidInput
	}
	nonceSize := s.gcm.NonceSize()
	if len(sealed) < nonceSize {
		return "", ErrInvalidInput
	}
	nonce, ciphertext := sealed[:nonceSize], sealed[nonceSize:]
	plain, err := s.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", ErrInvalidInput
	}
	return string(plain), nil
}
