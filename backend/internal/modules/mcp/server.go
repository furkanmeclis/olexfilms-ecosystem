// Package mcp is the MCP Streamable HTTP server (TEC-402, F4-03c): one code
// base behind /mcp/dealer, /mcp/customer and /mcp/user, on the official Go
// SDK in stateless mode.
//
// Every HTTP request is checked on its own:
//
//	Bearer token (F4-03a, audience = endpoint; 401 + WWW-Authenticate with
//	the RFC 9728 metadata link) → mcp module of the token's organization
//	(403) → principal: user + organization + endpoint realm with the
//	permissions held now (403 when membership or mcp.connect was lost) →
//	request limits per token (60/min) and per organization
//	(sysconfig mcp.requests_per_hour_per_org; 429 + JSON-RPC error) →
//	tools/list = AI tool registry Available(principal), listChanged false →
//	tools/call checks the same again (Registry.Call / Propose).
//
// Write tools never run here: they become an ai_pending_actions row
// (source mcp, F4-01e) and the result points the user to the panel, where
// the action is approved (F4-03d). Every tools/call is written to the
// activity log with the tool, duration and outcome; personal data in the
// arguments is masked. AI token quotas do not apply (the model runs on the
// client).
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	aitools "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	aiusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/usecase"
	oauthmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/model"
	oauthusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/sysconfig"
	"github.com/google/uuid"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Request limits.
const (
	// TokenRequestsPerMinute caps one connection (token family).
	TokenRequestsPerMinute = 60
	tokenWindow            = time.Minute
	orgWindow              = time.Hour
	// maxBodyBytes bounds a request body (also the SDK limit).
	maxBodyBytes = 4 << 20
)

// JSON-RPC error codes of the HTTP gate (implementation-defined server
// error range).
const (
	CodeUnauthorized = -32001
	CodeForbidden    = -32003
	CodeRateLimited  = -32029
	CodeInternal     = -32603
)

// TokenValidator resolves Bearer tokens (oauth usecase.Service).
type TokenValidator interface {
	ValidateAccessToken(ctx context.Context, raw, resource string) (oauthmodel.AccessToken, error)
	ResourceMetadataURL(resource string) string
}

// Resolver builds the tool principal of a token (StoreResolver).
type Resolver interface {
	Resolve(ctx context.Context, tok oauthmodel.AccessToken) (aitools.Principal, error)
}

// Registry is the AI tool registry surface (*aitools.Registry).
type Registry interface {
	Available(ctx context.Context, p aitools.Principal) ([]aitools.Tool, error)
	Call(ctx context.Context, p aitools.Principal, name string, input json.RawMessage) (aitools.Result, error)
}

// Proposer stores write-tool calls as pending actions (*aiusecase.Actions).
type Proposer interface {
	Propose(ctx context.Context, call aiusecase.ProposeCall) (aiusecase.ProposeOutcome, error)
}

// Limiter is a fixed window limiter (ratelimit.Limiter).
type Limiter interface {
	Allow(ctx context.Context, action, subject string, limit int, window time.Duration) (bool, time.Duration)
}

// Settings reads the organization request limit (*sysconfig.Service).
type Settings interface {
	MCPRequestsPerHourPerOrg(ctx context.Context) int
}

// ActivityRecorder records audit events (*activity.Recorder).
type ActivityRecorder interface {
	Record(ctx context.Context, actorID *int64, action, resource string, resourceUUID *uuid.UUID, payload map[string]any, r *http.Request)
}

// Config wires the server. Limiter, Settings and Activity may be nil
// (tests); FrontendURL is the public origin of the approval links.
type Config struct {
	Tokens      TokenValidator
	Principals  Resolver
	Tools       Registry
	Actions     Proposer
	Limiter     Limiter
	Settings    Settings
	Activity    ActivityRecorder
	FrontendURL string
	Version     string
	Log         *slog.Logger
}

// Server serves the three MCP endpoints.
type Server struct {
	cfg Config
	sdk *mcpsdk.StreamableHTTPHandler
	now func() time.Time
	// sdkLog keeps the SDK's per-request session lines out of the log.
	sdkLog *slog.Logger
}

// warnOnly passes warnings and errors of the SDK through.
type warnOnly struct{ slog.Handler }

func (h warnOnly) Enabled(ctx context.Context, l slog.Level) bool {
	return l >= slog.LevelWarn && h.Handler.Enabled(ctx, l)
}

func (h warnOnly) WithAttrs(a []slog.Attr) slog.Handler { return warnOnly{h.Handler.WithAttrs(a)} }

func (h warnOnly) WithGroup(n string) slog.Handler { return warnOnly{h.Handler.WithGroup(n)} }

// New builds the server.
func New(cfg Config) *Server {
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.Version == "" {
		cfg.Version = "1"
	}
	cfg.FrontendURL = strings.TrimRight(cfg.FrontendURL, "/")
	s := &Server{cfg: cfg, now: time.Now, sdkLog: slog.New(warnOnly{cfg.Log.Handler()})}
	s.sdk = mcpsdk.NewStreamableHTTPHandler(func(r *http.Request) *mcpsdk.Server {
		sess, ok := r.Context().Value(sessionKey{}).(*session)
		if !ok {
			return nil
		}
		return s.build(sess)
	}, &mcpsdk.StreamableHTTPOptions{
		// Every request carries its own Bearer token and gets a fresh tool
		// list; there is no server side session to resume.
		Stateless:    true,
		JSONResponse: true,
		Logger:       s.sdkLog,
		// The frontend proxies /mcp/* (only it is public), so the Host
		// header is the public one while the connection may be local;
		// Bearer auth (no cookies) makes DNS rebinding moot.
		DisableLocalhostProtection: true,
		MaxRequestBodyBytes:        maxBodyBytes,
	})
	return s
}

// SetClock replaces the clock (tests).
func (s *Server) SetClock(now func() time.Time) { s.now = now }

type sessionKey struct{}

// session is one authenticated HTTP request.
type session struct {
	resource  string
	token     oauthmodel.AccessToken
	principal aitools.Principal
	tools     []aitools.Tool
	allowed   map[string]aitools.Spec
	req       *http.Request
}

// ServeHTTP implements http.Handler for the endpoint at r.URL.Path.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	resource := r.URL.Path
	if !oauthmodel.IsResource(resource) {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	raw := bearer(r.Header.Get("Authorization"))
	if raw == "" {
		s.unauthorized(w, r, resource, "")
		return
	}
	tok, err := s.cfg.Tokens.ValidateAccessToken(ctx, raw, resource)
	switch {
	case errors.Is(err, oauthusecase.ErrUnauthorized):
		s.unauthorized(w, r, resource, "the access token is invalid, expired or not issued for this endpoint")
		return
	case errors.Is(err, oauthusecase.ErrFeatureDisabled):
		s.reject(w, r, http.StatusForbidden, CodeForbidden, "the mcp module is not enabled for this organization", 0)
		return
	case err != nil:
		s.internal(w, r, "token", err)
		return
	}
	p, err := s.cfg.Principals.Resolve(ctx, tok)
	if errors.Is(err, ErrForbidden) {
		s.reject(w, r, http.StatusForbidden, CodeForbidden, "this connection is no longer allowed to use this endpoint", 0)
		return
	}
	if err != nil {
		s.internal(w, r, "principal", err)
		return
	}
	if r.Method == http.MethodPost {
		if retry, limited := s.limited(ctx, tok); limited {
			s.reject(w, r, http.StatusTooManyRequests, CodeRateLimited, "MCP request limit exceeded; retry later", retry)
			return
		}
	}
	available, err := s.cfg.Tools.Available(ctx, p)
	if err != nil {
		s.internal(w, r, "tools", err)
		return
	}
	sess := &session{resource: resource, token: tok, principal: p, tools: available,
		allowed: make(map[string]aitools.Spec, len(available)), req: r}
	for _, t := range available {
		sess.allowed[t.Spec().Name] = t.Spec()
	}
	s.sdk.ServeHTTP(w, r.WithContext(context.WithValue(ctx, sessionKey{}, sess)))
}

// limited applies the token limit first, then the organization limit, so
// a connection over its own limit does not use up the organization's.
func (s *Server) limited(ctx context.Context, tok oauthmodel.AccessToken) (time.Duration, bool) {
	if s.cfg.Limiter == nil {
		return 0, false
	}
	if ok, retry := s.cfg.Limiter.Allow(ctx, "mcp_token", tok.Family.String(), TokenRequestsPerMinute, tokenWindow); !ok {
		return retry, true
	}
	limit := sysconfig.DefaultMCPRequestsPerHourPerOrg
	if s.cfg.Settings != nil {
		limit = s.cfg.Settings.MCPRequestsPerHourPerOrg(ctx)
	}
	if ok, retry := s.cfg.Limiter.Allow(ctx, "mcp_org", strconv.FormatInt(tok.OrganizationID, 10), limit, orgWindow); !ok {
		return retry, true
	}
	return 0, false
}

// unauthorized answers 401 with the RFC 9728 challenge (RFC 6750 §3).
func (s *Server) unauthorized(w http.ResponseWriter, r *http.Request, resource, desc string) {
	params := []string{}
	if desc != "" {
		params = append(params, `error="invalid_token"`, fmt.Sprintf("error_description=%q", desc))
	}
	params = append(params, fmt.Sprintf("resource_metadata=%q", s.cfg.Tokens.ResourceMetadataURL(resource)))
	w.Header().Set("WWW-Authenticate", "Bearer "+strings.Join(params, ", "))
	msg := "a Bearer access token is required"
	if desc != "" {
		msg = desc
	}
	s.reject(w, r, http.StatusUnauthorized, CodeUnauthorized, msg, 0)
}

func (s *Server) internal(w http.ResponseWriter, r *http.Request, step string, err error) {
	s.cfg.Log.ErrorContext(r.Context(), "mcp_request_failed", "step", step, "error", err)
	s.reject(w, r, http.StatusInternalServerError, CodeInternal, "internal error", 0)
}

// reject writes an HTTP error whose body is a JSON-RPC error response for
// the request id (null when the body is not a single request).
func (s *Server) reject(w http.ResponseWriter, r *http.Request, status, code int, msg string, retry time.Duration) {
	id := json.RawMessage("null")
	if r.Method == http.MethodPost && r.Body != nil {
		raw, _ := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var req struct {
			ID json.RawMessage `json:"id"`
		}
		if json.Unmarshal(raw, &req) == nil && len(req.ID) > 0 {
			id = req.ID
		}
	}
	if retry > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int((retry+time.Second-1)/time.Second)))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0", "id": id,
		"error": map[string]any{"code": code, "message": msg},
	})
}

func bearer(header string) string {
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}
