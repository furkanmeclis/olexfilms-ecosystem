package middleware

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
)

// ResolveLocale stores the request Accept-Language header on the context so
// i18n.Resolve can use it after the stored user/org/center preferences
// (K10). Handlers without a user read i18n.FromContext.
func ResolveLocale(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h := r.Header.Get("Accept-Language"); h != "" {
			r = r.WithContext(i18n.WithAcceptLanguage(r.Context(), h))
		}
		next.ServeHTTP(w, r)
	})
}
