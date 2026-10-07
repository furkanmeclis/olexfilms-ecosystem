"use client";

import { useState, type ComponentProps, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { FieldError } from "@/features/accounting/components/shared";
import {
  staffFormValues,
  validateStaffForm,
  type StaffFormValues,
} from "@/features/staff-reports/lib/staff";
import type { StaffProfile } from "@/features/staff-reports/services/staff-reports.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

/**
 * Staff card (TEC-345): name, title, hire day, monthly salary in the book
 * currency (K7) and the active flag. Inactive cards stay in the history
 * and are left out of the month-end payroll.
 */
export function StaffForm({
  staff,
  pending,
  onCancel,
  onSubmit,
}: {
  staff: StaffProfile | null;
  pending?: boolean;
  onCancel: () => void;
  onSubmit: (values: StaffFormValues) => Promise<unknown>;
}) {
  const { t } = useLocale();
  const [values, setValues] = useState(() => staffFormValues(staff));
  const [errors, setErrors] = useState<Record<string, string>>({});
  const set = <K extends keyof StaffFormValues>(
    key: K,
    value: StaffFormValues[K],
  ) => setValues((v) => ({ ...v, [key]: value }));

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const next = validateStaffForm(values, t);
    setErrors(next);
    if (Object.keys(next).length) return;
    try {
      await onSubmit(values);
    } catch (error) {
      if (isApiError(error) && error.isValidation) {
        setErrors(error.fieldErrors());
      }
    }
  };

  const field = (
    key: "name" | "title" | "monthly_salary",
    props: Partial<ComponentProps<typeof Input>> = {},
  ) => (
    <div className="grid gap-1.5">
      <Label htmlFor={`staff-${key}`}>{t(`staff_reports.fields.${key}`)}</Label>
      <Input
        id={`staff-${key}`}
        name={key}
        value={values[key]}
        aria-invalid={errors[key] ? true : undefined}
        aria-describedby={errors[key] ? `staff-${key}-error` : undefined}
        onChange={(e) => set(key, e.target.value)}
        {...props}
      />
      <FieldError id={`staff-${key}-error`} message={errors[key]} />
    </div>
  );

  return (
    <form
      onSubmit={submit}
      noValidate
      className="space-y-4"
      data-testid="staff-form"
    >
      {field("name", { maxLength: 200, autoComplete: "off" })}
      {field("title", { maxLength: 100, autoComplete: "off" })}
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="grid gap-1.5">
          <Label htmlFor="staff-hired_on">
            {t("staff_reports.fields.hired_on")}
          </Label>
          <DatePicker
            id="staff-hired_on"
            value={values.hired_on}
            aria-invalid={errors.hired_on ? true : undefined}
            onChange={(v) => set("hired_on", v)}
          />
          <FieldError id="staff-hired_on-error" message={errors.hired_on} />
        </div>
        {field("monthly_salary", {
          inputMode: "decimal",
          dir: "ltr",
          autoComplete: "off",
          className: "text-end tabular-nums",
        })}
      </div>
      <p className="text-muted-foreground text-xs">
        {t("staff_reports.staff.currency_hint")}
      </p>
      <div className="flex items-center gap-2">
        <Checkbox
          id="staff-active"
          checked={values.active}
          onCheckedChange={(v) => set("active", v === true)}
        />
        <Label htmlFor="staff-active">{t("staff_reports.fields.active")}</Label>
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
        <Button type="submit" disabled={pending} data-testid="staff-submit">
          {t("common.save")}
        </Button>
      </DialogFooter>
    </form>
  );
}

export function StaffFormDialog({
  open,
  staff,
  pending,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  staff: StaffProfile | null;
  pending?: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (values: StaffFormValues) => Promise<unknown>;
}) {
  const { t } = useLocale();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {staff
              ? t("staff_reports.staff.edit")
              : t("staff_reports.staff.create")}
          </DialogTitle>
          <DialogDescription>
            {t("staff_reports.staff.form_description")}
          </DialogDescription>
        </DialogHeader>
        {open ? (
          <StaffForm
            key={staff?.uuid ?? "new"}
            staff={staff}
            pending={pending}
            onCancel={() => onOpenChange(false)}
            onSubmit={onSubmit}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
