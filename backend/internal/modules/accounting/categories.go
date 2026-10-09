// Package accounting holds the shared, code-defined parts of the accounting
// module (TEC-99). The ledger rows live in finance_entries (000047); the
// source API is accounting/posting, the HTTP surface accounting/handler.
package accounting

import "regexp"

// Ledger directions (finance_entries.direction, chk_finance_entries_direction).
const (
	DirectionIncome     = "income"
	DirectionExpense    = "expense"
	DirectionCharge     = "charge"
	DirectionCollection = "collection"
	DirectionPayment    = "payment"
	// DirectionOpening is a cash/bank opening balance (TEC-198, 000056):
	// account only, never income or expense.
	DirectionOpening = "opening"
)

// Category is one entry of the F1 category catalog. F1 keeps the catalog in
// code (no table, no migration): finance_entries.category stores Key, which
// must satisfy chk_finance_entries_category. LabelKey is resolved by the
// backend i18n catalog in all 13 languages.
type Category struct {
	Key       string `json:"key"`
	Direction string `json:"direction"`
	LabelKey  string `json:"label_key"`
	// Manual reports whether a person may pick it for a manual entry.
	// System categories (sale, purchase) are written by the source API only
	// (hierarchical sale bridge, K9).
	Manual bool `json:"manual"`
}

// System categories written by accounting/posting (keep in sync with
// posting.CategorySale / posting.CategoryPurchase).
const (
	CategorySale          = "sale"
	CategoryPurchase      = "purchase"
	CategoryServiceIncome = "service_income"
	CategoryStaffAdvance  = "staff_advance"
	CategoryStaffBonus    = "staff_bonus"
	CategoryCollection    = "collection"
	CategoryPayment       = "payment"
	// CategoryCariTransfer: keep in sync with posting.CategoryCariTransfer.
	CategoryCariTransfer = "cari_transfer"
	// Warranty claim accounting (TEC-337, source_type warranty_claim): the
	// center's product cost of a re-application, the labor the center pays
	// down the chain (warranty_labor) and the child's side of it
	// (warranty_labor_income). System only.
	CategoryWarrantyCost        = "warranty_cost"
	CategoryWarrantyLabor       = "warranty_labor"
	CategoryWarrantyLaborIncome = "warranty_labor_income"
	// Service subscription accounting (TEC-308): a posted period is the
	// center's service_sale income and the receiver's service_purchase
	// expense (source_type service_subscription_period); an approved early
	// cancellation books service_cancellation_fee on both sides (seller
	// income, receiver expense; source_type service_subscription_cancel).
	// System only.
	CategoryServiceSale            = "service_sale"
	CategoryServicePurchase        = "service_purchase"
	CategoryServiceCancellationFee = "service_cancellation_fee"
)

func cat(key, direction string, manual bool) Category {
	return Category{Key: key, Direction: direction, LabelKey: "accounting.category." + key, Manual: manual}
}

// categories is the F1 catalog, in display order per direction.
var categories = []Category{
	// Income.
	cat(CategorySale, DirectionIncome, false),
	cat(CategoryServiceIncome, DirectionIncome, true),
	cat("interest_income", DirectionIncome, true),
	cat("other_income", DirectionIncome, true),
	cat(CategoryWarrantyLaborIncome, DirectionIncome, false),
	cat(CategoryServiceSale, DirectionIncome, false),
	cat(CategoryServiceCancellationFee, DirectionIncome, false),
	// Expense.
	cat(CategoryPurchase, DirectionExpense, false),
	cat("rent", DirectionExpense, true),
	cat("salary", DirectionExpense, true),
	cat(CategoryStaffAdvance, DirectionExpense, false),
	cat(CategoryStaffBonus, DirectionExpense, false),
	cat("utilities", DirectionExpense, true),
	cat("tax", DirectionExpense, true),
	cat("shipping", DirectionExpense, true),
	cat("marketing", DirectionExpense, true),
	cat("bank_fee", DirectionExpense, true),
	cat("other_expense", DirectionExpense, true),
	cat(CategoryWarrantyCost, DirectionExpense, false),
	cat(CategoryWarrantyLabor, DirectionExpense, false),
	cat(CategoryServicePurchase, DirectionExpense, false),
	// Cari charge (non-P&L debit of the counterparty).
	cat("opening_balance", DirectionCharge, true),
	cat("adjustment", DirectionCharge, true),
	// K25 re-parenting: closes the old parent's cari (TEC-198, system only).
	cat(CategoryCariTransfer, DirectionCharge, false),
	// Settlements (cash/bank movement + cari closing, never income).
	cat(CategoryCollection, DirectionCollection, true),
	cat(CategoryPayment, DirectionPayment, true),
}

// Same pattern as chk_finance_entries_category (000047).
var categoryKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_.]{0,63}$`)

// Categories returns a copy of the catalog.
func Categories() []Category {
	out := make([]Category, len(categories))
	copy(out, categories)
	return out
}

// LookupCategory returns the catalog entry of key.
func LookupCategory(key string) (Category, bool) {
	for _, c := range categories {
		if c.Key == key {
			return c, true
		}
	}
	return Category{}, false
}

// ValidCategoryKey reports whether key satisfies the database CHECK.
func ValidCategoryKey(key string) bool { return categoryKeyRe.MatchString(key) }
