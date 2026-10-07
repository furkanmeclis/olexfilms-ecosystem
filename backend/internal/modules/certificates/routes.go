package certificates

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/certificates/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

func RegisterRoutes(mux *http.ServeMux, h *handler.Handler, tokens *jwt.Manager, loader middleware.IdentityLoader, q *db.Queries, checker middleware.FeatureChecker) {
	authn := middleware.Authenticate(tokens, loader)
	org := middleware.RequireOrganization(tokens, q)
	module := middleware.RequireFeature(checker, features.ModuleCertificates)
	tenant := func(fn http.HandlerFunc, perm string) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, perm))
	}
	write := func(fn http.HandlerFunc, perm string) http.Handler {
		return middleware.Chain(fn, authn, org, module, middleware.RequireScope(q, perm))
	}
	mux.Handle("GET /v1/certificates", tenant(h.List, rbac.PermCertificatesRead))
	mux.Handle("POST /v1/certificates", tenant(h.Upload, rbac.PermCertificatesWrite))
	mux.Handle("GET /v1/certificates/{uuid}/file", tenant(h.Download, rbac.PermCertificatesRead))
	mux.Handle("POST /v1/certificates/{uuid}/verify", write(h.Verify, rbac.PermCertificatesVerify))
	mux.Handle("POST /v1/certificates/{uuid}/reject", write(h.Reject, rbac.PermCertificatesVerify))
	mux.Handle("POST /v1/certificates/{uuid}/revoke", write(h.Revoke, rbac.PermCertificatesVerify))
	mux.Handle("GET /v1/certificates/coverage", tenant(h.Coverage, rbac.PermCertificatesRead))
	mux.Handle("GET /v1/platform/certificate-types", tenant(h.ListTypes, rbac.PermCertificateTypesManage))
	mux.Handle("POST /v1/platform/certificate-types", write(h.CreateType, rbac.PermCertificateTypesManage))
	mux.Handle("PUT /v1/platform/certificate-types/{uuid}", write(h.UpdateType, rbac.PermCertificateTypesManage))
	mux.Handle("DELETE /v1/platform/certificate-types/{uuid}", write(h.DeleteType, rbac.PermCertificateTypesManage))
	mux.Handle("GET /v1/certificate-warnings", tenant(h.ListWarnings, rbac.PermCertificatesRead))
	mux.Handle("POST /v1/certificate-warnings/{uuid}/approve", write(h.ApproveWarning, rbac.PermCertificatesApproveService))
	mux.Handle("POST /v1/certificate-warnings/{uuid}/reject", write(h.RejectWarning, rbac.PermCertificatesApproveService))
}
