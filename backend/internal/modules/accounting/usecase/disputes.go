package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/accounting/posting"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/events"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/outbox"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/pkg/apiquery"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TEC-174 (F1-07d): cari disputes (K24).
//
// A child organization never deletes or rewrites what its parent posted to
// its ledger: it opens a dispute on the row (accounting.dispute). The parent
// (the counterparty of the cari, accounting.resolve) resolves it:
//
//   - reversal: every open row of the source is reversed (VoidBySourceTx),
//     on both ledgers; not for an order sale with a booked return
//     (ErrDisputeSaleReturned, TEC-229);
//   - revision: the source is reversed and reposted with the corrected
//     amount as revision + 1, in the same transaction;
//   - reject: nothing is posted; the note says why.
//
// A dispute is visible to its two sides within the accounting.read scope
// (the disputing organization and the parent it addresses); brand/all
// scopes see every dispute of the domain brand. Each step writes an outbox
// event and an audit row in its transaction.

// Dispute statuses (accounting_disputes.status).
const (
	DisputeOpen             = "open"
	DisputeResolvedReversal = "resolved_reversal"
	DisputeResolvedRevision = "resolved_revision"
	DisputeRejected         = "rejected"
)

// Resolutions accepted by ResolveDispute.
const (
	ResolutionReversal = "reversal"
	ResolutionRevision = "revision"
	ResolutionReject   = "reject"
)

// Audit resource of dispute rows (activity_events.resource).
const disputeResource = "accounting_dispute"

const maxDisputeText = 2000

// Dispute errors; the handler maps them to HTTP codes.
var (
	ErrDisputeNotFound = errors.New("accounting: dispute not found")
	// ErrNotDisputable: the entry is not a sourced original row the parent
	// posted to this organization's cari, or it has been reversed.
	ErrNotDisputable = errors.New("accounting: entry cannot be disputed")
	// ErrDisputeAlreadyOpen: the entry already has an open dispute.
	ErrDisputeAlreadyOpen = errors.New("accounting: entry already has an open dispute")
	// ErrDisputeClosed: the dispute is resolved or rejected (final).
	ErrDisputeClosed = errors.New("accounting: dispute is not open")
	// ErrDisputeNotResolvable: the ledger no longer allows the requested
	// resolution (e.g. revising a source that is already reversed).
	ErrDisputeNotResolvable = errors.New("accounting: dispute cannot be resolved this way")
	// ErrDisputeSaleReturned (TEC-229, K24): the disputed order sale has a
	// received (and booked) return; a reversal would reverse the same sale
	// a second time. Revise or reject the dispute instead.
	ErrDisputeSaleReturned = errors.New("accounting: the disputed sale already has a booked return")
)

// disputeSourceOrder is the accounting source type of an order sale (orders
// usecase AccountingSourceType).
const disputeSourceOrder = "order"

// WithOutbox sets the outbox the dispute events go to (nil: none).
func (s *Service) WithOutbox(out outbox.Enqueuer) *Service {
	s.out = out
	return s
}

// DisputeEntry is the disputed ledger row (in the disputing organization).
type DisputeEntry struct {
	UUID             uuid.UUID `json:"uuid"`
	Direction        string    `json:"direction"`
	Category         string    `json:"category"`
	CategoryLabelKey string    `json:"category_label_key"`
	OrigCurrency     string    `json:"orig_currency"`
	OrigAmount       string    `json:"orig_amount"`
	Currency         string    `json:"currency"`
	Amount           string    `json:"amount"`
	Revision         int32     `json:"revision"`
	CreatedAt        time.Time `json:"created_at"`
}

// Dispute is one dispute as the API shows it.
type Dispute struct {
	UUID                     uuid.UUID    `json:"uuid"`
	Status                   string       `json:"status"`
	Organization             Ref          `json:"organization"`
	CounterpartyOrganization Ref          `json:"counterparty_organization"`
	Entry                    DisputeEntry `json:"entry"`
	SourceType               string       `json:"source_type"`
	SourceUUID               uuid.UUID    `json:"source_uuid"`
	Reason                   string       `json:"reason"`
	CorrectedAmount          *string      `json:"corrected_amount"`
	ResolutionNote           *string      `json:"resolution_note"`
	ReversalEntryUUID        *uuid.UUID   `json:"reversal_entry_uuid"`
	RevisionEntryUUID        *uuid.UUID   `json:"revision_entry_uuid"`
	CreatedAt                time.Time    `json:"created_at"`
	ResolvedAt               *time.Time   `json:"resolved_at"`
}

func disputeOf(r db.ListAccountingDisputesRow) Dispute {
	d := Dispute{
		UUID: r.Uuid, Status: r.Status,
		Organization:             Ref{UUID: r.OrganizationUuid, Name: r.OrganizationName},
		CounterpartyOrganization: Ref{UUID: r.CounterpartyOrgUuid, Name: r.CounterpartyOrgName},
		Entry: DisputeEntry{
			UUID: r.EntryUuid, Direction: r.EntryDirection, Category: r.EntryCategory,
			CategoryLabelKey: "accounting.category." + r.EntryCategory,
			OrigCurrency:     r.EntryOrigCurrency, OrigAmount: posting.FormatNumeric(r.EntryOrigAmount),
			Currency: r.EntryCurrency, Amount: posting.FormatNumeric(r.EntryAmount),
			Revision: r.EntryRevision, CreatedAt: r.EntryCreatedAt.Time,
		},
		SourceType: r.SourceType, SourceUUID: r.SourceUuid, Reason: r.Reason,
		ReversalEntryUUID: uuidPtr(r.ReversalEntryUuid),
		RevisionEntryUUID: uuidPtr(r.RevisionEntryUuid),
		CreatedAt:         r.CreatedAt.Time,
	}
	if r.CorrectedAmount.Valid {
		v := posting.FormatNumeric(r.CorrectedAmount)
		d.CorrectedAmount = &v
	}
	if r.ResolutionNote.Valid {
		v := r.ResolutionNote.String
		d.ResolutionNote = &v
	}
	if r.ResolvedAt.Valid {
		t := r.ResolvedAt.Time
		d.ResolvedAt = &t
	}
	return d
}

// DisputeFilter narrows ListDisputes (ParseDisputeFilter builds it).
type DisputeFilter struct {
	Statuses                   []string
	OrganizationUUIDs          []uuid.UUID // disputing organizations
	CounterpartyUUIDs          []uuid.UUID // addressed (parent) organizations
	CreatedFrom, CreatedBefore *time.Time
	Q                          string
	Sort                       apiquery.ResolvedSort
	Limit, Offset              int32
}

func validDisputeStatus(s string) bool {
	switch s {
	case DisputeOpen, DisputeResolvedReversal, DisputeResolvedRevision, DisputeRejected:
		return true
	}
	return false
}

// ListDisputes lists the disputes the caller's scope reaches (default
// newest first).
func (s *Service) ListDisputes(ctx context.Context, c Caller, f DisputeFilter) ([]Dispute, int64, error) {
	if _, err := s.activeOrg(ctx, c); err != nil {
		return nil, 0, err
	}
	sort := sortOrDefault(f.Sort, DisputeSort)
	arg := db.ListAccountingDisputesParams{
		BrandID: c.Org.BrandID, OrgIds: c.Filter.OrgIDsArg(), Statuses: f.Statuses,
		OrganizationUuids: f.OrganizationUUIDs, CounterpartyUuids: f.CounterpartyUUIDs,
		CreatedFrom: timeArg(f.CreatedFrom), CreatedBefore: timeArg(f.CreatedBefore), Q: likeArg(f.Q),
		SortKey: sort.Key, SortDesc: sort.Desc, RowLimit: f.Limit, RowOffset: f.Offset,
	}
	for _, st := range f.Statuses {
		if !validDisputeStatus(st) {
			return nil, 0, invalid("status", "must be open, resolved_reversal, resolved_revision or rejected")
		}
	}
	if arg.RowLimit <= 0 {
		arg.RowLimit = 20
	}
	rows, err := s.q.ListAccountingDisputes(ctx, arg)
	if err != nil {
		return nil, 0, fmt.Errorf("accounting: disputes: %w", err)
	}
	total, err := s.q.CountAccountingDisputes(ctx, db.CountAccountingDisputesParams{
		BrandID: arg.BrandID, OrgIds: arg.OrgIds, Statuses: arg.Statuses,
		OrganizationUuids: arg.OrganizationUuids, CounterpartyUuids: arg.CounterpartyUuids,
		CreatedFrom: arg.CreatedFrom, CreatedBefore: arg.CreatedBefore, Q: arg.Q,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("accounting: count disputes: %w", err)
	}
	out := make([]Dispute, 0, len(rows))
	for _, r := range rows {
		out = append(out, disputeOf(r))
	}
	return out, total, nil
}

// GetDispute returns one dispute inside the caller's scope; anything else
// reads as not found.
func (s *Service) GetDispute(ctx context.Context, c Caller, id uuid.UUID) (Dispute, error) {
	if _, err := s.activeOrg(ctx, c); err != nil {
		return Dispute{}, err
	}
	return s.dispute(ctx, s.q, c, id)
}

func (s *Service) dispute(ctx context.Context, q *db.Queries, c Caller, id uuid.UUID) (Dispute, error) {
	r, err := q.GetAccountingDisputeView(ctx, db.GetAccountingDisputeViewParams{
		Uuid: id, BrandID: c.Org.BrandID, OrgIds: c.Filter.OrgIDsArg(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Dispute{}, ErrDisputeNotFound
	}
	if err != nil {
		return Dispute{}, fmt.Errorf("accounting: dispute: %w", err)
	}
	return disputeOf(db.ListAccountingDisputesRow(r)), nil
}

// DisputeInput opens a dispute on a ledger row of the active organization.
type DisputeInput struct {
	EntryUUID uuid.UUID
	Reason    string
}

func disputeText(field, v string, required bool) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		if required {
			return "", invalid(field, "is required")
		}
		return "", nil
	}
	if len([]rune(v)) > maxDisputeText {
		return "", invalid(field, fmt.Sprintf("must be at most %d characters", maxDisputeText))
	}
	return v, nil
}

// OpenDispute opens a dispute on a row the parent posted to the active
// organization's ledger (K24). The row must be an open (unreversed) sourced
// row on the organization's cari with its parent; one open dispute per row.
func (s *Service) OpenDispute(ctx context.Context, c Caller, in DisputeInput) (Dispute, error) {
	book, err := s.activeOrg(ctx, c)
	if err != nil {
		return Dispute{}, err
	}
	reason, err := disputeText("reason", in.Reason, true)
	if err != nil {
		return Dispute{}, err
	}
	entry, err := s.q.GetFinanceEntryInOrgByUUID(ctx, db.GetFinanceEntryInOrgByUUIDParams{Uuid: in.EntryUUID, OrganizationID: book.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Dispute{}, ErrEntryNotFound
	}
	if err != nil {
		return Dispute{}, fmt.Errorf("accounting: entry: %w", err)
	}
	if !book.ParentID.Valid || !entry.CariID.Valid || entry.ReversalOfID.Valid ||
		!entry.SourceType.Valid || entry.SourceType.String == SourceManual || !entry.SourceUuid.Valid {
		return Dispute{}, ErrNotDisputable
	}
	cari, err := s.q.GetCariAccount(ctx, db.GetCariAccountParams{ID: entry.CariID.Int64, OrganizationID: book.ID})
	if err != nil {
		return Dispute{}, fmt.Errorf("accounting: cari: %w", err)
	}
	if !cari.CounterpartyOrgID.Valid || cari.CounterpartyOrgID.Int64 != book.ParentID.Int64 {
		return Dispute{}, ErrNotDisputable
	}
	if _, err := s.q.GetFinanceEntryReversal(ctx, pgtype.Int8{Int64: entry.ID, Valid: true}); err == nil {
		return Dispute{}, ErrNotDisputable // already reversed
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Dispute{}, fmt.Errorf("accounting: entry reversal: %w", err)
	}

	var out Dispute
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		d, err := q.InsertAccountingDispute(ctx, db.InsertAccountingDisputeParams{
			OrganizationID: book.ID, BrandID: book.BrandID, CounterpartyOrgID: book.ParentID.Int64,
			EntryID: entry.ID, SourceType: entry.SourceType.String, SourceUuid: uuid.UUID(entry.SourceUuid.Bytes),
			Reason: reason, OpenedByUserID: i8(c.actor()),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDisputeAlreadyOpen
		}
		if err != nil {
			return fmt.Errorf("accounting: open dispute: %w", err)
		}
		if err := s.record(ctx, tx, events.AccountingDisputeOpened, d.CounterpartyOrgID, d, entry, nil, nil, c.actor()); err != nil {
			return err
		}
		out, err = s.disputeByID(ctx, q, d)
		return err
	})
	if err != nil {
		return Dispute{}, err
	}
	return out, nil
}

// disputeByID reads a dispute without a scope filter (after a write the
// caller is one of its sides).
func (s *Service) disputeByID(ctx context.Context, q *db.Queries, d db.AccountingDispute) (Dispute, error) {
	v, err := q.GetAccountingDisputeView(ctx, db.GetAccountingDisputeViewParams{Uuid: d.Uuid, BrandID: d.BrandID})
	if err == nil {
		return disputeOf(db.ListAccountingDisputesRow(v)), nil
	}
	return Dispute{}, fmt.Errorf("accounting: dispute: %w", err)
}

// ResolveInput resolves an open dispute.
type ResolveInput struct {
	Resolution      string // reversal, revision or reject
	Note            string // required for reject
	CorrectedAmount string // revision: the corrected amount in the row's original currency
}

// ResolveDispute resolves a dispute addressed to the active organization
// (the parent the disputed row came from). Disputes of other organizations
// read as not found; a final dispute is ErrDisputeClosed.
func (s *Service) ResolveDispute(ctx context.Context, c Caller, id uuid.UUID, in ResolveInput) (Dispute, error) {
	active, err := s.activeOrg(ctx, c)
	if err != nil {
		return Dispute{}, err
	}
	note, err := disputeText("note", in.Note, in.Resolution == ResolutionReject)
	if err != nil {
		return Dispute{}, err
	}
	switch in.Resolution {
	case ResolutionReversal, ResolutionReject:
		if strings.TrimSpace(in.CorrectedAmount) != "" {
			return Dispute{}, invalid("corrected_amount", "is only allowed with a revision")
		}
	case ResolutionRevision:
		if strings.TrimSpace(in.CorrectedAmount) == "" {
			return Dispute{}, invalid("corrected_amount", "is required for a revision")
		}
	default:
		return Dispute{}, invalid("resolution", "must be reversal, revision or reject")
	}

	var out Dispute
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		d, err := q.LockAccountingDispute(ctx, db.LockAccountingDisputeParams{Uuid: id, CounterpartyOrgID: active.ID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && d.BrandID != c.Org.BrandID) {
			return ErrDisputeNotFound
		}
		if err != nil {
			return fmt.Errorf("accounting: dispute: %w", err)
		}
		if d.Status != DisputeOpen {
			return ErrDisputeClosed
		}
		entry, err := q.GetFinanceEntry(ctx, db.GetFinanceEntryParams{ID: d.EntryID, OrganizationID: d.OrganizationID})
		if err != nil {
			return fmt.Errorf("accounting: disputed entry: %w", err)
		}
		src := posting.Source{Type: d.SourceType, UUID: d.SourceUuid}
		desc := "dispute " + d.Uuid.String()
		if note != "" {
			desc += ": " + note
		}
		arg := db.ResolveAccountingDisputeParams{
			ID: d.ID, ResolutionNote: pgtype.Text{String: note, Valid: note != ""},
			ResolvedByUserID: i8(c.actor()),
		}
		var reversal, revision *db.FinanceEntry
		event := events.AccountingDisputeResolved
		switch in.Resolution {
		case ResolutionReversal:
			if err := checkSaleNotReturned(ctx, q, d); err != nil {
				return err
			}
			res, err := s.poster.VoidBySourceTx(ctx, tx, src, desc, c.actor())
			if err != nil {
				return err
			}
			rev, err := reversalOf(ctx, q, res.Reversals, entry.ID)
			if err != nil {
				return err
			}
			reversal = &rev
			arg.Status = DisputeResolvedReversal
			arg.ReversalEntryID = pgtype.Int8{Int64: rev.ID, Valid: true}
		case ResolutionRevision:
			res, err := s.poster.ReviseBySourceTx(ctx, tx, src, strings.TrimSpace(in.CorrectedAmount), desc, c.actor())
			switch {
			case errors.Is(err, posting.ErrNothingOpen):
				return ErrDisputeNotResolvable
			case errors.Is(err, posting.ErrInvalid):
				return invalid("corrected_amount", strings.TrimPrefix(err.Error(), "posting: invalid request: "))
			case err != nil:
				return err
			}
			var rev, repost *db.FinanceEntry
			for i := range res.Reversals {
				if res.Reversals[i].ReversalOfID.Int64 == entry.ID {
					rev = &res.Reversals[i]
				}
			}
			for i := range res.Reposts {
				r := &res.Reposts[i]
				if r.OrganizationID == d.OrganizationID && r.Role == entry.Role {
					repost = r
				}
			}
			if rev == nil || repost == nil {
				return ErrDisputeNotResolvable // the disputed row was no longer open
			}
			if posting.FormatNumeric(repost.OrigAmount) == posting.FormatNumeric(entry.OrigAmount) {
				return invalid("corrected_amount", "must differ from the disputed amount")
			}
			reversal, revision = rev, repost
			arg.Status = DisputeResolvedRevision
			arg.CorrectedAmount = repost.OrigAmount
			arg.ReversalEntryID = pgtype.Int8{Int64: rev.ID, Valid: true}
			arg.RevisionEntryID = pgtype.Int8{Int64: repost.ID, Valid: true}
		case ResolutionReject:
			arg.Status = DisputeRejected
			event = events.AccountingDisputeRejected
		}
		resolved, err := q.ResolveAccountingDispute(ctx, arg)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDisputeClosed
		}
		if err != nil {
			return fmt.Errorf("accounting: resolve dispute: %w", err)
		}
		if err := s.record(ctx, tx, event, resolved.OrganizationID, resolved, entry, reversal, revision, c.actor()); err != nil {
			return err
		}
		out, err = s.disputeByID(ctx, q, resolved)
		return err
	})
	if err != nil {
		return Dispute{}, postingErr(err)
	}
	return out, nil
}

// checkSaleNotReturned (TEC-229, K24): a dispute on an order sale cannot be
// reversed once a return of a line of that order was received and booked
// (PostStockReturnTx already reversed that part of the sale). The order is
// locked FOR UPDATE so a concurrent return receipt (which locks it FOR
// SHARE) is serialized with this check.
func checkSaleNotReturned(ctx context.Context, q *db.Queries, d db.AccountingDispute) error {
	if d.SourceType != disputeSourceOrder {
		return nil
	}
	orderID, err := q.LockOrderForDisputeReversal(ctx, db.LockOrderForDisputeReversalParams{Uuid: d.SourceUuid, BrandID: d.BrandID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("accounting: disputed order: %w", err)
	}
	n, err := q.CountBookedReturnItemsOfOrder(ctx, orderID)
	if err != nil {
		return fmt.Errorf("accounting: returns of disputed order: %w", err)
	}
	if n > 0 {
		return ErrDisputeSaleReturned
	}
	return nil
}

// reversalOf returns the reversal of entry among rows, or the reversal an
// earlier void already wrote (the source was voided while the dispute was
// open; the reversal then resolves the dispute as well).
func reversalOf(ctx context.Context, q *db.Queries, rows []db.FinanceEntry, entryID int64) (db.FinanceEntry, error) {
	for _, r := range rows {
		if r.ReversalOfID.Int64 == entryID {
			return r, nil
		}
	}
	r, err := q.GetFinanceEntryReversal(ctx, pgtype.Int8{Int64: entryID, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.FinanceEntry{}, ErrDisputeNotResolvable
	}
	if err != nil {
		return db.FinanceEntry{}, fmt.Errorf("accounting: entry reversal: %w", err)
	}
	return r, nil
}

// record writes the dispute event (outbox) and its audit row in tx. tenant
// is the organization the notification is for: the parent when a dispute
// opens, the disputing organization when it is resolved or rejected.
func (s *Service) record(ctx context.Context, tx pgx.Tx, name string, tenant int64, d db.AccountingDispute,
	entry db.FinanceEntry, reversal, revision *db.FinanceEntry, actor *int64) error {
	payload := map[string]any{
		"dispute_id":          d.ID,
		"dispute_uuid":        d.Uuid.String(),
		"status":              d.Status,
		"organization_id":     d.OrganizationID,
		"counterparty_org_id": d.CounterpartyOrgID,
		"brand_id":            d.BrandID,
		"entry_id":            entry.ID,
		"entry_uuid":          entry.Uuid.String(),
		"source_type":         d.SourceType,
		"source_uuid":         d.SourceUuid.String(),
		"orig_currency":       entry.OrigCurrency,
		"orig_amount":         posting.FormatNumeric(entry.OrigAmount),
	}
	if reversal != nil {
		payload["reversal_entry_uuid"] = reversal.Uuid.String()
	}
	if revision != nil {
		payload["revision_entry_uuid"] = revision.Uuid.String()
		payload["corrected_amount"] = posting.FormatNumeric(revision.OrigAmount)
	}
	if s.out != nil {
		id, uid := d.ID, d.Uuid
		ev := events.New(name).WithTenant(tenant).WithEntity(disputeResource, &id, &uid).WithPayload(payload)
		if actor != nil {
			ev = ev.WithActor(*actor)
		}
		if err := s.out.Enqueue(ctx, tx, ev); err != nil {
			return fmt.Errorf("accounting: outbox: %w", err)
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("accounting: audit payload: %w", err)
	}
	uid := d.Uuid
	if _, err := s.q.WithTx(tx).InsertActivityEvent(ctx, db.InsertActivityEventParams{
		ActorUserID: i8(actor), Action: name, Resource: disputeResource,
		ResourceUuid: pgtype.UUID{Bytes: uid, Valid: true}, Payload: body,
	}); err != nil {
		return fmt.Errorf("accounting: audit: %w", err)
	}
	return nil
}

func i8(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}
