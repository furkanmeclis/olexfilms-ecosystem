import { describe, expect, it } from "vitest";

import {
  accountCreateInput,
  accountFormSchema,
  accountUpdateInput,
  isValidIban,
  normalizeAmount,
  settlementFormSchema,
  settlementInput,
} from "@/features/accounting/lib/form";

const t = (key: string) => key;

describe("normalizeAmount", () => {
  it.each([
    ["1500", "1500.00"],
    ["1500.5", "1500.50"],
    ["1500,5", "1500.50"],
    ["1.500,50", "1500.50"],
    ["1,500.50", "1500.50"],
    ["1 500,50", "1500.50"],
    ["1.500", "1500.00"],
    ["0,01", "0.01"],
  ])("%s → %s", (raw, want) => {
    expect(normalizeAmount(raw)).toBe(want);
  });

  it("reads three trailing digits as grouping", () => {
    expect(normalizeAmount("1,234")).toBe("1234.00");
  });

  it.each(["", "0", "0,00", "-5", "abc", "1.5e3"])("rejects %s", (raw) => {
    expect(normalizeAmount(raw)).toBeNull();
  });
});

describe("isValidIban", () => {
  it("accepts a valid IBAN with spaces and lowercase", () => {
    expect(isValidIban("TR33 0006 1005 1978 6457 8413 26")).toBe(true);
    expect(isValidIban("de89370400440532013000")).toBe(true);
  });

  it("rejects a wrong checksum or shape", () => {
    expect(isValidIban("TR33 0006 1005 1978 6457 8413 27")).toBe(false);
    expect(isValidIban("TR33")).toBe(false);
  });
});

describe("account form", () => {
  it("requires a name and checks the IBAN of a bank account", () => {
    const schema = accountFormSchema(t);
    const empty = schema.safeParse({
      type: "cash",
      name: " ",
      iban: "",
      active: true,
    });
    expect(empty.success).toBe(false);
    const badIban = schema.safeParse({
      type: "bank",
      name: "Ziraat",
      iban: "TR00 1234",
      active: true,
    });
    expect(badIban.success).toBe(false);
    expect(badIban.error?.issues[0]?.path).toEqual(["iban"]);
    // A cash account ignores whatever is in the IBAN field.
    expect(
      schema.safeParse({ type: "cash", name: "Kasa", iban: "x", active: true })
        .success,
    ).toBe(true);
  });

  it("builds create and update bodies", () => {
    expect(
      accountCreateInput({
        type: "bank",
        name: " Ziraat ",
        iban: "tr33 0006 1005 1978 6457 8413 26",
        active: true,
      }),
    ).toEqual({
      type: "bank",
      name: "Ziraat",
      iban: "TR330006100519786457841326",
    });
    expect(
      accountUpdateInput({ type: "bank", name: "Z", iban: "", active: false }),
    ).toEqual({ name: "Z", iban: null, active: false });
    expect(
      accountCreateInput({
        type: "cash",
        name: "Kasa",
        iban: "x",
        active: true,
      }),
    ).toEqual({ type: "cash", name: "Kasa", iban: null });
  });
});

describe("settlement form", () => {
  const valid = {
    account_uuid: "a1",
    cari_uuid: "c1",
    amount: "1.250,5",
    currency: "try",
    description: "  makbuz 12 ",
  };

  it("needs an account, a cari, a positive amount and a currency", () => {
    const res = settlementFormSchema(t).safeParse({
      account_uuid: "",
      cari_uuid: "",
      amount: "0",
      currency: "",
      description: "",
    });
    expect(res.success).toBe(false);
    const fields = new Set(res.error?.issues.map((i) => i.path[0]));
    expect(fields).toEqual(
      new Set(["account_uuid", "cari_uuid", "amount", "currency"]),
    );
  });

  it("normalizes the body and carries the idempotency key", () => {
    expect(settlementFormSchema(t).safeParse(valid).success).toBe(true);
    expect(settlementInput(valid, "k1")).toEqual({
      account_uuid: "a1",
      cari_uuid: "c1",
      amount: "1250.50",
      currency: "TRY",
      description: "makbuz 12",
      idempotency_key: "k1",
    });
  });
});
