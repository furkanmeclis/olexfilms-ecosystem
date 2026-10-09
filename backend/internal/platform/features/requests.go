package features

import (
	"context"
	"errors"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Module request statuses (module_requests.status, TEC-508).
const (
	RequestPending   = "pending"
	RequestApproved  = "approved"
	RequestRejected  = "rejected"
	RequestCancelled = "cancelled"
)

// RequestStatuses lists every request status (list filter whitelist).
var RequestStatuses = []string{RequestPending, RequestApproved, RequestRejected, RequestCancelled}

// Decision queues: who decides a request.
const (
	// QueueDistributor: a dealer's request, decided by its distributor.
	QueueDistributor = "distributor"
	// QueuePlatform: a distributor's request, or a dealer's without a
	// distributor parent; decided by the platform admins (the center).
	QueuePlatform = "platform"
)

// AutoDecisionNote marks a request approved because the module opened some
// other way.
const AutoDecisionNote = "auto"

// MaxRequestNote is the longest request or decision note.
const MaxRequestNote = 1000

var (
	// ErrModuleEnabled: the module is already on for the organization.
	ErrModuleEnabled = errors.New("features: module is already enabled")
	// ErrRequestNotFound: no request, or not in the caller's queue.
	ErrRequestNotFound = errors.New("features: module request not found")
	// ErrRequestDecided: the request is no longer pending.
	ErrRequestDecided = errors.New("features: module request is not pending")
)

// DecisionNotifier is told about every decided request (approved by hand or
// automatically, rejected). It must not fail the decision.
type DecisionNotifier interface {
	ModuleRequestDecided(ctx context.Context, req db.ModuleRequest)
}

// WithDecisionNotifier sets the hook that notifies the requester.
func (s *Service) WithDecisionNotifier(n DecisionNotifier) *Service {
	s.notify = n
	return s
}

// RequestInput is a module request of an organization.
type RequestInput struct {
	OrgID   int64
	BrandID int64
	ActorID int64
	Key     string
	Note    string
}

// RequestModule records (or refreshes) the pending request of an
// organization for a module. A module that is already on, or closed system
// wide, cannot be requested.
func (s *Service) RequestModule(ctx context.Context, in RequestInput) (db.ModuleRequest, error) {
	if _, err := switchable(in.Key); err != nil {
		return db.ModuleRequest{}, err
	}
	st, err := s.stateOf(ctx, in.OrgID, in.Key)
	if err != nil {
		return db.ModuleRequest{}, err
	}
	if st.Enabled {
		return db.ModuleRequest{}, ErrModuleEnabled
	}
	if st.Source == FromSystem {
		return db.ModuleRequest{}, ErrUpstreamDisabled
	}
	return s.q.UpsertPendingModuleRequest(ctx, db.UpsertPendingModuleRequestParams{
		OrganizationID: in.OrgID, BrandID: in.BrandID, ModuleKey: in.Key,
		Note: strings.TrimSpace(in.Note), RequestedByUserID: actorArg(in.ActorID),
	})
}

// CancelRequest withdraws the organization's pending request of a module.
func (s *Service) CancelRequest(ctx context.Context, orgID, actorID int64, key string) (db.ModuleRequest, error) {
	if _, ok := ModuleByKey(key); !ok {
		return db.ModuleRequest{}, ErrUnknownModule
	}
	r, err := s.q.CancelPendingModuleRequest(ctx, db.CancelPendingModuleRequestParams{
		OrganizationID: orgID, ModuleKey: key, DecidedByUserID: actorArg(actorID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ModuleRequest{}, ErrRequestNotFound
	}
	return r, err
}

// LatestRequests returns the newest request of every module of an
// organization, by module key.
func (s *Service) LatestRequests(ctx context.Context, orgID int64) (map[string]db.ModuleRequest, error) {
	rows, err := s.q.ListLatestModuleRequestsForOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]db.ModuleRequest, len(rows))
	for _, r := range rows {
		out[r.ModuleKey] = r
	}
	return out, nil
}

// QueueOf returns the decision queue of an organization's requests and,
// for QueueDistributor, the deciding distributor.
func QueueOf(n OrgNode) (string, int64) {
	if n.Type == OrgDealer && n.ParentType == OrgDistributor && n.ParentID != 0 {
		return QueueDistributor, n.ParentID
	}
	return QueuePlatform, 0
}

// DecideInput is a decision on a pending request. DistributorID is the
// deciding distributor for QueueDistributor; QueuePlatform needs none.
type DecideInput struct {
	ActorID       int64
	Queue         string
	DistributorID int64
	UUID          uuid.UUID
	Approve       bool
	Note          string
}

// DecideRequest approves or rejects a pending request of the caller's queue.
// Approval opens the module in the same transaction: a distributor opens it
// for its dealer (SetForDealers rules: 409 when the distributor itself does
// not have it, or the admin set the dealer's value), the platform admin sets
// the organization's value (SetByAdmin rules).
func (s *Service) DecideRequest(ctx context.Context, in DecideInput) (db.ModuleRequest, error) {
	req, err := s.q.GetModuleRequestByUUID(ctx, in.UUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.ModuleRequest{}, ErrRequestNotFound
	}
	if err != nil {
		return db.ModuleRequest{}, err
	}
	n, err := s.node(ctx, s.q, req.OrganizationID)
	if errors.Is(err, ErrOrganizationNotFound) {
		return db.ModuleRequest{}, ErrRequestNotFound
	}
	if err != nil {
		return db.ModuleRequest{}, err
	}
	queue, dist := QueueOf(n)
	if queue != in.Queue || (queue == QueueDistributor && dist != in.DistributorID) {
		return db.ModuleRequest{}, ErrRequestNotFound
	}
	if req.Status != RequestPending {
		return db.ModuleRequest{}, ErrRequestDecided
	}
	status := RequestRejected
	if in.Approve {
		status = RequestApproved
	}
	var out db.ModuleRequest
	err = s.inTx(ctx, func(q *db.Queries) error {
		if in.Approve {
			var err error
			if queue == QueueDistributor {
				err = s.setForDealers(ctx, q, in.ActorID, dist, []int64{req.OrganizationID}, req.ModuleKey, true)
			} else {
				err = s.setByAdmin(ctx, q, in.ActorID, req.OrganizationID, req.ModuleKey, true)
			}
			if err != nil {
				return err
			}
		}
		var err error
		out, err = q.DecideModuleRequest(ctx, db.DecideModuleRequestParams{
			ID: req.ID, Status: status, DecidedByUserID: actorArg(in.ActorID), DecisionNote: strings.TrimSpace(in.Note),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRequestDecided
		}
		return err
	})
	if err != nil {
		return db.ModuleRequest{}, err
	}
	if in.Approve {
		s.invalidateTree(ctx, req.OrganizationID)
		s.reconcileKey(ctx, req.ModuleKey)
	}
	s.decided(ctx, out)
	return out, nil
}

func (s *Service) decided(ctx context.Context, r db.ModuleRequest) {
	if s.notify != nil {
		s.notify.ModuleRequestDecided(ctx, r)
	}
}

// reconcileKey approves every pending request of key whose organization now
// has the module on (opened by the admin, a distributor, a dealer standard
// or a default change). Best effort: errors are logged.
func (s *Service) reconcileKey(ctx context.Context, key string) {
	rows, err := s.q.ListPendingModuleRequestsByKey(ctx, key)
	if err != nil {
		s.log.Warn("module_requests_reconcile_failed", "key", key, "error", err)
		return
	}
	s.reconcile(ctx, rows)
}

// reconcileOrg does the same for the pending requests of orgID and its
// subtree (module bundle subscriptions).
func (s *Service) reconcileOrg(ctx context.Context, orgID int64) {
	ids := []int64{orgID}
	if below, err := s.q.Descendants(ctx, orgID); err == nil {
		for _, o := range below {
			ids = append(ids, o.ID)
		}
	}
	rows, err := s.q.ListPendingModuleRequestsForOrgs(ctx, ids)
	if err != nil {
		s.log.Warn("module_requests_reconcile_failed", "organization_id", orgID, "error", err)
		return
	}
	s.reconcile(ctx, rows)
}

func (s *Service) reconcile(ctx context.Context, rows []db.ModuleRequest) {
	for _, r := range rows {
		st, err := s.stateOf(ctx, r.OrganizationID, r.ModuleKey)
		if err != nil || !st.Enabled {
			continue
		}
		out, err := s.q.DecideModuleRequest(ctx, db.DecideModuleRequestParams{
			ID: r.ID, Status: RequestApproved, DecisionNote: AutoDecisionNote,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			s.log.Warn("module_request_auto_approve_failed", "request_id", r.ID, "error", err)
			continue
		}
		s.decided(ctx, out)
	}
}
