package middleware

import (
	"context"
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/logging"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

type ctxKey string

const requestIDKey ctxKey = "request_id"

// RequestIDHeader is the correlation id HTTP header.
const RequestIDHeader = "X-Request-ID"

func init() {
	response.RequestIDFunc = GetRequestID
	logging.RequestIDFromContext = GetRequestID
}

// RequestID ensures every request has a correlation id in context and response headers.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set(RequestIDHeader, id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID extracts the request id from context.
func GetRequestID(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}
