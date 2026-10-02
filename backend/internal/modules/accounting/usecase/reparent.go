package usecase

import (
	"context"
	"errors"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/fxrates"
	"github.com/jackc/pgx/v5"
)

// AuditCariTransferred is the audit action of every ledger row a
// re-parenting cari transfer writes (TEC-198, K25).
const AuditCariTransferred = "accounting.cari_transferred"

// SourceCariTransfer is the source_type of cari transfer rows.
const SourceCariTransfer = posting.SourceCariTransfer

// TransferCariOnReparentTx carries the open cari between an organization
// and its old parent over to the new parent inside the re-parenting
// transaction (organizations ChangeParent, K25) and writes one audit row per
// ledger row. The ledger rules are in posting/transfer.go; a repeated call
// with the same change writes nothing. A missing exchange rate (parents in
// different currencies) is ErrRateNotFound and rolls the move back.
func (s *Service) TransferCariOnReparentTx(ctx context.Context, tx pgx.Tx, t posting.CariTransfer) (posting.CariTransferResult, error) {
	res, err := s.poster.TransferCariTx(ctx, tx, t)
	if err != nil {
		if errors.Is(err, fxrates.ErrRateNotFound) {
			return posting.CariTransferResult{}, ErrRateNotFound
		}
		return posting.CariTransferResult{}, err
	}
	for _, e := range res.Written {
		if err := s.auditEntry(ctx, tx, AuditCariTransferred, e, t.ActorUserID, map[string]any{
			"change_uuid":     t.Change.String(),
			"role":            e.Role,
			"old_parent_id":   t.OldParentID,
			"new_parent_id":   t.NewParentID,
			"transferred_org": t.OrganizationID,
		}); err != nil {
			return posting.CariTransferResult{}, err
		}
	}
	return res, nil
}
