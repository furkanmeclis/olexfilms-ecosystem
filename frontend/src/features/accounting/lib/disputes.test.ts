import { describe, expect, it } from "vitest";

import {
  canResolveDispute,
  isEntryDisputable,
  isStatementLineDisputable,
  resolveFormSchema,
  resolveInput,
  type ResolveFormValues,
} from "@/features/accounting/lib/disputes";
import type {
  AccountingDispute,
  CariAccount,
  CariStatementLine,
  FinanceEntry,
} from "@/features/accounting/services/accounting.service";

const t = (key: string) => key;
const PARENT = "parent-org";

const entry = (over: Partial<FinanceEntry> = {}): FinanceEntry => ({
  uuid: "e1",
  direction: "charge",
  category: "order",
  category_label_key: "accounting.category.order",
  account: null,
  cari_uuid: "c1",
  counterparty_organization: { uuid: PARENT, name: "Distributor" },
  orig_currency: "TRY",
  orig_amount: "100.00",
  currency: "TRY",
  amount: "100.00",
  rate: "1.00000000",
  rate_date: "2026-10-01",
  source_type: "order",
  source_uuid: "o1",
  revision: 1,
  reversal_of_uuid: null,
  reversed_by_uuid: null,
  voided: false,
  description: null,
  created_at: "2026-10-01T10:00:00Z",
  ...over,
});

const cari = (over: Partial<CariAccount> = {}): CariAccount => ({
  uuid: "c1",
  counterparty: {
    type: "organization",
    uuid: PARENT,
    name: "Distributor",
    org_type: "distributor",
  },
  currency: "TRY",
  active: true,
  balance: "-100.00",
  entry_count: 1,
  last_entry_at: null,
  created_at: "2026-10-01T00:00:00Z",
  ...over,
});

const line = (over: Partial<CariStatementLine> = {}): CariStatementLine => ({
  uuid: "e1",
  date: "2026-10-01T10:00:00Z",
  direction: "charge",
  category: "order",
  category_label: "Order",
  description: "Order",
  source_type: "order",
  source_uuid: "o1",
  source_label: "Order",
  reversal_of_uuid: null,
  reversed: false,
  orig_currency: "TRY",
  orig_amount: "100.00",
  debit: "0.00",
  credit: "100.00",
  balance: "-100.00",
  ...over,
});

const dispute = (over: Partial<AccountingDispute> = {}): AccountingDispute => ({
  uuid: "d1",
  status: "open",
  organization: { uuid: "dealer", name: "Dealer" },
  counterparty_organization: { uuid: PARENT, name: "Distributor" },
  entry: {
    uuid: "e1",
    direction: "charge",
    category: "order",
    category_label_key: "accounting.category.order",
    orig_currency: "EUR",
    orig_amount: "100.00",
    currency: "TRY",
    amount: "3500.00",
    revision: 1,
    created_at: "2026-10-01T10:00:00Z",
  },
  source_type: "order",
  source_uuid: "o1",
  reason: "Wrong amount",
  corrected_amount: null,
  resolution_note: null,
  reversal_entry_uuid: null,
  revision_entry_uuid: null,
  created_at: "2026-10-02T10:00:00Z",
  resolved_at: null,
  ...over,
});

describe("isEntryDisputable (TEC-174 rule)", () => {
  it("accepts an open sourced row the parent posted", () => {
    expect(isEntryDisputable(entry(), PARENT)).toBe(true);
  });

  it("rejects manual, unsourced, reversal and reversed rows", () => {
    expect(isEntryDisputable(entry({ source_type: "manual" }), PARENT)).toBe(
      false,
    );
    expect(isEntryDisputable(entry({ source_type: null }), PARENT)).toBe(false);
    expect(isEntryDisputable(entry({ source_uuid: null }), PARENT)).toBe(false);
    expect(isEntryDisputable(entry({ reversal_of_uuid: "e0" }), PARENT)).toBe(
      false,
    );
    expect(
      isEntryDisputable(
        entry({ voided: true, reversed_by_uuid: "e2" }),
        PARENT,
      ),
    ).toBe(false);
  });

  it("rejects a row of another counterparty and the center (no parent)", () => {
    expect(
      isEntryDisputable(
        entry({ counterparty_organization: { uuid: "dealer-b", name: "B" } }),
        PARENT,
      ),
    ).toBe(false);
    expect(isEntryDisputable(entry({ cari_uuid: null }), PARENT)).toBe(false);
    expect(isEntryDisputable(entry(), null)).toBe(false);
  });
});

describe("isStatementLineDisputable", () => {
  it("accepts an open sourced row of the parent's cari", () => {
    expect(isStatementLineDisputable(line(), cari(), PARENT)).toBe(true);
  });

  it("rejects rows of another cari or a customer cari", () => {
    expect(
      isStatementLineDisputable(
        line(),
        cari({
          counterparty: { type: "organization", uuid: "x", name: "X" },
        }),
        PARENT,
      ),
    ).toBe(false);
    expect(
      isStatementLineDisputable(
        line(),
        cari({ counterparty: { type: "user", uuid: PARENT, name: "U" } }),
        PARENT,
      ),
    ).toBe(false);
  });

  it("rejects manual, reversal and reversed rows", () => {
    expect(
      isStatementLineDisputable(
        line({ source_type: "manual" }),
        cari(),
        PARENT,
      ),
    ).toBe(false);
    expect(
      isStatementLineDisputable(
        line({ reversal_of_uuid: "e0" }),
        cari(),
        PARENT,
      ),
    ).toBe(false);
    expect(
      isStatementLineDisputable(line({ reversed: true }), cari(), PARENT),
    ).toBe(false);
  });
});

describe("canResolveDispute", () => {
  it("only the addressed parent resolves an open dispute", () => {
    expect(canResolveDispute(dispute(), PARENT, true)).toBe(true);
    expect(canResolveDispute(dispute(), PARENT, false)).toBe(false);
    expect(canResolveDispute(dispute(), "other-dist", true)).toBe(false);
    expect(canResolveDispute(dispute(), "dealer", true)).toBe(false);
    expect(
      canResolveDispute(dispute({ status: "rejected" }), PARENT, true),
    ).toBe(false);
  });
});

describe("resolve form", () => {
  const values = (over: Partial<ResolveFormValues>): ResolveFormValues => ({
    resolution: "reversal",
    corrected_amount: "",
    note: "",
    ...over,
  });

  it("reversal needs nothing else", () => {
    expect(resolveFormSchema(t).safeParse(values({})).success).toBe(true);
    expect(resolveInput(values({}))).toEqual({ resolution: "reversal" });
  });

  it("revision needs a positive amount and normalizes it", () => {
    const empty = resolveFormSchema(t).safeParse(
      values({ resolution: "revision" }),
    );
    expect(empty.success).toBe(false);
    const bad = resolveFormSchema(t).safeParse(
      values({ resolution: "revision", corrected_amount: "-5" }),
    );
    expect(bad.success).toBe(false);
    expect(
      resolveInput(
        values({
          resolution: "revision",
          corrected_amount: "1.250,5",
          note: " ok ",
        }),
      ),
    ).toEqual({
      resolution: "revision",
      corrected_amount: "1250.50",
      note: "ok",
    });
  });

  it("reject needs a note", () => {
    const parsed = resolveFormSchema(t).safeParse(
      values({ resolution: "reject" }),
    );
    expect(parsed.success).toBe(false);
    expect(
      resolveInput(
        values({ resolution: "reject", corrected_amount: "10", note: "No" }),
      ),
    ).toEqual({ resolution: "reject", note: "No" });
  });
});
