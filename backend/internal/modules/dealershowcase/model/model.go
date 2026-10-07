// Package model holds the dealer showcase vocabulary (TEC-466, F5-01a). The
// values mirror the CHECK constraints of migration 000111.
package model

// Showcase status. It follows the latest submission; the public page reads
// the published snapshot, which survives later submissions.
const (
	StatusDraft         = "draft"
	StatusPendingReview = "pending_review"
	StatusPublished     = "published"
	StatusRejected      = "rejected"
)

// Statuses lists every showcase status (list filter whitelist).
var Statuses = []string{StatusDraft, StatusPendingReview, StatusPublished, StatusRejected}

// Transitions is the status graph. With showcase.approval_required on, an
// edited showcase goes to pending_review and the center publishes or
// rejects it; with it off the owner publishes directly. A published or
// rejected showcase is submitted again after an edit.
var Transitions = map[string][]string{
	StatusDraft:         {StatusPendingReview, StatusPublished},
	StatusPendingReview: {StatusPublished, StatusRejected},
	StatusPublished:     {StatusPendingReview, StatusPublished},
	StatusRejected:      {StatusPendingReview, StatusPublished},
}

// CanTransition reports whether the graph allows from → to.
func CanTransition(from, to string) bool {
	for _, s := range Transitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// Service kinds: a catalog product category of the brand or a free text
// entry. Services never carry a price (F4 S10, F5 S8).
const (
	ServiceKindProductCategory = "product_category"
	ServiceKindCustom          = "custom"
)

// ServiceKinds lists every service kind.
var ServiceKinds = []string{ServiceKindProductCategory, ServiceKindCustom}

// Google rating sources: the Places worker or the owner's manual entry
// (F5 S7: manual while no GOOGLE_PLACES_API_KEY is set).
const (
	RatingSourcePlaces = "places"
	RatingSourceManual = "manual"
)

// Social link networks; every value is an https URL.
var SocialNetworks = []string{"instagram", "facebook", "youtube", "tiktok", "website"}

// Content fields of one locale in content / published_content.
const (
	ContentHeadline = "headline"
	ContentAbout    = "about"
)

// Gallery photo types and the size cap of one photo (the DB CHECK).
var PhotoMimes = []string{"image/jpeg", "image/png", "image/webp"}

const MaxPhotoBytes = 20 << 20
