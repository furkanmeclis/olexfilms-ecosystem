// Package model holds the photo standard types shared with the services and
// contracts modules (TEC-499, F5-07b) without importing the use case.
package model

import (
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
)

// CodeIncomplete is the 422 code of a service whose required intake angles
// are not all photographed.
const CodeIncomplete = "PHOTO_STANDARD_INCOMPLETE"

// ServiceRef identifies the service an intake check runs for.
type ServiceRef struct {
	ID             int64
	OrganizationID int64
	BrandID        int64
}

// IncompleteError lists the required angle keys that have no active photo.
type IncompleteError struct {
	Missing []string
}

func (e *IncompleteError) Error() string {
	return "photo_standard: missing required angles: " + strings.Join(e.Missing, ", ")
}

// WriteIncomplete writes the 422 PHOTO_STANDARD_INCOMPLETE envelope: one
// detail per missing angle and data.missing_angles with the keys.
func WriteIncomplete(w http.ResponseWriter, r *http.Request, e *IncompleteError) {
	details := make([]response.Detail, 0, len(e.Missing))
	for _, key := range e.Missing {
		details = append(details, response.Detail{Field: "intake_photos." + key, Message: "required intake photo is missing", Code: "missing"})
	}
	response.ErrorWithData(w, r, http.StatusUnprocessableEntity, CodeIncomplete,
		"Every required intake photo angle must be photographed first", details,
		map[string]any{"missing_angles": e.Missing})
}
