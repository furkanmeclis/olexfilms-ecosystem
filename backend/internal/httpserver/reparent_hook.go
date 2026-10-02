package httpserver

import (
	"context"
	"errors"
	"fmt"

	accountingposting "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	accountingusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/usecase"
	orgusecase "github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/organizations/usecase"
	"github.com/jackc/pgx/v5"
)

// reparentHook carries the open cari over to the new parent inside the
// re-parenting transaction (K25, TEC-198). A missing exchange rate answers
// 400 and keeps the organization where it was.
func reparentHook(svc *accountingusecase.Service) orgusecase.ParentChangeHook {
	return orgusecase.ParentChangeHookFunc(func(ctx context.Context, tx pgx.Tx, c orgusecase.ParentChange) error {
		_, err := svc.TransferCariOnReparentTx(ctx, tx, accountingposting.CariTransfer{
			Change: c.UUID, OrganizationID: c.OrganizationID,
			OldParentID: c.OldParentID, NewParentID: c.NewParentID,
			Description: "K25 re-parenting", ActorUserID: c.ActorUserID,
		})
		if errors.Is(err, accountingusecase.ErrRateNotFound) {
			return fmt.Errorf("%w: no exchange rate to carry the open cari over to the new parent", orgusecase.ErrInvalidRequest)
		}
		return err
	})
}
