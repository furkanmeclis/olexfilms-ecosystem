package usecase

// TEC-1062 decision: a recipient of the center's waiting pool (a dealer
// candidate or a retail point, organization status pending) has no member
// who signs in, so nobody would move its order to received and the K9 cari
// row would never be booked. The seller (orders.ship or orders.write) then
// receives the order on the buyer's behalf; the history row carries
// on_behalf_of_buyer. A buyer with an active member receives on its own,
// exactly as before.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// MetaOnBehalfOfBuyer marks a received history row written by the seller
// for the buyer.
const MetaOnBehalfOfBuyer = "on_behalf_of_buyer"

// receiveOnBehalfPerms: the seller permissions that receive for the buyer.
var receiveOnBehalfPerms = []string{rbac.PermOrdersShip, rbac.PermOrdersWrite}

// receivesOnBehalf reports whether the seller c may move o to received for
// the buyer: o is shipped or delivered and the buyer is a pending or retail
// recipient of the seller's pool without an active member.
func (s *Service) receivesOnBehalf(ctx context.Context, q *db.Queries, c Caller, o db.Order, to string) (bool, error) {
	if to != StatusReceived || partyOf(c, o) != PartySeller {
		return false, nil
	}
	if o.Status != StatusShipped && o.Status != StatusDelivered {
		return false, nil
	}
	if !slices.ContainsFunc(receiveOnBehalfPerms, c.can) {
		return false, nil
	}
	ok, err := q.IsMemberlessPoolRecipient(ctx, db.IsMemberlessPoolRecipientParams{
		RecipientOrgID: o.BuyerOrgID, CenterOrgID: o.SellerOrgID,
	})
	if err != nil {
		return false, fmt.Errorf("orders: pool recipient: %w", err)
	}
	return ok, nil
}

// withOnBehalfReceive adds received to the seller's transitions (in flow
// order) and sets ReceiveOnBehalf when the seller may receive for the buyer.
func (s *Service) withOnBehalfReceive(ctx context.Context, q *db.Queries, c Caller, o db.Order, v *OrderView) error {
	if slices.Contains(v.AvailableTransitions, StatusReceived) {
		return nil
	}
	ok, err := s.receivesOnBehalf(ctx, q, c, o, StatusReceived)
	if err != nil || !ok {
		return err
	}
	out := make([]string, 0, len(v.AvailableTransitions)+1)
	for _, st := range Statuses {
		if st == StatusReceived || slices.Contains(v.AvailableTransitions, st) {
			out = append(out, st)
		}
	}
	v.AvailableTransitions = out
	v.ReceiveOnBehalf = true
	return nil
}

// historyOnBehalf reads MetaOnBehalfOfBuyer from a history row's metadata.
func historyOnBehalf(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return false
	}
	v, _ := m[MetaOnBehalfOfBuyer].(bool)
	return v
}
