package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/config"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
)

// TEC-469 (F5-01d): PUT /v1/showcase/google-rating. With a Places key set
// and a place id the rating belongs to the worker (409); out-of-range
// values are 422; without a place id the owner enters it by hand. No
// Places call is made here (the key only marks Places as configured).
func TestIntegrationDealerShowcaseGoogleRating(t *testing.T) {
	it := newIntegrationWithDeps(t, func(c *config.Config) { c.Places.APIKey = "test-key" }, nil)
	ctx := context.Background()
	center := it.brandCenter("olex")
	dist := it.org("grdist", "distributor", center)
	dealer := it.org("grdealer", "dealer", dist)
	for _, o := range []db.Organization{dist, dealer} {
		id := o.ID
		t.Cleanup(func() {
			_, _ = it.pool.Exec(context.Background(), "DELETE FROM dealer_showcases WHERE organization_id = $1", id)
			_, _ = it.pool.Exec(context.Background(), "DELETE FROM module_flags WHERE organization_id = $1", id)
		})
	}
	owner, pw := it.user("growner")
	it.member(dealer, owner, "owner")
	admin, _ := it.user("gradmin")
	ownerTok := it.loginOrg(owner, pw, dealer)
	if _, err := it.srv.features.SetByAdmin(ctx, admin.ID, dealer.ID, features.ModuleDealerShowcase, true); err != nil {
		t.Fatal(err)
	}
	type view struct {
		GoogleRating        *float64 `json:"google_rating"`
		GoogleReviewCount   *int     `json:"google_review_count"`
		GoogleRatingSource  *string  `json:"google_rating_source"`
		PlacesConfigured    bool     `json:"places_configured"`
		ManualRatingAllowed bool     `json:"manual_rating_allowed"`
	}
	rating := func(body map[string]any, wantCode int, wantErr string) view {
		t.Helper()
		code, env := it.do("PUT", "/v1/showcase/google-rating", hostOlex, ownerTok, body)
		if code != wantCode || (wantErr != "" && errCode(env) != wantErr) {
			t.Fatalf("PUT google-rating %v: %d %s, want %d %s", body, code, errCode(env), wantCode, wantErr)
		}
		var v view
		_ = json.Unmarshal(env.Data, &v)
		return v
	}

	// No place id yet: manual entry is accepted even with Places configured.
	v := rating(map[string]any{"rating": 4.5, "review_count": 12}, http.StatusOK, "")
	if v.GoogleRating == nil || *v.GoogleRating != 4.5 || v.GoogleRatingSource == nil || *v.GoogleRatingSource != "manual" ||
		!v.PlacesConfigured || !v.ManualRatingAllowed {
		t.Fatalf("manual entry: %+v", v)
	}
	// Out of range: 422 SHOWCASE_RATING_OUT_OF_RANGE.
	rating(map[string]any{"rating": 5.5, "review_count": 1}, http.StatusUnprocessableEntity, "SHOWCASE_RATING_OUT_OF_RANGE")
	rating(map[string]any{"rating": 0.5, "review_count": 1}, http.StatusUnprocessableEntity, "SHOWCASE_RATING_OUT_OF_RANGE")
	rating(map[string]any{"rating": 4, "review_count": -3}, http.StatusUnprocessableEntity, "SHOWCASE_RATING_OUT_OF_RANGE")
	// Shape errors stay 400.
	rating(map[string]any{"rating": 4}, http.StatusBadRequest, "VALIDATION_ERROR")
	rating(map[string]any{"rating": "x", "review_count": 1}, http.StatusBadRequest, "VALIDATION_ERROR")

	// With a place id the Places worker owns the rating: 409.
	if code, env := it.do("PUT", "/v1/showcase", hostOlex, ownerTok, map[string]any{
		"content": map[string]any{}, "working_hours": map[string]any{}, "social_links": map[string]any{},
		"seo_keywords": []string{}, "google_place_id": "ChIJintegration469",
	}); code != http.StatusOK {
		t.Fatalf("save place id: %d %s", code, errCode(env))
	}
	rating(map[string]any{"rating": 4.0, "review_count": 3}, http.StatusConflict, "SHOWCASE_RATING_MANAGED_BY_PLACES")
	code, env := it.do("GET", "/v1/showcase", hostOlex, ownerTok, nil)
	_ = json.Unmarshal(env.Data, &v)
	if code != http.StatusOK || v.ManualRatingAllowed || v.GoogleRating == nil || *v.GoogleRating != 4.5 {
		t.Fatalf("view after 409: %d %+v", code, v)
	}
}
