package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
)

// TEC-333: a non-center organization sees a library folder only if it (or a
// descendant folder) holds an item it may see, plus the ancestors needed to
// reach it. The center sees the whole brand tree.

type libraryFixture struct {
	it     *itest
	center db.Organization
	tag    string
	track  func(id int64)
}

func (it *itest) libraryFixture() *libraryFixture {
	it.t.Helper()
	lf := &libraryFixture{it: it, center: it.brandCenter("olex"), tag: "t333-" + it.suffix}
	var folders []int64
	it.t.Cleanup(func() {
		ctx := context.Background()
		_, _ = it.pool.Exec(ctx, "DELETE FROM library_items WHERE name LIKE $1", lf.tag+"%")
		for i := len(folders) - 1; i >= 0; i-- {
			_, _ = it.pool.Exec(ctx, "DELETE FROM library_folders WHERE id = $1", folders[i])
		}
	})
	lf.track = func(id int64) { folders = append(folders, id) }
	return lf
}

// folder creates a center folder (parent 0 = root) and returns its id and uuid.
func (lf *libraryFixture) folder(name string, parent int64) (int64, string) {
	lf.it.t.Helper()
	var id int64
	var uid string
	var parentArg any
	if parent != 0 {
		parentArg = parent
	}
	if err := lf.it.pool.QueryRow(context.Background(),
		`INSERT INTO library_folders (organization_id, brand_id, parent_id, name)
		 VALUES ($1, $2, $3, $4) RETURNING id, uuid::text`,
		lf.center.ID, lf.center.BrandID, parentArg, lf.tag+" "+name).Scan(&id, &uid); err != nil {
		lf.it.t.Fatalf("folder %s: %v", name, err)
	}
	lf.track(id)
	return id, uid
}

func (lf *libraryFixture) item(name string, folder int64, access string) {
	lf.it.t.Helper()
	lf.it.exec(`INSERT INTO library_items (organization_id, brand_id, folder_id, name, access_level)
		VALUES ($1, $2, $3, $4, $5)`, lf.center.ID, lf.center.BrandID, folder, lf.tag+" "+name, access)
}

func (lf *libraryFixture) folderUUIDs(token string) []string {
	lf.it.t.Helper()
	code, env := lf.it.do("GET", "/v1/library/folders", hostOlex, token, nil)
	if code != http.StatusOK {
		lf.it.t.Fatalf("GET /v1/library/folders = %d %s", code, errCode(env))
	}
	var body struct {
		Folders []struct {
			UUID string `json:"uuid"`
		} `json:"folders"`
	}
	if err := json.Unmarshal(env.Data, &body); err != nil {
		lf.it.t.Fatalf("folders body: %s", env.Data)
	}
	out := make([]string, 0, len(body.Folders))
	for _, f := range body.Folders {
		out = append(out, f.UUID)
	}
	return out
}

func (it *itest) libraryCenterToken() string {
	it.t.Helper()
	u, pw := it.user("t333-center")
	center := it.brandCenter("olex")
	it.member(center, u, "owner")
	return it.loginOrg(u, pw, center)
}

func TestIntegrationLibraryFoldersDealerHidesCenterOnly(t *testing.T) {
	it := newIntegration(t)
	net := it.featureNet(1)
	lf := it.libraryFixture()
	hiddenID, hidden := lf.folder("center only", 0)
	lf.item("secret", hiddenID, "center_only")
	lf.item("distributor note", hiddenID, "distributors")

	if got := lf.folderUUIDs(net.dealerTok); slices.Contains(got, hidden) {
		t.Fatalf("dealer folders %v must not contain the center-only folder %s", got, hidden)
	}
	if code, env := it.do("GET", "/v1/library?folder="+hidden, hostOlex, net.dealerTok, nil); code != http.StatusNotFound {
		t.Fatalf("dealer folder filter on hidden folder = %d %s, want 404", code, errCode(env))
	}
	// The center still sees the whole brand tree.
	if got := lf.folderUUIDs(it.libraryCenterToken()); !slices.Contains(got, hidden) {
		t.Fatalf("center folders %v must contain %s", got, hidden)
	}
}

func TestIntegrationLibraryFoldersDealerSeesVisibleItemAndAncestors(t *testing.T) {
	it := newIntegration(t)
	net := it.featureNet(1)
	lf := it.libraryFixture()
	rootID, root := lf.folder("root", 0)
	midID, mid := lf.folder("mid", rootID)
	leafID, leaf := lf.folder("leaf", midID)
	siblingID, sibling := lf.folder("sibling", rootID)
	lf.item("dealer guide", leafID, "dealers")
	lf.item("center memo", siblingID, "center_only")

	got := lf.folderUUIDs(net.dealerTok)
	for name, id := range map[string]string{"root": root, "mid": mid, "leaf": leaf} {
		if !slices.Contains(got, id) {
			t.Fatalf("dealer folders %v must contain %s (%s)", got, name, id)
		}
	}
	if slices.Contains(got, sibling) {
		t.Fatalf("dealer folders %v must not contain the center-only sibling %s", got, sibling)
	}
	code, env := it.do("GET", "/v1/library?folder="+leaf, hostOlex, net.dealerTok, nil)
	if code != http.StatusOK {
		t.Fatalf("dealer folder filter on leaf = %d %s", code, errCode(env))
	}
	var page struct {
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(env.Data, &page); err != nil || page.Total != 1 {
		t.Fatalf("leaf items: %s", env.Data)
	}
	if code, _ := it.do("GET", "/v1/library?folder="+root, hostOlex, net.dealerTok, nil); code != http.StatusOK {
		t.Fatalf("dealer folder filter on ancestor = %d, want 200", code)
	}
}
