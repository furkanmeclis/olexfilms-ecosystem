package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/features"
	"github.com/jackc/pgx/v5/pgtype"
)

type contractRequiredSettings bool

func (s contractRequiredSettings) ContractsIntakeRequired(context.Context) bool { return bool(s) }

type contractRequiredFeatures bool

func (f contractRequiredFeatures) Enabled(_ context.Context, _ int64, key string) (bool, error) {
	if key != features.ModuleIntakeContracts {
		return false, nil
	}
	return bool(f), nil
}

type contractRequiredStore struct {
	status string
	err    error
}

func (s contractRequiredStore) GetContractInstanceByID(context.Context, int64) (db.ContractInstance, error) {
	return db.ContractInstance{Status: s.status}, s.err
}

func TestNeedsExecutedContract(t *testing.T) {
	cases := []struct {
		from string
		to   string
		want bool
	}{
		{StatusDraft, StatusProcessing, true},
		{StatusPending, StatusProcessing, true},
		{StatusDraft, StatusCompleted, true},
		{StatusProcessing, StatusCompleted, false},
		{StatusReady, StatusCompleted, false},
		{StatusPending, StatusReady, false},
	}
	for _, tc := range cases {
		if got := needsExecutedContract(tc.from, tc.to); got != tc.want {
			t.Fatalf("needsExecutedContract(%s, %s) = %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}

func TestRequireExecutedContract(t *testing.T) {
	ctx := context.Background()
	svc := db.Service{OrganizationID: 10}

	if err := New(nil, nil, nil).
		WithContractRequirement(contractRequiredSettings(false), contractRequiredFeatures(true)).
		requireExecutedContract(ctx, contractRequiredStore{}, svc); err != nil {
		t.Fatalf("setting off without contract = %v", err)
	}
	if err := New(nil, nil, nil).
		WithContractRequirement(contractRequiredSettings(true), contractRequiredFeatures(false)).
		requireExecutedContract(ctx, contractRequiredStore{}, svc); err != nil {
		t.Fatalf("module off without contract = %v", err)
	}

	guarded := New(nil, nil, nil).
		WithContractRequirement(contractRequiredSettings(true), contractRequiredFeatures(true))
	if err := guarded.requireExecutedContract(ctx, contractRequiredStore{}, svc); !errors.Is(err, ErrContractRequired) {
		t.Fatalf("required without contract = %v, want ErrContractRequired", err)
	}

	svc.ContractID = pgtype.Int8{Int64: 123, Valid: true}
	if err := guarded.requireExecutedContract(ctx, contractRequiredStore{status: "pending"}, svc); !errors.Is(err, ErrContractRequired) {
		t.Fatalf("pending contract = %v, want ErrContractRequired", err)
	}
	if err := guarded.requireExecutedContract(ctx, contractRequiredStore{status: "executed"}, svc); err != nil {
		t.Fatalf("executed contract = %v", err)
	}
}
