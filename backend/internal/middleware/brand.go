package middleware

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// CodeBrandMismatch is returned when an organization does not belong to the
// brand of the domain the request was made on (K1, K20).
const CodeBrandMismatch = response.CodeBrandMismatch

// ResolveBrand puts the request brand on the context:
// X-Forwarded-Host (set by the BFF) -> Host -> default brand.
// When the catalog cannot be loaded the request continues without a brand;
// brand-scoped handlers then fail closed.
func ResolveBrand(resolver *brandctx.Resolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if resolver == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := r.Header.Get("X-Forwarded-Host")
			if brandctx.NormalizeHost(host) == "" {
				host = r.Host
			}
			if b, ok, err := resolver.Resolve(r.Context(), host); err == nil && ok {
				r = r.WithContext(brandctx.WithBrand(r.Context(), b))
			}
			next.ServeHTTP(w, r)
		})
	}
}
