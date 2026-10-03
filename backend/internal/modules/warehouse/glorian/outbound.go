package glorian

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
)

// Shared helpers of the outbound side of the sync: the barcode push
// (TEC-270) and the order outbound (TEC-271). Both resolve the client from
// the row's connection, refuse a missing or inactive connection without a
// request (held, never a silent fallback), wrap every attempt in an
// integration_sync_runs row and tell Asynq whether a failure is worth a
// retry.

// Outbound sync run kinds (chk_integration_sync_runs_kind).
const (
	KindPushBarcodes = "push_barcodes"
	KindOutbound     = "outbound"
)

// Held reasons. The values match order_outbounds.held_reason
// (chk_order_outbounds_held_reason) where the table has one.
const (
	HeldInactiveConnection = "inactive_connection"
	HeldMissingCustomer    = "missing_customer_link"
)

// HeldError is an outbound call that was not made: the work waits for the
// reason (e.g. the connection is activated) instead of failing over to
// another target. It is recorded and never retried by Asynq.
type HeldError struct {
	Reason string
}

func (e *HeldError) Error() string { return "glorian: held: " + e.Reason }

// Is matches ErrInactiveConnection for an inactive connection hold.
func (e *HeldError) Is(target error) bool {
	return target == ErrInactiveConnection && e.Reason == HeldInactiveConnection
}

// IsHeld reports whether err is a hold and returns its reason.
func IsHeld(err error) (string, bool) {
	var h *HeldError
	if errors.As(err, &h) {
		return h.Reason, true
	}
	return "", false
}

// Permanent reports whether an outbound failure would fail again on retry
// unchanged: the hub refused the request (validation, conflict, auth,
// version) or the input is invalid. Transport errors, 5xx, 429, 404 and
// database errors are transient.
func Permanent(err error) bool {
	if _, held := IsHeld(err); held {
		return true
	}
	for _, target := range []error{
		ErrValidation, ErrConflict, ErrUnauthorized, ErrForbidden, ErrUnsupportedVersion,
		ErrNotImplemented, ErrInvalidInput, ErrMisconfigured, ErrInvalidResponse,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// TaskError turns an outbound result into an Asynq handler result: nil
// stays nil, a hold is done (recorded, nothing to retry), a permanent
// failure skips the retries, anything else is returned for a retry.
func TaskError(err error) error {
	if err == nil {
		return nil
	}
	if _, held := IsHeld(err); held {
		return nil
	}
	if Permanent(err) {
		return fmt.Errorf("%w: %w", asynq.SkipRetry, err)
	}
	return err
}

// Enqueuer schedules an Asynq task (*queue.Client).
type Enqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// outbound resolves connections and clients for outbound calls.
type outbound struct {
	q        db.Querier
	store    *Store
	resolver *ClientResolver
	log      *slog.Logger
}

func newOutbound(q db.Querier, box SecretBox, factory ClientFactory, log *slog.Logger) outbound {
	if log == nil {
		log = slog.Default()
	}
	store := NewStore(q, box)
	return outbound{q: q, store: store, resolver: NewClientResolver(store, factory), log: log}
}

// connection loads a connection row by id (ErrConnectionNotFound).
func (o outbound) connection(ctx context.Context, id int64) (db.IntegrationConnection, error) {
	row, err := o.q.GetIntegrationConnectionByID(ctx, id)
	return row, mapStoreError(err)
}

// client builds the client of conn. An inactive or unusable connection is a
// HeldError (no request is made).
func (o outbound) client(conn db.IntegrationConnection) (InventoryClient, error) {
	if !conn.Active {
		return nil, &HeldError{Reason: HeldInactiveConnection}
	}
	c, err := o.store.Decrypt(conn)
	if err != nil {
		return nil, err
	}
	client, err := o.resolver.ForConnection(c)
	if errors.Is(err, ErrInactiveConnection) {
		return nil, &HeldError{Reason: HeldInactiveConnection}
	}
	return client, err
}

// syncRun is one integration_sync_runs row of an outbound attempt.
type syncRun struct {
	q   db.Querier
	row db.IntegrationSyncRun
}

func (o outbound) startRun(ctx context.Context, conn db.IntegrationConnection, kind string) (*syncRun, error) {
	row, err := o.q.StartIntegrationSyncRun(ctx, db.StartIntegrationSyncRunParams{
		OrganizationID: conn.OrganizationID, BrandID: conn.BrandID, ConnectionID: conn.ID, Kind: kind,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: start run: %w", kind, err)
	}
	return &syncRun{q: o.q, row: row}, nil
}

// finish closes the run: succeeded without runErr, failed with it (a hold
// is a failed run whose error starts with "held: <reason>"). watermark is
// stored only on success and when not zero. It returns runErr, or the
// finish error when the row could not be written.
func (r *syncRun) finish(ctx context.Context, counts any, watermark time.Time, runErr error) error {
	countsJSON, err := json.Marshal(counts)
	if err != nil {
		countsJSON = []byte("{}")
	}
	arg := db.FinishIntegrationSyncRunParams{ID: r.row.ID, Status: RunSucceeded, Counts: countsJSON}
	switch {
	case runErr != nil:
		arg.Status = RunFailed
		msg := runErr.Error()
		if reason, held := IsHeld(runErr); held {
			msg = "held: " + reason
		}
		arg.Error = pgtype.Text{String: truncateRunes(msg, 2000), Valid: true}
	case !watermark.IsZero():
		arg.Watermark = pgtype.Timestamptz{Time: watermark, Valid: true}
	}
	if _, err := r.q.FinishIntegrationSyncRun(ctx, arg); err != nil {
		return errors.Join(runErr, fmt.Errorf("%s: finish run: %w", r.row.Kind, err))
	}
	return runErr
}
