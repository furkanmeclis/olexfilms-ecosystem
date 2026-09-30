package totp

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const (
	// DefaultIssuer is used when app name is empty.
	DefaultIssuer = "App"
	recoveryCount = 10
	recoveryBytes = 4
)

// GenerateSecret creates a new TOTP key for the account.
func GenerateSecret(issuer, accountName string) (*otp.Key, error) {
	if strings.TrimSpace(issuer) == "" {
		issuer = DefaultIssuer
	}
	return totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: accountName,
		Period:      30,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
}

// ValidateCode checks a 6-digit TOTP code against the secret.
func ValidateCode(secret, code string) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	ok, err := totp.ValidateCustom(code, secret, time.Now().UTC(), totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return err == nil && ok
}

// GenerateRecoveryCodes returns plaintext codes and matching SHA-256 hashes.
func GenerateRecoveryCodes() (plain []string, hashes []string, err error) {
	plain = make([]string, 0, recoveryCount)
	hashes = make([]string, 0, recoveryCount)
	for range recoveryCount {
		buf := make([]byte, recoveryBytes)
		if _, err = rand.Read(buf); err != nil {
			return nil, nil, fmt.Errorf("totp: recovery code: %w", err)
		}
		raw := hex.EncodeToString(buf)
		code := raw[:4] + "-" + raw[4:]
		plain = append(plain, code)
		hashes = append(hashes, HashRecoveryCode(code))
	}
	return plain, hashes, nil
}

// HashRecoveryCode normalizes and hashes a recovery code.
func HashRecoveryCode(code string) string {
	normalized := normalizeRecovery(code)
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

// ConsumeRecoveryCode returns remaining hashes when code matches, or ok=false.
func ConsumeRecoveryCode(hashes []string, code string) (remaining []string, ok bool) {
	want := HashRecoveryCode(code)
	for i, h := range hashes {
		if h == want {
			out := make([]string, 0, len(hashes)-1)
			out = append(out, hashes[:i]...)
			out = append(out, hashes[i+1:]...)
			return out, true
		}
	}
	return hashes, false
}

func normalizeRecovery(code string) string {
	s := strings.ToLower(strings.TrimSpace(code))
	s = strings.ReplaceAll(s, " ", "")
	return s
}
