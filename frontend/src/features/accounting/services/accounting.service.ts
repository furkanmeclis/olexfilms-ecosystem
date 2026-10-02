import type { components } from "@/generated/api";
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
export type Currency = Schemas["Currency"];

export type AccountType = FinanceAccount["type"];
export const ACCOUNT_TYPES: AccountType[] = ["cash", "bank"];

export const ACCOUNTING_DIRECTIONS: AccountingDirection[] = [
  "income",
  "expense",
  "charge",
  "collection",
  "payment",
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

export type ListCariParams = {
  limit?: number;
  offset?: number;
  q?: string;
  active?: boolean;
};

export type ListEntriesParams = {
  limit?: number;
  offset?: number;
  cari_uuid?: string;
  account_uuid?: string;
  direction?: AccountingDirection;
  source_type?: string;
  /** YYYY-MM-DD, inclusive (UTC day of created_at). */
  date_from?: string;
  date_to?: string;
};

function boolQuery(value: boolean | undefined) {
  return value === undefined ? undefined : String(value);
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
      {
        query: {
          limit: params.limit,
          offset: params.offset,
          q: params.q,
          active: boolQuery(params.active),
        },
      },
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
      { query: { ...params } },
    );
  },

  /** Collection or payment; a repeated idempotency_key answers the same row. */
  settle(kind: SettlementKind, body: FinanceSettlementInput) {
    return platformRequest<FinanceEntry>(
      "POST",
      kind === "collection"
        ? "/v1/accounting/collections"
        : "/v1/accounting/payments",
      { body },
    );
  },
};
