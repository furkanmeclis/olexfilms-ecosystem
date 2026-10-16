package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/i18n"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const LocationImportResource = "tenant.warehouse.locations"

const (
	locationImportInvalidPath   = "LOCATION_IMPORT_INVALID_PATH"
	locationImportInvalidCode   = "LOCATION_IMPORT_INVALID_CODE"
	locationImportDuplicate     = "LOCATION_IMPORT_DUPLICATE"
	locationImportTypeConflict  = "LOCATION_IMPORT_TYPE_CONFLICT"
	locationImportInvalidActive = "LOCATION_IMPORT_INVALID_ACTIVE"
)

// LocationImporter stages paths and owns only the nodes it creates. Warehouse
// locking serializes imports targeting the same warehouse during apply.
type LocationImporter struct{ pool TxBeginner }

func NewLocationImporter(pool TxBeginner) *LocationImporter { return &LocationImporter{pool: pool} }
func (*LocationImporter) Resource() string                  { return LocationImportResource }
func (*LocationImporter) ExportColumns() []ioengine.Column  { return nil }
func (*LocationImporter) Export(context.Context, ioengine.ExportQuery, i18n.Locale) (ioengine.Dataset, error) {
	return ioengine.Dataset{}, errors.New("location import has no export")
}
func (*LocationImporter) ImportPermission() string { return rbac.PermWarehouseWrite }
func (*LocationImporter) MaxImportRows() int       { return 5000 }
func (*LocationImporter) ImportSchema() []ioengine.ImportField {
	fields := []ioengine.ImportField{
		{Key: "room_code", Required: true, Kind: ioengine.FieldCode},
		{Key: "room_name"}, {Key: "aisle_code", Kind: ioengine.FieldCode},
		{Key: "shelf_code", Kind: ioengine.FieldCode}, {Key: "bin_code", Kind: ioengine.FieldCode},
		{Key: "name"}, {Key: "active", Enum: []string{"evet", "hayır", "yes", "no"}},
	}
	for i := range fields {
		fields[i].Type = ioengine.ColumnTypeString
		fields[i].LabelKey = "warehouse_import." + fields[i].Key
		fields[i].HintKey = "warehouse_import.hint." + fields[i].Key
	}
	return fields
}
func (*LocationImporter) SampleRows() []map[string]any {
	return []map[string]any{
		{"room_code": "ODA1", "room_name": "Ana oda", "aisle_code": "A1", "shelf_code": "R1", "bin_code": "G1", "name": "Göz 1", "active": "evet"},
		{"room_code": "ODA1", "shelf_code": "R2", "bin_code": "G1", "active": "evet"},
	}
}
func (*LocationImporter) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errors.New("location import is staged")
}
func (*LocationImporter) RevertRow(context.Context, string, string, map[string]any) error {
	return errors.New("location import is staged")
}

type locationPath struct {
	Room     string `json:"room_code"`
	RoomName string `json:"room_name"`
	Aisle    string `json:"aisle_code"`
	Shelf    string `json:"shelf_code"`
	Bin      string `json:"bin_code"`
	Name     string `json:"name"`
	Active   bool   `json:"active"`
}
type importNode struct {
	id   int64
	kind string
}
type stagedLocationRow struct {
	number               int
	path                 locationPath
	status, code, target string
	createCount          int
}

func cell(raw map[string]any, key string) string {
	if raw[key] == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw[key]))
}
func parseLocationPath(raw map[string]any) (locationPath, string) {
	p := locationPath{RoomName: cell(raw, "room_name"), Name: cell(raw, "name"), Active: true}
	for _, f := range []struct {
		key string
		dst *string
	}{{"room_code", &p.Room}, {"aisle_code", &p.Aisle}, {"shelf_code", &p.Shelf}, {"bin_code", &p.Bin}} {
		v := cell(raw, f.key)
		if v == "<nil>" {
			v = ""
		}
		if v == "" {
			continue
		}
		code, err := normCode(f.key, v)
		if err != nil {
			return p, locationImportInvalidCode
		}
		*f.dst = code
	}
	if p.Room == "" || (p.Aisle == "" && p.Shelf == "" && p.Bin == "") || (p.Bin != "" && p.Shelf == "") {
		return p, locationImportInvalidPath
	}
	if _, err := normName("room_name", p.RoomName, p.Room); err != nil {
		return p, locationImportInvalidCode
	}
	if _, err := normName("name", p.Name, p.Room); err != nil {
		return p, locationImportInvalidCode
	}
	a := strings.ToLower(cell(raw, "active"))
	switch a {
	case "", "<nil>", "evet", "yes", "true", "1":
	case "hayır", "hayir", "no", "false", "0":
		p.Active = false
	default:
		return p, locationImportInvalidActive
	}
	return p, ""
}
func (p locationPath) codes() []struct{ kind, code string } {
	n := []struct{ kind, code string }{{"room", p.Room}}
	if p.Aisle != "" {
		n = append(n, struct{ kind, code string }{"aisle", p.Aisle})
	}
	if p.Shelf != "" {
		n = append(n, struct{ kind, code string }{"shelf", p.Shelf})
	}
	if p.Bin != "" {
		n = append(n, struct{ kind, code string }{"bin", p.Bin})
	}
	return n
}

func (im *LocationImporter) warehouse(ctx context.Context, tx pgx.Tx, job ioengine.ImportJob, defaults map[string]any, lock bool) (int64, string, error) {
	u, err := uuid.Parse(cell(defaults, "warehouse_uuid"))
	if err != nil {
		return 0, "", fmt.Errorf("%w: warehouse_uuid is required", ioengine.ErrImportRejected)
	}
	var id int64
	var code string
	query := `SELECT w.id,w.code FROM warehouses w JOIN organizations o ON o.id=w.organization_id WHERE w.uuid=$1 AND w.organization_id=$2 AND o.type IN ('center','distributor')`
	if lock {
		query += " FOR UPDATE OF w"
	}
	if err = tx.QueryRow(ctx, query, u, job.OrganizationID).Scan(&id, &code); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, "", fmt.Errorf("%w: warehouse outside organization", ioengine.ErrImportRejected)
		}
		return 0, "", err
	}
	return id, code, nil
}
func (im *LocationImporter) existing(ctx context.Context, tx pgx.Tx, warehouseID int64, warehouseCode string) (map[string]importNode, error) {
	nodes := map[string]importNode{}
	rooms, err := tx.Query(ctx, `SELECT id,code FROM rooms WHERE warehouse_id=$1`, warehouseID)
	if err != nil {
		return nil, err
	}
	for rooms.Next() {
		var id int64
		var code string
		if err = rooms.Scan(&id, &code); err != nil {
			rooms.Close()
			return nil, err
		}
		nodes[warehouseCode+"-"+code] = importNode{id, "room"}
	}
	err = rooms.Err()
	rooms.Close()
	if err != nil {
		return nil, err
	}
	locs, err := tx.Query(ctx, `SELECT id,full_code,type FROM warehouse_locations WHERE warehouse_id=$1 AND room_id IS NOT NULL`, warehouseID)
	if err != nil {
		return nil, err
	}
	for locs.Next() {
		var id int64
		var path, kind string
		if err = locs.Scan(&id, &path, &kind); err != nil {
			locs.Close()
			return nil, err
		}
		nodes[path] = importNode{id, kind}
	}
	err = locs.Err()
	locs.Close()
	return nodes, err
}
func (im *LocationImporter) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := im.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (im *LocationImporter) Stage(ctx context.Context, job ioengine.ImportJob, rows []map[string]any, defaults map[string]any) (ioengine.PreviewSummary, error) {
	if len(rows) > 5000 {
		return ioengine.PreviewSummary{}, fmt.Errorf("%w: maximum 5000 rows", ioengine.ErrImportRejected)
	}
	var summary ioengine.PreviewSummary
	err := im.tx(ctx, func(tx pgx.Tx) error {
		wid, wcode, err := im.warehouse(ctx, tx, job, defaults, false)
		if err != nil {
			return err
		}
		nodes, err := im.existing(ctx, tx, wid, wcode)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM warehouse_location_import_rows WHERE job_id=$1`, job.ID); err != nil {
			return err
		}
		seen := map[string]bool{}
		summary = ioengine.PreviewSummary{Total: len(rows), BatchUUID: job.UUID.String(), Counts: map[string]int{}}
		for i, raw := range rows {
			p, code := parseLocationPath(raw)
			row := stagedLocationRow{number: i + 1, path: p, code: code, status: "invalid"}
			if code == "" {
				path := wcode
				pending := map[string]importNode{}
				for _, part := range p.codes() {
					path += "-" + part.code
					node, ok := nodes[path]
					if !ok {
						node, ok = pending[path]
					}
					if ok && node.kind != part.kind {
						row.code = locationImportTypeConflict
						row.status = "conflict"
						break
					}
					if !ok {
						pending[path] = importNode{kind: part.kind}
					}
				}
				row.target = path
				if row.code == "" {
					if seen[path] {
						row.code = locationImportDuplicate
					} else if len(pending) > 0 {
						row.status = "new"
						row.createCount = len(pending)
					} else {
						row.status = "exists"
					}
					seen[path] = true
					if row.code == "" {
						for key, node := range pending {
							nodes[key] = node
							countKey := node.kind + "s"
							if node.kind == "shelf" {
								countKey = "shelves"
							}
							summary.Counts[countKey]++
						}
					}
				}
			}
			if row.code != "" && row.status != "conflict" {
				row.status = "invalid"
			}
			summary.Counts[row.status]++
			if row.status == "new" {
				summary.Valid++
			} else if row.status == "invalid" || row.status == "conflict" {
				summary.Invalid++
			}
			data, _ := json.Marshal(p)
			if _, err = tx.Exec(ctx, `INSERT INTO warehouse_location_import_rows(job_id,row_number,path,status,error_code,target) VALUES($1,$2,$3,$4,$5,$6)`, job.ID, row.number, data, row.status, nullable(row.code), nullable(row.target)); err != nil {
				return err
			}
			if len(summary.Rows) < 1000 {
				summary.Rows = append(summary.Rows, im.preview(row, job.Locale))
				if row.code != "" {
					summary.Errors = append(summary.Errors, im.rowError(row, job.Locale))
				}
			}
		}
		return nil
	})
	return summary, err
}
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func (im *LocationImporter) rowError(r stagedLocationRow, locale string) ioengine.RowError {
	return ioengine.RowError{Index: r.number, Code: r.code, Error: i18n.Translate(i18n.Normalize(locale), "warehouse_import.error."+r.code)}
}
func (im *LocationImporter) preview(r stagedLocationRow, locale string) ioengine.RowPreview {
	data, _ := json.Marshal(r.path)
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	return ioengine.RowPreview{Index: r.number, Data: m, Status: r.status, StatusLabel: i18n.Translate(i18n.Normalize(locale), "warehouse_import.status."+r.status), Target: map[string]any{"full_code": r.target, "create_count": r.createCount}}
}
func (im *LocationImporter) loadRows(ctx context.Context, tx pgx.Tx, jobID int64) ([]stagedLocationRow, error) {
	rows, err := tx.Query(ctx, `SELECT row_number,path,status,COALESCE(error_code,''),COALESCE(target,'') FROM warehouse_location_import_rows WHERE job_id=$1 ORDER BY row_number`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []stagedLocationRow{}
	for rows.Next() {
		var r stagedLocationRow
		var data []byte
		if err = rows.Scan(&r.number, &data, &r.status, &r.code, &r.target); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(data, &r.path); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (im *LocationImporter) summary(ctx context.Context, tx pgx.Tx, job ioengine.ImportJob) (ioengine.PreviewSummary, error) {
	rows, err := im.loadRows(ctx, tx, job.ID)
	if err != nil {
		return ioengine.PreviewSummary{}, err
	}
	s := ioengine.PreviewSummary{Total: len(rows), BatchUUID: job.UUID.String(), Counts: map[string]int{}}
	for _, r := range rows {
		s.Counts[r.status]++
		if r.status == "new" || r.status == "applied" {
			s.Valid++
		}
		if r.status == "invalid" || r.status == "conflict" {
			s.Invalid++
		}
		if len(s.Rows) < 1000 {
			s.Rows = append(s.Rows, im.preview(r, job.Locale))
		}
		if r.code != "" && len(s.Errors) < 1000 {
			s.Errors = append(s.Errors, im.rowError(r, job.Locale))
		}
	}
	return s, nil
}
func (im *LocationImporter) Apply(ctx context.Context, job ioengine.ImportJob) (ioengine.PreviewSummary, error) {
	var summary ioengine.PreviewSummary
	err := im.tx(ctx, func(tx pgx.Tx) error {
		var defaults map[string]any
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT defaults_json FROM import_jobs WHERE id=$1 FOR UPDATE`, job.ID).Scan(&raw); err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &defaults); err != nil {
			return err
		}
		wid, wcode, err := im.warehouse(ctx, tx, job, defaults, true)
		if err != nil {
			return err
		}
		rows, err := im.loadRows(ctx, tx, job.ID)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return fmt.Errorf("%w: preview missing", ioengine.ErrImportRejected)
		}
		nodes, err := im.existing(ctx, tx, wid, wcode)
		if err != nil {
			return err
		}
		for _, r := range rows {
			if r.status != "new" {
				continue
			}
			path := wcode
			var roomID, parentID int64
			for depth, part := range r.path.codes() {
				path += "-" + part.code
				if n, ok := nodes[path]; ok {
					if n.kind != part.kind {
						return fmt.Errorf("%w: path type changed", ioengine.ErrImportRejected)
					}
					if part.kind == "room" {
						roomID = n.id
					} else {
						parentID = n.id
					}
					continue
				}
				name := part.code
				if depth == 0 && r.path.RoomName != "" {
					name = r.path.RoomName
				}
				if depth == len(r.path.codes())-1 && r.path.Name != "" {
					name = r.path.Name
				}
				active := true
				if depth == len(r.path.codes())-1 {
					active = r.path.Active
				}
				var id int64
				if part.kind == "room" {
					err = tx.QueryRow(ctx, `INSERT INTO rooms(organization_id,warehouse_id,code,name,active,sort_order)
						VALUES($1,$2,$3,$4,$5,(SELECT COALESCE(MAX(sort_order)+1,0) FROM rooms WHERE warehouse_id=$2)) RETURNING id`, job.OrganizationID, wid, part.code, name, active).Scan(&id)
					roomID = id
				} else {
					var parent any
					if parentID != 0 {
						parent = parentID
					}
					err = tx.QueryRow(ctx, `INSERT INTO warehouse_locations(organization_id,warehouse_id,room_id,parent_id,type,code,name,active,sort_order)
						VALUES($1,$2,$3,$4,$5,$6,$7,$8,(SELECT COALESCE(MAX(sort_order)+1,0) FROM warehouse_locations WHERE room_id=$3 AND parent_id IS NOT DISTINCT FROM $4::bigint)) RETURNING id`, job.OrganizationID, wid, roomID, parent, part.kind, part.code, name, active).Scan(&id)
					parentID = id
				}
				if err != nil {
					return err
				}
				nodes[path] = importNode{id, part.kind}
				if _, err = tx.Exec(ctx, `INSERT INTO warehouse_location_import_nodes(job_id,kind,node_id,depth) VALUES($1,$2,$3,$4)`, job.ID, part.kind, id, depth); err != nil {
					return err
				}
			}
			if _, err = tx.Exec(ctx, `UPDATE warehouse_location_import_rows SET status='applied' WHERE job_id=$1 AND row_number=$2`, job.ID, r.number); err != nil {
				return err
			}
		}
		summary, err = im.summary(ctx, tx, job)
		return err
	})
	return summary, err
}
func (im *LocationImporter) Undo(ctx context.Context, job ioengine.ImportJob) (ioengine.PreviewSummary, error) {
	var summary ioengine.PreviewSummary
	err := im.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT id FROM import_jobs WHERE id=$1 FOR UPDATE`, job.ID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT kind,node_id FROM warehouse_location_import_nodes WHERE job_id=$1 AND NOT undone ORDER BY depth DESC,node_id DESC`, job.ID)
		if err != nil {
			return err
		}
		type node struct {
			kind string
			id   int64
		}
		var nodes []node
		for rows.Next() {
			var n node
			if err = rows.Scan(&n.kind, &n.id); err != nil {
				rows.Close()
				return err
			}
			nodes = append(nodes, n)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		rejected := false
		for _, n := range nodes {
			if _, err = tx.Exec(ctx, "SAVEPOINT location_import_node"); err != nil {
				return err
			}
			table := "warehouse_locations"
			if n.kind == "room" {
				table = "rooms"
			}
			_, err = tx.Exec(ctx, "DELETE FROM "+table+" WHERE id=$1", n.id)
			if err != nil {
				if _, rollbackErr := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT location_import_node"); rollbackErr != nil {
					return rollbackErr
				}
				var pgErr *pgconn.PgError
				if !errors.As(err, &pgErr) || (pgErr.Code != "23503" && pgErr.Code != "23001") {
					return err
				}
				rejected = true
			} else {
				if _, err = tx.Exec(ctx, `UPDATE warehouse_location_import_nodes SET undone=true WHERE job_id=$1 AND kind=$2 AND node_id=$3`, job.ID, n.kind, n.id); err != nil {
					return err
				}
			}
			if _, err = tx.Exec(ctx, "RELEASE SAVEPOINT location_import_node"); err != nil {
				return err
			}
		}
		status := "undone"
		if rejected {
			status = "undo_rejected"
		}
		if _, err = tx.Exec(ctx, `UPDATE warehouse_location_import_rows SET status=$2 WHERE job_id=$1 AND status='applied'`, job.ID, status); err != nil {
			return err
		}
		summary, err = im.summary(ctx, tx, job)
		return err
	})
	return summary, err
}
func (im *LocationImporter) ReportRows(ctx context.Context, job ioengine.ImportJob) ([]ioengine.RowReport, error) {
	var reports []ioengine.RowReport
	err := im.tx(ctx, func(tx pgx.Tx) error {
		rows, err := im.loadRows(ctx, tx, job.ID)
		if err != nil {
			return err
		}
		for _, r := range rows {
			rep := ioengine.RowReport{Index: r.number, Status: r.status, StatusLabel: i18n.Translate(i18n.Normalize(job.Locale), "warehouse_import.status."+r.status), Failed: r.code != "" || r.status == "undo_rejected"}
			if r.code != "" {
				rep.Errors = []string{im.rowError(r, job.Locale).Error}
			}
			reports = append(reports, rep)
		}
		return nil
	})
	return reports, err
}

var _ ioengine.StagedImporter = (*LocationImporter)(nil)
var _ ioengine.ResourceAdapter = (*LocationImporter)(nil)
var _ ioengine.ImportPermissioner = (*LocationImporter)(nil)
var _ ioengine.RowLimiter = (*LocationImporter)(nil)
var _ ioengine.RowReporter = (*LocationImporter)(nil)
