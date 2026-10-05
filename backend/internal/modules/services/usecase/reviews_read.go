package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type ServiceReviewListItem struct {
	UUID           uuid.UUID           `json:"uuid"`
	ServiceUUID    uuid.UUID           `json:"service_uuid"`
	ServiceNo      string              `json:"service_no"`
	Plate          *string             `json:"plate"`
	PlatformRating int                 `json:"platform_rating"`
	ProductRating  int                 `json:"product_rating"`
	Comment        *string             `json:"comment"`
	IsAnonymous    bool                `json:"is_anonymous"`
	Source         string              `json:"source"`
	Customer       *ReviewCustomerView `json:"customer"`
	Answers        []ReviewAnswerView  `json:"answers"`
	CreatedAt      time.Time           `json:"created_at"`
}

type ServiceReviewFilter struct {
	DealerUUID, ProductUUID *uuid.UUID
	MinRating, MaxRating    *int
	CreatedFrom, CreatedTo  *time.Time
	Limit, Offset           int32
}

type ReviewCustomerView struct {
	UUID  uuid.UUID `json:"uuid"`
	Name  string    `json:"name"`
	Phone *string   `json:"phone"`
}

type ReviewDealerStat struct {
	DealerUUID    uuid.UUID `json:"dealer_uuid"`
	DealerName    string    `json:"dealer_name"`
	ReviewCount   int64     `json:"review_count"`
	AverageRating float64   `json:"average_rating"`
}

type ReviewProductStat struct {
	ProductUUID   uuid.UUID `json:"product_uuid"`
	SKU           string    `json:"sku"`
	ProductName   string    `json:"product_name"`
	ReviewCount   int64     `json:"review_count"`
	AverageRating float64   `json:"average_rating"`
}

func (s *Service) ListServiceReviews(ctx context.Context, c Caller, f ServiceReviewFilter) ([]ServiceReviewListItem, int64, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	orgIDs := reviewScopeOrgIDs(c, rbac.PermReviewsRead)
	arg, err := reviewListParams(c.Org.BrandID, orgIDs, f)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.q.ListServiceReviewsInScope(ctx, arg)
	if err != nil {
		return nil, 0, fmt.Errorf("services: list reviews: %w", err)
	}
	total, err := s.q.CountServiceReviewsInScope(ctx, db.CountServiceReviewsInScopeParams{
		BrandID: arg.BrandID, OrganizationIds: arg.OrganizationIds,
		DealerUuid: arg.DealerUuid, ProductUuid: arg.ProductUuid,
		MinRating: arg.MinRating, MaxRating: arg.MaxRating,
		CreatedFrom: arg.CreatedFrom, CreatedTo: arg.CreatedTo,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("services: count reviews: %w", err)
	}
	out := make([]ServiceReviewListItem, 0, len(rows))
	center := c.Principal.Can(rbac.PermReviewsRead, rbac.ScopeBrand) || c.Principal.Can(rbac.PermReviewsRead, rbac.ScopeAll)
	for _, row := range rows {
		answers, err := s.reviewAnswers(ctx, row.BrandID, row.ID)
		if err != nil {
			return nil, 0, err
		}
		item := ServiceReviewListItem{
			UUID: row.Uuid, ServiceUUID: row.ServiceUuid, ServiceNo: row.ServiceNo,
			Plate: textPtr(row.Plate), PlatformRating: int(row.PlatformRating),
			ProductRating: int(row.ProductRating), Comment: textPtr(row.Comment),
			IsAnonymous: row.IsAnonymous, Source: row.Source, Answers: answers,
			CreatedAt: row.CreatedAt.Time,
		}
		if !row.IsAnonymous || center {
			item.Customer = &ReviewCustomerView{
				UUID:  row.CustomerUuid,
				Name:  row.CustomerName + " " + row.CustomerSurname,
				Phone: textPtr(pgtype.Text{String: row.CustomerPhone.String, Valid: row.CustomerPhone.Valid}),
			}
		}
		out = append(out, item)
	}
	return out, total, nil
}

func (s *Service) ReviewDealerStats(ctx context.Context, c Caller, from, to *time.Time) ([]ReviewDealerStat, error) {
	rows, err := s.q.ReviewDealerStats(ctx, db.ReviewDealerStatsParams{
		BrandID: c.Org.BrandID, OrganizationIds: reviewScopeOrgIDs(c, rbac.PermReviewsRead),
		CreatedFrom: tstzArg(from), CreatedTo: tstzArg(to),
	})
	if err != nil {
		return nil, fmt.Errorf("services: dealer review stats: %w", err)
	}
	out := make([]ReviewDealerStat, 0, len(rows))
	for _, r := range rows {
		out = append(out, ReviewDealerStat{
			DealerUUID: r.DealerUuid, DealerName: r.DealerName,
			ReviewCount: r.ReviewCount, AverageRating: numericFloat(r.AverageRating),
		})
	}
	return out, nil
}

func (s *Service) ReviewProductStats(ctx context.Context, c Caller, from, to *time.Time) ([]ReviewProductStat, error) {
	rows, err := s.q.ReviewProductStats(ctx, db.ReviewProductStatsParams{
		BrandID: c.Org.BrandID, OrganizationIds: reviewScopeOrgIDs(c, rbac.PermReviewsRead),
		CreatedFrom: tstzArg(from), CreatedTo: tstzArg(to),
	})
	if err != nil {
		return nil, fmt.Errorf("services: product review stats: %w", err)
	}
	out := make([]ReviewProductStat, 0, len(rows))
	for _, r := range rows {
		out = append(out, ReviewProductStat{
			ProductUUID: r.ProductUuid, SKU: r.Sku, ProductName: r.ProductName,
			ReviewCount: r.ReviewCount, AverageRating: numericFloat(r.AverageRating),
		})
	}
	return out, nil
}

func reviewListParams(brandID int64, orgIDs []int64, f ServiceReviewFilter) (db.ListServiceReviewsInScopeParams, error) {
	arg := db.ListServiceReviewsInScopeParams{
		BrandID: brandID, OrganizationIds: orgIDs, RowLimit: f.Limit, RowOffset: f.Offset,
		CreatedFrom: tstzArg(f.CreatedFrom), CreatedTo: tstzArg(f.CreatedTo),
	}
	if f.DealerUUID != nil {
		arg.DealerUuid = pgtype.UUID{Bytes: *f.DealerUUID, Valid: true}
	}
	if f.ProductUUID != nil {
		arg.ProductUuid = pgtype.UUID{Bytes: *f.ProductUUID, Valid: true}
	}
	if f.MinRating != nil {
		if *f.MinRating < 1 || *f.MinRating > 5 {
			return arg, invalid("min_rating", "must be between 1 and 5")
		}
		arg.MinRating = pgtype.Int2{Int16: int16(*f.MinRating), Valid: true}
	}
	if f.MaxRating != nil {
		if *f.MaxRating < 1 || *f.MaxRating > 5 {
			return arg, invalid("max_rating", "must be between 1 and 5")
		}
		arg.MaxRating = pgtype.Int2{Int16: int16(*f.MaxRating), Valid: true}
	}
	if f.MinRating != nil && f.MaxRating != nil && *f.MinRating > *f.MaxRating {
		return arg, invalid("max_rating", "must be greater than or equal to min_rating")
	}
	return arg, nil
}

func tstzArg(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

func numericFloat(n pgtype.Numeric) float64 {
	v, err := n.Float64Value()
	if err != nil || !v.Valid {
		return 0
	}
	return v.Float64
}
