package glorian

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-271 (F2-02f): order outbound. An order of the center to a Glorian
// dealer whose lines hold units of products synced from a connection is
// sent to that connection's hub (warehouse CreateOrderOutboundsAction +
// SubmitOrderOutboundAction + MutateOrderOutboundsAction):
//
//   - When the order is ready (every line assigned) one order_outbounds row
//     is created per connection; external_reference (the order uuid) is the
//     idempotency key of POST /orders.
//   - Every run reads the local order and the hub order (GET /orders by
//     external_reference) and sends the missing steps in order: POST
//     /orders (auto_prepare, the barcodes are reserved on the hub), then
//     ship, receive; cancel when the local order is cancelled. The hub
//     answers a repeated step unchanged, so a retried, duplicated or late
//     task never repeats or reorders a step. No new order status is added:
//     shipped and cancelling map to hub shipped, received to delivered
//     (receive), cancelled to cancelled.
//   - The customer link is the buyer organization's phone (E.164) matching
//     exactly one active dealer of the connection (integration_external_parties).
//     Without it the row is held missing_customer_link; an inactive
//     connection holds it inactive_connection. Held rows wait for a replay
//     (cron task, or the admin endpoint of F2-02h): the link is evaluated
//     again and the order goes out.
//   - Olex orders (no synced product, no connection) produce nothing.

// Outbound states (chk_order_outbounds_state).
const (
	OutboundPending   = "pending"
	OutboundHeld      = "held"
	OutboundSent      = "sent"
	OutboundFailed    = "failed"
	OutboundCancelled = "cancelled"
)

// Hub order statuses (Inventory API OrderStatusEnum).
const (
	RemotePending    = "pending"
	RemoteProcessing = "processing"
	RemoteShipped    = "shipped"
	RemoteDelivered  = "delivered"
	RemoteCancelled  = "cancelled"
)

// ErrOrderTransition: the hub order cannot reach the local state (e.g. ship
// after cancel, cancel after delivery). Permanent; no request is made.
var ErrOrderTransition = errors.New("glorian: order transition not allowed")

// DefaultCargoCompany is sent as cargo_company on ship when the order has no
// delivery mode (the hub requires one; the app has no carrier field).
const DefaultCargoCompany = "olexfilms"

// OutboundCounts are the per-run totals of an outbound run.
type OutboundCounts struct {
	Created     int `json:"created"`
	Transitions int `json:"transitions"`
	Held        int `json:"held"`
	Cancelled   int `json:"cancelled"`
}

// ReplayResult sums a replay of held outbounds.
type ReplayResult struct {
	Replayed  int `json:"replayed"`
	StillHeld int `json:"still_held"`
	Failed    int `json:"failed"`
}

// maxReplayRows bounds one replay page.
const maxReplayRows = 100

// OrderOutbounder runs the order outbound.
type OrderOutbounder struct {
	outbound
}

// NewOrderOutbounder wires the order outbound; factory builds the client of
// a connection (HTTPClientFactory in production), log may be nil.
func NewOrderOutbounder(q db.Querier, box SecretBox, factory ClientFactory, log *slog.Logger) *OrderOutbounder {
	return &OrderOutbounder{outbound: newOutbound(q, box, factory, log)}
}

// OrderTask is the glorian:order_outbound handler.
func (o *OrderOutbounder) OrderTask(ctx context.Context, orderID int64) error {
	return TaskError(o.SyncOrder(ctx, orderID))
}

// ReplayTask is the glorian:order_outbound_replay handler.
func (o *OrderOutbounder) ReplayTask(ctx context.Context, connectionID int64) error {
	res, err := o.ReplayHeld(ctx, connectionID)
	if res.Replayed+res.StillHeld+res.Failed > 0 {
		o.log.Info("glorian_order_replay_done", "connection_id", connectionID,
			"replayed", res.Replayed, "still_held", res.StillHeld, "failed", res.Failed)
	}
	return TaskError(err)
}

// SyncOrder creates the outbounds of a ready order and brings every one of
// them in line with the order. A held outbound returns its HeldError (a
// joined error when several fail).
func (o *OrderOutbounder) SyncOrder(ctx context.Context, orderID int64) error {
	order, err := o.q.GetGlorianOutboundOrder(ctx, orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		o.log.Warn("glorian_order_missing", "order_id", orderID)
		return nil
	}
	if err != nil {
		return fmt.Errorf("glorian order: order %d: %w", orderID, err)
	}
	outbounds, err := o.EnsureOutbounds(ctx, order)
	if err != nil {
		return err
	}
	// A hold is recorded and waits for a replay; any other failure wins so
	// the task is retried (TaskError treats a hold as done).
	var errs []error
	var held error
	for _, ob := range outbounds {
		err := o.sync(ctx, ob, order)
		if _, isHeld := IsHeld(err); isHeld {
			if held == nil {
				held = err
			}
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("outbound %s: %w", ob.Uuid, err))
		}
	}
	if len(errs) == 0 {
		return held
	}
	return errors.Join(errs...)
}

// EnsureOutbounds returns the outbounds of order, creating the missing ones
// (one per connection of its synced units) once the order has been ready.
// An order that never reached ready (or has no synced unit, e.g. Olex) has
// none.
func (o *OrderOutbounder) EnsureOutbounds(ctx context.Context, order db.Order) ([]db.OrderOutbound, error) {
	existing, err := o.q.ListOrderOutboundsByOrder(ctx, order.ID)
	if err != nil {
		return nil, fmt.Errorf("glorian order: outbounds: %w", err)
	}
	if !order.ReadyAt.Valid {
		return existing, nil
	}
	have := map[int64]bool{}
	for _, ob := range existing {
		have[ob.ConnectionID] = true
	}
	units, err := o.q.ListGlorianOrderUnits(ctx, order.ID)
	if err != nil {
		return nil, fmt.Errorf("glorian order: units: %w", err)
	}
	for _, u := range units {
		if have[u.ConnectionID] {
			continue
		}
		have[u.ConnectionID] = true
		ob, err := o.createOutbound(ctx, order, u.ConnectionID)
		if err != nil {
			return nil, err
		}
		existing = append(existing, ob)
	}
	return existing, nil
}

func (o *OrderOutbounder) createOutbound(ctx context.Context, order db.Order, connectionID int64) (db.OrderOutbound, error) {
	conn, err := o.connection(ctx, connectionID)
	if err != nil {
		return db.OrderOutbound{}, fmt.Errorf("glorian order: connection %d: %w", connectionID, err)
	}
	_, reason, err := o.link(ctx, conn, order)
	if err != nil {
		return db.OrderOutbound{}, err
	}
	arg := db.CreateOrderOutboundParams{
		OrganizationID: conn.OrganizationID, BrandID: conn.BrandID, OrderID: order.ID,
		ConnectionID: conn.ID, ExternalReference: ExternalReference(order), State: OutboundPending,
	}
	if reason != "" {
		arg.State, arg.HeldReason = OutboundHeld, pgtype.Text{String: reason, Valid: true}
	}
	ob, err := o.q.CreateOrderOutbound(ctx, arg)
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		// A concurrent run created it first.
		ob, err = o.q.GetOrderOutbound(ctx, db.GetOrderOutboundParams{OrderID: order.ID, ConnectionID: conn.ID})
	}
	if err != nil {
		return db.OrderOutbound{}, fmt.Errorf("glorian order: create outbound: %w", err)
	}
	return ob, nil
}

// ExternalReference is the idempotency key of an order's hub order.
func ExternalReference(order db.Order) string { return order.Uuid.String() }

// link returns the hub dealer id of the order's buyer on conn, or the hold
// reason: missing_customer_link before inactive_connection (warehouse
// CreateOrderOutboundsAction).
func (o *OrderOutbounder) link(ctx context.Context, conn db.IntegrationConnection, order db.Order) (string, string, error) {
	buyer, err := o.q.GetOrganizationByID(ctx, order.BuyerOrgID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", "", fmt.Errorf("glorian order: buyer: %w", err)
	}
	dealer := ""
	if phone := strings.TrimSpace(buyer.Phone); phone != "" {
		parties, err := o.q.ListGlorianPartiesByPhone(ctx, db.ListGlorianPartiesByPhoneParams{
			ConnectionID: conn.ID, PhoneE164: pgtype.Text{String: phone, Valid: true},
		})
		if err != nil {
			return "", "", fmt.Errorf("glorian order: dealer link: %w", err)
		}
		if len(parties) == 1 {
			dealer = parties[0].RemoteID
		}
	}
	switch {
	case dealer == "":
		return "", HeldMissingCustomer, nil
	case !conn.Active:
		return dealer, HeldInactiveConnection, nil
	}
	return dealer, "", nil
}

// ReplayOutbound replays one outbound (held or failed) by id; the admin
// endpoint of F2-02h calls it.
func (o *OrderOutbounder) ReplayOutbound(ctx context.Context, outboundID int64) error {
	ob, err := o.q.GetOrderOutboundByID(ctx, outboundID)
	if err != nil {
		return fmt.Errorf("glorian order: outbound %d: %w", outboundID, err)
	}
	order, err := o.q.GetGlorianOutboundOrder(ctx, ob.OrderID)
	if err != nil {
		return fmt.Errorf("glorian order: order %d: %w", ob.OrderID, err)
	}
	return o.sync(ctx, ob, order)
}

// ReplayHeld replays every held outbound of a connection (0: all). A row
// whose link or connection is now in place goes out; the others stay held.
func (o *OrderOutbounder) ReplayHeld(ctx context.Context, connectionID int64) (ReplayResult, error) {
	var res ReplayResult
	var errs []error
	var after int64
	for {
		rows, err := o.q.ListHeldOrderOutbounds(ctx, db.ListHeldOrderOutboundsParams{
			ConnectionID: connectionID, AfterID: after, RowLimit: maxReplayRows,
		})
		if err != nil {
			return res, fmt.Errorf("glorian order: held outbounds: %w", err)
		}
		for _, ob := range rows {
			after = ob.ID
			order, err := o.q.GetGlorianOutboundOrder(ctx, ob.OrderID)
			if err == nil {
				err = o.sync(ctx, ob, order)
			}
			if _, held := IsHeld(err); held {
				res.StillHeld++
				continue
			}
			if err != nil {
				res.Failed++
				errs = append(errs, fmt.Errorf("outbound %s: %w", ob.Uuid, err))
				continue
			}
			res.Replayed++
		}
		if len(rows) < maxReplayRows {
			return res, errors.Join(errs...)
		}
	}
}

// sync brings one outbound in line with its order inside an outbound sync
// run. A held outbound records a failed run ("held: <reason>") and returns
// the HeldError.
func (o *OrderOutbounder) sync(ctx context.Context, ob db.OrderOutbound, order db.Order) error {
	if ob.State == OutboundCancelled {
		return nil
	}
	target, ok := remoteTarget(order.Status)
	if !ok {
		return nil
	}
	conn, err := o.connection(ctx, ob.ConnectionID)
	if err != nil {
		return fmt.Errorf("glorian order: connection %d: %w", ob.ConnectionID, err)
	}
	run, err := o.startRun(ctx, conn, KindOutbound)
	if err != nil {
		return err
	}
	var counts OutboundCounts
	state, runErr := o.drive(ctx, conn, ob, order, target, &counts)
	if err := o.record(ctx, ob, state, runErr); err != nil {
		runErr = errors.Join(runErr, err)
	}
	if reason, held := IsHeld(runErr); held {
		counts.Held = 1
		o.log.Warn("glorian_order_held", "outbound", ob.Uuid, "order_id", order.ID, "reason", reason)
	}
	return run.finish(ctx, counts, time.Time{}, runErr)
}

// drive makes the calls and returns the state to record.
func (o *OrderOutbounder) drive(ctx context.Context, conn db.IntegrationConnection, ob db.OrderOutbound, order db.Order, target string, counts *OutboundCounts) (string, error) {
	dealer, reason, err := o.link(ctx, conn, order)
	if err != nil {
		return ob.State, err
	}
	// The dealer link is needed only to create the hub order: a sent order,
	// or a cancel (which creates nothing), goes on without it while the
	// connection is active.
	if reason == HeldMissingCustomer && (ob.State == OutboundSent || target == RemoteCancelled) {
		reason = ""
		if !conn.Active {
			reason = HeldInactiveConnection
		}
	}
	if reason != "" {
		return OutboundHeld, &HeldError{Reason: reason}
	}
	client, err := o.client(conn)
	if err != nil {
		if _, held := IsHeld(err); held {
			return OutboundHeld, err
		}
		return OutboundFailed, err
	}
	remote, found, err := findOrder(ctx, client, ob.ExternalReference)
	if err != nil {
		return OutboundFailed, err
	}
	if !found {
		if target == RemoteCancelled {
			// Cancelled before it ever reached the hub.
			counts.Cancelled = 1
			return OutboundCancelled, nil
		}
		if dealer == "" {
			// A sent order the hub does not know (any more) needs the link
			// to be created again.
			return OutboundHeld, &HeldError{Reason: HeldMissingCustomer}
		}
		in, err := o.createInput(ctx, ob, order, dealer)
		if err != nil {
			return OutboundFailed, err
		}
		if remote, err = client.CreateOrder(ctx, in); err != nil {
			return OutboundFailed, fmt.Errorf("create order: %w", err)
		}
		counts.Created = 1
	}
	actions, err := PlanOrderActions(remote.Status, target)
	if err != nil {
		return OutboundFailed, err
	}
	for _, a := range actions {
		in := TransitionInput{}
		if a == OrderActionShip {
			in = shipInput(order)
		}
		next, err := client.TransitionOrder(ctx, remote.ID, a, in)
		if err != nil {
			return OutboundFailed, fmt.Errorf("%s order %s: %w", a, remote.ID, err)
		}
		remote = next
		counts.Transitions++
	}
	if remote.Status == RemoteCancelled {
		counts.Cancelled = 1
		return OutboundCancelled, nil
	}
	return OutboundSent, nil
}

// record writes the attempt onto the outbound row.
func (o *OrderOutbounder) record(ctx context.Context, ob db.OrderOutbound, state string, runErr error) error {
	arg := db.UpdateOrderOutboundStateParams{ID: ob.ID, State: state, AttemptIncrement: 1}
	if reason, held := IsHeld(runErr); held {
		arg.State = OutboundHeld
		arg.HeldReason = pgtype.Text{String: reason, Valid: true}
		arg.AttemptIncrement = 0
	} else if runErr != nil {
		arg.State = OutboundFailed
		arg.LastError = pgtype.Text{String: truncateRunes(runErr.Error(), 2000), Valid: true}
	}
	if _, err := o.q.UpdateOrderOutboundState(ctx, arg); err != nil {
		return fmt.Errorf("glorian order: record outbound: %w", err)
	}
	return nil
}

// createInput is the POST /orders body: one line per synced product with
// the assigned serial barcodes of the connection.
func (o *OrderOutbounder) createInput(ctx context.Context, ob db.OrderOutbound, order db.Order, dealer string) (CreateOrderInput, error) {
	units, err := o.q.ListGlorianOrderUnits(ctx, order.ID)
	if err != nil {
		return CreateOrderInput{}, fmt.Errorf("glorian order: units: %w", err)
	}
	var items []OrderItemInput
	index := map[int64]int{}
	for _, u := range units {
		if u.ConnectionID != ob.ConnectionID {
			continue
		}
		i, ok := index[u.OrderItemID]
		if !ok {
			i = len(items)
			index[u.OrderItemID] = i
			items = append(items, OrderItemInput{ProductID: u.ProductExternalID})
		}
		items[i].Barcodes = append(items[i].Barcodes, u.Barcode)
		items[i].Quantity++
	}
	prepare := true
	in := CreateOrderInput{
		DealerID: dealer, ExternalReference: ob.ExternalReference, AutoPrepare: &prepare, Items: items,
	}
	if note := strings.TrimSpace(order.Note.String); order.Note.Valid && note != "" {
		note = truncateRunes(note, MaxOrderNotesRunes)
		in.Notes = &note
	}
	return in, nil
}

func shipInput(order db.Order) TransitionInput {
	in := TransitionInput{CargoCompany: DefaultCargoCompany}
	if order.DeliveryMode.Valid && order.DeliveryMode.String != "" {
		in.CargoCompany = order.DeliveryMode.String
	}
	if order.TrackingNo.Valid {
		in.TrackingNumber = order.TrackingNo.String
	}
	return in
}

// findOrder looks the hub order up by its external reference.
func findOrder(ctx context.Context, client InventoryClient, ref string) (Order, bool, error) {
	page, err := client.ListOrders(ctx, ListParams{Filters: map[string]string{"external_reference": ref}, PerPage: 5})
	if err != nil {
		return Order{}, false, fmt.Errorf("find order %s: %w", ref, err)
	}
	for _, r := range page.Items {
		if r.ExternalReference != nil && *r.ExternalReference == ref {
			return r, true, nil
		}
	}
	return Order{}, false, nil
}

// remoteTarget maps a local order status to the hub status it must reach
// (false: nothing to send yet).
func remoteTarget(status string) (string, bool) {
	switch status {
	case "ready":
		return RemoteProcessing, true
	case "shipped", "cancelling":
		// cancelling: the goods are on their way back; the hub order is
		// cancelled once they are in (cancelled).
		return RemoteShipped, true
	case "received":
		return RemoteDelivered, true
	case "cancelled":
		return RemoteCancelled, true
	}
	return "", false
}

var remoteRank = map[string]int{RemoteProcessing: 1, RemoteShipped: 2, RemoteDelivered: 3}

// PlanOrderActions lists the hub actions, in order, that move a hub order
// from status from to status to. A hub order already at or past the target
// needs none. Ship after cancel, cancel after delivery and a pending hub
// order (never prepared) are ErrOrderTransition.
func PlanOrderActions(from, to string) ([]OrderAction, error) {
	if to == RemoteCancelled {
		switch from {
		case RemoteCancelled:
			return nil, nil
		case RemoteDelivered:
			return nil, fmt.Errorf("%w: cancel a %s order", ErrOrderTransition, from)
		}
		return []OrderAction{OrderActionCancel}, nil
	}
	cur, ok := remoteRank[from]
	if !ok {
		return nil, fmt.Errorf("%w: %s order to %s", ErrOrderTransition, from, to)
	}
	want := remoteRank[to]
	var out []OrderAction
	for r := cur; r < want; r++ {
		switch r {
		case 1:
			out = append(out, OrderActionShip)
		case 2:
			out = append(out, OrderActionReceive)
		}
	}
	return out, nil
}
