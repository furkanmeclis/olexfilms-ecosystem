package activity

import "context"

type originKey struct{}

// Origin marks activity recorded on behalf of another channel (e.g. the AI
// assistant). Every Record call made with a context carrying an Origin adds
// "via" (and the optional reference fields) to the event payload, so domain
// events created by an assistant action show up as "via AI" in the audit log.
type Origin struct {
	Via              string
	ActionUUID       string
	ConversationUUID string
}

// WithOrigin returns a context whose activity events are marked with o.
func WithOrigin(ctx context.Context, o Origin) context.Context {
	return context.WithValue(ctx, originKey{}, o)
}

// OriginFrom returns the origin stored in ctx, if any.
func OriginFrom(ctx context.Context) (Origin, bool) {
	o, ok := ctx.Value(originKey{}).(Origin)
	return o, ok && o.Via != ""
}

func withOriginPayload(ctx context.Context, payload map[string]any) map[string]any {
	o, ok := OriginFrom(ctx)
	if !ok {
		return payload
	}
	out := make(map[string]any, len(payload)+3)
	for k, v := range payload {
		out[k] = v
	}
	out["via"] = o.Via
	if o.ActionUUID != "" {
		out["ai_action_uuid"] = o.ActionUUID
	}
	if o.ConversationUUID != "" {
		out["ai_conversation_uuid"] = o.ConversationUUID
	}
	return out
}
