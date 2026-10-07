package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
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
	dealer, err := h.svc.PublicDealerByCode(r.Context(), brand.ID, r.PathValue("code"), publicLocale(r))
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

// publicLocale is the requested showcase locale (TEC-467): ?locale=, then
// the Accept-Language header; empty falls back to the organization locale.
func publicLocale(r *http.Request) string {
	if l, ok := i18n.Parse(r.URL.Query().Get("locale")); ok {
		return string(l)
	}
	if l, ok := i18n.ParseAcceptLanguage(r.Header.Get("Accept-Language")); ok {
		return string(l)
	}
	return ""
}

// TEC-251: per-IP limit of the public dealer code list (the sitemap reads
// it from the frontend server).
const (
	dealerCodesRateAction = "dealer_codes"
	dealerCodesRateLimit  = 30
	dealerCodesRateWindow = time.Minute
)

// PublicDealerCodes serves GET /v1/public/dealers (TEC-251): code and last
// change of the request brand's active dealers and distributors, for the
// sitemap's `/bayi/{code}` URLs. No other field is exposed.
func (h *Handler) PublicDealerCodes(w http.ResponseWriter, r *http.Request) {
	if h.limiter != nil {
		if ok, retry := h.limiter.Allow(r.Context(), dealerCodesRateAction, sessionMeta(r).IP, dealerCodesRateLimit, dealerCodesRateWindow); !ok {
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
	items, err := h.svc.PublicDealerCodes(r.Context(), brand.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}
