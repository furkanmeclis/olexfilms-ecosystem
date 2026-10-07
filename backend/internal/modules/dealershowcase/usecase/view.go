package usecase

import (
	"context"
	"encoding/json"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// OrganizationRef names the showcase owner.
type OrganizationRef struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
	Type string    `json:"type"`
	City string    `json:"city"`
}

// Showcase is the editor view of GET/PUT /v1/showcase.
type Showcase struct {
	UUID                  *uuid.UUID      `json:"uuid"`
	Organization          OrganizationRef `json:"organization"`
	Status                string          `json:"status"`
	Content               json.RawMessage `json:"content"`
	WorkingHours          json.RawMessage `json:"working_hours"`
	SocialLinks           json.RawMessage `json:"social_links"`
	SeoKeywords           []string        `json:"seo_keywords"`
	GooglePlaceID         *string         `json:"google_place_id"`
	GoogleRating          *float64        `json:"google_rating"`
	GoogleReviewCount     *int32          `json:"google_review_count"`
	GoogleRatingSource    *string         `json:"google_rating_source"`
	GoogleRatingUpdatedAt *time.Time      `json:"google_rating_updated_at"`
	PublishedContent      json.RawMessage `json:"published_content"`
	PublishedAt           *time.Time      `json:"published_at"`
	SubmittedAt           *time.Time      `json:"submitted_at"`
	ReviewedAt            *time.Time      `json:"reviewed_at"`
	ReviewNote            *string         `json:"review_note"`
	UpdatedAt             *time.Time      `json:"updated_at"`
	ApprovalRequired      bool            `json:"approval_required"`
	MaxPhotos             int64           `json:"max_photos"`
	Services              []ServiceItem   `json:"services"`
	Photos                []Photo         `json:"photos"`
}

// CategoryRef is the product category of a service.
type CategoryRef struct {
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
}

// ServiceItem is one showcase service (no price, F5 S8).
type ServiceItem struct {
	UUID        uuid.UUID         `json:"uuid"`
	Kind        string            `json:"kind"`
	Category    *CategoryRef      `json:"category"`
	Title       map[string]string `json:"title"`
	Description map[string]string `json:"description"`
	Visible     bool              `json:"visible"`
	SortOrder   int32             `json:"sort_order"`
}

// Photo is one gallery photo of the editor.
type Photo struct {
	UUID      uuid.UUID         `json:"uuid"`
	Mime      string            `json:"mime"`
	SizeBytes int64             `json:"size_bytes"`
	Caption   map[string]string `json:"caption"`
	SortOrder int32             `json:"sort_order"`
	CreatedAt time.Time         `json:"created_at"`
}

func orgRef(o db.Organization) OrganizationRef {
	return OrganizationRef{UUID: o.Uuid, Code: o.Slug, Name: o.Name, Type: o.Type, City: o.City}
}

func (s *Service) view(ctx context.Context, q *db.Queries, o db.Organization, row db.DealerShowcase, exists bool) (Showcase, error) {
	v := Showcase{
		Organization: orgRef(o), Status: row.Status,
		Content: json.RawMessage(`{}`), WorkingHours: json.RawMessage(`{}`), SocialLinks: json.RawMessage(`{}`),
		SeoKeywords: []string{}, PublishedContent: json.RawMessage(`null`),
		ApprovalRequired: s.approvalRequired(ctx), MaxPhotos: s.maxPhotos(ctx),
		Services: []ServiceItem{}, Photos: []Photo{},
	}
	if !exists {
		v.Status = "draft"
		return v, nil
	}
	id := row.Uuid
	v.UUID = &id
	v.Content, v.WorkingHours, v.SocialLinks = row.Content, row.WorkingHours, row.SocialLinks
	if row.SeoKeywords != nil {
		v.SeoKeywords = row.SeoKeywords
	}
	v.GooglePlaceID = textPtr(row.GooglePlaceID)
	v.GoogleRating = numericPtr(row.GoogleRating)
	if row.GoogleReviewCount.Valid {
		n := row.GoogleReviewCount.Int32
		v.GoogleReviewCount = &n
	}
	v.GoogleRatingSource = textPtr(row.GoogleRatingSource)
	v.GoogleRatingUpdatedAt = tsPtr(row.GoogleRatingUpdatedAt)
	if len(row.PublishedContent) > 0 {
		v.PublishedContent = row.PublishedContent
	}
	v.PublishedAt, v.SubmittedAt, v.ReviewedAt = tsPtr(row.PublishedAt), tsPtr(row.SubmittedAt), tsPtr(row.ReviewedAt)
	v.ReviewNote = textPtr(row.ReviewNote)
	v.UpdatedAt = tsPtr(row.UpdatedAt)

	services, err := q.ListDealerShowcaseServices(ctx, row.ID)
	if err != nil {
		return Showcase{}, err
	}
	for _, sv := range services {
		item, err := serviceItem(ctx, q, sv)
		if err != nil {
			return Showcase{}, err
		}
		v.Services = append(v.Services, item)
	}
	photos, err := q.ListDealerShowcasePhotos(ctx, row.ID)
	if err != nil {
		return Showcase{}, err
	}
	for _, p := range photos {
		v.Photos = append(v.Photos, photoView(p))
	}
	return v, nil
}

func serviceItem(ctx context.Context, q *db.Queries, sv db.DealerShowcaseService) (ServiceItem, error) {
	item := ServiceItem{
		UUID: sv.Uuid, Kind: sv.Kind, Title: textMap(sv.Title), Description: textMap(sv.Description),
		Visible: sv.Visible, SortOrder: sv.SortOrder,
	}
	if sv.CategoryID.Valid {
		cat, err := q.GetProductCategory(ctx, db.GetProductCategoryParams{ID: sv.CategoryID.Int64, BrandID: sv.BrandID})
		if err != nil {
			return ServiceItem{}, err
		}
		item.Category = &CategoryRef{UUID: cat.Uuid, Name: cat.Name}
	}
	return item, nil
}

func photoView(p db.DealerShowcasePhoto) Photo {
	return Photo{
		UUID: p.Uuid, Mime: p.Mime, SizeBytes: p.SizeBytes, Caption: textMap(p.Caption),
		SortOrder: p.SortOrder, CreatedAt: p.CreatedAt.Time.UTC(),
	}
}

func textMap(raw []byte) map[string]string {
	out := map[string]string{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

func tsPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

func numericPtr(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return nil
	}
	v := f.Float64
	return &v
}
