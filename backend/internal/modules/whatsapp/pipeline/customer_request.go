package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	aitools "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	leadsusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/leads/usecase"
)

// RequestAppointment is the customer tool of a WhatsApp turn (TEC-639): the
// assistant cannot book, so an appointment request becomes a lead at the
// customer's dealer, who schedules it. Without it the model used to tell
// the customer an appointment "is being created".
type RequestAppointment struct{}

// Spec implements aitools.Tool.
func (RequestAppointment) Spec() aitools.Spec {
	str := func(desc string, max int) map[string]any {
		return map[string]any{"type": "string", "maxLength": max, "description": desc}
	}
	return aitools.Spec{
		Name: "request_appointment",
		Description: "Forward the customer's appointment request to their dealer (the dealer of their latest service, " +
			"else the brand center). This does not book an appointment: the dealer contacts the customer to agree " +
			"on the time. Call it once the customer confirmed the summary of the request. A request within 30 days " +
			"updates the earlier one.",
		InputSchema: map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{
				"preferred_time": str("The date and time the customer asked for, as they wrote it.", 200),
				"vehicle":        str("The vehicle (plate, or brand, model and year).", 500),
				"note":           str("The service asked for (e.g. paint protection film) and any short note.", 500),
			},
		},
		Kind:  aitools.KindSelf,
		Realm: aitools.RealmCustomer,
	}
}

// Request tool statuses (result field status).
const (
	RequestStatusCreated = "forwarded"
	RequestStatusUpdated = "updated"
)

// Run implements aitools.Tool.
func (RequestAppointment) Run(ctx context.Context, env aitools.Env, raw json.RawMessage) (aitools.Result, error) {
	t, ok := visitorTurnFrom(ctx)
	if !ok || t.p.d.Leads == nil {
		return aitools.ErrorResult(aitools.CodeToolNotAllowed,
			"Appointment requests can only be forwarded on WhatsApp. You cannot book appointments: tell the "+
				"customer to contact their dealer to schedule one."), nil
	}
	var in struct {
		PreferredTime string `json:"preferred_time"`
		Vehicle       string `json:"vehicle"`
		Note          string `json:"note"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return aitools.ErrorResult(aitools.CodeInvalidInput, "invalid input: "+err.Error()), nil
	}
	p, r := t.p, t.r
	res, err := p.d.Leads.UpsertCustomerAppointmentLead(ctx, t.a.brand.ID, leadsusecase.CustomerAppointmentInput{
		CustomerUserID: env.Principal.Auth.UserInternal,
		PreferredTime:  in.PreferredTime, Vehicle: in.Vehicle, Message: in.Note,
		Language: strings.ReplaceAll(r.locale, "_", "-"), ConversationUUID: r.conv.Uuid.String(),
	})
	var verr *leadsusecase.ValidationError
	switch {
	case errors.As(err, &verr):
		return aitools.ErrorResult(aitools.CodeInvalidInput, "invalid "+verr.Field+": "+verr.Message+
			". You cannot book appointments: tell the customer to contact their dealer."), nil
	case err != nil:
		return aitools.Result{}, err
	}
	status := RequestStatusUpdated
	if res.Created {
		status = RequestStatusCreated
	}
	r.stage(p.now(), StageLead, map[string]any{"status": status, "routed_by": res.RoutedBy, "customer": true})
	return aitools.JSONResult(leadResult{Status: status,
		Note: "The request was forwarded to the dealer; no appointment is booked yet. Tell the customer the dealer " +
			"will contact them to agree on the time. Do not say an appointment was created or confirmed, and do " +
			"not promise a time or a price."})
}
