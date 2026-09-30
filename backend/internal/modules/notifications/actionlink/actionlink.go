package actionlink

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const DefaultTTL = 7 * 24 * time.Hour

// Sign returns HMAC-SHA256 hex for a notification action.
func Sign(secret []byte, id uuid.UUID, dest string, exp int64) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(canonical(id, dest, exp)))
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify reports whether sig matches dest + exp for id.
func Verify(secret []byte, id uuid.UUID, dest, sig string, exp int64) bool {
	if len(secret) == 0 || sig == "" || dest == "" {
		return false
	}
	if exp <= 0 || time.Now().Unix() > exp {
		return false
	}
	want, err := hex.DecodeString(Sign(secret, id, dest, exp))
	if err != nil {
		return false
	}
	got, err := hex.DecodeString(sig)
	if err != nil || len(got) != len(want) {
		return false
	}
	return hmac.Equal(got, want)
}

// URL builds `/v1/notifications/{uuid}/action?exp=&sig=`.
func URL(secret []byte, id uuid.UUID, dest string, ttl time.Duration) string {
	if len(secret) == 0 || dest == "" {
		return ""
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	exp := time.Now().Add(ttl).Unix()
	q := url.Values{}
	q.Set("exp", strconv.FormatInt(exp, 10))
	q.Set("sig", Sign(secret, id, dest, exp))
	return fmt.Sprintf("/v1/notifications/%s/action?%s", id.String(), q.Encode())
}

func canonical(id uuid.UUID, dest string, exp int64) string {
	return id.String() + "\n" + dest + "\n" + strconv.FormatInt(exp, 10)
}
