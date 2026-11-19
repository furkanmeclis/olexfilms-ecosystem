package usecase

// TEC-1058 (RV-5.6): the "sent plotter cut" mark of an order line. It is
// information only — no stock, price or flow rule reads it — and feeds the
// monthly plotter report. Next to the line edits (create, PUT items) the
// mark is set on one line with PATCH, also after shipping: the seller's
// sales or warehouse staff (orders.write or orders.ship) on any order that
// is not cancelled, the buyer (orders.write) on its draft.

import (
	"context"
	"errors"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ActionPlotterMarked is the timeline step of a plotter mark change.
const ActionPlotterMarked = "order.plotter_marked"

// canMarkPlotter reports whether the caller may change a line's plotter
// mark on o now.
func canMarkPlotter(c Caller, o db.Order) bool {
	switch partyOf(c, o) {
	case PartyBuyer:
		return o.Status == StatusDraft && c.can(rbac.PermOrdersWrite)
	case PartySeller:
		return o.Status != StatusCancelled && o.Status != StatusDraft &&
			(c.can(rbac.PermOrdersWrite) || c.can(rbac.PermOrdersShip))
	}
	return false
}

// SetPlotterCut sets the plotter mark of one line; the same value is a
// no-op (no timeline step).
//
// TEC-1221: credits (optional) are the plotter line's usage credits; nil
// keeps the stored value, 0 clears it; turning the mark off clears it.
func (s *Service) SetPlotterCut(ctx context.Context, c Caller, orderUUID, itemUUID uuid.UUID, on bool, credits *int32) (OrderView, error) {
	var result db.Order
	err := s.inTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		o, err := s.lockVisible(ctx, q, c, orderUUID)
		if err != nil {
			return err
		}
		party := partyOf(c, o)
		allowed := c.can(rbac.PermOrdersWrite) || (party == PartySeller && c.can(rbac.PermOrdersShip))
		if party == "" || !allowed {
			return ErrForbidden
		}
		if !canMarkPlotter(c, o) {
			return ErrNotEditable
		}
		it, err := q.GetOrderItemByUUID(ctx, db.GetOrderItemByUUIDParams{Uuid: itemUUID, BrandID: o.BrandID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && it.OrderID != o.ID) {
			return ErrItemNotFound
		}
		if err != nil {
			return fmt.Errorf("orders: line: %w", err)
		}
		result = o
		// TEC-1238 (K12): a meter line of a piece product (it carries
		// line credits) shares its credits with the plotter mark: a
		// credits value sets them, the mark keeps none of its own.
		creditLine := it.LineCredits.Valid
		var next pgtype.Int4
		if creditLine {
			if credits != nil && *credits != 0 {
				if *credits < 1 || *credits > MaxLineCredits {
					return invalid("plotter_credits", fmt.Sprintf("must be between 1 and %d", MaxLineCredits))
				}
				if *credits != it.LineCredits.Int32 {
					if it, err = q.SetOrderItemLineCredits(ctx, db.SetOrderItemLineCreditsParams{
						ID: it.ID, LineCredits: pgtype.Int4{Int32: *credits, Valid: true},
					}); err != nil {
						return fmt.Errorf("orders: line credits: %w", err)
					}
					if it.PlotterCut == on {
						p := activityPayload{ItemUUID: it.Uuid.String(), PlotterCut: &on, PlotterCredits: int4Ptr(it.LineCredits)}
						if err := s.recordActivity(ctx, q, c, o, ActionPlotterMarked, p); err != nil {
							return err
						}
						return s.emit(ctx, tx, events.OrdersUpdated, o, o.Status, c)
					}
				}
			}
		} else if next, err = plotterCredits("plotter_credits", on, it.PlotterCredits, credits); err != nil {
			return err
		}
		if it.PlotterCut == on && it.PlotterCredits == next {
			return nil
		}
		if _, err := q.SetOrderItemPlotterCut(ctx, db.SetOrderItemPlotterCutParams{
			ID: it.ID, PlotterCut: on, PlotterCredits: next,
		}); err != nil {
			return fmt.Errorf("orders: plotter mark: %w", err)
		}
		shown := next
		if creditLine {
			shown = it.LineCredits
		}
		p := activityPayload{ItemUUID: it.Uuid.String(), PlotterCut: &on, PlotterCredits: int4Ptr(shown)}
		if pr, err := q.GetProduct(ctx, db.GetProductParams{ID: it.ProductID, BrandID: o.BrandID}); err == nil {
			p.Product = pr.Name
		}
		if err := s.recordActivity(ctx, q, c, o, ActionPlotterMarked, p); err != nil {
			return err
		}
		return s.emit(ctx, tx, events.OrdersUpdated, o, o.Status, c)
	})
	if err != nil {
		return OrderView{}, err
	}
	return s.view(ctx, s.q, c, result)
}
