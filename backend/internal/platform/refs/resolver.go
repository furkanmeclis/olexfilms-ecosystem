package refs

import (
	"context"
	"errors"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
)

// Lookup is the subset of *db.Queries the resolver reads.
type Lookup interface {
	UserRefByID(ctx context.Context, id int64) (db.UserRefByIDRow, error)
	VehicleRefByID(ctx context.Context, id int64) (db.VehicleRefByIDRow, error)
	ServiceRefByID(ctx context.Context, id int64) (db.ServiceRefByIDRow, error)
	LeadRefByID(ctx context.Context, id int64) (db.LeadRefByIDRow, error)
	AppointmentRefByID(ctx context.Context, id int64) (db.AppointmentRefByIDRow, error)
	OrganizationRefByID(ctx context.Context, id int64) (db.OrganizationRefByIDRow, error)
	SubscriptionRefByID(ctx context.Context, id int64) (db.SubscriptionRefByIDRow, error)
}

// Resolver turns internal foreign keys into refs. A nil or zero id, or a
// row that no longer exists, is a nil ref. Callers pass only foreign keys
// of records already inside the caller's scope.
type Resolver struct{ q Lookup }

// NewResolver builds a resolver over q (usually *db.Queries).
func NewResolver(q Lookup) Resolver { return Resolver{q: q} }

func lookup[T any](ctx context.Context, id *int64, what string, fn func(context.Context, int64) (T, error)) (*T, error) {
	if id == nil || *id == 0 {
		return nil, nil
	}
	row, err := fn(ctx, *id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("refs: %s: %w", what, err)
	}
	return &row, nil
}

// User is a staff user ref (kind user).
func (r Resolver) User(ctx context.Context, id *int64) (*Ref, error) {
	row, err := lookup(ctx, id, "user", r.q.UserRefByID)
	if row == nil || err != nil {
		return nil, err
	}
	ref := UserRef(row.Uuid, FullName(row.Name, row.Surname))
	return &ref, nil
}

// Customer is a customer ref (kind customer); an anonymized customer has
// an empty name (K19).
func (r Resolver) Customer(ctx context.Context, id *int64) (*Ref, error) {
	row, err := lookup(ctx, id, "customer", r.q.UserRefByID)
	if row == nil || err != nil {
		return nil, err
	}
	name := FullName(row.Name, row.Surname)
	if row.Status == "anonymized" {
		name = ""
	}
	ref := CustomerRef(row.Uuid, name)
	return &ref, nil
}

// Vehicle is a vehicle ref named by its plate, else its VIN.
func (r Resolver) Vehicle(ctx context.Context, id *int64) (*Ref, error) {
	row, err := lookup(ctx, id, "vehicle", r.q.VehicleRefByID)
	if row == nil || err != nil {
		return nil, err
	}
	ref := New(KindVehicle, row.Uuid, row.Name)
	return &ref, nil
}

// Service is a service ref named by its service number.
func (r Resolver) Service(ctx context.Context, id *int64) (*Ref, error) {
	row, err := lookup(ctx, id, "service", r.q.ServiceRefByID)
	if row == nil || err != nil {
		return nil, err
	}
	ref := New(KindService, row.Uuid, row.Name)
	return &ref, nil
}

// Lead is a lead ref named by the candidate company / contact or the
// customer.
func (r Resolver) Lead(ctx context.Context, id *int64) (*Ref, error) {
	row, err := lookup(ctx, id, "lead", r.q.LeadRefByID)
	if row == nil || err != nil {
		return nil, err
	}
	ref := New(KindLead, row.Uuid, row.Name)
	return &ref, nil
}

// Appointment is an appointment ref named by its start (UTC, YYYY-MM-DD HH:MM).
func (r Resolver) Appointment(ctx context.Context, id *int64) (*Ref, error) {
	row, err := lookup(ctx, id, "appointment", r.q.AppointmentRefByID)
	if row == nil || err != nil {
		return nil, err
	}
	ref := New(KindAppointment, row.Uuid, row.Name)
	return &ref, nil
}

// Organization is an organization ref with its type as the sub line.
func (r Resolver) Organization(ctx context.Context, id *int64) (*Ref, error) {
	row, err := lookup(ctx, id, "organization", r.q.OrganizationRefByID)
	if row == nil || err != nil {
		return nil, err
	}
	ref := OrgRef(row.Uuid, row.Name, row.Type)
	return &ref, nil
}

// Subscription is a service subscription ref named by its catalog item.
func (r Resolver) Subscription(ctx context.Context, id *int64) (*Ref, error) {
	row, err := lookup(ctx, id, "subscription", r.q.SubscriptionRefByID)
	if row == nil || err != nil {
		return nil, err
	}
	ref := New(KindSubscription, row.Uuid, row.Name)
	return &ref, nil
}
