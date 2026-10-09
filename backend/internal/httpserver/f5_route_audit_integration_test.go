package httpserver

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/response"
	"github.com/google/uuid"
)

// TEC-508 (F5-10a): RequireFeature audit of the F5 add-on route groups.
//
// The route table is the routes.go of each F5 module: every mux.Handle /
// mux.HandleFunc pattern is read from the source (a pattern that is not a
// string literal cannot be audited and fails). Each route is then called
// through the real server by the owner of an organization that has the
// module off; it must answer 403 FEATURE_DISABLED, whether the gate is the
// middleware or the use case. The exceptions below are deliberate and
// documented.

// f5RouteGroups maps an F5 module directory to its module key.
var f5RouteGroups = []struct{ dir, key string }{
	{"dealershowcase", features.ModuleDealerShowcase},
	{"fleet", features.ModuleFleet},
	{"certificates", features.ModuleCertificates},
	{"stockforecast", features.ModuleStockForecast},
	{"performance", features.ModulePerformance},
	{"efficiency", features.ModuleEfficiency},
	{"photostandard", features.ModulePhotoStandard},
	{"einvoice", features.ModuleEInvoice},
}

// f5RouteExceptions are the F5 routes without a module gate on the caller's
// active organization, with the reason.
var f5RouteExceptions = map[string]string{
	// Public showcase: no session; the use case serves a photo only while
	// the dealer's showcase is published and its module is on (else 404).
	"GET /v1/public/dealers/{code}/photos/{uuid}": "public showcase, module checked in the use case",
	// Center review queue of dealer showcases: the module belongs to the
	// reviewed dealer, not to the reviewing center.
	"GET /v1/platform/showcases":                    "center review queue, module of the dealer",
	"GET /v1/platform/showcases/{org_uuid}":         "center review queue, module of the dealer",
	"POST /v1/platform/showcases/{org_uuid}/review": "center review queue, module of the dealer",
	// Fleet portal: a fleet session has no tree organization; the use case
	// opens it while an actively linked dealer has the fleet module (else
	// 403 FEATURE_DISABLED) and hides dealers that switched it off.
	"GET /v1/portal/fleet/links":                "fleet portal, linked dealers' module checked in the use case",
	"POST /v1/portal/fleet/links/{uuid}/accept": "fleet portal, linked dealers' module checked in the use case",
	"POST /v1/portal/fleet/links/{uuid}/reject": "fleet portal, linked dealers' module checked in the use case",
	"GET /v1/portal/fleet/overview":             "fleet portal, linked dealers' module checked in the use case",
	"GET /v1/portal/fleet/vehicles":             "fleet portal, linked dealers' module checked in the use case",
	"GET /v1/portal/fleet/vehicles/{uuid}":      "fleet portal, linked dealers' module checked in the use case",
	"GET /v1/portal/fleet/services":             "fleet portal, linked dealers' module checked in the use case",
	"GET /v1/portal/fleet/warranties":           "fleet portal, linked dealers' module checked in the use case",
	"GET /v1/portal/fleet/accounting":           "fleet portal, linked dealers' module checked in the use case",
	"GET /v1/portal/fleet/reports":              "fleet portal, linked dealers' module checked in the use case",
	"GET /v1/portal/fleet/reports/{uuid}/file":  "fleet portal, linked dealers' module checked in the use case",
}

type auditRoute struct {
	Key     string
	Pattern string
}

// routePatterns reads the route patterns registered in a routes.go source.
// Unauditable registrations (pattern not a string literal) are returned
// separately.
func routePatterns(filename string, src any) (patterns, unauditable []string, err error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return nil, nil, err
	}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (sel.Sel.Name != "Handle" && sel.Sel.Name != "HandleFunc") || len(call.Args) != 2 {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); !ok || id.Name != "mux" {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			unauditable = append(unauditable, fset.Position(call.Pos()).String())
			return true
		}
		p, err := strconv.Unquote(lit.Value)
		if err != nil {
			unauditable = append(unauditable, fset.Position(call.Pos()).String())
			return true
		}
		patterns = append(patterns, p)
		return true
	})
	return patterns, unauditable, nil
}

// f5RouteTable reads the route table of every F5 module.
func f5RouteTable(t *testing.T) []auditRoute {
	t.Helper()
	var out []auditRoute
	for _, g := range f5RouteGroups {
		file := filepath.Join("..", "modules", g.dir, "routes.go")
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		patterns, unauditable, err := routePatterns(file, src)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		if len(unauditable) > 0 {
			t.Errorf("%s: route patterns must be string literals to be audited: %v", g.dir, unauditable)
		}
		if len(patterns) == 0 {
			t.Errorf("%s: no routes found", g.dir)
		}
		for _, p := range patterns {
			out = append(out, auditRoute{Key: g.key, Pattern: p})
		}
	}
	return out
}

var pathParam = regexp.MustCompile(`\{([a-z_]+)\}`)

// probePath fills the path parameters of a pattern with harmless values.
func probePath(pattern string) (string, string) {
	method, path, _ := strings.Cut(pattern, " ")
	path = pathParam.ReplaceAllStringFunc(path, func(m string) string {
		name := strings.Trim(m, "{}")
		if strings.Contains(name, "key") || name == "code" {
			return "audit"
		}
		return uuid.NewString()
	})
	return method, path
}

// auditFeatureGates calls every route as tok (an organization with all the
// audited modules off) and returns the routes that do not answer 403
// FEATURE_DISABLED and are not documented exceptions.
func (it *itest) auditFeatureGates(routes []auditRoute, tok string, exceptions map[string]string) []string {
	it.t.Helper()
	var bad []string
	for _, r := range routes {
		if _, ok := exceptions[r.Pattern]; ok {
			continue
		}
		method, path := probePath(r.Pattern)
		var body any
		if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
			body = map[string]any{}
		}
		code, env := it.do(method, path, hostOlex, tok, body)
		if code != http.StatusForbidden || errCode(env) != response.CodeFeatureDisabled {
			bad = append(bad, fmt.Sprintf("%s (%s): %d %s", r.Pattern, r.Key, code, errCode(env)))
		}
	}
	sort.Strings(bad)
	return bad
}

// auditOrg returns the owner token of a fresh distributor with every
// audited module switched off by the admin.
func (it *itest) auditOrg() string {
	it.t.Helper()
	center := it.brandCenter("olex")
	dist := it.org("audit-dist", "distributor", center)
	owner, pw := it.user("audit-owner")
	it.member(dist, owner, "owner")
	for _, g := range f5RouteGroups {
		if _, err := it.srv.features.SetByAdmin(context.Background(), 0, dist.ID, g.key, false); err != nil {
			it.t.Fatalf("close %s: %v", g.key, err)
		}
	}
	return it.loginOrg(owner, pw, dist)
}

// Acceptance: every F5 add-on route applies the module check (middleware
// or a documented use case exception).
func TestIntegrationF5RouteAuditRequireFeature(t *testing.T) {
	it := newIntegration(t)
	routes := f5RouteTable(t)
	seen := map[string]bool{}
	for _, r := range routes {
		seen[r.Pattern] = true
	}
	for p := range f5RouteExceptions {
		if !seen[p] {
			t.Errorf("stale exception %q: no such F5 route", p)
		}
	}
	if bad := it.auditFeatureGates(routes, it.auditOrg(), f5RouteExceptions); len(bad) > 0 {
		t.Fatalf("F5 routes without a module check:\n  %s", strings.Join(bad, "\n  "))
	}
	t.Logf("audited %d F5 routes (%d documented exceptions)", len(routes), len(f5RouteExceptions))
}

// Acceptance (negative): the audit catches a route left unprotected on
// purpose, and a registration it cannot read.
func TestIntegrationF5RouteAuditCatchesUnprotectedRoute(t *testing.T) {
	it := newIntegration(t)
	authn := middleware.Authenticate(it.srv.tokens, it.srv.loader)
	org := middleware.RequireOrganization(it.srv.tokens, it.q)
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response.JSON(w, r, http.StatusOK, map[string]string{"status": "ok"})
	})
	// Same chain as the fleet routes, minus RequireFeature.
	it.srv.mux.Handle("GET /v1/fleets/_audit/unprotected", middleware.Chain(ok, authn, org))
	it.srv.mux.Handle("GET /v1/fleets/_audit/protected", middleware.Chain(ok, authn, org,
		middleware.RequireFeature(it.srv.features, features.ModuleFleet)))

	src := `package fleet
func RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /v1/fleets/_audit/protected", protected(h.Get))
	mux.Handle("GET /v1/fleets/_audit/unprotected", unprotected(h.Get))
	mux.Handle(pattern, unprotected(h.Get))
}`
	patterns, unauditable, err := routePatterns("fixture.go", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(patterns) != 2 || len(unauditable) != 1 {
		t.Fatalf("parsed patterns=%v unauditable=%v", patterns, unauditable)
	}
	routes := []auditRoute{}
	for _, p := range patterns {
		routes = append(routes, auditRoute{Key: features.ModuleFleet, Pattern: p})
	}
	tok := it.auditOrg()
	bad := it.auditFeatureGates(routes, tok, nil)
	if len(bad) != 1 || !strings.HasPrefix(bad[0], "GET /v1/fleets/_audit/unprotected") {
		t.Fatalf("audit findings = %v, want only the unprotected route", bad)
	}
	// A documented exception is skipped.
	if bad := it.auditFeatureGates(routes, tok, map[string]string{
		"GET /v1/fleets/_audit/unprotected": "test",
	}); len(bad) != 0 {
		t.Fatalf("exception not honored: %v", bad)
	}
}
