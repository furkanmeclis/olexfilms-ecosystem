package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// TEC-240: per-IP limit of the public nearby-dealers lookup.
const (
	nearbyRateAction = "dealers_nearby"
	nearbyRateLimit  = 60
	nearbyRateWindow = time.Minute
)

// PublicNearbyDealers serves GET /v1/public/dealers/nearby?lat=&lng=&radius_km=
// (TEC-240): the request brand's active dealers and distributors with a map
// position, nearest first (haversine, at most 50).
func (h *Handler) PublicNearbyDealers(w http.ResponseWriter, r *http.Request) {
	if h.limiter != nil {
		if ok, retry := h.limiter.Allow(r.Context(), nearbyRateAction, sessionMeta(r).IP, nearbyRateLimit, nearbyRateWindow); !ok {
			if retry > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			}
			response.TooManyRequests(w, r, "Too many requests. Try again later.")
			return
		}
	}
	in, details := parseNearbyQuery(r)
	if len(details) > 0 {
		response.ErrorWithDetails(w, r, http.StatusBadRequest, response.CodeValidationError, "Invalid nearby query", details)
		return
	}
	brand, err := orgusecase.RequestBrand(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, err := h.svc.NearbyDealers(r.Context(), brand.ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// parseNearbyQuery validates lat (-90..90), lng (-180..180) and the
// optional radius_km (0 < r <= 1000, default 100).
func parseNearbyQuery(r *http.Request) (orgusecase.NearbyInput, []response.Detail) {
	q := r.URL.Query()
	var details []response.Detail
	in := orgusecase.NearbyInput{RadiusKm: orgusecase.NearbyDefaultRadiusKm}
	lat, err := strconv.ParseFloat(strings.TrimSpace(q.Get("lat")), 64)
	if err != nil || !orgusecase.ValidLatitude(lat) {
		details = append(details, response.Detail{Field: "lat", Message: "lat must be a number between -90 and 90", Code: "invalid"})
	}
	lng, err := strconv.ParseFloat(strings.TrimSpace(q.Get("lng")), 64)
	if err != nil || !orgusecase.ValidLongitude(lng) {
		details = append(details, response.Detail{Field: "lng", Message: "lng must be a number between -180 and 180", Code: "invalid"})
	}
	if raw := strings.TrimSpace(q.Get("radius_km")); raw != "" {
		radius, err := strconv.ParseFloat(raw, 64)
		if err != nil || !(radius > 0 && radius <= orgusecase.NearbyMaxRadiusKm) {
			details = append(details, response.Detail{Field: "radius_km", Message: "radius_km must be greater than 0 and at most 1000", Code: "invalid"})
		} else {
			in.RadiusKm = radius
		}
	}
	in.Lat, in.Lng = lat, lng
	return in, details
}
