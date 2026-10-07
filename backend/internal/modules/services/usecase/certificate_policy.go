package usecase

import (
	"context"
	"errors"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
)

var ErrCertificateApprovalRequired = errors.New("services: certificate approval required")

// CertificatePolicy is implemented by the certificates add-on. Keeping this
// interface in services avoids a services -> certificates import cycle.
type CertificatePolicy interface {
	EvaluateService(ctx context.Context, q *db.Queries, tx pgx.Tx, svc db.Service, actorUserID int64) error
	BeforeStatusChange(ctx context.Context, q *db.Queries, tx pgx.Tx, svc db.Service, actorUserID int64) error
}

func (s *Service) WithCertificatePolicy(policy CertificatePolicy) *Service {
	s.certificatePolicy = policy
	return s
}

func (s *Service) evaluateCertificatePolicy(ctx context.Context, q *db.Queries, tx pgx.Tx, svc db.Service, c Caller) error {
	if s.certificatePolicy == nil {
		return nil
	}
	return s.certificatePolicy.EvaluateService(ctx, q, tx, svc, c.Principal.UserInternal)
}

func (s *Service) checkCertificateStatusPolicy(ctx context.Context, q *db.Queries, tx pgx.Tx, svc db.Service, c Caller, to string) error {
	if s.certificatePolicy == nil || to == StatusCancelled {
		return nil
	}
	return s.certificatePolicy.BeforeStatusChange(ctx, q, tx, svc, c.Principal.UserInternal)
}
