package mcp

import (
	"net/http"

	oauthmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/model"
)

// RegisterRoutes mounts the three MCP endpoints outside /v1 (the frontend
// proxies /mcp/*). POST carries the JSON-RPC messages; GET and DELETE are
// answered by the stateless SDK handler (405) after the same Bearer check.
func RegisterRoutes(mux *http.ServeMux, s *Server) {
	for _, path := range oauthmodel.Resources {
		for _, method := range []string{http.MethodPost, http.MethodGet, http.MethodDelete} {
			mux.Handle(method+" "+path, s)
		}
	}
}
