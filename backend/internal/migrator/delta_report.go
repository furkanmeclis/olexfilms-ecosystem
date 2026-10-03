package migrator

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Cutover delta report (TEC-276, rollback support of TEC-111). After the
// cutover the legacy systems are read-only; if the cutover is rolled back,
// the records born in this application since then have to be re-entered in
// the legacy systems by hand (there is no reverse import). DeltaReport lists
// them: rows of the scanned business tables with created_at >= since whose
// uuid is not a migration_map target (the migrator did not bring them).
//
// The report only reads; the command runs it in a READ ONLY transaction.

// DeltaTables is the scanned business tables, in report order.
var DeltaTables = []string{
	"services", "warranties", "users", "customer_organizations", "vehicles",
	"vehicle_transfers", "orders", "stock_movements", "finance_entries",
	"measurement_results",
}

// DeltaCSVHeader is the fixed CSV header row of the report.
var DeltaCSVHeader = []string{"table", "uuid", "organization", "created_at", "summary"}

// DeltaRow is one record born in this application after the cutover.
type DeltaRow struct {
	Table string    `json:"table"`
	UUID  uuid.UUID `json:"uuid"`
	// Organization is the organization name; empty for rows without one
	// (users, vehicles a customer added).
	Organization string    `json:"organization"`
	CreatedAt    time.Time `json:"created_at"`
	Summary      string    `json:"summary"`
}

// DeltaQuerier is the read access the report needs (a pool or a transaction).
type DeltaQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// notMapped is the migration_map filter of a table whose rows carry a uuid.
func notMapped(table, alias string) string {
	return fmt.Sprintf(`NOT EXISTS (SELECT 1 FROM migration_map mm
		WHERE mm.target_table = '%s' AND mm.target_uuid = %s.uuid)`, table, alias)
}

// deltaQuery unions the scanned tables. $1 is since.
//
// customer_organizations has no uuid of its own and no migration_map rows:
// the migrator writes the links of a customer while it migrates the
// customer (CustomersStep), keeping the legacy created_at. Such a link is
// reported with the customer's user uuid and counts as migrated when that
// user is a migration_map target mapped at or after the link's created_at;
// a link created in this application to an already migrated customer is
// newer than the customer's mapping and is listed.
var deltaQuery = `
SELECT 'services', s.uuid, o.name, s.created_at,
       concat_ws(' ', s.service_no, s.status, s.plate_country, s.plate, s.vin)
FROM services s
JOIN organizations o ON o.id = s.organization_id
WHERE s.created_at >= $1 AND ` + notMapped("services", "s") + `
UNION ALL
SELECT 'warranties', w.uuid, o.name, w.created_at,
       concat_ws(' ', w.public_code, w.status, 'service=' || sv.service_no)
FROM warranties w
JOIN organizations o ON o.id = w.organization_id
JOIN services sv ON sv.id = w.service_id
WHERE w.created_at >= $1 AND ` + notMapped("warranties", "w") + `
UNION ALL
SELECT 'users', u.uuid, '', u.created_at,
       concat_ws(' ', u.name, u.surname, u.email, u.phone_e164, u.status,
                 CASE WHEN EXISTS (SELECT 1 FROM customer_profiles cp WHERE cp.user_id = u.id)
                      THEN 'customer' END)
FROM users u
WHERE u.created_at >= $1 AND ` + notMapped("users", "u") + `
UNION ALL
SELECT 'customer_organizations', u.uuid, o.name, co.created_at,
       concat_ws(' ', u.name, u.surname, u.email, u.phone_e164)
FROM customer_organizations co
JOIN users u ON u.id = co.user_id
JOIN organizations o ON o.id = co.organization_id
WHERE co.created_at >= $1
  AND NOT EXISTS (SELECT 1 FROM migration_map mm
      WHERE mm.target_table = 'users' AND mm.target_uuid = u.uuid AND mm.migrated_at >= co.created_at)
UNION ALL
SELECT 'vehicles', v.uuid, COALESCE(o.name, ''), v.created_at,
       concat_ws(' ', v.plate_country, v.plate, v.vin, 'owner=' || u.uuid::text)
FROM vehicles v
JOIN users u ON u.id = v.user_id
LEFT JOIN organizations o ON o.id = v.organization_id
WHERE v.created_at >= $1 AND ` + notMapped("vehicles", "v") + `
UNION ALL
SELECT 'vehicle_transfers', vt.uuid, o.name, vt.created_at,
       concat_ws(' ', vt.status, 'vehicle=' || v.uuid::text, v.plate, 'to=' || vt.to_phone)
FROM vehicle_transfers vt
JOIN organizations o ON o.id = vt.organization_id
JOIN vehicles v ON v.id = vt.vehicle_id
WHERE vt.created_at >= $1 AND ` + notMapped("vehicle_transfers", "vt") + `
UNION ALL
SELECT 'orders', od.uuid, o.name, od.created_at,
       concat_ws(' ', od.order_no, od.status, od.total::text, od.currency,
                 'seller=' || so.name, 'buyer=' || bo.name)
FROM orders od
JOIN organizations o ON o.id = od.organization_id
JOIN organizations so ON so.id = od.seller_org_id
JOIN organizations bo ON bo.id = od.buyer_org_id
WHERE od.created_at >= $1 AND ` + notMapped("orders", "od") + `
UNION ALL
SELECT 'stock_movements', sm.uuid, o.name, sm.created_at,
       concat_ws(' ', sm.type, 'unit=' || un.uuid::text, 'qty=' || sm.quantity_delta::text,
                 'meters=' || sm.meters_delta::text, sm.reference_type || '#' || sm.reference_id::text)
FROM stock_movements sm
JOIN organizations o ON o.id = sm.organization_id
JOIN units un ON un.id = sm.unit_id
WHERE sm.created_at >= $1 AND ` + notMapped("stock_movements", "sm") + `
UNION ALL
SELECT 'finance_entries', fe.uuid, o.name, fe.created_at,
       concat_ws(' ', fe.direction, fe.category, fe.amount::text, fe.currency, fe.source_type, fe.description)
FROM finance_entries fe
JOIN organizations o ON o.id = fe.organization_id
WHERE fe.created_at >= $1 AND ` + notMapped("finance_entries", "fe") + `
UNION ALL
SELECT 'measurement_results', mr.uuid, o.name, mr.created_at,
       concat_ws(' ', mr.status, mr.vin, mr.source, mr.device_serial)
FROM measurement_results mr
JOIN organizations o ON o.id = mr.organization_id
WHERE mr.created_at >= $1 AND ` + notMapped("measurement_results", "mr") + `
ORDER BY 4, 1, 2`

// DeltaReport lists the records created at or after since that the
// migrator did not bring, oldest first.
func DeltaReport(ctx context.Context, q DeltaQuerier, since time.Time) ([]DeltaRow, error) {
	rows, err := q.Query(ctx, deltaQuery, since)
	if err != nil {
		return nil, fmt.Errorf("delta report: %w", err)
	}
	defer rows.Close()
	var out []DeltaRow
	for rows.Next() {
		var r DeltaRow
		if err := rows.Scan(&r.Table, &r.UUID, &r.Organization, &r.CreatedAt, &r.Summary); err != nil {
			return nil, fmt.Errorf("delta report scan: %w", err)
		}
		r.CreatedAt = r.CreatedAt.UTC()
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("delta report: %w", err)
	}
	return out, nil
}

// WriteDeltaCSV writes the rows under the fixed DeltaCSVHeader.
func WriteDeltaCSV(w io.Writer, rows []DeltaRow) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(DeltaCSVHeader); err != nil {
		return err
	}
	for _, r := range rows {
		if err := cw.Write([]string{
			r.Table, r.UUID.String(), r.Organization, r.CreatedAt.UTC().Format(time.RFC3339), r.Summary,
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteDeltaJSON writes the rows as a JSON array (an empty report is []).
func WriteDeltaJSON(w io.Writer, rows []DeltaRow) error {
	if rows == nil {
		rows = []DeltaRow{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
}
