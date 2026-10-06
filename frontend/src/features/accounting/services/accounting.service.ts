import type { components } from "@/generated/api";
import {
  platformDownloadFile,
  triggerBrowserDownload,
} from "@/lib/api/platform-form-request";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type AccountingDirection = Schemas["AccountingDirection"];
export type AccountingCategory = Schemas["AccountingCategory"];
export type FinanceAccount = Schemas["FinanceAccount"];
export type FinanceAccountCreateInput = Schemas["FinanceAccountCreateInput"];
export type FinanceAccountUpdateInput = Schemas["FinanceAccountUpdateInput"];
export type CariAccount = Schemas["CariAccount"];
export type FinanceEntry = Schemas["FinanceEntry"];
export type FinanceSettlementInput = Schemas["FinanceSettlementInput"];
export type FinanceAccountOpeningInput = Schemas["FinanceAccountOpeningInput"];
export type Currency = Schemas["Currency"];
export type CariStatement = Schemas["CariStatement"];
export type CariStatementLine = Schemas["CariStatementLine"];
export type AccountingDispute = Schemas["AccountingDispute"];
export type AccountingDisputeStatus = Schemas["AccountingDisputeStatus"];
export type AccountingDisputeInput = Schemas["AccountingDisputeInput"];
export type AccountingDisputeResolveInput =
  Schemas["AccountingDisputeResolveInput"];
export type AccountingExportJob = Schemas["ExportJob"];
export type AccountingExportFormat = Schemas["AccountingExportInput"]["format"];

export const DISPUTE_STATUSES: AccountingDisputeStatus[] = [
  "open",
  "resolved_reversal",
  "resolved_revision",
  "rejected",
];

export const STATEMENT_EXPORT_FORMATS: AccountingExportFormat[] = [
  "pdf",
  "xlsx",
  "csv",
];

export type AccountType = FinanceAccount["type"];
export const ACCOUNT_TYPES: AccountType[] = ["cash", "bank"];

export const ACCOUNTING_DIRECTIONS: AccountingDirection[] = [
  "income",
  "expense",
  "charge",
  "collection",
  "payment",
  "opening",
];

/** Settlement directions (POST /v1/accounting/collections | payments). */
export type SettlementKind = "collection" | "payment";

/** Ledger source types the backend writes (accounting.source.* labels). */
export const ENTRY_SOURCE_TYPES = [
  "manual",
  "order",
  "service",
  "transfer",
] as const;

export type AccountingPage<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

/** List query params (limit, offset, sort, q and the mapped filters). */
type ListQuery = Record<string, string | number | boolean | undefined>;

/**
 * GET /v1/accounting/cari (TEC-379): sort name (default), balance,
 * entry_count, last_entry_at, created_at; q; active; CSV
 * counterparty_kind (center, distributor, dealer, customer);
 * balance_min / balance_max.
 */
export type ListCariParams = ListQuery & {
  limit?: number;
  offset?: number;
  q?: string;
  active?: boolean;
};

/**
 * GET /v1/accounting/entries (TEC-379): sort created_at (default
 * -created_at), amount, direction, category; q; account_uuid, cari_uuid;
 * CSV direction / category / source_type; created_from / created_to
 * (YYYY-MM-DD, inclusive) and amount_min / amount_max.
 */
export type ListEntriesParams = ListQuery & {
  limit?: number;
  offset?: number;
  cari_uuid?: string;
  account_uuid?: string;
};

/** Cari counterparty kinds of the `counterparty_kind` filter. */
export const CARI_COUNTERPARTY_KINDS = [
  "center",
  "distributor",
  "dealer",
  "customer",
] as const;

export type StatementPeriod = {
  /** YYYY-MM-DD, inclusive (UTC calendar day). */
  from?: string;
  to?: string;
};

/**
 * GET /v1/accounting/disputes (TEC-379): sort created_at (default
 * -created_at), resolved_at, status, amount, organization; q; CSV status /
 * organization_uuid / counterparty_organization_uuid; created_from /
 * created_to.
 */
export type ListDisputesParams = ListQuery & {
  limit?: number;
  offset?: number;
  status?: string;
};

function boolQuery(value: boolean | undefined) {
  return value === undefined ? undefined : String(value);
}

/** List params → query (booleans as "true" / "false"). */
function listQuery(params: ListQuery) {
  const out: Record<string, string | number | undefined> = {};
  for (const [key, value] of Object.entries(params)) {
    out[key] = typeof value === "boolean" ? String(value) : value;
  }
  return out;
}

const path = (base: string, uuid: string) =>
  `${base}/${encodeURIComponent(uuid)}`;

/**
 * Tenant accounting API (TEC-172/173). Every call works on the active
 * organization's book: the backend resolves the organization from the
 * session context and limits reads to it and the organizations below it
 * (accounting.read scope), so the UI never sends another book's uuid.
 */
export const accountingService = {
  listCategories() {
    return platformRequest<{ items: AccountingCategory[] }>(
      "GET",
      "/v1/accounting/categories",
    );
  },

  listCurrencies() {
    return platformRequest<{ items: Currency[] }>("GET", "/v1/currencies");
  },

  listAccounts(params: { active?: boolean } = {}) {
    return platformRequest<{ items: FinanceAccount[] }>(
      "GET",
      "/v1/accounting/accounts",
      { query: { active: boolQuery(params.active) } },
    );
  },

  createAccount(body: FinanceAccountCreateInput) {
    return platformRequest<FinanceAccount>("POST", "/v1/accounting/accounts", {
      body,
    });
  },

  updateAccount(uuid: string, body: FinanceAccountUpdateInput) {
    return platformRequest<FinanceAccount>(
      "PATCH",
      path("/v1/accounting/accounts", uuid),
      { body },
    );
  },

  listCari(params: ListCariParams = {}) {
    return platformRequest<AccountingPage<CariAccount>>(
      "GET",
      "/v1/accounting/cari",
      { query: listQuery(params) },
    );
  },

  /**
   * Queues a ledger list export (TEC-379, 202): the list filters, q and
   * sort; polled and downloaded through /v1/accounting/exports/{uuid}.
   */
  exportEntries(
    format: AccountingExportFormat,
    query: Record<string, string>,
    locale?: string,
  ) {
    return platformRequest<AccountingExportJob>(
      "POST",
      "/v1/accounting/entries/export",
      { body: { format, query, ...(locale ? { locale } : {}) } },
    );
  },

  getCari(uuid: string) {
    return platformRequest<CariAccount>(
      "GET",
      path("/v1/accounting/cari", uuid),
    );
  },

  listEntries(params: ListEntriesParams = {}) {
    return platformRequest<AccountingPage<FinanceEntry>>(
      "GET",
      "/v1/accounting/entries",
      { query: listQuery(params) },
    );
  },

  /** Collection or payment; a repeated idempotency_key answers the same row. */
  /**
   * One-off cash/bank opening balance (TEC-198, step-up): moves the
   * account balance only, never income/expense. 409 OPENING_BALANCE_EXISTS
   * while one is open.
   */
  createAccountOpening(uuid: string, body: FinanceAccountOpeningInput) {
    return platformRequest<FinanceEntry>(
      "POST",
      `/v1/accounting/accounts/${encodeURIComponent(uuid)}/opening-balance`,
      { body },
    );
  },

  settle(kind: SettlementKind, body: FinanceSettlementInput) {
    return platformRequest<FinanceEntry>(
      "POST",
      kind === "collection"
        ? "/v1/accounting/collections"
        : "/v1/accounting/payments",
      { body },
    );
  },

  /** Cari statement (TEC-175): opening, rows with running balance, closing. */
  getStatement(cariUuid: string, period: StatementPeriod = {}) {
    return platformRequest<CariStatement>(
      "GET",
      `${path("/v1/accounting/cari", cariUuid)}/statement`,
      { query: { from: period.from || undefined, to: period.to || undefined } },
    );
  },

  /** Queues a statement export job on worker-docs (202). */
  exportStatement(
    cariUuid: string,
    format: AccountingExportFormat,
    period: StatementPeriod = {},
    locale?: string,
  ) {
    return platformRequest<AccountingExportJob>(
      "POST",
      `${path("/v1/accounting/cari", cariUuid)}/statement/export`,
      {
        body: {
          format,
          ...(period.from ? { from: period.from } : {}),
          ...(period.to ? { to: period.to } : {}),
          ...(locale ? { locale } : {}),
        },
      },
    );
  },

  getExport(uuid: string) {
    return platformRequest<AccountingExportJob>(
      "GET",
      path("/v1/accounting/exports", uuid),
    );
  },

  async downloadExport(job: AccountingExportJob) {
    const { blob, filename } = await platformDownloadFile(
      `${path("/v1/accounting/exports", job.uuid)}/download`,
    );
    const ext = job.format === "xlsx" ? "xlsx" : job.format;
    triggerBrowserDownload(
      blob,
      filename ?? `statement-${job.created_at.slice(0, 10)}.${ext}`,
    );
  },

  listDisputes(params: ListDisputesParams = {}) {
    return platformRequest<AccountingPage<AccountingDispute>>(
      "GET",
      "/v1/accounting/disputes",
      { query: listQuery(params) },
    );
  },

  getDispute(uuid: string) {
    return platformRequest<AccountingDispute>(
      "GET",
      path("/v1/accounting/disputes", uuid),
    );
  },

  /** Child organization disputes a row its parent posted (K24). */
  openDispute(body: AccountingDisputeInput) {
    return platformRequest<AccountingDispute>(
      "POST",
      "/v1/accounting/disputes",
      { body },
    );
  },

  /** The parent the row came from: reversal, revision or reject. */
  resolveDispute(uuid: string, body: AccountingDisputeResolveInput) {
    return platformRequest<AccountingDispute>(
      "POST",
      `${path("/v1/accounting/disputes", uuid)}/resolve`,
      { body },
    );
  },
};
