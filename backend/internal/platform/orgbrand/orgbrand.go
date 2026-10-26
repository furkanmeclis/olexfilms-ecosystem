// Package orgbrand resolves the name and logo an organization shows to
// customers (TEC-1003). A brand center with its service point on (AFM
// Florya, the center's own service point) shows the service point name and
// logo on customer surfaces — warranty page and PDF, service and
// measurement PDFs, portal, WhatsApp — while the panel keeps the
// organization name.
package orgbrand

import (
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

// ServicePoint reports whether org shows a service point to customers.
func ServicePoint(org db.Organization) bool {
	return org.Type == "center" && org.ServicePointEnabled &&
		org.ServicePointName.Valid && strings.TrimSpace(org.ServicePointName.String) != ""
}

// ForCustomer returns org with Name and LogoObjectKey replaced by the
// service point's (the logo falls back to the organization logo).
func ForCustomer(org db.Organization) db.Organization {
	if !ServicePoint(org) {
		return org
	}
	org.Name = strings.TrimSpace(org.ServicePointName.String)
	if k := org.ServicePointLogoObjectKey; k.Valid && strings.TrimSpace(k.String) != "" {
		org.LogoObjectKey = k
	}
	return org
}

// Name is the customer-facing name of org.
func Name(org db.Organization) string { return ForCustomer(org).Name }

// Fields is the customer-facing name and logo key when only the columns are
// at hand (list queries that do not return the whole row).
func Fields(orgType, name string, enabled bool, spName, logo, spLogo pgtype.Text) (string, pgtype.Text) {
	org := ForCustomer(db.Organization{
		Type: orgType, Name: name, ServicePointEnabled: enabled, ServicePointName: spName,
		LogoObjectKey: logo, ServicePointLogoObjectKey: spLogo,
	})
	return org.Name, org.LogoObjectKey
}
