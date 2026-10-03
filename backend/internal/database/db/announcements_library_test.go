package db_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-329: database-level guards of announcements and the document library
// (migration 000086). Reuses the order fixture (center > dist > dealer,
// dealer2; rolled-back transaction, savepoint per failure).

func (f *orderFixture) announcementUser(t *testing.T) db.User {
	t.Helper()
	f.seq++
	u, err := f.q.CreateUser(f.ctx, db.CreateUserParams{
		PasswordHash: "x", Name: "Duyuru", Surname: "T329", Status: "active",
		Email: text(fmt.Sprintf("t329-%d-%d@example.test", time.Now().UnixNano(), f.seq)),
	})
	if err != nil {
		t.Fatalf("user: %v", err)
	}
	return u
}

func (f *orderFixture) announcementParams(org db.Organization) db.CreateAnnouncementParams {
	return db.CreateAnnouncementParams{
		OrganizationID: org.ID, BrandID: org.BrandID, DefaultLocale: "tr",
		Title: "Duyuru", Body: "Metin", BodyFormat: "markdown", Status: "published",
		PublishAt: ts(time.Now().Add(-time.Minute)),
	}
}

func (f *orderFixture) announcement(t *testing.T, org db.Organization) db.Announcement {
	t.Helper()
	a, err := f.q.CreateAnnouncement(f.ctx, f.announcementParams(org))
	if err != nil {
		t.Fatalf("announcement of %s: %v", org.Type, err)
	}
	return a
}

// expectNoError runs fn in a savepoint that is rolled back afterwards.
func (f *orderFixture) expectNoError(t *testing.T, name string, fn func(sp pgx.Tx) error) {
	t.Helper()
	sp, err := f.tx.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sp.Rollback(f.ctx) }()
	if err := fn(sp); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

func (f *orderFixture) audience(a db.Announcement, typ string, org int64) func(sp pgx.Tx) error {
	return func(sp pgx.Tx) error {
		arg := db.AddAnnouncementAudienceParams{AnnouncementID: a.ID, TargetType: typ}
		if org != 0 {
			arg.TargetOrganizationID = pgtype.Int8{Int64: org, Valid: true}
		}
		_, err := db.New(sp).AddAnnouncementAudience(f.ctx, arg)
		return err
	}
}

func TestAnnouncementSchemaConstraints(t *testing.T) {
	f := newOrderFixture(t)
	ctx := f.ctx
	center, err := f.q.GetOrganizationByID(ctx, f.centerID)
	if err != nil {
		t.Fatalf("center: %v", err)
	}

	t.Run("only a center or a distributor writes", func(t *testing.T) {
		f.expectCode(t, "dealer author", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateAnnouncement(ctx, f.announcementParams(f.dealer))
			return err
		}, "23514")
		f.announcement(t, center)
		f.announcement(t, f.dist)
	})

	t.Run("published needs publish_at", func(t *testing.T) {
		arg := f.announcementParams(center)
		arg.PublishAt = pgtype.Timestamptz{}
		f.expectConstraint(t, "published without publish_at", "23514", "chk_announcements_published", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateAnnouncement(ctx, arg)
			return err
		})
	})

	t.Run("distributor targets only its subtree", func(t *testing.T) {
		a := f.announcement(t, f.dist)
		for _, typ := range []string{"all_network", "distributors", "dealers"} {
			f.expectCode(t, "distributor "+typ, f.audience(a, typ, 0), "23514")
		}
		f.expectCode(t, "distributor role", func(sp pgx.Tx) error {
			_, err := db.New(sp).AddAnnouncementAudience(ctx, db.AddAnnouncementAudienceParams{
				AnnouncementID: a.ID, TargetType: "role", RoleSlug: text("dealer_staff"),
			})
			return err
		}, "23514")
		// The center is above the distributor, so it is outside its subtree.
		f.expectCode(t, "distributor subtree of the center", f.audience(a, "subtree", f.centerID), "23514")
		otherDist := f.org(t, "dist2", "distributor", f.centerID)
		otherDealer := f.org(t, "dealer3", "dealer", otherDist.ID)
		f.expectCode(t, "distributor subtree of another distributor", f.audience(a, "subtree", otherDealer.ID), "23514")
		f.expectNoError(t, "own subtree", f.audience(a, "subtree", f.dist.ID))
		f.expectNoError(t, "dealer below", f.audience(a, "subtree", f.dealer.ID))

		// A subtree target needs an organization, other targets must not
		// carry one.
		f.expectConstraint(t, "subtree without org", "23514", "chk_announcement_audiences_org", f.audience(a, "subtree", 0))
	})

	t.Run("center targets the whole network", func(t *testing.T) {
		a := f.announcement(t, center)
		for _, typ := range []string{"all_network", "distributors", "dealers"} {
			f.expectNoError(t, "center "+typ, f.audience(a, typ, 0))
		}
		f.expectConstraint(t, "all_network with org", "23514", "chk_announcement_audiences_org", f.audience(a, "all_network", f.dist.ID))
		f.expectConstraint(t, "role without slug", "23514", "chk_announcement_audiences_role", f.audience(a, "role", 0))
	})

	t.Run("author organization is immutable", func(t *testing.T) {
		a := f.announcement(t, f.dist)
		f.expectCode(t, "move author", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `UPDATE announcements SET organization_id = $2 WHERE id = $1`, a.ID, f.centerID)
			return err
		}, "23514")
	})

	t.Run("second read of the same user", func(t *testing.T) {
		a := f.announcement(t, center)
		u := f.announcementUser(t)
		first, err := f.q.MarkAnnouncementRead(ctx, db.MarkAnnouncementReadParams{AnnouncementID: a.ID, UserID: u.ID})
		if err != nil {
			t.Fatalf("first read: %v", err)
		}
		// The application upserts: the second read keeps the first read_at.
		again, err := f.q.MarkAnnouncementRead(ctx, db.MarkAnnouncementReadParams{AnnouncementID: a.ID, UserID: u.ID})
		if err != nil || !again.ReadAt.Time.Equal(first.ReadAt.Time) {
			t.Fatalf("upsert read = %+v, %v (first %+v)", again, err, first)
		}
		if n, err := f.q.CountAnnouncementReads(ctx, a.ID); err != nil || n != 1 {
			t.Fatalf("reads = %d, %v", n, err)
		}
		// A plain second insert hits the unique key.
		f.expectConstraint(t, "second read", "23505", "uq_announcement_reads_user", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `INSERT INTO announcement_reads (announcement_id, user_id) VALUES ($1, $2)`, a.ID, u.ID)
			return err
		})
	})

	t.Run("feed follows the audience", func(t *testing.T) {
		u := f.announcementUser(t)
		a := f.announcement(t, f.dist)
		otherDist := f.org(t, "dist4", "distributor", f.centerID)
		otherDealer := f.org(t, "dealer4", "dealer", otherDist.ID)
		f.expectNoError(t, "subtree audience", func(sp pgx.Tx) error {
			if err := f.audience(a, "subtree", f.dist.ID)(sp); err != nil {
				return err
			}
			q := db.New(sp)
			feed := func(org db.Organization, lineage ...int64) ([]db.ListVisibleAnnouncementsRow, error) {
				return q.ListVisibleAnnouncements(ctx, db.ListVisibleAnnouncementsParams{
					Locale: "en", UserID: u.ID, BrandID: f.brandID, Now: ts(time.Now()),
					ViewerRoleSlugs: []string{"dealer_staff"}, ViewerOrgType: org.Type,
					ViewerOrgLineage: append([]int64{org.ID}, lineage...), PageLimit: 100,
				})
			}
			has := func(rows []db.ListVisibleAnnouncementsRow) bool {
				for _, r := range rows {
					if r.ID == a.ID {
						return true
					}
				}
				return false
			}
			rows, err := feed(f.dealer, f.dist.ID, f.centerID)
			if err != nil {
				return err
			}
			if !has(rows) {
				return errors.New("dealer below the distributor does not see the announcement")
			}
			rows, err = feed(otherDealer, otherDist.ID, f.centerID)
			if err != nil {
				return err
			}
			if has(rows) {
				return errors.New("dealer of another distributor sees the announcement")
			}
			return nil
		})
	})
}

func TestLibrarySchemaConstraints(t *testing.T) {
	f := newOrderFixture(t)
	ctx := f.ctx

	item, err := f.q.CreateLibraryItem(ctx, db.CreateLibraryItemParams{
		OrganizationID: f.centerID, BrandID: f.brandID, Name: "Montaj kılavuzu",
		Tags: []string{"kilavuz"}, AccessLevel: "all_network",
	})
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	sha := strings.Repeat("ab", 32)
	v, err := f.q.CreateLibraryItemVersion(ctx, db.CreateLibraryItemVersionParams{
		ItemID: item.ID, Locale: "tr", VersionNo: 1, StorageKey: "library/t329/v1.pdf",
		Mime: "application/pdf", SizeBytes: 1024, Sha256: sha,
	})
	if err != nil {
		t.Fatalf("version: %v", err)
	}

	t.Run("versions are append-only", func(t *testing.T) {
		f.expectCode(t, "update version", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `UPDATE library_item_versions SET mime = 'text/plain' WHERE id = $1`, v.ID)
			return err
		}, "23001")
		f.expectCode(t, "delete version", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `DELETE FROM library_item_versions WHERE id = $1`, v.ID)
			return err
		}, "23001")
	})

	t.Run("version number per item and locale", func(t *testing.T) {
		next, err := f.q.NextLibraryItemVersionNo(ctx, db.NextLibraryItemVersionNoParams{ItemID: item.ID, Locale: "tr"})
		if err != nil || next != 2 {
			t.Fatalf("next version = %d, %v", next, err)
		}
		f.expectConstraint(t, "duplicate version", "23505", "uq_library_item_versions_no", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateLibraryItemVersion(ctx, db.CreateLibraryItemVersionParams{
				ItemID: item.ID, Locale: "tr", VersionNo: 1, StorageKey: "library/t329/dup.pdf",
				Mime: "application/pdf", SizeBytes: 1, Sha256: sha,
			})
			return err
		})
		f.expectConstraint(t, "bad sha256", "23514", "chk_library_item_versions_sha256", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateLibraryItemVersion(ctx, db.CreateLibraryItemVersionParams{
				ItemID: item.ID, Locale: "en", VersionNo: 1, StorageKey: "library/t329/en.pdf",
				Mime: "application/pdf", SizeBytes: 1, Sha256: strings.Repeat("AB", 32),
			})
			return err
		})
	})

	t.Run("access level", func(t *testing.T) {
		f.expectConstraint(t, "unknown access level", "23514", "chk_library_items_access_level", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateLibraryItem(ctx, db.CreateLibraryItemParams{
				OrganizationID: f.centerID, BrandID: f.brandID, Name: "x", Tags: []string{}, AccessLevel: "public",
			})
			return err
		})
	})

	t.Run("folder tree has no cycle", func(t *testing.T) {
		folder := func(name string, parent int64) db.LibraryFolder {
			arg := db.CreateLibraryFolderParams{OrganizationID: f.centerID, BrandID: f.brandID, Name: name}
			if parent != 0 {
				arg.ParentID = pgtype.Int8{Int64: parent, Valid: true}
			}
			fo, err := f.q.CreateLibraryFolder(ctx, arg)
			if err != nil {
				t.Fatalf("folder %s: %v", name, err)
			}
			return fo
		}
		root := folder("t329-root", 0)
		child := folder("t329-child", root.ID)
		f.expectCode(t, "root under its child", func(sp pgx.Tx) error {
			_, err := sp.Exec(ctx, `UPDATE library_folders SET parent_id = $2 WHERE id = $1`, root.ID, child.ID)
			return err
		}, "23514")
		// A parent in another organization fails the composite key.
		f.expectConstraint(t, "parent of another organization", "23503", "fk_library_folders_parent", func(sp pgx.Tx) error {
			_, err := db.New(sp).CreateLibraryFolder(ctx, db.CreateLibraryFolderParams{
				OrganizationID: f.dist.ID, BrandID: f.brandID, Name: "t329-foreign",
				ParentID: pgtype.Int8{Int64: root.ID, Valid: true},
			})
			return err
		})
	})
}
