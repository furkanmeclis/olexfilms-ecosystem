package legacymobile

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// The old hub envelope (olexfilms RespondsWithJson, docs/mobile-api.md
// "JSON yanıt formatı"):
//
//	{"success": true, "message": "...", "data": {}}
//	{"success": false, "message": "...", "errors": {"field": ["..."]}}
//
// Validation errors are 422 there (Laravel FormRequest). The aliases keep
// that, so the old app's error handling works unchanged; the rest of the
// API answers 400 VALIDATION_ERROR (HANDOFF §6).

// legacySuccess is the old success envelope.
type legacySuccess struct {
	Success bool    `json:"success"`
	Message *string `json:"message"`
	Data    any     `json:"data"`
}

// legacyError is the old error envelope. Code is additive (the new error
// code), so a support log still shows why a call failed; the old app reads
// success, message and errors only.
type legacyError struct {
	Success bool                `json:"success"`
	Message string              `json:"message"`
	Errors  map[string][]string `json:"errors,omitempty"`
	Code    string              `json:"code,omitempty"`
}

// newEnvelope is the part of the new envelope the adapters read.
type newEnvelope struct {
	Success bool                `json:"success"`
	Data    json.RawMessage     `json:"data"`
	Error   *response.ErrorBody `json:"error"`
}

// capture buffers what the wrapped handler writes, so the alias can rewrite
// it before anything reaches the client.
type capture struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newCapture() *capture { return &capture{header: http.Header{}} }

func (c *capture) Header() http.Header { return c.header }

func (c *capture) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
}

func (c *capture) Write(b []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	return c.body.Write(b)
}

func (c *capture) code() int {
	if c.status == 0 {
		return http.StatusOK
	}
	return c.status
}

// copyHeaders forwards the wrapped handler's headers (Retry-After,
// X-Request-ID, ...) except the body framing ones.
func (c *capture) copyHeaders(w http.ResponseWriter) {
	for k, vs := range c.header {
		switch http.CanonicalHeaderKey(k) {
		case "Content-Length", "Content-Type":
			continue
		}
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeSuccess(w http.ResponseWriter, status int, message string, data any) {
	var msg *string
	if message != "" {
		msg = &message
	}
	writeJSON(w, status, legacySuccess{Success: true, Message: msg, Data: data})
}

func writeLegacyError(w http.ResponseWriter, status int, e legacyError) {
	e.Success = false
	writeJSON(w, status, e)
}

// validationFailed is the old 422 with one field error.
func validationFailed(w http.ResponseWriter, field, message string) {
	writeLegacyError(w, http.StatusUnprocessableEntity, legacyError{
		Message: message, Errors: map[string][]string{field: {message}}, Code: response.CodeValidationError,
	})
}

// legacyLocale picks the message language like the old api.locale
// middleware: ?locale=, then Accept-Language; Turkish when neither is set.
func legacyLocale(r *http.Request) string {
	if l := strings.TrimSpace(r.URL.Query().Get("locale")); l != "" {
		return l
	}
	if l := strings.TrimSpace(r.Header.Get("Accept-Language")); l != "" {
		return l
	}
	return "tr"
}

// withLocale makes ?locale= visible to the adapted handlers (status labels
// and other translated values follow it, as on the old hub).
func withLocale(r *http.Request) *http.Request {
	l := strings.TrimSpace(r.URL.Query().Get("locale"))
	if l == "" {
		return r
	}
	r2 := r.Clone(i18n.WithAcceptLanguage(r.Context(), strings.ReplaceAll(l, "_", "-")))
	r2.Header.Set("Accept-Language", strings.ReplaceAll(l, "_", "-"))
	return r2
}

// envelope wraps one alias (its gates included): a success body written in
// the old shape by the adapter passes through; a new-contract error (from a
// gate or the adapted handler) is rewritten into the old error envelope.
func envelope(name string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = withLocale(r)
		c := newCapture()
		next.ServeHTTP(c, r)
		c.copyHeaders(w)
		var env newEnvelope
		if err := json.Unmarshal(c.body.Bytes(), &env); err != nil || env.Error == nil {
			if ct := c.header.Get("Content-Type"); ct != "" {
				w.Header().Set("Content-Type", ct)
			}
			w.WriteHeader(c.code())
			_, _ = w.Write(c.body.Bytes())
			return
		}
		status, e := mapError(name, legacyLocale(r), c.code(), *env.Error)
		writeLegacyError(w, status, e)
	})
}

// mapError turns a new-contract error into the old status and envelope:
//   - 400 VALIDATION_ERROR is the old 422 with errors[field];
//   - wrong credentials on login are the old 422 errors.email
//     (AuthController::login throws a ValidationException);
//   - a disabled account on login is the old 403 account_inactive;
//   - anything else keeps its status, the message and the code.
func mapError(name, locale string, status int, e response.ErrorBody) (int, legacyError) {
	out := legacyError{Message: e.Message, Code: e.Code}
	switch {
	case e.Code == response.CodeValidationError:
		out.Errors = map[string][]string{}
		for _, d := range e.Details {
			field := d.Field
			if field == "" {
				field = "body"
			}
			out.Errors[field] = append(out.Errors[field], d.Message)
		}
		if len(out.Errors) == 0 {
			out.Errors["body"] = []string{e.Message}
		}
		return http.StatusUnprocessableEntity, out
	case name == "login" && e.Code == response.CodeInvalidCredentials:
		msg := message(locale, msgCredentialsMismatch)
		return http.StatusUnprocessableEntity, legacyError{
			Message: msg, Errors: map[string][]string{"email": {msg}}, Code: e.Code,
		}
	case name == "login" && status == http.StatusForbidden && e.Code == response.CodeForbidden:
		out.Message = message(locale, msgAccountInactive)
	}
	return status, out
}
