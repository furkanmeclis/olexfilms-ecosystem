package usecase

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	customeruc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/customers/usecase"
	orguc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	serviceuc "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/services/usecase"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/authctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/brandctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/scopefilter"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Two concurrent converts on committed data: exactly one wins, the other gets
// ErrAlreadyConverted, and only one owner user / organization is created.
func TestConvertConcurrentSecondConflictsWithoutOrphans(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	var brandID int64
	var brandUUID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id, uuid FROM brands WHERE slug = 'olex'`).Scan(&brandID, &brandUUID); err != nil {
		t.Fatalf("brand: %v", err)
	}
	ctx = brandctx.WithBrand(ctx, brandctx.Brand{ID: brandID, UUID: brandUUID, Slug: "olex", Name: "Olex", Status: "active"})
	center, err := q.GetBrandCenter(ctx, brandID)
	if err != nil {
		t.Fatalf("center: %v", err)
	}
	stamp := time.Now().UnixNano()
	dist, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{
		Slug: fmt.Sprintf("tec316-race-dist-%d", stamp), Name: "TEC316 race dist", Status: "active", Type: "distributor",
		ParentID: pgtype.Int8{Int64: center.ID, Valid: true}, BrandID: brandID,
		Currency: "TRY", Locale: "tr", Timezone: "UTC", Settings: []byte(`{}`),
		AccessStartsAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	if err != nil {
		t.Fatalf("distributor: %v", err)
	}
	actor, err := q.CreateUser(ctx, db.CreateUserParams{
		Email:        pgtype.Text{String: fmt.Sprintf("tec316-race-%d@example.test", stamp), Valid: true},
		PasswordHash: "x", Name: "Race", Surname: "Actor", Status: "active",
	})
	if err != nil {
		t.Fatalf("actor: %v", err)
	}
	svc := New(pool, q, nil)
	svc.SetConverters(customeruc.New(pool, q, nil, nil), serviceuc.New(pool, q, outbox.NewMemory()), orguc.New(pool, q))
	scope := rbac.ScopeSubtree
	c := Caller{
		Principal: authctx.Principal{UserInternal: actor.ID, PermissionScopes: map[string]rbac.Scope{
			rbac.PermLeadsRead: scope, rbac.PermLeadsWrite: scope, rbac.PermLeadsConvertOrg: scope,
		}},
		Org:    orgctx.Scope{InternalID: dist.ID, UUID: dist.Uuid, OrgType: dist.Type, BrandID: brandID},
		Filter: scopefilter.Filter{Permission: rbac.PermLeadsRead, Scope: scope, UserID: actor.ID, OrgID: dist.ID, OrgIDs: []int64{dist.ID}, BrandID: brandID},
	}
	ph := fmt.Sprintf("+90554%07d", stamp%10000000)
	company := fmt.Sprintf("TEC316 race dealer %d", stamp)
	lead, err := svc.Create(ctx, c, CreateInput{
		TargetType: "dealer_candidate", Source: "website", Temperature: "hot",
		CandidateCompanyName: &company, CandidateContactName: strp("Race Owner"), CandidatePhoneE164: &ph,
	})
	if err != nil {
		t.Fatalf("create lead: %v", err)
	}

	const n = 8
	errs := make([]error, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = svc.Convert(ctx, c, lead.UUID, ConvertInput{Kind: ConvertKindDealerCandidate})
		}(i)
	}
	close(start)
	wg.Wait()
	ok, conflicts := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrAlreadyConverted):
			conflicts++
		default:
			t.Errorf("unexpected convert error: %v", err)
		}
	}
	if ok != 1 || conflicts != n-1 {
		t.Fatalf("ok=%d conflicts=%d, want 1/%d", ok, conflicts, n-1)
	}
	var orgs, users, events int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM organizations WHERE name = $1`, company).Scan(&orgs); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE phone_e164 = $1`, ph).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM lead_events e JOIN leads l ON l.id = e.lead_id
		WHERE l.uuid = $1 AND e.event_type = 'converted'`, lead.UUID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if orgs != 1 || users != 1 || events != 1 {
		t.Fatalf("orgs=%d users=%d converted events=%d, want 1/1/1", orgs, users, events)
	}
}
