import { describe, expect, it } from "vitest";

import {
  AGING_BUCKETS,
  agingChartRows,
} from "@/features/staff-reports/lib/reports";
import {
  canCancelPayment,
  isFutureDay,
  isSalaryConflict,
  paymentFormValues,
  paymentStatusTone,
  paymentInput,
  payrollAlreadyRan,
  payrollPreview,
  periodOf,
  staffCreateInput,
  staffPatchInput,
  validatePaymentForm,
} from "@/features/staff-reports/lib/staff";
import type { StaffProfile } from "@/features/staff-reports/services/staff-reports.service";
import { ApiError } from "@/lib/api";
import en from "@/locales/en/staff_reports.json";
import tr from "@/locales/tr/staff_reports.json";

const t = (key: string) => key;

function staffCard(over: Partial<StaffProfile> = {}): StaffProfile {
  return {
    uuid: "s-1",
    user_uuid: null,
    name: "Ali Usta",
    title: "Usta",
    hired_on: "2025-01-01",
    monthly_salary: "30000.00",
    currency: "TRY",
    active: true,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...over,
  };
}

describe("payrollPreview (TEC-349)", () => {
  it("lists only active staff; salaried ones are paid, the rest skipped", () => {
    const preview = payrollPreview([
      staffCard({ uuid: "a", monthly_salary: "30000.00" }),
      staffCard({ uuid: "b", monthly_salary: "12500.50" }),
      staffCard({ uuid: "off", active: false, monthly_salary: "99999.00" }),
      staffCard({ uuid: "none", monthly_salary: null }),
    ]);
    expect(preview.payable.map((s) => s.uuid)).toEqual(["a", "b"]);
    expect(preview.noSalary.map((s) => s.uuid)).toEqual(["none"]);
    expect(preview.total).toBe(42500.5);
    expect(preview.currency).toBe("TRY");
    const listed = [...preview.payable, ...preview.noSalary].map((s) => s.uuid);
    expect(listed).not.toContain("off");
  });

  it("an all-skipped run of a salaried period means it already ran", () => {
    expect(
      payrollAlreadyRan({
        period: "2026-10",
        created: 0,
        skipped: 3,
        items: [],
      }),
    ).toBe(true);
    expect(
      payrollAlreadyRan({
        period: "2026-10",
        created: 2,
        skipped: 1,
        items: [],
      }),
    ).toBe(false);
  });

  it("recognises the 409 salary conflict", () => {
    const conflict = new ApiError({
      status: 409,
      code: "STAFF_SALARY_EXISTS",
      message: "Salary already exists for this staff period",
    });
    expect(isSalaryConflict(conflict)).toBe(true);
    expect(
      isSalaryConflict(new ApiError({ status: 400, code: "X", message: "x" })),
    ).toBe(false);
    expect(isSalaryConflict(new Error("x"))).toBe(false);
  });
});

describe("staff and payment forms", () => {
  it("formats the period of a day", () => {
    expect(periodOf(new Date(2026, 0, 31))).toBe("2026-01");
  });

  it("salary may use the card salary; advance and bonus need an amount", () => {
    const base = { ...paymentFormValues("salary"), account_uuid: "acc-1" };
    expect(validatePaymentForm(base, staffCard(), t)).toEqual({});
    expect(
      validatePaymentForm(base, staffCard({ monthly_salary: null }), t).amount,
    ).toBeDefined();
    expect(
      validatePaymentForm({ ...base, type: "advance" }, staffCard(), t).amount,
    ).toBeDefined();
    expect(
      validatePaymentForm({ ...base, account_uuid: "" }, staffCard(), t)
        .account_uuid,
    ).toBeDefined();
    expect(
      validatePaymentForm({ ...base, period: "2026-13" }, staffCard(), t)
        .period,
    ).toBeDefined();
  });

  it("builds the payment body (locale amounts normalised)", () => {
    expect(
      paymentInput({
        type: "bonus",
        period: "2026-10",
        amount: "1.500,50",
        account_uuid: "acc-1",
        paid_on: "2026-10-31",
        description: "  ",
        target_note: "IBAN",
      }),
    ).toEqual({
      type: "bonus",
      period: "2026-10",
      amount: "1500.50",
      account_uuid: "acc-1",
      paid_on: "2026-10-31",
      description: null,
      target_note: "IBAN",
    });
  });

  it("clears optional card fields with null on PATCH, omits them on create", () => {
    const values = {
      name: " Ayşe ",
      title: "",
      hired_on: "",
      monthly_salary: "",
      active: false,
    };
    expect(staffPatchInput(values)).toEqual({
      name: "Ayşe",
      title: null,
      hired_on: null,
      monthly_salary: null,
      active: false,
    });
    expect(staffCreateInput(values)).toEqual({
      name: "Ayşe",
      title: null,
      active: false,
    });
  });
});

describe("cari aging buckets", () => {
  const value = (dict: Record<string, string>, key: string) =>
    dict[key.replace(/^staff_reports\./, "")];

  it("keeps the API bucket order with the right headers", () => {
    expect(AGING_BUCKETS.map((b) => b.key)).toEqual([
      "days_0_30",
      "days_31_60",
      "days_61_90",
      "days_90_plus",
    ]);
    expect(AGING_BUCKETS.map((b) => value(en, b.labelKey))).toEqual([
      "0–30 days",
      "31–60 days",
      "61–90 days",
      "90+ days",
    ]);
    expect(AGING_BUCKETS.map((b) => value(tr, b.labelKey))).toEqual([
      "0–30 gün",
      "31–60 gün",
      "61–90 gün",
      "90+ gün",
    ]);
  });

  it("charts receivable and payable (as a positive bar) per bucket", () => {
    const buckets = (a: string, b: string, c: string, d: string) => ({
      days_0_30: a,
      days_31_60: b,
      days_61_90: c,
      days_90_plus: d,
    });
    const rows = agingChartRows(
      {
        totals: {
          receivable: {
            balance: "100.00",
            buckets: buckets("10", "20", "30", "40"),
          },
          payable: {
            balance: "-6.00",
            buckets: buckets("-1", "-2", "-3", "0"),
          },
        },
      },
      (key) => key,
    );
    expect(rows).toEqual([
      { label: "days_0_30", receivable: 10, payable: 1 },
      { label: "days_31_60", receivable: 20, payable: 2 },
      { label: "days_61_90", receivable: 30, payable: 3 },
      { label: "days_90_plus", receivable: 40, payable: 0 },
    ]);
  });
});

describe("planned payments (TEC-381)", () => {
  it("only a planned payment can be cancelled", () => {
    expect(canCancelPayment({ status: "planned" })).toBe(true);
    expect(canCancelPayment({ status: "posted" })).toBe(false);
    expect(canCancelPayment({ status: "cancelled" })).toBe(false);
  });

  it("status tones tell planned from posted", () => {
    expect(paymentStatusTone("planned")).toBe("warning");
    expect(paymentStatusTone("posted")).toBe("success");
    expect(paymentStatusTone("cancelled")).toBe("default");
  });

  it("a day after today is a future (planned) day", () => {
    expect(isFutureDay("2026-10-08", "2026-10-07")).toBe(true);
    expect(isFutureDay("2026-10-07", "2026-10-07")).toBe(false);
    expect(isFutureDay("2026-09-30", "2026-10-07")).toBe(false);
    expect(isFutureDay("", "2026-10-07")).toBe(false);
  });

  it("every payment state has a label in tr and en", () => {
    for (const s of ["planned", "posted", "cancelled"]) {
      const key = `payment_statuses.${s}`;
      expect((tr as Record<string, string>)[key]).toBeTruthy();
      expect((en as Record<string, string>)[key]).toBeTruthy();
    }
  });
});
