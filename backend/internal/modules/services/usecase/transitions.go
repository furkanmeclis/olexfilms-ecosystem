package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Service statuses (chk_services_status, migration 000050; legacy
// ServiceStatusEnum).
const (
	StatusDraft      = "draft"
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusReady      = "ready"
	StatusCompleted  = "completed"
	StatusCancelled  = "cancelled"
)

// Statuses lists every service status in flow order.
var Statuses = []string{StatusDraft, StatusPending, StatusProcessing, StatusReady, StatusCompleted, StatusCancelled}

// IsStatus reports whether s is a known service status.
func IsStatus(s string) bool {
	for _, v := range Statuses {
		if v == s {
			return true
		}
	}
	return false
}

// IsFinal reports whether no transition leaves status (F1: completed and
// cancelled are final).
func IsFinal(status string) bool { return status == StatusCompleted || status == StatusCancelled }

// itemsEditable: items change only before the service is ready (TEC-179;
// the DB trigger locks them from completed on).
func itemsEditable(status string) bool {
	return status == StatusDraft || status == StatusPending || status == StatusProcessing
}

// rule is one allowed transition: the permission it needs and whether only
// the center may take it (legacy center shortcuts).
type rule struct {
	perm       string
	centerOnly bool
}

// transitions is the stock-free part of the legacy state machine
// (ViewService):
//
//	draft -> pending -> processing -> ready (services.write)
//	draft -> processing, pending -> ready (center shortcuts)
//	draft | pending | processing | ready -> cancelled (services.cancel, center only)
//	draft | processing | ready -> completed (services.complete; consumes the
//	  stock of the items in the same transaction, TEC-180)
var transitions = map[string]map[string]rule{
	StatusDraft: {
		StatusPending:    {perm: rbac.PermServicesWrite},
		StatusProcessing: {perm: rbac.PermServicesWrite, centerOnly: true},
		StatusCompleted:  {perm: rbac.PermServicesComplete},
		StatusCancelled:  {perm: rbac.PermServicesCancel},
	},
	StatusPending: {
		StatusProcessing: {perm: rbac.PermServicesWrite},
		StatusReady:      {perm: rbac.PermServicesWrite, centerOnly: true},
		StatusCancelled:  {perm: rbac.PermServicesCancel},
	},
	StatusProcessing: {
		StatusReady:     {perm: rbac.PermServicesWrite},
		StatusCompleted: {perm: rbac.PermServicesComplete},
		StatusCancelled: {perm: rbac.PermServicesCancel},
	},
	StatusReady: {
		StatusCompleted: {perm: rbac.PermServicesComplete},
		StatusCancelled: {perm: rbac.PermServicesCancel},
	},
}

func lookupTransition(from, to string) (rule, bool) {
	r, ok := transitions[from][to]
	return r, ok
}

func (r rule) allows(c Caller, svc db.Service) bool {
	if r.centerOnly && !c.isCenter() {
		return false
	}
	return c.allows(r.perm, svc)
}

// availableTransitions lists the statuses the caller may move svc to.
func availableTransitions(c Caller, svc db.Service) []string {
	out := []string{}
	for _, to := range Statuses {
		if r, ok := lookupTransition(svc.Status, to); ok && r.allows(c, svc) {
			out = append(out, to)
		}
	}
	return out
}

// eventFor maps a target status to its outbox event.
func eventFor(to string) string {
	switch to {
	case StatusPending:
		return events.ServicePending
	case StatusProcessing:
		return events.ServiceProcessing
	case StatusReady:
		return events.ServiceReady
	case StatusCancelled:
		return events.ServiceCancelled
	}
	return events.ServiceUpdated
}

// TransitionInput moves a service to Status; Note goes to the status log
// (and is the cancel reason).
type TransitionInput struct {
	Status string
	Note   *string
}

// Transition moves a service along the state machine. A repeated request
// for the current status is a no-op (completing a completed service writes
// no second consumption and no second event); completed and cancelled are
// final.
func (s *Service) Transition(ctx context.Context, c Caller, id uuid.UUID, in TransitionInput) (ServiceView, error) {
	to := strings.TrimSpace(in.Status)
	if !IsStatus(to) {
		return ServiceView{}, invalid("status", "unknown service status")
	}
	if err := checkLen("note", in.Note, MaxNoteLength); err != nil {
		return ServiceView{}, err
	}
	var result db.Service
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		svc, err := s.lockVisible(ctx, q, c, id)
		if err != nil {
			return err
		}
		result = svc
		if svc.Status == to {
			return nil
		}
		if IsFinal(svc.Status) {
			return ErrInvalidTransition
		}
		r, ok := lookupTransition(svc.Status, to)
		if !ok {
			return ErrInvalidTransition
		}
		if !r.allows(c, svc) {
			return ErrForbidden
		}
		if needsExecutedContract(svc.Status, to) {
			if err := s.requireExecutedContract(ctx, q, svc); err != nil {
				return err
			}
		}
		if err := s.checkCertificateStatusPolicy(ctx, q, tx, svc, c, to); err != nil {
			return err
		}
		if to == StatusCompleted {
			result, err = s.complete(ctx, q, tx, svc, c, in.Note)
			return err
		}
		from := svc.Status
		if to == StatusCancelled {
			svc, err = q.CancelService(ctx, db.CancelServiceParams{
				ID: svc.ID, ActorUserID: c.actor(), CancelReason: textOrNull(in.Note),
			})
		} else {
			svc, err = q.UpdateServiceStatus(ctx, db.UpdateServiceStatusParams{
				ID: svc.ID, Status: to, ActorUserID: c.actor(),
			})
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidTransition
		}
		if err != nil {
			return fmt.Errorf("services: transition: %w", err)
		}
		if err := s.log(ctx, q, svc, from, to, c, in.Note, nil); err != nil {
			return err
		}
		result = svc
		return s.emit(ctx, tx, eventFor(to), svc, from, c, nil)
	})
	if err != nil {
		return ServiceView{}, err
	}
	return s.view(ctx, s.q, c, result)
}
