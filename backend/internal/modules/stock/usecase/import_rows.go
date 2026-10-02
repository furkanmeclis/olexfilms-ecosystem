package usecase

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/database/db"
	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/modules/stock/ledger"
)

// Stock import row statuses (stock_import_rows.row_status) and the report
// class of an applied row the undo refused.
const (
	ImportRowNew          = "new"
	ImportRowDuplicate    = "duplicate"
	ImportRowInvalid      = "invalid"
	ImportRowConflict     = "conflict"
	ImportRowApplied      = "applied"
	ImportRowUndone       = "undone"
	ImportRowUndoRejected = "undo_rejected"
)

// Stock import row error codes. The localized message is the i18n catalog
// key "stock_import.error.<lower-case code without the prefix>".
const (
	ImportErrBarcodeRequired   = "STOCK_IMPORT_BARCODE_REQUIRED"
	ImportErrBarcodeTooLong    = "STOCK_IMPORT_BARCODE_TOO_LONG"
	ImportErrSKURequired       = "STOCK_IMPORT_SKU_REQUIRED"
	ImportErrProductNotFound   = "STOCK_IMPORT_PRODUCT_NOT_FOUND"
	ImportErrProductInactive   = "STOCK_IMPORT_PRODUCT_INACTIVE"
	ImportErrQuantityInvalid   = "STOCK_IMPORT_QUANTITY_INVALID"
	ImportErrQuantityForbidden = "STOCK_IMPORT_QUANTITY_NOT_ALLOWED"
	ImportErrMetersInvalid     = "STOCK_IMPORT_METERS_INVALID"
	ImportErrMetersForbidden   = "STOCK_IMPORT_METERS_NOT_ALLOWED"
	ImportErrLocationNotFound  = "STOCK_IMPORT_LOCATION_NOT_FOUND"
	ImportErrDuplicateInFile   = "STOCK_IMPORT_DUPLICATE_IN_FILE"
	ImportErrAlreadyInStock    = "STOCK_IMPORT_ALREADY_IN_STOCK"
	ImportErrHeldByOtherOrg    = "STOCK_IMPORT_HELD_BY_OTHER_ORG"
	ImportErrProductMismatch   = "STOCK_IMPORT_PRODUCT_MISMATCH"
	ImportErrBarcodeTaken      = "STOCK_IMPORT_BARCODE_TAKEN"
	ImportErrUndoUnitTouched   = "STOCK_IMPORT_UNDO_UNIT_TOUCHED"
)

// ImportErrorCodes lists every row error code (catalog parity test).
var ImportErrorCodes = []string{
	ImportErrBarcodeRequired, ImportErrBarcodeTooLong, ImportErrSKURequired,
	ImportErrProductNotFound, ImportErrProductInactive, ImportErrQuantityInvalid,
	ImportErrQuantityForbidden, ImportErrMetersInvalid, ImportErrMetersForbidden,
	ImportErrLocationNotFound, ImportErrDuplicateInFile, ImportErrAlreadyInStock,
	ImportErrHeldByOtherOrg, ImportErrProductMismatch, ImportErrBarcodeTaken,
	ImportErrUndoUnitTouched,
}

// ImportRowStatuses lists every report class (catalog parity test).
var ImportRowStatuses = []string{
	ImportRowNew, ImportRowDuplicate, ImportRowInvalid, ImportRowConflict,
	ImportRowApplied, ImportRowUndone, ImportRowUndoRejected,
}

// ImportErrorKey is the i18n catalog key of a row error code.
func ImportErrorKey(code string) string {
	return "stock_import.error." + strings.ToLower(strings.TrimPrefix(code, "STOCK_IMPORT_"))
}

// ImportStatusKey is the i18n catalog key of a row status.
func ImportStatusKey(status string) string { return "stock_import.status." + status }

// importErrorField names the import column an error code is about.
var importErrorField = map[string]string{
	ImportErrBarcodeRequired: "barcode", ImportErrBarcodeTooLong: "barcode",
	ImportErrSKURequired: "product_sku", ImportErrProductNotFound: "product_sku",
	ImportErrProductInactive: "product_sku", ImportErrProductMismatch: "product_sku",
	ImportErrQuantityInvalid: "quantity", ImportErrQuantityForbidden: "quantity",
	ImportErrMetersInvalid: "meters", ImportErrMetersForbidden: "meters",
	ImportErrLocationNotFound: "location_code", ImportErrDuplicateInFile: "barcode",
	ImportErrAlreadyInStock: "barcode", ImportErrHeldByOtherOrg: "barcode",
	ImportErrBarcodeTaken: "barcode",
}

// VARCHAR limits of stock_import_rows / units / warehouse_locations.
const (
	maxBarcodeLen  = 64
	maxSKULen      = 64
	maxLocationLen = 64
	// maxCentimeters is NUMERIC(10,2) (99 999 999.99 m).
	maxCentimeters = 9_999_999_999
	maxQuantity    = 1_000_000
)

// importRow is one parsed import row before the database checks.
type importRow struct {
	barcode  string
	sku      string
	location string
	quantity *int32
	cm       *int64 // meters in centimeters
	errs     []string
	qtyRaw   string
	mRaw     string
}

// parseImportRow reads the mapped cells (defaults fill empty cells) and
// collects the format errors.
func parseImportRow(row, defaults map[string]any) importRow {
	get := func(k string) string {
		if v := importCell(row, k); v != "" {
			return v
		}
		return importCell(defaults, k)
	}
	r := importRow{barcode: get("barcode"), sku: get("product_sku"), location: get("location_code"),
		qtyRaw: get("quantity"), mRaw: get("meters")}
	switch {
	case r.barcode == "":
		r.errs = append(r.errs, ImportErrBarcodeRequired)
	case utf8.RuneCountInString(r.barcode) > maxBarcodeLen:
		r.errs = append(r.errs, ImportErrBarcodeTooLong)
	}
	if r.sku == "" {
		r.errs = append(r.errs, ImportErrSKURequired)
	}
	if utf8.RuneCountInString(r.location) > maxLocationLen {
		r.errs = append(r.errs, ImportErrLocationNotFound)
		r.location = ""
	}
	if r.qtyRaw != "" {
		n, err := strconv.ParseInt(strings.TrimSuffix(r.qtyRaw, ".0"), 10, 32)
		if err != nil || n <= 0 || n > maxQuantity {
			r.errs = append(r.errs, ImportErrQuantityInvalid)
		} else {
			q := int32(n)
			r.quantity = &q
		}
	}
	if r.mRaw != "" {
		cm, err := ledger.ParseMeters(strings.ReplaceAll(r.mRaw, ",", "."))
		if err != nil || cm <= 0 || cm > maxCentimeters {
			r.errs = append(r.errs, ImportErrMetersInvalid)
		} else {
			r.cm = &cm
		}
	}
	return r
}

// checkImportShape checks the quantity/meters cells against the product:
// a fixed barcode needs a quantity, a roll needs its meters, a piece is
// one unit per barcode.
func checkImportShape(p db.Product, r importRow) []string {
	var errs []string
	fixed := p.UsesFixedBarcode
	roll := p.UnitType == "roll_meter"
	switch {
	case fixed:
		if r.quantity == nil && r.qtyRaw == "" {
			errs = append(errs, ImportErrQuantityInvalid)
		}
	case r.quantity != nil && *r.quantity != 1:
		errs = append(errs, ImportErrQuantityForbidden)
	}
	switch {
	case roll && !fixed:
		if r.cm == nil && r.mRaw == "" {
			errs = append(errs, ImportErrMetersInvalid)
		}
	case r.mRaw != "":
		errs = append(errs, ImportErrMetersForbidden)
	}
	return errs
}

func importCell(m map[string]any, k string) string {
	if m == nil {
		return ""
	}
	v, ok := m[k]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case float64:
		return strings.TrimSpace(strconv.FormatFloat(t, 'f', -1, 64))
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

// truncate cuts s to n runes (staging VARCHAR columns; raw keeps the cell).
func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
