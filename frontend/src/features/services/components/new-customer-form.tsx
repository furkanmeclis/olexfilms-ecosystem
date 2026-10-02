"use client";

import { useMutation } from "@tanstack/react-query";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  serviceWizardService,
  type CustomerWrite,
} from "@/features/services/services/service-wizard.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * Minimal customer create (POST /v1/customers, TEC-160): phone (any format,
 * E.164 on the server, K29) and name. An existing phone links that user
 * instead (one phone = one user, K11).
 */
export function NewCustomerForm({
  onCreated,
  onCancel,
}: {
  onCreated: (customer: CustomerWrite) => void;
  onCancel: () => void;
}) {
  const { t } = useLocale();
  const [phone, setPhone] = useState("");
  const [name, setName] = useState("");
  const [surname, setSurname] = useState("");
  const [errors, setErrors] = useState<Record<string, string>>({});

  const create = useMutation({
    mutationFn: () =>
      serviceWizardService.createCustomer({
        phone: phone.trim(),
        name: name.trim(),
        ...(surname.trim() ? { surname: surname.trim() } : {}),
      }),
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
    const next: Record<string, string> = {};
    if (!phone.trim()) next.phone = t("services.customer.phone_required");
    if (!name.trim()) next.name = t("services.customer.name_required");
    setErrors(next);
    if (Object.keys(next).length === 0) create.mutate();
  };

  const field = (
    key: string,
    label: string,
    value: string,
    set: (v: string) => void,
    extra?: { required?: boolean; type?: string; dir?: "ltr" },
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
        value={value}
        aria-invalid={errors[key] ? true : undefined}
        onChange={(e) => set(e.target.value)}
      />
      {errors[key] ? (
        <p className="text-destructive text-xs">{errors[key]}</p>
      ) : null}
    </div>
  );

  return (
    <div
      className="bg-muted/30 space-y-4 rounded-lg border p-4"
      data-testid="new-customer-form"
    >
      <div className="grid gap-4 sm:grid-cols-3">
        {field("phone", t("services.customer.phone"), phone, setPhone, {
          required: true,
          type: "tel",
          dir: "ltr",
        })}
        {field("name", t("services.customer.name"), name, setName, {
          required: true,
        })}
        {field("surname", t("services.customer.surname"), surname, setSurname)}
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
