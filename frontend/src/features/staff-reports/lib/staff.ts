import { normalizeAmount } from "@/features/accounting/lib/form";
import type {
  StaffPaymentCreateInput,
  StaffPaymentType,
  StaffPayrollResult,
  StaffProfile,
  StaffProfileCreateInput,
  StaffProfilePatchInput,
} from "@/features/staff-reports/services/staff-reports.service";
import { isApiError } from "@/lib/api";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** Backend 409 code of a second open salary for the same staff period. */
export const SALARY_EXISTS_CODE = "STAFF_SALARY_EXISTS";

const PERIOD_RE = /^[0-9]{4}-(0[1-9]|1[0-2])$/;
const DATE_RE = /^\d{4}-\d{2}-\d{2}$/;

export function isValidPeriod(period: string): boolean {
  return PERIOD_RE.test(period);
}

/** The payroll period of a day: "YYYY-MM" (local calendar). */
export function periodOf(day: Date = new Date()): string {
  const month = String(day.getMonth() + 1).padStart(2, "0");
  return `${day.getFullYear()}-${month}`;
}

/** Today as "YYYY-MM-DD" (local calendar). */
export function todayIso(day: Date = new Date()): string {
  return `${periodOf(day)}-${String(day.getDate()).padStart(2, "0")}`;
}

/** First day of a "YYYY-MM" period (for Intl month labels). */
export function periodDate(period: string): Date {
  const [y, m] = period.split("-").map(Number);
  return new Date(y!, (m ?? 1) - 1, 1);
}

/** A staff card salary as a number (null / "0.00" → 0). */
export function salaryOf(staff: Pick<StaffProfile, "monthly_salary">): number {
  return staff.monthly_salary ? Number(staff.monthly_salary) : 0;
}

export type PayrollPreview = {
  /** Active staff with a monthly salary: one salary payment each. */
  payable: StaffProfile[];
  /** Active staff without a salary: the payroll skips them. */
  noSalary: StaffProfile[];
  /** Sum of the payable salaries. */
  total: number;
  currency: string | null;
};

/**
 * Month-end payroll preview: only active staff are listed (the backend
 * runs over active cards only); inactive cards never appear.
 */
export function payrollPreview(staff: readonly StaffProfile[]): PayrollPreview {
  const active = staff.filter((s) => s.active);
  const payable = active.filter((s) => salaryOf(s) > 0);
  const noSalary = active.filter((s) => salaryOf(s) <= 0);
  const cents = payable.reduce(
    (sum, s) => sum + Math.round(salaryOf(s) * 100),
    0,
  );
  return {
    payable,
    noSalary,
    total: cents / 100,
    currency: payable[0]?.currency ?? active[0]?.currency ?? null,
  };
}

/**
 * The payroll is idempotent: a second run of the same period creates no
 * salary and answers 201 with every card skipped. The UI treats that, and
 * a 409, as "this period already ran".
 */
export function payrollAlreadyRan(result: StaffPayrollResult): boolean {
  return result.created === 0 && result.skipped > 0;
}

/** 409 STAFF_SALARY_EXISTS (or any 409 of the payment / payroll call). */
export function isSalaryConflict(error: unknown): boolean {
  return (
    isApiError(error) &&
    (error.code === SALARY_EXISTS_CODE || error.status === 409)
  );
}

// --- Staff card form ---------------------------------------------------------

export type StaffFormValues = {
  name: string;
  title: string;
  hired_on: string;
  monthly_salary: string;
  active: boolean;
};

export function staffFormValues(staff: StaffProfile | null): StaffFormValues {
  return {
    name: staff?.name ?? "",
    title: staff?.title ?? "",
    hired_on: staff?.hired_on ?? "",
    monthly_salary: staff?.monthly_salary ?? "",
    active: staff?.active ?? true,
  };
}

export function validateStaffForm(
  values: StaffFormValues,
  t: Translate,
): Record<string, string> {
  const errors: Record<string, string> = {};
  const name = values.name.trim();
  if (!name) errors.name = t("accounting.validation.required");
  else if (name.length > 200)
    errors.name = t("accounting.validation.too_long", { max: 200 });
  if (values.title.trim().length > 100)
    errors.title = t("accounting.validation.too_long", { max: 100 });
  if (values.hired_on && !DATE_RE.test(values.hired_on))
    errors.hired_on = t("accounting.validation.date");
  if (values.monthly_salary.trim() && !normalizeAmount(values.monthly_salary))
    errors.monthly_salary = t("accounting.validation.amount");
  return errors;
}

export function staffCreateInput(v: StaffFormValues): StaffProfileCreateInput {
  const salary = normalizeAmount(v.monthly_salary);
  return {
    name: v.name.trim(),
    title: v.title.trim() || null,
    ...(v.hired_on ? { hired_on: v.hired_on } : {}),
    ...(salary ? { monthly_salary: salary } : {}),
    active: v.active,
  };
}

/** PATCH body: cleared optional fields are sent as null. */
export function staffPatchInput(v: StaffFormValues): StaffProfilePatchInput {
  return {
    name: v.name.trim(),
    title: v.title.trim() || null,
    hired_on: v.hired_on || null,
    monthly_salary: normalizeAmount(v.monthly_salary),
    active: v.active,
  };
}

// --- Payment form ------------------------------------------------------------

export type PaymentFormValues = {
  type: StaffPaymentType;
  period: string;
  amount: string;
  account_uuid: string;
  paid_on: string;
  description: string;
  target_note: string;
};

export function paymentFormValues(
  type: StaffPaymentType = "salary",
  day: Date = new Date(),
): PaymentFormValues {
  return {
    type,
    period: periodOf(day),
    amount: "",
    account_uuid: "",
    paid_on: todayIso(day),
    description: "",
    target_note: "",
  };
}

/**
 * Salary may leave the amount empty when the card has a monthly salary
 * (the backend books that); advance and bonus need one. The cash / bank
 * account is required for a single payment.
 */
export function validatePaymentForm(
  values: PaymentFormValues,
  staff: Pick<StaffProfile, "monthly_salary">,
  t: Translate,
): Record<string, string> {
  const errors: Record<string, string> = {};
  if (!isValidPeriod(values.period))
    errors.period = t("staff_reports.validation.period");
  const amount = values.amount.trim();
  if (amount) {
    if (!normalizeAmount(amount))
      errors.amount = t("accounting.validation.amount");
  } else if (values.type !== "salary" || salaryOf(staff) <= 0) {
    errors.amount = t("accounting.validation.amount");
  }
  if (!values.account_uuid)
    errors.account_uuid = t("accounting.validation.account");
  if (values.paid_on && !DATE_RE.test(values.paid_on))
    errors.paid_on = t("accounting.validation.date");
  if (values.description.trim().length > 1500)
    errors.description = t("accounting.validation.too_long", { max: 1500 });
  if (values.target_note.trim().length > 400)
    errors.target_note = t("accounting.validation.too_long", { max: 400 });
  return errors;
}

export function paymentInput(v: PaymentFormValues): StaffPaymentCreateInput {
  const amount = normalizeAmount(v.amount);
  return {
    type: v.type,
    period: v.period,
    ...(amount ? { amount } : {}),
    account_uuid: v.account_uuid,
    ...(v.paid_on ? { paid_on: v.paid_on } : {}),
    description: v.description.trim() || null,
    target_note: v.target_note.trim() || null,
  };
}
