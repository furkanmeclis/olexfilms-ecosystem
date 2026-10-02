package warranty

import (
	"net/http"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/warranty/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ratelimit"
)

// RegisterPublicRoutes mounts the public warranty lookup (TEC-189). The
// frontend page /garanti/{public_code} reads it; no authentication, a per-IP
// limit of limit hits per window.
func RegisterPublicRoutes(mux *http.ServeMux, q *db.Queries, limiter *ratelimit.Limiter, limit int, window time.Duration) {
	h := handler.NewPublic(usecase.NewPublicLookup(q), limiter, limit, window)
	mux.HandleFunc("GET /v1/public/warranties/{public_code}", h.Get)
}
