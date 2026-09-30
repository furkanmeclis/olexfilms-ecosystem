package logging

import "context"

// RequestIDFromContext extracts a correlation id from context.
// Wired by HTTP middleware so persist handlers can attach request_id.
var RequestIDFromContext = func(context.Context) string { return "" }
