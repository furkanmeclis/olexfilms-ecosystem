package ubl

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
//
// The Schematron "IssueDate is not in the future" rule is judged by today
// in Türkiye at time.Now; ValidateAt takes the clock.
func Validate(ctx context.Context, xml []byte, profile string) (Validation, error) {
	return ValidateAt(ctx, xml, profile, time.Now())
}

// ValidateAt is Validate with "today" taken from now (in Türkiye time).
func ValidateAt(ctx context.Context, xml []byte, profile string, now time.Time) (Validation, error) {
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
			if len(se.Violations) == 0 {
				return Validation{Status: StatusRuleWarnings, Messages: nonEmpty(se.Messages, se.Error())}, nil
			}
			vs := dropFutureDateOutsideTR(se.Violations, now)
			if len(vs) == 0 {
				return Validation{Status: StatusValid, Messages: []string{}}, nil
			}
			return Validation{Status: StatusRuleWarnings, Messages: violationMessages(vs)}, nil
		}
		return Validation{}, fmt.Errorf("einvoice: schematron: %w", err)
	}
	return Validation{Status: StatusValid, Messages: []string{}}, nil
}

// futureIssueDateRule is the message of the GİB Schematron TimeCheck assert
// "xs:date(cbc:IssueDate) le current-date()".
const futureIssueDateRule = "günün tarihinden ileri bir tarih olamaz"

// dropFutureDateOutsideTR re-evaluates the Schematron TimeCheck rule in
// Türkiye time (TEC-775). The XPath engine takes current-date() from the
// process zone (UTC in the container), while IssueDate is written in
// Türkiye time (Build); between 00:00 and 03:00 TRT the rule would reject
// an invoice dated today. The violation is kept when IssueDate is after
// today in Türkiye; every other violation is kept as is.
func dropFutureDateOutsideTR(vs []ubltr.Violation, now time.Time) []ubltr.Violation {
	today := now.In(trTime).Format(dateLayout)
	out := make([]ubltr.Violation, 0, len(vs))
	for _, v := range vs {
		if strings.Contains(v.Message, futureIssueDateRule) {
			if d, ok := quotedDate(v.Message); ok && d <= today {
				continue
			}
		}
		out = append(out, v)
	}
	return out
}

// quotedDate returns the first '...' quoted YYYY-MM-DD value of msg.
func quotedDate(msg string) (string, bool) {
	_, rest, ok := strings.Cut(msg, "'")
	if !ok {
		return "", false
	}
	d, _, ok := strings.Cut(rest, "'")
	if !ok {
		return "", false
	}
	if _, err := time.Parse(dateLayout, d); err != nil {
		return "", false
	}
	return d, true
}

// violationMessages formats violations like go-ubltr does.
func violationMessages(vs []ubltr.Violation) []string {
	msgs := make([]string, 0, len(vs))
	for _, v := range vs {
		if v.Path != "" {
			msgs = append(msgs, v.Layer+": "+v.Message+" ("+v.Path+")")
			continue
		}
		msgs = append(msgs, v.Layer+": "+v.Message)
	}
	return msgs
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
