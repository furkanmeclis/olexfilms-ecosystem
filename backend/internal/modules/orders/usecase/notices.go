package usecase

import (
	"context"
	"fmt"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// statusNotice resolves the notification recipients carried by the
// orders.shipped, orders.delivered and orders.received events (demo flow,
// TEC-1024/1028 follow-up): shipped and delivered tell the buyer's members
// holding orders.read; received tells the seller's members holding
// orders.approve. The actor is never told. Other statuses carry nothing.
func (s *Service) statusNotice(ctx context.Context, q *db.Queries, o db.Order, to string, c Caller) (map[string]any, error) {
	var orgID int64
	var perm string
	switch to {
	case StatusShipped, StatusDelivered:
		orgID, perm = o.BuyerOrgID, rbac.PermOrdersRead
	case StatusReceived:
		orgID, perm = o.SellerOrgID, rbac.PermOrdersApprove
	default:
		return nil, nil
	}
	ids, err := q.ListTransferNotifyUserIDs(ctx, db.ListTransferNotifyUserIDsParams{
		OrganizationID: orgID, PermissionSlug: perm,
	})
	if err != nil {
		return nil, fmt.Errorf("orders: recipients: %w", err)
	}
	notify := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id != c.Principal.UserInternal {
			notify = append(notify, id)
		}
	}
	seller, err := q.GetOrganizationByID(ctx, o.SellerOrgID)
	if err != nil {
		return nil, fmt.Errorf("orders: seller: %w", err)
	}
	buyer, err := q.GetOrganizationByID(ctx, o.BuyerOrgID)
	if err != nil {
		return nil, fmt.Errorf("orders: buyer: %w", err)
	}
	out := map[string]any{
		"notify_user_ids": notify, "seller_name": seller.Name, "buyer_name": buyer.Name,
		"tracking_no": "", "delivery_method": "",
	}
	if o.TrackingNo.Valid {
		out["tracking_no"] = o.TrackingNo.String
	}
	if o.DeliveryMethodID.Valid {
		m, err := q.GetDeliveryMethodByID(ctx, o.DeliveryMethodID.Int64)
		if err != nil {
			return nil, fmt.Errorf("orders: delivery method: %w", err)
		}
		out["delivery_method"] = m.Name
	}
	return out, nil
}
