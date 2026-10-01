package errtrack

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/getsentry/sentry-go"
)

func init() {
	// response.InternalErr runs deep in the handler chain where the request
	// context already holds the organization scope and principal.
	response.ServerErrorHook = captureServerError
}

// applyContextTags copies request_id, organization_id, brand_id and the user
// id (never e-mail) from ctx onto scope.
func applyContextTags(ctx context.Context, scope *sentry.Scope) {
	if id := response.RequestIDFunc(ctx); id != "" {
		scope.SetTag(TagRequestID, id)
	}
	if s, ok := orgctx.ScopeFrom(ctx); ok {
		scope.SetTag(TagOrganizationID, s.UUID.String())
		if s.BrandID != 0 {
			scope.SetTag(TagBrandID, strconv.FormatInt(s.BrandID, 10))
		}
	}
	if p, ok := authctx.PrincipalFrom(ctx); ok {
		scope.SetUser(sentry.User{ID: p.UserID.String()})
		if _, hasOrg := orgctx.ScopeFrom(ctx); !hasOrg && p.OrganizationUUID != nil {
			scope.SetTag(TagOrganizationID, p.OrganizationUUID.String())
		}
	}
}

func requestModule(r *http.Request) Module {
	if m, ok := ModuleFrom(r.Context()); ok {
		return m
	}
	return ModuleFromPath(r.URL.Path)
}

func captureServerError(r *http.Request, err error) {
	if !Enabled() || r == nil {
		return
	}
	Capture(r.Context(), requestModule(r), err, nil)
}

// Middleware gives each request its own hub (so scope data never leaks
// between requests) and records method + path (no query, headers, body or
// cookies). It belongs inside middleware.RequestID so the request id is
// already on the context. No-op when the SDK is disabled.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !Enabled() {
			next.ServeHTTP(w, r)
			return
		}
		hub := sentry.CurrentHub().Clone()
		scope := hub.Scope()
		scope.SetRequest(&http.Request{Method: r.Method, URL: r.URL, Host: r.Host, Header: http.Header{}})
		scope.SetTag(TagModule, string(ModuleFromPath(r.URL.Path)))
		if id := response.RequestIDFunc(r.Context()); id != "" {
			scope.SetTag(TagRequestID, id)
		}
		next.ServeHTTP(w, r.WithContext(sentry.SetHubOnContext(r.Context(), hub)))
	})
}

// Recover turns a handler panic into a reported event and a 500 response
// (instead of net/http dropping the connection). http.ErrAbortHandler is
// re-panicked as net/http expects. Works with the SDK disabled too.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(v)
				}
				perr := &PanicError{Value: v}
				log.ErrorContext(r.Context(), "http_handler_panic",
					"method", r.Method, "path", r.URL.Path, "panic", perr.Error())
				// InternalErr records the error for ServerErrors and reports
				// it through ServerErrorHook (with the hub from Middleware).
				response.InternalErr(w, r, perr, "Internal server error")
			}()
			next.ServeHTTP(w, r)
		})
	}
}
