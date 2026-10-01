package response

import "net/http"

type serverErrorRecorder interface {
	RecordServerError(err error, publicMessage string)
}

// ServerErrorHook, when set, receives every error passed to InternalErr with
// the handler's request (error tracking; set by internal/errtrack).
var ServerErrorHook func(r *http.Request, err error)

// InternalErr writes a generic 500 response and records err for server-side logging.
func InternalErr(w http.ResponseWriter, r *http.Request, err error, publicMessage string) {
	if err != nil {
		if rec, ok := w.(serverErrorRecorder); ok {
			rec.RecordServerError(err, publicMessage)
		}
		if ServerErrorHook != nil {
			ServerErrorHook(r, err)
		}
	}
	Internal(w, r, publicMessage)
}
