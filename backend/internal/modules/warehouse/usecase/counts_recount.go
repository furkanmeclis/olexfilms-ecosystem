package usecase

// TEC-1229: recount of the difference lines of a completed count. The
// approver sends unresolved difference lines back (recount_status
// requested); the counter records the recounted values on each line
// (recounted) and the result is evaluated again with the same rules as
// completion. The first counted values stay on the line (first_count).
// Lines waiting for a recount cannot be resolved; ApprovePartial resolves a
// part of the lines and the count is approved once nothing is open.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Recount states (stock_count_lines.recount_status).
const (
	CountRecountRequested = "requested"
	CountRecountRecounted = "recounted"
)

// ErrCountLineNotFound: the line is not a line of the count.
var ErrCountLineNotFound = errors.New("warehouse: stock count line not found")

// CountRecountInput is the recounted value of one line. Serial lines take
// 0 (not found) or 1; a found serial unit names its location (optional for
// a unit expected without a location) and, for a roll, its meters.
type CountRecountInput struct {
	CountedQuantity int32   `json:"counted_quantity"`
	CountedMeters   *string `json:"counted_meters"`
	LocationUUID    *string `json:"location_uuid"`
}

// RequestRecount sends unresolved difference lines of a count waiting for
// review back for a recount.
func (s *Counts) RequestRecount(ctx context.Context, c ScanCaller, id uuid.UUID, lineUUIDs []string) (CountReport, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return CountReport{}, err
	}
	if len(lineUUIDs) == 0 {
		return CountReport{}, invalid("line_uuids", "is required")
	}
	err = s.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
		cnt, err := s.lockCount(ctx, q, c, org, id)
		if err != nil {
			return err
		}
		if cnt.Status != CountStatusPendingReview {
			return ErrCountStatus
		}
		lines, err := q.ListStockCountLines(ctx, cnt.ID)
		if err != nil {
			return fmt.Errorf("warehouse: count lines: %w", err)
		}
		byUUID := make(map[uuid.UUID]db.StockCountLine, len(lines))
		for _, l := range lines {
			byUUID[l.Uuid] = l
		}
		seen := map[uuid.UUID]bool{}
		for _, raw := range lineUUIDs {
			lid, err := parseUUIDField("line_uuids", raw)
			if err != nil {
				return err
			}
			l, ok := byUUID[lid]
			if !ok {
				return invalid("line_uuids", "line "+lid.String()+" is not a line of this count")
			}
			if seen[lid] {
				continue
			}
			seen[lid] = true
			if l.ResolvedAt.Valid || l.Result == CountResultMatched {
				return invalid("line_uuids", "line "+lid.String()+" has no open difference")
			}
			if _, err := q.RequestStockCountLineRecount(ctx, db.RequestStockCountLineRecountParams{
				UserID: userArg(c), ID: l.ID,
			}); err != nil {
				return fmt.Errorf("warehouse: request recount: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return CountReport{}, err
	}
	return s.Report(ctx, c, id)
}

// Recount records the recounted values of one line waiting for a recount
// and evaluates its result again.
func (s *Counts) Recount(ctx context.Context, c ScanCaller, id, lineID uuid.UUID, in CountRecountInput) (CountReport, error) {
	org, err := guard(c.Caller)
	if err != nil {
		return CountReport{}, err
	}
	if in.CountedQuantity < 0 || in.CountedQuantity > MaxCountQuantity {
		return CountReport{}, invalid("counted_quantity", "must be between 0 and 100000")
	}
	err = s.inTx(ctx, func(_ pgx.Tx, q *db.Queries) error {
		cnt, err := s.lockCount(ctx, q, c, org, id)
		if err != nil {
			return err
		}
		if cnt.Status != CountStatusPendingReview {
			return ErrCountStatus
		}
		lines, err := q.ListStockCountLines(ctx, cnt.ID)
		if err != nil {
			return fmt.Errorf("warehouse: count lines: %w", err)
		}
		var line *db.StockCountLine
		for i := range lines {
			if lines[i].Uuid == lineID {
				line = &lines[i]
				break
			}
		}
		if line == nil {
			return ErrCountLineNotFound
		}
		if line.RecountStatus.String != CountRecountRequested || line.ResolvedAt.Valid {
			return ErrCountStatus
		}
		arg := db.RecordStockCountLineRecountParams{ID: line.ID, UserID: userArg(c), CountedQuantity: in.CountedQuantity}
		if in.LocationUUID != nil && strings.TrimSpace(*in.LocationUUID) != "" {
			lid, err := parseUUIDField("location_uuid", *in.LocationUUID)
			if err != nil {
				return err
			}
			loc, err := q.GetWarehouseLocationByUUID(ctx, lid)
			if err != nil || loc.OrganizationID != cnt.OrganizationID {
				return invalid("location_uuid", "is not a location of the organization")
			}
			scope, _, err := scopeLocations(ctx, q, cnt)
			if err != nil {
				return err
			}
			if !scope[loc.ID] {
				return ErrCountLocationOutOfScope
			}
			arg.CountedLocationID = pgtype.Int8{Int64: loc.ID, Valid: true}
		}
		if in.CountedMeters != nil && strings.TrimSpace(*in.CountedMeters) != "" {
			cmv, err := ledger.ParseMeters(*in.CountedMeters)
			if err != nil || cmv < 0 {
				return invalid("counted_meters", "must be a non-negative number of meters")
			}
			arg.CountedMeters = cmToNum(cmv)
		}
		result, err := RecountResult(*line, arg.CountedQuantity, arg.CountedLocationID, arg.CountedMeters)
		if err != nil {
			return err
		}
		arg.Result = result
		if _, err := q.RecordStockCountLineRecount(ctx, arg); err != nil {
			return fmt.Errorf("warehouse: record recount: %w", err)
		}
		return nil
	})
	if err != nil {
		return CountReport{}, err
	}
	return s.Report(ctx, c, id)
}

// RecountResult evaluates a recounted line like completion does: a serial
// unit is missing (0 found but expected), unexpected (found but not
// expected), matched at its expected location (meter_variance when the
// measured roll meters differ), unlocated when expected without a location
// and wrong_location elsewhere; fixed barcodes and product quantities
// match or are a qty_variance.
func RecountResult(l db.StockCountLine, counted int32, loc pgtype.Int8, meters pgtype.Numeric) (string, error) {
	if l.LineKind != CountScanSerial {
		if counted == l.ExpectedQuantity {
			return CountResultMatched, nil
		}
		return CountResultQtyVariance, nil
	}
	switch {
	case counted > 1:
		return "", invalid("counted_quantity", "a serial unit is counted as 0 or 1")
	case counted == 0 && l.ExpectedQuantity > 0:
		return CountResultMissing, nil
	case counted == 0:
		return CountResultMatched, nil
	case l.ExpectedQuantity == 0:
		return CountResultUnexpected, nil
	}
	if l.ExpectedLocationID.Valid && !loc.Valid {
		return "", invalid("location_uuid", "is required for a unit found in a location")
	}
	switch {
	case loc.Int64 == l.ExpectedLocationID.Int64 && loc.Valid == l.ExpectedLocationID.Valid:
		if meters.Valid && l.ExpectedMeters.Valid {
			got, _ := numToCm(meters)
			want, _ := numToCm(l.ExpectedMeters)
			if got != want {
				return CountResultMeterVariance, nil
			}
		}
		return CountResultMatched, nil
	case !l.ExpectedLocationID.Valid:
		return CountResultUnlocated, nil
	default:
		return CountResultWrongLocation, nil
	}
}
