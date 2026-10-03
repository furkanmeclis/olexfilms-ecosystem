package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// TEC-250: per-IP limit of the public dealer showcase lookup.
const (
	showcaseRateAction = "dealer_showcase"
	showcaseRateLimit  = 120
	showcaseRateWindow = time.Minute
)

// PublicDealerShowcase serves GET /v1/public/dealers/{code} (TEC-250): the
// showcase (name, logo, address, map position, WhatsApp) of an active
// dealer or distributor of the request brand. Anything else is 404.
func (h *Handler) PublicDealerShowcase(w http.ResponseWriter, r *http.Request) {
	if h.limiter != nil {
		if ok, retry := h.limiter.Allow(r.Context(), showcaseRateAction, sessionMeta(r).IP, showcaseRateLimit, showcaseRateWindow); !ok {
			if retry > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			}
			response.TooManyRequests(w, r, "Too many requests. Try again later.")
			return
		}
	}
	brand, err := orgusecase.RequestBrand(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	dealer, err := h.svc.PublicDealerByCode(r.Context(), brand.ID, r.PathValue("code"))
	if err != nil {
		if errors.Is(err, orgusecase.ErrNotFound) {
			response.NotFound(w, r, "Dealer was not found")
			return
		}
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, dealer)
}
