package ubl

import (
	"errors"
	"net/http"
)

// Business-rule codes of the mapper. Handlers answer them with 422 and the
// frontend translates them under errors.json "codes.<CODE>".
const (
	CodeInvalidTaxID      = "EINVOICE_INVALID_TAX_ID"
	CodeUnsupportedBuyer  = "EINVOICE_UNSUPPORTED_BUYER"
	CodeInvalidDocument   = "EINVOICE_INVALID_DOCUMENT"
	CodeInvalidVATRate    = "EINVOICE_INVALID_VAT_RATE"
	CodeInvalidStylesheet = "EINVOICE_INVALID_STYLESHEET"
)

// Sentinels for errors.Is; every *Error unwraps to one of them.
var (
	ErrInvalidTaxID = errors.New("einvoice: invalid VKN/TCKN")
	// ErrUnsupportedBuyer: F5 invoices only TRY to buyers in Türkiye
	// (QUESTIONS S28); foreign buyers and currencies wait for IHRACAT.
	ErrUnsupportedBuyer  = errors.New("einvoice: unsupported buyer or currency")
	ErrInvalidDocument   = errors.New("einvoice: invalid document")
	ErrInvalidVATRate    = errors.New("einvoice: invalid VAT rate")
	ErrInvalidStylesheet = errors.New("einvoice: invalid XSLT stylesheet")
)

// Error is a named business-rule failure (422). Field names the input part
// ("buyer.vkn", "lines[2].vat_rate", ...).
type Error struct {
	Code    string
	Field   string
	Message string
	base    error
}

func (e *Error) Error() string {
	if e.Field == "" {
		return e.base.Error() + ": " + e.Message
	}
	return e.base.Error() + ": " + e.Field + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.base }

// HTTPStatus is the status a handler answers with.
func (e *Error) HTTPStatus() int { return http.StatusUnprocessableEntity }

func taxIDError(field, msg string) error {
	return &Error{Code: CodeInvalidTaxID, Field: field, Message: msg, base: ErrInvalidTaxID}
}

func unsupportedBuyer(field, msg string) error {
	return &Error{Code: CodeUnsupportedBuyer, Field: field, Message: msg, base: ErrUnsupportedBuyer}
}

func invalidDocument(field, msg string) error {
	return &Error{Code: CodeInvalidDocument, Field: field, Message: msg, base: ErrInvalidDocument}
}

func invalidVATRate(field, msg string) error {
	return &Error{Code: CodeInvalidVATRate, Field: field, Message: msg, base: ErrInvalidVATRate}
}

func stylesheetError(msg string) error {
	return &Error{Code: CodeInvalidStylesheet, Message: msg, base: ErrInvalidStylesheet}
}

// StylesheetError is the 422 ErrInvalidStylesheet of an uploaded XSLT the
// caller rejected before rendering (wrong root element, too large).
func StylesheetError(msg string) error { return stylesheetError(msg) }
