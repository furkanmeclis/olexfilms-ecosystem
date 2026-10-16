package response

import (
	"errors"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
)

// QueryValidation writes a list query error from pkg/apiquery (sort,
// filter, range) as 400 VALIDATION_ERROR and reports whether it did
// (TEC-365). Other errors are left to the caller.
func QueryValidation(w http.ResponseWriter, r *http.Request, err error) bool {
	var ve *apiquery.ValidationError
	if !errors.As(err, &ve) {
		return false
	}
	details := make([]Detail, 0, len(ve.Details))
	for _, d := range ve.Details {
		details = append(details, Detail{Field: d.Field, Message: d.Message, Code: d.Code})
	}
	ValidationError(w, r, details)
	return true
}

// codedError is an error carrying a stable API error code (e.g.
// ioengine.RowLimitError, IMPORT_TOO_MANY_ROWS).
type codedError interface {
	error
	APICode() string
}

// Coded writes an error with a stable code as 400 with that code and its
// (localized) message, and reports whether it did. Other errors are left
// to the caller.
func Coded(w http.ResponseWriter, r *http.Request, err error) bool {
	var ce codedError
	if !errors.As(err, &ce) {
		return false
	}
	BadRequest(w, r, ce.APICode(), ce.Error())
	return true
}
