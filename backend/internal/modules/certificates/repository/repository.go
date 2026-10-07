package repository

import (
	"context"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
)

// CertificateListSort is the list-contract whitelist of certificate rows.
var CertificateListSort = apiquery.SortSpec{
	Columns: apiquery.SortColumns{
		"expires_at": "expires_at",
		"issued_at":  "issued_at",
		"status":     "status",
		"user_name":  "user_name",
		"created_at": "created_at",
	},
	Default: apiquery.SortField{Field: "expires_at"},
}

// Repository is the thin certificate data adapter. The API/usecase layer
// owns authorization and business policy; the SQL keeps tenant and brand
// consistency.
type Repository struct {
	q *db.Queries
}

func New(q *db.Queries) *Repository {
	return &Repository{q: q}
}

func (r *Repository) ListValidForServiceUser(ctx context.Context, arg db.ListValidCertificatesForServiceUserParams) ([]db.Certificate, error) {
	return r.q.ListValidCertificatesForServiceUser(ctx, arg)
}

func (r *Repository) ListCertificates(ctx context.Context, arg db.ListCertificatesParams, sort []apiquery.SortField) ([]db.ListCertificatesRow, error) {
	resolved, err := apiquery.ResolveSort(sort, CertificateListSort)
	if err != nil {
		return nil, err
	}
	arg.SortKey = resolved.Key
	arg.SortDesc = resolved.Desc
	if arg.RowLimit == 0 {
		arg.RowLimit = int32(apiquery.DefaultLimit)
	}
	return r.q.ListCertificates(ctx, arg)
}
