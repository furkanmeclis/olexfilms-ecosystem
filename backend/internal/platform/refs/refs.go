// Package refs is the shared cross-record reference of API responses
// (TEC-1215 / TEC-1216). Every foreign key an external response carries is
// a Ref: the stable public uuid, a display name and the record kind the
// clients map to a detail route. Internal numeric ids never leave the API.
//
// The kind vocabulary is the same one report rows use in _link.kind
// (docs/refs.md); web entityHref(kind, uuid) and mobile useEntityHref(kind)
// resolve these values.
package refs

import (
	"slices"

	"github.com/google/uuid"
)

// Kind names the kind of a referenced record.
type Kind = string

// Record kinds (docs/refs.md).
const (
	KindService       Kind = "service"
	KindWarranty      Kind = "warranty"
	KindWarrantyClaim Kind = "warranty_claim"
	KindCustomer      Kind = "customer"
	KindVehicle       Kind = "vehicle"
	KindOrder         Kind = "order"
	KindProduct       Kind = "product"
	KindStockUnit     Kind = "stock_unit"
	KindTransfer      Kind = "transfer"
	KindOrganization  Kind = "organization"
	KindUser          Kind = "user"
	KindLead          Kind = "lead"
	KindAppointment   Kind = "appointment"
	KindTask          Kind = "task"
	KindMeasurement   Kind = "measurement"
	KindContract      Kind = "contract"
	KindInvoice       Kind = "invoice"
	KindCari          Kind = "cari"
	KindAnnouncement  Kind = "announcement"
	KindWarehouse     Kind = "warehouse"
	KindSubscription  Kind = "subscription"
)

// Kinds is the whole vocabulary in documentation order.
var Kinds = []Kind{
	KindService, KindWarranty, KindWarrantyClaim, KindCustomer, KindVehicle, KindOrder,
	KindProduct, KindStockUnit, KindTransfer, KindOrganization, KindUser, KindLead,
	KindAppointment, KindTask, KindMeasurement, KindContract, KindInvoice, KindCari,
	KindAnnouncement, KindWarehouse, KindSubscription,
}

// IsKind reports whether k is in the vocabulary.
func IsKind(k string) bool { return slices.Contains(Kinds, k) }

// Ref names a related record. Sub is an optional second line (a plate, an
// SKU, an organization type) clients show under the name.
type Ref struct {
	Kind Kind      `json:"kind"`
	UUID uuid.UUID `json:"uuid"`
	Name string    `json:"name"`
	Sub  *string   `json:"sub,omitempty"`
}

// New builds a ref without a sub line.
func New(kind Kind, id uuid.UUID, name string) Ref { return Ref{Kind: kind, UUID: id, Name: name} }

// WithSub builds a ref with a sub line; an empty sub is omitted.
func WithSub(kind Kind, id uuid.UUID, name, sub string) Ref {
	r := New(kind, id, name)
	if sub != "" {
		r.Sub = &sub
	}
	return r
}

// Ptr is a ref pointer, nil when id is the zero uuid (an unset foreign key).
func Ptr(kind Kind, id uuid.UUID, name string) *Ref {
	if id == uuid.Nil {
		return nil
	}
	r := New(kind, id, name)
	return &r
}

// UserRef is a staff user (kind user).
func UserRef(id uuid.UUID, name string) Ref { return New(KindUser, id, name) }

// UserRefPtr is UserRef for an optional foreign key (nil on uuid.Nil).
func UserRefPtr(id uuid.UUID, name string) *Ref { return Ptr(KindUser, id, name) }

// FullName joins a name and surname with one space (either may be empty).
func FullName(name, surname string) string {
	switch {
	case name == "":
		return surname
	case surname == "":
		return name
	}
	return name + " " + surname
}

// OrgRef is an organization; orgType (center, distributor, dealer, …) is
// the sub line.
func OrgRef(id uuid.UUID, name, orgType string) Ref {
	return WithSub(KindOrganization, id, name, orgType)
}

// ProductRef is a product; the SKU is the sub line.
func ProductRef(id uuid.UUID, name, sku string) Ref { return WithSub(KindProduct, id, name, sku) }

// CustomerRef is a customer (the user uuid of the customer account).
func CustomerRef(id uuid.UUID, name string) Ref { return New(KindCustomer, id, name) }
