"use client";

import { useMutation } from "@tanstack/react-query";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  COMPANY_MAX,
  EMPTY_CUSTOMER_FORM,
  NAME_MAX,
  TAX_OFFICE_MAX,
  buildCustomerCreate,
  validateCustomerForm,
  type CustomerFormField,
  type CustomerFormValues,
} from "@/features/customers/lib/form";
import type { CustomerType } from "@/features/customers/services/customers.service";
import {
  serviceWizardService,
  type CustomerWrite,
} from "@/features/services/services/service-wizard.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const TYPES: CustomerType[] = ["individual", "corporate"];

/** Fields of the inline form; the identity number stays on the full form. */
type InlineField = Exclude<CustomerFormField, "email" | "national_id" | "type">;

/**
 * Inline customer create of the service wizard (POST /v1/customers, TEC-160,
 * TEC-867): individual / corporate choice, then name, surname and phone
 * (any format, E.164 on the server, K29); a corporate customer also gets the
 * company fields of the full customer form. Validation and the request body
 * are shared with that form (features/customers/lib/form). An existing phone
 * links that user instead (one phone = one user, K11).
 */
export function NewCustomerForm({
  onCreated,
  onCancel,
}: {
  onCreated: (customer: CustomerWrite) => void;
  onCancel: () => void;
}) {
  const { t } = useLocale();
  const [values, setValues] = useState<CustomerFormValues>(EMPTY_CUSTOMER_FORM);
  const [errors, setErrors] = useState<Record<string, string>>({});

  const set = <K extends keyof CustomerFormValues>(
    key: K,
    value: CustomerFormValues[K],
  ) => setValues((prev) => ({ ...prev, [key]: value }));

  const create = useMutation({
    mutationFn: () =>
      serviceWizardService.createCustomer(buildCustomerCreate(values)),
    onSuccess: (customer) => {
      appToast.success(
        customer.existing_user
          ? t("services.customer.linked_existing")
          : t("services.customer.created"),
      );
      onCreated(customer);
    },
    onError: (error: unknown) => {
      if (isApiError(error)) {
        const fields = error.fieldErrors();
        if (Object.keys(fields).length > 0) setErrors(fields);
        appToast.error(error.message);
        return;
      }
      appToast.error(t("services.wizard.save_failed"));
    },
  });

  const submit = () => {
    const next = validateCustomerForm(values, "create");
    const localized: Record<string, string> = {};
    for (const [k, code] of Object.entries(next)) {
      localized[k] = t(`customers.validation.${code}`, {
        max:
          k === "company_name"
            ? COMPANY_MAX
            : k === "tax_office"
              ? TAX_OFFICE_MAX
              : NAME_MAX,
      });
    }
    setErrors(localized);
    if (Object.keys(localized).length === 0) create.mutate();
  };

  const field = (
    key: InlineField,
    label: string,
    extra?: {
      required?: boolean;
      type?: string;
      dir?: "ltr";
      maxLength?: number;
    },
  ) => (
    <div className="space-y-2">
      <Label htmlFor={`new-customer-${key}`}>
        {label}
        {extra?.required ? (
          <span className="text-destructive ms-1">*</span>
        ) : null}
      </Label>
      <Input
        id={`new-customer-${key}`}
        name={key}
        type={extra?.type ?? "text"}
        dir={extra?.dir}
        maxLength={extra?.maxLength}
        value={values[key]}
        aria-invalid={errors[key] ? true : undefined}
        onChange={(e) => set(key, e.target.value)}
      />
      {errors[key] ? (
        <p className="text-destructive text-xs" data-error={key}>
          {errors[key]}
        </p>
      ) : null}
    </div>
  );

  return (
    <div
      className="bg-muted/30 space-y-4 rounded-lg border p-4"
      data-testid="new-customer-form"
    >
      <div
        className="flex flex-wrap gap-2"
        role="group"
        aria-label={t("customers.fields.type")}
      >
        {TYPES.map((type) => (
          <Button
            key={type}
            type="button"
            size="sm"
            variant={values.type === type ? "default" : "outline"}
            aria-pressed={values.type === type}
            data-testid={`new-customer-type-${type}`}
            onClick={() => set("type", type)}
          >
            {t(`customers.type.${type}`)}
          </Button>
        ))}
      </div>
      {values.type === "corporate" ? (
        <div className="grid gap-4 sm:grid-cols-3">
          {field("company_name", t("customers.fields.company_name"), {
            required: true,
            maxLength: COMPANY_MAX,
          })}
          {field("tax_office", t("customers.fields.tax_office"), {
            maxLength: TAX_OFFICE_MAX,
          })}
          {field("tax_no", t("customers.fields.tax_no"), {
            dir: "ltr",
            maxLength: 24,
          })}
        </div>
      ) : null}
      <div className="grid gap-4 sm:grid-cols-3">
        {field("name", t("services.customer.name"), {
          required: true,
          maxLength: NAME_MAX,
        })}
        {field("surname", t("services.customer.surname"), {
          maxLength: NAME_MAX,
        })}
        {field("phone", t("services.customer.phone"), {
          required: true,
          type: "tel",
          dir: "ltr",
        })}
      </div>
      <p className="text-muted-foreground text-xs">
        {t("services.customer.phone_hint")}
      </p>
      <div className="flex justify-end gap-2">
        <Button type="button" variant="ghost" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
        <Button
          type="button"
          data-testid="new-customer-submit"
          disabled={create.isPending}
          onClick={submit}
        >
          {t("services.customer.create")}
        </Button>
      </div>
    </div>
  );
}
