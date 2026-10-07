package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	aimodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/model"
	aitools "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/tools"
	aiusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/ai/usecase"
	oauthmodel "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/oauth/model"
	"github.com/google/uuid"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Outcomes of a tools/call (activity payload "result").
const (
	OutcomeOK              = "ok"
	OutcomeError           = "error"
	OutcomeNotAllowed      = "not_allowed"
	OutcomePendingApproval = "pending_approval"
)

// CodePendingApproval is the structured status of a write-tool result.
const CodePendingApproval = "PENDING_APPROVAL"

// ApprovalsPath is the panel screen of the pending AI actions (F4-03d)
// under the tenant prefix; ?action=<uuid> opens one card.
const ApprovalsPath = "/ai/approvals"

// ApprovalURL is the panel link of one pending action.
func ApprovalURL(frontendURL, orgSlug string, action uuid.UUID) string {
	return frontendURL + "/t/" + url.PathEscape(orgSlug) + ApprovalsPath + "?action=" + action.String()
}

var instructions = map[string]string{
	oauthmodel.RealmDealer: "Olexfilms dealer / distributor tools. Data is limited to the organization this connection " +
		"was approved for and the user's permissions there. Tools that change data do not run directly: they create " +
		"an approval request that the user confirms in the Olexfilms panel.",
	oauthmodel.RealmUser: "Olexfilms panel tools. Data is limited to the organization this connection was approved " +
		"for and the user's permissions there. Tools that change data do not run directly: they create an approval " +
		"request that the user confirms in the Olexfilms panel.",
	oauthmodel.RealmCustomer: "Olexfilms customer tools: the signed-in customer's own vehicles, services, warranties " +
		"and appointments only.",
}

// build is the SDK server of one request: exactly the tools the principal
// may use now.
func (s *Server) build(sess *session) *mcpsdk.Server {
	realm := oauthmodel.RealmOf(sess.resource)
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "olexfilms-" + realm, Title: "Olexfilms", Version: s.cfg.Version},
		&mcpsdk.ServerOptions{
			Instructions: instructions[realm],
			// The list depends on the user's permissions and modules; it is
			// read again at the start of every client session.
			Capabilities: &mcpsdk.ServerCapabilities{Tools: &mcpsdk.ToolCapabilities{ListChanged: false}},
			Logger:       s.sdkLog,
		})
	for _, t := range sess.tools {
		spec := t.Spec()
		srv.AddTool(&mcpsdk.Tool{
			Name: spec.Name, Description: description(spec), InputSchema: spec.InputSchema,
			Annotations: annotations(spec),
		}, s.handler(sess, spec))
	}
	srv.AddReceivingMiddleware(s.gate(sess))
	return srv
}

func description(spec aitools.Spec) string {
	if spec.Kind == aitools.KindWrite {
		return spec.Description + " This tool does not change anything directly: it creates an approval request " +
			"that the user must confirm in the Olexfilms panel."
	}
	return spec.Description
}

func annotations(spec aitools.Spec) *mcpsdk.ToolAnnotations {
	closed, destructive := false, false
	return &mcpsdk.ToolAnnotations{
		ReadOnlyHint: spec.Kind == aitools.KindRead, DestructiveHint: &destructive, OpenWorldHint: &closed,
	}
}

// gate answers a tools/call of a tool the principal may not use (the
// client's list is old, or it invents a name) like the registry does, and
// records it.
func (s *Server) gate(sess *session) mcpsdk.Middleware {
	return func(next mcpsdk.MethodHandler) mcpsdk.MethodHandler {
		return func(ctx context.Context, method string, req mcpsdk.Request) (mcpsdk.Result, error) {
			call, ok := req.(*mcpsdk.CallToolRequest)
			if method != "tools/call" || !ok || call.Params == nil {
				return next(ctx, method, req)
			}
			if _, ok := sess.allowed[call.Params.Name]; ok {
				return next(ctx, method, req)
			}
			start := s.now()
			res := aitools.ErrorResult(aitools.CodeToolNotAllowed, "TOOL_NOT_ALLOWED: the tool "+call.Params.Name+
				" is not available to this user. Do not call it again; answer with the tools you have.")
			s.record(ctx, sess, call.Params.Name, call.Params.Arguments, start, OutcomeNotAllowed, res.Code, nil)
			return toResult(call.Params.Name, res), nil
		}
	}
}

// handler runs one tool: read and self tools through Registry.Call (which
// checks the principal again), write tools through the pending action flow.
func (s *Server) handler(sess *session, spec aitools.Spec) mcpsdk.ToolHandler {
	return func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		start := s.now()
		args := json.RawMessage(req.Params.Arguments)
		if spec.Kind == aitools.KindWrite {
			return s.propose(ctx, sess, spec.Name, args, start), nil
		}
		res, err := s.cfg.Tools.Call(ctx, sess.principal, spec.Name, args)
		outcome := OutcomeOK
		switch {
		case errors.Is(err, aitools.ErrToolNotAllowed):
			outcome = OutcomeNotAllowed
		case err != nil:
			s.cfg.Log.ErrorContext(ctx, "mcp_tool_failed", "tool", spec.Name, "error", err)
			outcome = OutcomeError
		case res.IsError:
			outcome = OutcomeError
		}
		s.record(ctx, sess, spec.Name, args, start, outcome, res.Code, nil)
		return toResult(spec.Name, res), nil
	}
}

// pendingApproval is the structured result of a write-tool call.
type pendingApproval struct {
	Status      string          `json:"status"`
	ActionUUID  uuid.UUID       `json:"action_uuid"`
	Tool        string          `json:"tool"`
	Summary     string          `json:"summary"`
	Preview     aitools.Preview `json:"preview"`
	ExpiresAt   time.Time       `json:"expires_at"`
	ApprovalURL string          `json:"approval_url"`
}

// propose stores the call as a pending action; nothing is written to the
// target module. The panel approval (F4-03d) runs it.
func (s *Server) propose(ctx context.Context, sess *session, name string, args json.RawMessage, start time.Time) *mcpsdk.CallToolResult {
	if s.cfg.Actions == nil {
		res := aitools.ErrorResult(aitools.CodeToolFailed, "Changing data is not available on this server.")
		s.record(ctx, sess, name, args, start, OutcomeError, res.Code, nil)
		return toResult(name, res)
	}
	out, err := s.cfg.Actions.Propose(ctx, aiusecase.ProposeCall{
		Principal: sess.principal, Source: aimodel.SourceMCP, SourceRef: sess.token.Family.String(),
		ToolUseID: "mcp_" + uuid.NewString(), ToolName: name, Input: args,
	})
	if err != nil {
		s.cfg.Log.ErrorContext(ctx, "mcp_propose_failed", "tool", name, "error", err)
		res := aitools.ErrorResult(aitools.CodeToolFailed, "The tool failed because of an internal error. Tell the user to try again later.")
		s.record(ctx, sess, name, args, start, OutcomeError, res.Code, nil)
		return toResult(name, res)
	}
	if out.Card == nil {
		res := aitools.ErrorResult(aitools.CodeToolFailed, "The tool failed because of an internal error.")
		if out.Result != nil {
			res = *out.Result
		}
		outcome := OutcomeError
		if res.Code == aitools.CodeToolNotAllowed {
			outcome = OutcomeNotAllowed
		}
		s.record(ctx, sess, name, args, start, outcome, res.Code, nil)
		return toResult(name, res)
	}
	card := out.Card
	slug := ""
	if sess.principal.Org != nil {
		slug = sess.principal.Org.Slug
	}
	pa := pendingApproval{
		Status: CodePendingApproval, ActionUUID: card.ActionUUID, Tool: name, Summary: card.Preview.Summary,
		Preview: card.Preview, ExpiresAt: card.ExpiresAt, ApprovalURL: ApprovalURL(s.cfg.FrontendURL, slug, card.ActionUUID),
	}
	text := fmt.Sprintf("Waiting for approval in the panel: %s Nothing has been changed yet. The user must approve "+
		"this action in the Olexfilms panel before %s: %s . Do not call the tool again for the same request.",
		card.Preview.Summary, card.ExpiresAt.UTC().Format(time.RFC3339), pa.ApprovalURL)
	raw, _ := json.Marshal(pa)
	s.record(ctx, sess, name, args, start, OutcomePendingApproval, CodePendingApproval, &card.ActionUUID)
	return &mcpsdk.CallToolResult{
		Content:           []mcpsdk.Content{&mcpsdk.TextContent{Text: text}, &mcpsdk.TextContent{Text: string(raw)}},
		StructuredContent: pa,
	}
}

// toResult converts a registry result: a short text plus the JSON, and the
// JSON object as structured content; errors carry {"error": {code,
// message}}.
func toResult(name string, r aitools.Result) *mcpsdk.CallToolResult {
	if r.IsError {
		code := r.Code
		if code == "" {
			code = aitools.CodeToolFailed
		}
		return &mcpsdk.CallToolResult{
			IsError:           true,
			Content:           []mcpsdk.Content{&mcpsdk.TextContent{Text: r.Content}},
			StructuredContent: map[string]any{"error": map[string]string{"code": code, "message": r.Content}},
		}
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(r.Content), &obj); err != nil {
		return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: r.Content}}}
	}
	return &mcpsdk.CallToolResult{
		Content:           []mcpsdk.Content{&mcpsdk.TextContent{Text: summary(name, obj)}, &mcpsdk.TextContent{Text: r.Content}},
		StructuredContent: obj,
	}
}

// summary is the short text line of a JSON result.
func summary(name string, obj map[string]any) string {
	items, isList := obj["items"].([]any)
	if total, ok := obj["total"].(float64); ok && isList {
		return fmt.Sprintf("%s: %d of %d items.", name, len(items), int64(total))
	}
	return name + ": done."
}
