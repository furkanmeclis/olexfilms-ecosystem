// Package shorturls is the short URL service (TEC-249, F2-04d): internal
// frontend links shortened to /s/{token}.
package shorturls

import (
	"net/http"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/shorturls/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/shorturls/usecase"
)

// New builds the service over the generated queries.
func New(q *db.Queries) *usecase.Service { return usecase.New(q) }

// NewLinker is the template helper (WhatsApp, SMS, notifications):
// Link stores a short URL and returns {frontendURL}/s/{token}.
func NewLinker(q *db.Queries, frontendURL string) *usecase.Linker {
	return usecase.NewLinker(usecase.New(q), frontendURL)
}

// RegisterPublicRoutes mounts the public resolver behind the frontend
// route /s/{token}: no authentication, limit hits per window per client IP.
func RegisterPublicRoutes(mux *http.ServeMux, svc *usecase.Service, limiter handler.Limiter, limit int, window time.Duration) {
	h := handler.NewPublic(svc, limiter, limit, window)
	mux.HandleFunc("GET /v1/public/short-urls/{token}", h.Get)
}
