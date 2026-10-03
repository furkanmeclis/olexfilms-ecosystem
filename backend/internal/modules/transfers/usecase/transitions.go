package usecase

import (
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

// Request statuses (chk_stock_transfer_requests_status, migration 000055).
const (
	StatusRequested = "requested"
	StatusApproved  = "approved"
	StatusRejected  = "rejected"
	StatusShipped   = "shipped"
	StatusReceived  = "received"
	StatusCancelled = "cancelled"
)

// Statuses lists every request status in flow order.
var Statuses = []string{
	StatusRequested, StatusApproved, StatusRejected, StatusShipped, StatusReceived, StatusCancelled,
}

// IsStatus reports whether s is a known request status.
func IsStatus(s string) bool {
	for _, v := range Statuses {
		if v == s {
			return true
		}
	}
	return false
}

// Party is the caller's side of a request.
type Party string

const (
	// PartySender is the giving organization (from_org_id).
	PartySender Party = "sender"
	// PartyReceiver is the sibling receiving the units (to_org_id).
	PartyReceiver Party = "receiver"
	// PartyParent is the common parent (approver_org_id, K13).
	PartyParent Party = "parent"
)

// grant is one side allowed to make a transition with any of perms.
type grant struct {
	party Party
	perms []string
}

var (
	reqPerm     = []string{rbac.PermTransfersRequest}
	approvePerm = []string{rbac.PermTransfersApprove}
)

// transitions is the request state machine (TEC-197):
//
//	requested -> approved | rejected (receiver with transfers.request, or the
//	    common parent with transfers.approve; rejected writes no movement)
//	requested -> cancelled (sender or parent; no movement)
//	approved -> shipped (sender; transfer_out per unit, in transit to the receiver)
//	approved -> cancelled (any side; no movement)
//	shipped -> received (receiver; transfer_in per unit, available at the receiver)
//	shipped -> cancelled (sender: the goods are back; transfer_cancel_restore per unit)
//
// rejected, received and cancelled are final.
var transitions = map[string]map[string][]grant{
	StatusRequested: {
		StatusApproved:  {{PartyReceiver, reqPerm}, {PartyParent, approvePerm}},
		StatusRejected:  {{PartyReceiver, reqPerm}, {PartyParent, approvePerm}},
		StatusCancelled: {{PartySender, reqPerm}, {PartyParent, approvePerm}},
	},
	StatusApproved: {
		StatusShipped:   {{PartySender, reqPerm}},
		StatusCancelled: {{PartySender, reqPerm}, {PartyReceiver, reqPerm}, {PartyParent, approvePerm}},
	},
	StatusShipped: {
		StatusReceived:  {{PartyReceiver, reqPerm}},
		StatusCancelled: {{PartySender, reqPerm}},
	},
}

// Request kinds (stock_transfer_requests.kind, migration 000065).
const (
	// KindSibling is a K13 transfer between siblings (TEC-197).
	KindSibling = "sibling"
	// KindReturn is a return to the giver's direct parent (TEC-223).
	KindReturn = "return"
)

// IsKind reports whether k is a known request kind.
func IsKind(k string) bool { return k == KindSibling || k == KindReturn }

// returnTransitions is the state machine of a return (TEC-223); the
// receiver is the parent, so every receiving step belongs to it:
//
//	requested -> approved | rejected (parent; rejected writes no movement)
//	requested -> cancelled (giver or parent; no movement)
//	approved -> shipped (giver; transfer_out per unit, in transit to the parent)
//	approved -> cancelled (giver or parent; no movement)
//	shipped -> received (parent; transfer_in per unit + accounting reversal)
//	shipped -> cancelled (giver: the goods are back; transfer_cancel_restore)
var returnTransitions = map[string]map[string][]grant{
	StatusRequested: {
		StatusApproved:  {{PartyParent, approvePerm}},
		StatusRejected:  {{PartyParent, approvePerm}},
		StatusCancelled: {{PartySender, reqPerm}, {PartyParent, approvePerm}},
	},
	StatusApproved: {
		StatusShipped:   {{PartySender, reqPerm}},
		StatusCancelled: {{PartySender, reqPerm}, {PartyParent, approvePerm}},
	},
	StatusShipped: {
		StatusReceived:  {{PartyParent, []string{rbac.PermTransfersApprove, rbac.PermTransfersRequest}}},
		StatusCancelled: {{PartySender, reqPerm}},
	},
}

func machine(kind string) map[string]map[string][]grant {
	if kind == KindReturn {
		return returnTransitions
	}
	return transitions
}

// lookupTransition returns the grants of from -> to of a sibling transfer,
// or ok=false.
func lookupTransition(from, to string) ([]grant, bool) {
	return lookupKindTransition(KindSibling, from, to)
}

// lookupKindTransition returns the grants of from -> to for a request kind.
func lookupKindTransition(kind, from, to string) ([]grant, bool) {
	g, ok := machine(kind)[from][to]
	return g, ok
}

// allowed reports whether a caller on side p holding a permission check can
// make a transition with grants gs.
func allowed(gs []grant, p Party, can func(slug string) bool) bool {
	for _, g := range gs {
		if g.party != p {
			continue
		}
		for _, slug := range g.perms {
			if can(slug) {
				return true
			}
		}
	}
	return false
}

// availableTransitions lists the statuses a caller on side p may move a
// sibling request in status from to, in flow order.
func availableTransitions(from string, p Party, can func(slug string) bool) []string {
	return availableKindTransitions(KindSibling, from, p, can)
}

// availableKindTransitions is availableTransitions for a request kind.
func availableKindTransitions(kind, from string, p Party, can func(slug string) bool) []string {
	out := []string{}
	if p == "" {
		return out
	}
	for _, to := range Statuses {
		if gs, ok := lookupKindTransition(kind, from, to); ok && allowed(gs, p, can) {
			out = append(out, to)
		}
	}
	return out
}

// eventFor maps a target status to its outbox event.
func eventFor(to string) string {
	switch to {
	case StatusRequested:
		return events.TransfersRequested
	case StatusApproved:
		return events.TransfersApproved
	case StatusRejected:
		return events.TransfersRejected
	case StatusShipped:
		return events.TransfersShipped
	case StatusReceived:
		return events.TransfersReceived
	case StatusCancelled:
		return events.TransfersCancelled
	}
	return ""
}
