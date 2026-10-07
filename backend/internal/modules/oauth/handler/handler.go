// Package handler serves the public MCP OAuth endpoints (TEC-400): RFC 8414
// and RFC 9728 metadata, RFC 7591 registration, the token endpoint and RFC
// 7009 revocation. Bodies follow the OAuth RFCs ({"error",
// "error_description"}), not the /v1 response envelope, because MCP clients
// parse them as OAuth.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/model"
)

// maxBody bounds registration, token and revocation bodies.
const maxBody = 16 << 10

// Service is the authorization server usecase.
type Service interface {
	Register(ctx context.Context, ip string, in model.RegisterInput) (model.Client, error)
	Exchange(ctx context.Context, ip string, in model.TokenInput) (model.TokenResponse, error)
	Revoke(ctx context.Context, ip, clientID, token string) error
	AuthorizationServerMetadata() map[string]any
	ProtectedResourceMetadata(resource string) map[string]any
}

// Handler serves the endpoints.
type Handler struct {
	svc Service
	log *slog.Logger
}

// New builds the handler.
func New(svc Service, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{svc: svc, log: log}
}

// cors opens the public endpoints to browser based MCP clients: no cookies
// are involved (public clients, Bearer tokens).
func cors(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, MCP-Protocol-Version")
	h.Set("Access-Control-Max-Age", "86400")
}

// Preflight answers CORS preflight requests.
func (h *Handler) Preflight(w http.ResponseWriter, _ *http.Request) {
	cors(w)
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	cors(w)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var oe *model.Error
	if !errors.As(err, &oe) {
		oe = &model.Error{Status: http.StatusInternalServerError, Code: model.ErrServerError, Description: "internal server error", Cause: err}
	}
	if oe.Status >= http.StatusInternalServerError {
		h.log.ErrorContext(r.Context(), "oauth_server_error", "path", r.URL.Path, "error", oe.Cause)
	}
	if oe.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(oe.RetryAfter.Seconds())+1))
	}
	if oe.Status == http.StatusUnauthorized && oe.Code == model.ErrInvalidClient {
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_client"`)
	}
	writeJSON(w, oe.Status, map[string]string{"error": oe.Code, "error_description": oe.Description})
}

// clientIP is the client address the frontend proxy forwards (first
// X-Forwarded-For hop, same as the other public limits), else the peer.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := strings.TrimSpace(strings.Split(xff, ",")[0]); ip != "" {
			return ip
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// AuthorizationServer serves GET /.well-known/oauth-authorization-server.
func (h *Handler) AuthorizationServer(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.AuthorizationServerMetadata())
}

// ProtectedResource serves GET /.well-known/oauth-protected-resource/mcp/{endpoint}.
func (h *Handler) ProtectedResource(w http.ResponseWriter, r *http.Request) {
	resource := "/mcp/" + r.PathValue("endpoint")
	if !model.IsResource(resource) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found", "error_description": "unknown protected resource"})
		return
	}
	writeJSON(w, http.StatusOK, h.svc.ProtectedResourceMetadata(resource))
}

// Register serves POST /oauth/register (RFC 7591, public clients).
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	var in model.RegisterInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		h.writeError(w, r, model.BadRequest(model.ErrInvalidClientMetadata, "body must be JSON client metadata"))
		return
	}
	c, err := h.svc.Register(r.Context(), clientIP(r), in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id":                  c.ClientID,
		"client_id_issued_at":        c.IssuedAt.Unix(),
		"client_name":                c.ClientName,
		"redirect_uris":              c.RedirectURIs,
		"grant_types":                []string{model.GrantAuthorizationCode, model.GrantRefreshToken},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "none",
		"scope":                      model.ScopeMCP,
	})
}

func parseForm(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	return r.ParseForm() == nil
}

// Token serves POST /oauth/token (form encoded).
func (h *Handler) Token(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		h.writeError(w, r, model.BadRequest(model.ErrInvalidRequest, "form body required"))
		return
	}
	f := r.PostForm
	out, err := h.svc.Exchange(r.Context(), clientIP(r), model.TokenInput{
		GrantType: f.Get("grant_type"), ClientID: f.Get("client_id"), Code: f.Get("code"),
		RedirectURI: f.Get("redirect_uri"), CodeVerifier: f.Get("code_verifier"),
		RefreshToken: f.Get("refresh_token"), Resource: f.Get("resource"), Scope: f.Get("scope"),
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// Revoke serves POST /oauth/revoke (RFC 7009): 200 for unknown tokens too.
func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		h.writeError(w, r, model.BadRequest(model.ErrInvalidRequest, "form body required"))
		return
	}
	if err := h.svc.Revoke(r.Context(), clientIP(r), r.PostForm.Get("client_id"), r.PostForm.Get("token")); err != nil {
		h.writeError(w, r, err)
		return
	}
	cors(w)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
}
