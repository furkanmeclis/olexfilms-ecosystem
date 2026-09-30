package httpserver

import (
	"net/http"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/docs"
)

func (s *Server) mountDocs(mux *http.ServeMux) {
	openapi, err := docs.FS.ReadFile("openapi.yaml")
	if err != nil {
		s.log.Error("docs_openapi_missing", "error", err)
		return
	}

	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs/", http.StatusFound)
	})
	mux.HandleFunc("GET /docs/", s.serveDocsIndex)
	mux.HandleFunc("GET /docs/openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(openapi)
	})
}

func (s *Server) serveDocsIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/docs/" && !strings.HasSuffix(r.URL.Path, "/docs/") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(scalarIndexHTML))
}

const scalarIndexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>App API Docs</title>
</head>
<body>
  <div id="app"></div>
  <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference@1.62.4"></script>
  <script>
    Scalar.createApiReference('#app', {
      url: '/docs/openapi.yaml',
      persistAuth: true
    });
  </script>
</body>
</html>
`
