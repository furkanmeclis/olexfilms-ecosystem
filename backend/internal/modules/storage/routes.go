package storage

import (
	"net/http"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/middleware"
	storagehandler "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/storage/handler"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/jwt"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
)

func RegisterRoutes(
	mux *http.ServeMux,
	h *storagehandler.Handler,
	tokens *jwt.Manager,
	loader middleware.IdentityLoader,
) {
	authn := middleware.Authenticate(tokens, loader)
	read := middleware.RequirePermission(rbac.PermPlatformStorageRead)
	write := middleware.RequirePermission(rbac.PermPlatformStorageWrite)

	mux.Handle("GET /v1/platform/storage/meta", middleware.Chain(
		http.HandlerFunc(h.Meta), authn, read,
	))
	mux.Handle("GET /v1/platform/storage/objects", middleware.Chain(
		http.HandlerFunc(h.List), authn, read,
	))
	mux.Handle("GET /v1/platform/storage/objects/item", middleware.Chain(
		http.HandlerFunc(h.Get), authn, read,
	))
	mux.Handle("GET /v1/platform/storage/usage", middleware.Chain(
		http.HandlerFunc(h.Usage), authn, read,
	))
	mux.Handle("GET /v1/platform/storage/objects/download", middleware.Chain(
		http.HandlerFunc(h.Download), authn, read,
	))
	mux.Handle("GET /v1/platform/storage/objects/preview", middleware.Chain(
		http.HandlerFunc(h.Preview), authn, read,
	))
	mux.Handle("GET /v1/platform/storage/versions", middleware.Chain(
		http.HandlerFunc(h.Versions), authn, read,
	))
	mux.Handle("GET /v1/platform/storage/activity", middleware.Chain(
		http.HandlerFunc(h.Activity), authn, read,
	))
	mux.Handle("GET /v1/platform/storage/shares", middleware.Chain(
		http.HandlerFunc(h.Shares), authn, read,
	))
	mux.Handle("GET /v1/platform/storage/links", middleware.Chain(
		http.HandlerFunc(h.Links), authn, read,
	))

	mux.Handle("POST /v1/platform/storage/folders", middleware.Chain(
		http.HandlerFunc(h.CreateFolder), authn, write,
	))
	mux.Handle("POST /v1/platform/storage/uploads", middleware.Chain(
		http.HandlerFunc(h.Upload), authn, write,
	))
	mux.Handle("POST /v1/platform/storage/uploads/session", middleware.Chain(
		http.HandlerFunc(h.CreateUploadSession), authn, write,
	))
	mux.Handle("POST /v1/platform/storage/uploads/complete", middleware.Chain(
		http.HandlerFunc(h.CompleteUpload), authn, write,
	))
	mux.Handle("POST /v1/platform/storage/objects/copy", middleware.Chain(
		http.HandlerFunc(h.Copy), authn, write,
	))
	mux.Handle("POST /v1/platform/storage/objects/move", middleware.Chain(
		http.HandlerFunc(h.Move), authn, write,
	))
	mux.Handle("POST /v1/platform/storage/objects/rename", middleware.Chain(
		http.HandlerFunc(h.Rename), authn, write,
	))
	mux.Handle("POST /v1/platform/storage/objects/delete", middleware.Chain(
		http.HandlerFunc(h.Delete), authn, write,
	))
	mux.Handle("POST /v1/platform/storage/objects/restore", middleware.Chain(
		http.HandlerFunc(h.Restore), authn, write,
	))
	mux.Handle("POST /v1/platform/storage/objects/purge", middleware.Chain(
		http.HandlerFunc(h.Purge), authn, write,
	))
	mux.Handle("POST /v1/platform/storage/versions/restore", middleware.Chain(
		http.HandlerFunc(h.RestoreVersion), authn, write,
	))
	mux.Handle("POST /v1/platform/storage/versions/delete", middleware.Chain(
		http.HandlerFunc(h.DeleteVersion), authn, write,
	))
	mux.Handle("POST /v1/platform/storage/star", middleware.Chain(
		http.HandlerFunc(h.Star), authn, write,
	))
	mux.Handle("DELETE /v1/platform/storage/star", middleware.Chain(
		http.HandlerFunc(h.Unstar), authn, write,
	))
	mux.Handle("POST /v1/platform/storage/shares", middleware.Chain(
		http.HandlerFunc(h.Share), authn, write,
	))
	mux.Handle("DELETE /v1/platform/storage/shares/{uuid}", middleware.Chain(
		http.HandlerFunc(h.Unshare), authn, write,
	))
	mux.Handle("POST /v1/platform/storage/links", middleware.Chain(
		http.HandlerFunc(h.CreateLink), authn, write,
	))
	mux.Handle("DELETE /v1/platform/storage/links/{uuid}", middleware.Chain(
		http.HandlerFunc(h.RevokeLink), authn, write,
	))

	mux.HandleFunc("GET /v1/public/storage/s/{token}", h.PublicSigned)
	mux.HandleFunc("GET /v1/public/storage/{slug}", h.PublicSlug)
}
