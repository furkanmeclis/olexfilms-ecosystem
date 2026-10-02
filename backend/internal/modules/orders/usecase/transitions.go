package usecase

import (
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// Order statuses (chk_orders_status, migration 000049).
const (
	StatusDraft      = "draft"
	StatusSubmitted  = "submitted"
	StatusApproved   = "approved"
	StatusPreparing  = "preparing"
	StatusReady      = "ready"
	StatusProcessing = "processing"
	StatusShipped    = "shipped"
	StatusDelivered  = "delivered"
	StatusReceived   = "received"
	StatusCancelling = "cancelling"
	StatusCancelled  = "cancelled"
)

// Statuses lists every order status in flow order.
var Statuses = []string{
	StatusDraft, StatusSubmitted, StatusApproved, StatusPreparing, StatusReady, StatusProcessing,
	StatusShipped, StatusDelivered, StatusReceived, StatusCancelling, StatusCancelled,
}

// IsStatus reports whether s is a known order status.
func IsStatus(s string) bool {
	for _, v := range Statuses {
		if v == s {
			return true
		}
	}
	return false
}

// Party is the side of the order a transition is made from.
type Party string

const (
	PartyBuyer  Party = "buyer"
	PartySeller Party = "seller"
	PartyEither Party = "either"
)

// rule is one allowed transition: who may make it and with which
// permission (any of the listed slugs).
type rule struct {
	party Party
	perms []string
}

// transitions is the state machine up to shipping (TEC-166, TEC-167):
//
//	draft -> submitted (buyer, orders.write)
//	submitted -> approved (seller, orders.approve; prices and rate freeze)
//	approved -> preparing | processing (seller, orders.ship or orders.approve)
//	preparing -> ready (seller, orders.ship; every line fully assigned)
//	ready -> shipped (seller, orders.ship; order_out per assigned unit)
//	draft | submitted | approved | preparing | ready -> cancelled
//	    (either side, orders.cancel; active reservations are released)
//
// and after shipping (TEC-168):
//
//	shipped -> received (buyer, orders.receive; received per shipped unit,
//	    the units become available at the buyer; all units or none)
//	shipped -> cancelling (either side, orders.cancel; nothing moves yet)
//	cancelling -> cancelled (seller, orders.cancel or orders.ship: the goods
//	    are back; order_cancel_restore per shipped unit)
//
// received is final here (no cancel; returns are a separate flow).
// delivered is in the schema but not used: received follows shipped
// directly, delivered answers 409 ORDER_TRANSITION_UNAVAILABLE.
var transitions = map[string]map[string]rule{
	StatusDraft: {
		StatusSubmitted: {PartyBuyer, []string{rbac.PermOrdersWrite}},
		StatusCancelled: {PartyEither, []string{rbac.PermOrdersCancel}},
	},
	StatusSubmitted: {
		StatusApproved:  {PartySeller, []string{rbac.PermOrdersApprove}},
		StatusCancelled: {PartyEither, []string{rbac.PermOrdersCancel}},
	},
	StatusApproved: {
		StatusPreparing:  {PartySeller, []string{rbac.PermOrdersShip, rbac.PermOrdersApprove}},
		StatusProcessing: {PartySeller, []string{rbac.PermOrdersShip, rbac.PermOrdersApprove}},
		StatusCancelled:  {PartyEither, []string{rbac.PermOrdersCancel}},
	},
	StatusPreparing: {
		StatusReady:     {PartySeller, []string{rbac.PermOrdersShip}},
		StatusCancelled: {PartyEither, []string{rbac.PermOrdersCancel}},
	},
	StatusReady: {
		StatusShipped:   {PartySeller, []string{rbac.PermOrdersShip}},
		StatusCancelled: {PartyEither, []string{rbac.PermOrdersCancel}},
	},
	StatusShipped: {
		StatusReceived:   {PartyBuyer, []string{rbac.PermOrdersReceive}},
		StatusCancelling: {PartyEither, []string{rbac.PermOrdersCancel}},
	},
	StatusCancelling: {
		StatusCancelled: {PartySeller, []string{rbac.PermOrdersCancel, rbac.PermOrdersShip}},
	},
}

// supportedTargets are the statuses this API moves an order to.
var supportedTargets = map[string]bool{
	StatusSubmitted: true, StatusApproved: true, StatusPreparing: true,
	StatusProcessing: true, StatusReady: true, StatusShipped: true, StatusReceived: true,
	StatusCancelling: true, StatusCancelled: true,
}

// lookupTransition returns the rule of from -> to, or ok=false.
func lookupTransition(from, to string) (rule, bool) {
	r, ok := transitions[from][to]
	return r, ok
}

// allows reports whether a caller on side p holding a permission check can
// make the transition.
func (r rule) allows(p Party, can func(slug string) bool) bool {
	if r.party != PartyEither && r.party != p {
		return false
	}
	for _, slug := range r.perms {
		if can(slug) {
			return true
		}
	}
	return false
}

// availableTransitions lists the statuses a caller on side p may move an
// order in status from to, in flow order.
func availableTransitions(from string, p Party, can func(slug string) bool) []string {
	out := []string{}
	if p != PartyBuyer && p != PartySeller {
		return out
	}
	for _, to := range Statuses {
		if r, ok := lookupTransition(from, to); ok && r.allows(p, can) {
			out = append(out, to)
		}
	}
	return out
}

// eventFor maps a target status to its outbox event (events catalog).
func eventFor(to string) string {
	switch to {
	case StatusSubmitted:
		return events.OrdersSubmitted
	case StatusApproved:
		return events.OrdersApproved
	case StatusPreparing:
		return events.OrdersPreparing
	case StatusProcessing:
		return events.OrdersProcessing
	case StatusReady:
		return events.OrdersReady
	case StatusShipped:
		return events.OrdersShipped
	case StatusReceived:
		return events.OrdersReceived
	case StatusCancelling:
		return events.OrdersCancelRequested
	case StatusCancelled:
		return events.OrdersCancelled
	}
	return ""
}
