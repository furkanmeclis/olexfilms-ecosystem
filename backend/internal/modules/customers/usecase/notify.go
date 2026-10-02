package usecase

// TEC-164: customer.created. When an organization creates a new customer
// (a new user, not the link of an existing phone), the create transaction
// writes customer.created to the outbox; the notification module turns it
// into the CUSTOMER_WELCOME WhatsApp message (13 locales, catalog
// templates) with the /portal link. A customer without a phone, or one
// whose WhatsApp preference is off, gets no event.

import (
	"context"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/jackc/pgx/v5"
)

// SetPortalURL sets the frontend base URL; the welcome message links
// <base>/portal. Empty disables the customer.created event.
func (s *Service) SetPortalURL(frontendURL string) {
	base := strings.TrimRight(strings.TrimSpace(frontendURL), "/")
	if base == "" {
		s.portal = ""
		return
	}
	s.portal = base + "/portal"
}

// PortalURL is the link of the welcome message ("" when not configured).
func (s *Service) PortalURL() string { return s.portal }

// welcomeWanted reports whether a new customer gets the welcome message:
// it needs a phone and must not have switched WhatsApp off at creation.
func welcomeWanted(user db.User, p profilePatch) bool {
	if !user.PhoneE164.Valid || strings.TrimSpace(user.PhoneE164.String) == "" {
		return false
	}
	if p.Prefs.Set && p.Prefs.Value != nil {
		if on, ok := (*p.Prefs.Value)["whatsapp"]; ok && !on {
			return false
		}
	}
	return true
}

// CreatedEvent builds the customer.created outbox event. The payload holds
// no phone or e-mail (the dispatcher reads the recipient's phone itself).
func CreatedEvent(c Caller, user db.User, portalURL string) events.Event {
	id, u := user.ID, user.Uuid
	ev := events.New(events.CustomerCreated).WithTenant(c.Org.InternalID).
		WithEntity("user", &id, &u).
		WithPayload(map[string]any{
			"customer_user_id":  user.ID,
			"customer_uuid":     user.Uuid.String(),
			"customer_name":     strings.TrimSpace(user.Name + " " + user.Surname),
			"organization_id":   c.Org.InternalID,
			"organization_name": c.Org.Name,
			"brand_id":          c.Org.BrandID,
			"portal_url":        portalURL,
			"has_phone":         user.PhoneE164.Valid && user.PhoneE164.String != "",
		})
	if c.UserID != 0 {
		ev = ev.WithActor(c.UserID)
	}
	return ev
}

func (s *Service) enqueueCreated(ctx context.Context, tx pgx.Tx, c Caller, user db.User, p profilePatch) error {
	if s.out == nil || s.portal == "" || !welcomeWanted(user, p) {
		return nil
	}
	if err := s.out.Enqueue(ctx, tx, CreatedEvent(c, user, s.portal)); err != nil {
		return fmt.Errorf("customers: outbox: %w", err)
	}
	return nil
}
