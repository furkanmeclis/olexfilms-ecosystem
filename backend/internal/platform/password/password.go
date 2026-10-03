package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

const (
	memory      = 64 * 1024
	iterations  = 3
	parallelism = 2
	saltLength  = 16
	keyLength   = 32
)

var ErrInvalidPassword = errors.New("password does not meet policy")

// ValidatePassword enforces the minimum password policy.
func ValidatePassword(value string) error {
	if len(value) < 8 {
		return fmt.Errorf("%w: must be at least 8 characters", ErrInvalidPassword)
	}

	var hasLower, hasUpper, hasDigit bool
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	if !hasLower || !hasUpper || !hasDigit {
		return fmt.Errorf("%w: must include upper, lower, and digit characters", ErrInvalidPassword)
	}
	return nil
}

// Hash validates and hashes a password using Argon2id.
func Hash(value string) (string, error) {
	if err := ValidatePassword(value); err != nil {
		return "", err
	}
	return hashArgon2(value)
}

func hashArgon2(value string) (string, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("password: generate salt: %w", err)
	}
	hash := argon2.IDKey([]byte(value), salt, iterations, memory, parallelism, keyLength)
	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		memory,
		iterations,
		parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// ResetRequired is stored as password_hash when a migrated account has no
// usable hash (TEC-254). It never verifies, so the user signs in with OTP or
// the password reset flow and sets a new password.
const ResetRequired = "!reset-required"

// IsBcrypt reports whether encoded is a well-formed bcrypt hash ($2a$, $2b$
// or Laravel's $2y$). Legacy hub accounts keep such hashes until their next
// successful login (TEC-254).
func IsBcrypt(encoded string) bool {
	if len(encoded) < 4 || encoded[0] != '$' || encoded[1] != '2' || encoded[3] != '$' {
		return false
	}
	switch encoded[2] {
	case 'a', 'b', 'y':
	default:
		return false
	}
	_, err := bcrypt.Cost([]byte(encoded))
	return err == nil
}

// NeedsRehash reports whether a verified hash should be replaced with the
// current Argon2id format (legacy bcrypt hashes).
func NeedsRehash(encoded string) bool { return IsBcrypt(encoded) }

// Rehash hashes value with Argon2id without the policy check. It is only for
// upgrading the stored hash of a password that has just been verified: a
// legacy password that predates the policy must keep working.
func Rehash(value string) (string, error) { return hashArgon2(value) }

// Verify checks a plaintext password against an Argon2id hash, or against a
// legacy bcrypt hash migrated from the hub (TEC-254). ResetRequired never
// verifies.
func Verify(encoded, value string) (bool, error) {
	if encoded == ResetRequired {
		return false, nil
	}
	if IsBcrypt(encoded) {
		err := bcrypt.CompareHashAndPassword([]byte(encoded), []byte(value))
		if err == nil {
			return true, nil
		}
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return false, nil
		}
		return false, fmt.Errorf("password: bcrypt: %w", err)
	}
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, errors.New("password: invalid hash format")
	}
	var mem uint32
	var iter uint32
	var par uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &iter, &par); err != nil {
		return false, fmt.Errorf("password: parse hash parameters: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("password: decode salt: %w", err)
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("password: decode hash: %w", err)
	}
	actual := argon2.IDKey([]byte(value), salt, iter, mem, par, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}
