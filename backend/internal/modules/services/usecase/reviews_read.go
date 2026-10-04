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

type ReviewCustomerView struct {
	UUID  uuid.UUID `json:"uuid"`
	Name  string    `json:"name"`
	Phone *string   `json:"phone"`
}

func (s *Service) ListServiceReviews(ctx context.Context, c Caller, limit, offset int32) ([]ServiceReviewListItem, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	orgIDs := reviewScopeOrgIDs(c, rbac.PermReviewsRead)
	rows, err := s.q.ListServiceReviewsInScope(ctx, db.ListServiceReviewsInScopeParams{
		BrandID: c.Org.BrandID, OrganizationIds: orgIDs, RowLimit: limit, RowOffset: offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("services: list reviews: %w", err)
	}
	total, err := s.q.CountServiceReviewsInScope(ctx, db.CountServiceReviewsInScopeParams{
		BrandID: c.Org.BrandID, OrganizationIds: orgIDs,
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
