package usecase

import (
	"context"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	psmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/photostandard/model"
)

// IntakePhotoGate is implemented by the photo standard add-on (TEC-499,
// F5-07b): it returns *psmodel.IncompleteError while a required intake angle
// has no photo and the module is on for the service organization. Kept as
// an interface so services does not import the add-on use case.
type IntakePhotoGate interface {
	RequireComplete(ctx context.Context, q *db.Queries, ref psmodel.ServiceRef) error
}

// WithIntakePhotoGate wires the photo standard rule into status transitions.
func (s *Service) WithIntakePhotoGate(g IntakePhotoGate) *Service {
	s.intakePhotos = g
	return s
}

// needsIntakePhotos: leaving draft for any non-cancel status and completing
// need the full intake photo set (cancelling never does).
func needsIntakePhotos(from, to string) bool {
	if to == StatusCancelled {
		return false
	}
	return from == StatusDraft || to == StatusCompleted
}

func (s *Service) checkIntakePhotos(ctx context.Context, q *db.Queries, svc db.Service, to string) error {
	if s.intakePhotos == nil || !needsIntakePhotos(svc.Status, to) {
		return nil
	}
	return s.intakePhotos.RequireComplete(ctx, q, psmodel.ServiceRef{
		ID: svc.ID, OrganizationID: svc.OrganizationID, BrandID: svc.BrandID,
	})
}
