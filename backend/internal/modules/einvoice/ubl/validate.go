package ubl

import (
	"context"
	"errors"
	"fmt"

	ubltr "github.com/furkanmeclis/go-ubltr"
	"github.com/furkanmeclis/go-ubltr/schema/ubltr121/invoice"
)

// Validation statuses (einvoices.validation_status).
const (
	StatusValid        = "valid"
	StatusRuleWarnings = "rule_warnings"
	StatusInvalid      = "invalid"
)

// Validation is the outcome of Validate. Messages holds the XSD errors
// (invalid) or the Schematron violations (rule_warnings).
type Validation struct {
	Status   string   `json:"status"`
	Messages []string `json:"messages"`
}

// Archivable reports whether the invoice may be archived. Schematron
// violations block archiving too (conservative default, QUESTIONS S29).
func (v Validation) Archivable() bool { return v.Status == StatusValid }

// Validate checks xml against the GİB XSD and then the Schematron of
// profile (TypeEArchive for EARSIVFATURA, TypeEFatura otherwise). An XSD
// failure is StatusInvalid and Schematron is not run; Schematron
// violations are StatusRuleWarnings. The error is only for engine
// failures (unreadable XML, cancelled context).
func Validate(ctx context.Context, xml []byte, profile string) (Validation, error) {
	if err := ubltr.ValidateXSD(ctx, xml); err != nil {
		var xe *ubltr.XSDValidationError
		if errors.As(err, &xe) {
			return Validation{Status: StatusInvalid, Messages: nonEmpty(xe.Messages, xe.Error())}, nil
		}
		return Validation{}, fmt.Errorf("einvoice: xsd: %w", err)
	}
	if err := ubltr.ValidateSchematronType(ctx, xml, SchematronType(profile)); err != nil {
		var se *ubltr.SchematronValidationError
		if errors.As(err, &se) {
			return Validation{Status: StatusRuleWarnings, Messages: nonEmpty(se.Messages, se.Error())}, nil
		}
		return Validation{}, fmt.Errorf("einvoice: schematron: %w", err)
	}
	return Validation{Status: StatusValid, Messages: []string{}}, nil
}

func nonEmpty(msgs []string, fallback string) []string {
	if len(msgs) == 0 {
		return []string{fallback}
	}
	return msgs
}

// Marshal encodes inv as UBL-TR XML (ubltr.Marshal).
func Marshal(inv *invoice.Invoice) ([]byte, error) {
	if inv == nil {
		return nil, invalidDocument("", "nil invoice")
	}
	return ubltr.Marshal(inv)
}
