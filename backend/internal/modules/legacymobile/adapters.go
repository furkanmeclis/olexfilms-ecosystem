package legacymobile

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

// Store is the read access the adapters need for the old contract's
// integer ids (the old app keys every record by an integer id and sends it
// back in paths and filters; the new contract only carries uuids). The ids
// are this database's internal ids: stable, and the same for every call, so
// the old app can round-trip them. *db.Queries implements it.
type Store interface {
	GetUserByUUID(ctx context.Context, id uuid.UUID) (db.User, error)
	GetUserByID(ctx context.Context, id int64) (db.User, error)
	GetOrganizationByUUID(ctx context.Context, id uuid.UUID) (db.Organization, error)
	GetServiceByUUID(ctx context.Context, arg db.GetServiceByUUIDParams) (db.Service, error)
	GetService(ctx context.Context, arg db.GetServiceParams) (db.Service, error)
	ListServiceItems(ctx context.Context, serviceID int64) ([]db.ServiceItem, error)
	ListServiceImages(ctx context.Context, serviceID int64) ([]db.ServiceImage, error)
	GetWarrantyByServiceItem(ctx context.Context, serviceItemID int64) (db.Warranty, error)
	FindMeasurementResultByKeys(ctx context.Context, arg db.FindMeasurementResultByKeysParams) (db.MeasurementResult, error)
}

// adapters holds the adapted handlers and what the adapters need around
// them. Each adapter rewrites the old request into the new one, calls the
// adapted handler and writes its success in the old shape; an error answer
// is passed on untouched and the envelope wrapper rewrites it.
type adapters struct {
	h     Handlers
	store Store // nil: integer ids are 0 (unit tests without a database)
	// authn authenticates the login's internal follow-up calls
	// (organization switch, me) with the token the login just issued.
	authn func(http.Handler) http.Handler
}

// run calls fn and returns what it wrote.
func run(fn http.Handler, r *http.Request) *capture {
	c := newCapture()
	fn.ServeHTTP(c, r)
	return c
}

// passThrough forwards a captured answer as is (an error the envelope
// wrapper rewrites).
func passThrough(w http.ResponseWriter, c *capture) {
	c.copyHeaders(w)
	if ct := c.header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(c.code())
	_, _ = w.Write(c.body.Bytes())
}

// ok reports a 2xx answer and decodes its envelope data into dst.
func ok(c *capture, dst any) bool {
	if c.code() < 200 || c.code() > 299 {
		return false
	}
	var env newEnvelope
	if err := json.Unmarshal(c.body.Bytes(), &env); err != nil || !env.Success {
		return false
	}
	if dst == nil {
		return true
	}
	return json.Unmarshal(env.Data, dst) == nil
}

// withJSON clones r with body as its JSON body.
func withJSON(r *http.Request, body any) *http.Request {
	b, _ := json.Marshal(body)
	r2 := r.Clone(r.Context())
	r2.Body = io.NopCloser(bytes.NewReader(b))
	r2.ContentLength = int64(len(b))
	r2.Header.Set("Content-Type", "application/json")
	return r2
}

// withBearer clones r (no body) with token as its Bearer.
func withBearer(r *http.Request, method, token string, body any) *http.Request {
	r2 := withJSON(r, body)
	r2.Method = method
	r2.Header.Set("Authorization", "Bearer "+token)
	return r2
}

// readObject reads the request body as a JSON object (an empty body is an
// empty object, as Laravel reads it).
func readObject(r *http.Request) (map[string]json.RawMessage, bool) {
	b, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 8<<20))
	if err != nil {
		return nil, false
	}
	out := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(b)) == 0 {
		return out, true
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, false
	}
	return out, true
}

// str reads a string field; numbers are taken as their text (Laravel casts
// form values the same way). Missing or null is "".
func str(obj map[string]json.RawMessage, key string) string {
	raw, found := obj[key]
	if !found {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String()
	}
	return ""
}

// platformFromUA guesses the device platform for the login (the old login
// body has no platform; the new session requires ios or android).
func platformFromUA(ua string) string {
	l := strings.ToLower(ua)
	for _, k := range []string{"iphone", "ipad", "ios", "darwin", "cfnetwork"} {
		if strings.Contains(l, k) {
			return "ios"
		}
	}
	return "android"
}
