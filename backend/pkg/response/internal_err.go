package response

import "net/http"

type serverErrorRecorder interface {
	RecordServerError(err error, publicMessage string)
}

// InternalErr writes a generic 500 response and records err for server-side logging.
func InternalErr(w http.ResponseWriter, r *http.Request, err error, publicMessage string) {
	if err != nil {
		if rec, ok := w.(serverErrorRecorder); ok {
			rec.RecordServerError(err, publicMessage)
		}
	}
	Internal(w, r, publicMessage)
}
