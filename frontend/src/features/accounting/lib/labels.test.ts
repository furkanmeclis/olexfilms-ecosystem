import fs from "node:fs";
import path from "node:path";

import { describe, expect, it } from "vitest";

import {
  ACCOUNT_TYPES,
  ACCOUNTING_DIRECTIONS,
  CARI_COUNTERPARTY_KINDS,
  DISPUTE_STATUSES,
  ENTRY_SOURCE_TYPES,
  type AccountingExportJob,
  type CariAccount,
} from "@/features/accounting/services/accounting.service";

const LOCALES = path.resolve(__dirname, "../../../locales");

type OrgType = NonNullable<CariAccount["counterparty"]["org_type"]>;

/**
 * Every counterparty organization type the API can return (TEC-869: fleet
 * leaked as a raw key). A new OpenAPI org_type fails typecheck here until
 * it is listed, and the test below then demands its label.
 */
const ORG_TYPES: Record<OrgType, true> = {
  center: true,
  distributor: true,
  dealer: true,
  fleet: true,
};

const EXPORT_STATUSES: Record<AccountingExportJob["status"], true> = {
  queued: true,
  processing: true,
  completed: true,
  failed: true,
  expired: true,
};

describe("accounting enum labels", () => {
  it("lists every organization type as a cari kind", () => {
    expect(
      Object.keys(ORG_TYPES).filter(
        (k) => !(CARI_COUNTERPARTY_KINDS as readonly string[]).includes(k),
      ),
    ).toEqual([]);
  });

  // Keys rendered as t(`accounting.<group>.${value}`).
  const expected = [
    ...CARI_COUNTERPARTY_KINDS.map((k) => `counterparty_types.${k}`),
    ...Object.keys(ORG_TYPES).map((k) => `counterparty_types.${k}`),
    // counterpartyKind() fallback when org_type is missing.
    "counterparty_types.organization",
    ...ENTRY_SOURCE_TYPES.map((s) => `sources.${s}`),
    ...ACCOUNTING_DIRECTIONS.map((d) => `directions.${d}`),
    ...ACCOUNT_TYPES.map((a) => `account_types.${a}`),
    ...DISPUTE_STATUSES.map((s) => `disputes.statuses.${s}`),
    ...DISPUTE_STATUSES.map((s) => `disputes.resolved.${s}`),
    ...Object.keys(EXPORT_STATUSES).map((s) => `statement.export_status.${s}`),
  ];

  for (const locale of fs.readdirSync(LOCALES)) {
    const file = path.join(LOCALES, locale, "accounting.json");
    if (!fs.existsSync(file)) continue;
    it(`${locale} has every enum label`, () => {
      const messages = JSON.parse(fs.readFileSync(file, "utf8")) as Record<
        string,
        string
      >;
      expect(expected.filter((k) => !messages[k])).toEqual([]);
    });
  }
});
