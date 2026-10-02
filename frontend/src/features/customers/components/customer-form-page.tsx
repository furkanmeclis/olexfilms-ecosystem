"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { UserPlus } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { customerErrorMessage } from "@/features/customers/lib/errors";
import {
  COMPANY_MAX,
  EMPTY_CUSTOMER_FORM,
  NAME_MAX,
  TAX_OFFICE_MAX,
  buildCustomerCreate,
  buildCustomerUpdate,
  customerFormFromDetail,
  validateCustomerForm,
  type CustomerFormField,
  type CustomerFormValues,
} from "@/features/customers/lib/form";
import {
  customerKeys,
  customersService,
  type CustomerDetail,
  type CustomerType,
  type CustomerWrite,
} from "@/features/customers/services/customers.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const TYPES: CustomerType[] = ["individual", "corporate"];

/**
 * Customer create / edit (TEC-163). Create links an existing user with the
 * same phone instead of a second account (K11, `existing_user`); edit sends
 * only the changed fields, and a shared customer only fills empty fields
 * (`identity_editable`, the rest comes back in `ignored_fields`).
 */
export function CustomerFormPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid?: string;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.customers.write);
  const edit = Boolean(uuid);
  const detail = useQuery({
    queryKey: customerKeys.detail(uuid ?? ""),
    queryFn: () => customersService.get(uuid ?? ""),
    enabled: canWrite && edit,
  });

  const title = edit ? t("customers.form.edit_title") : t("customers.nav_new");
  const header = (
    <PageHeader
      title={title}
      icon={<UserPlus className="size-6" />}
      description={
        edit
          ? t("customers.form.edit_description")
          : t("customers.form.create_description")
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("customers.list.title"),
          href: routes.tenant.customers.list(slug),
        },
        { label: title },
      ]}
    />
  );

  if (!canWrite) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("customers.form.forbidden")}
        />
      </div>
    );
  }
  if (edit && detail.isLoading) return <Loading />;
  if (edit && (detail.isError || !detail.data)) {
    const notFound = isApiError(detail.error) && detail.error.status === 404;
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={
            notFound
              ? t("customers.detail.not_found")
              : t("common.error_generic")
          }
          onRetry={notFound ? undefined : () => void detail.refetch()}
          retryLabel={t("common.retry")}
        />
      </div>
    );
  }
  if (edit && detail.data && !detail.data.editable) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("customers.form.not_editable")}
          description={t("customers.form.not_editable_description")}
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}
      <CustomerForm slug={slug} customer={detail.data ?? null} />
    </div>
  );
}

export function CustomerForm({
  slug,
  customer,
}: {
  slug: string;
  customer: CustomerDetail | null;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const original = customer
    ? customerFormFromDetail(customer)
    : EMPTY_CUSTOMER_FORM;
  const [values, setValues] = useState<CustomerFormValues>(original);
  const [errors, setErrors] = useState<Record<string, string>>({});

  const set = (key: CustomerFormField, value: string) => {
    setValues((v) => ({ ...v, [key]: value }));
    setErrors((e) => {
      if (!e[key]) return e;
      const next = { ...e };
      delete next[key];
      return next;
    });
  };

  const done = (saved: CustomerWrite, created: boolean) => {
    if (created) {
      toast.success(
        saved.existing_user
          ? t("customers.form.linked_existing")
          : t("customers.form.created"),
      );
    } else {
      toast.success(t("customers.form.saved"));
    }
    if (saved.ignored_fields.length > 0) {
      toast.warning(
        t("customers.form.ignored_fields", {
          fields: saved.ignored_fields
            .map((f) => t(`customers.fields.${f}`))
            .join(", "),
        }),
      );
    }
    qc.setQueryData(customerKeys.detail(saved.uuid), saved);
    void qc.invalidateQueries({ queryKey: ["customers", "list"] });
    router.push(routes.tenant.customers.detail(slug, saved.uuid));
  };

  const save = useMutation({
    mutationFn: () =>
      customer
        ? customersService.update(
            customer.uuid,
            buildCustomerUpdate(values, original),
          )
        : customersService.create(buildCustomerCreate(values)),
    onSuccess: (saved) => done(saved, !customer),
    onError: (err) => {
      if (isApiError(err)) {
        const fields = err.fieldErrors();
        if (Object.keys(fields).length > 0) setErrors(fields);
      }
      toast.error(customerErrorMessage(err, t, t("customers.form.failed")));
    },
  });

  const submit = () => {
    const next = validateCustomerForm(values, customer ? "edit" : "create");
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
    if (Object.keys(localized).length === 0) save.mutate();
  };

  const field = (
    key: CustomerFormField,
    opts?: {
      required?: boolean;
      type?: string;
      dir?: "ltr";
      maxLength?: number;
      disabled?: boolean;
      hint?: string;
      placeholder?: string;
    },
  ) => (
    <div className="space-y-1.5">
      <Label htmlFor={`customer-${key}`}>
        {t(`customers.fields.${key}`)}
        {opts?.required ? (
          <span className="text-destructive ms-1">*</span>
        ) : null}
      </Label>
      <Input
        id={`customer-${key}`}
        name={key}
        type={opts?.type ?? "text"}
        dir={opts?.dir}
        maxLength={opts?.maxLength}
        disabled={opts?.disabled}
        placeholder={opts?.placeholder}
        value={values[key]}
        aria-invalid={errors[key] ? true : undefined}
        onChange={(e) => set(key, e.target.value)}
      />
      {errors[key] ? (
        <p className="text-destructive text-xs" data-error={key}>
          {errors[key]}
        </p>
      ) : opts?.hint ? (
        <p className="text-muted-foreground text-xs">{opts.hint}</p>
      ) : null}
    </div>
  );

  const masked = (last4: string | null | undefined) =>
    last4 ? t("customers.form.identity_stored", { last4 }) : undefined;

  return (
    <Card>
      <CardContent className="space-y-6 pt-6" data-testid="customer-form">
        {customer && !customer.identity_editable ? (
          <p
            className="bg-muted text-muted-foreground rounded-md p-3 text-sm"
            data-testid="fill-only-notice"
          >
            {t("customers.form.fill_only")}
          </p>
        ) : null}
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
              data-type={type}
              onClick={() => set("type", type)}
            >
              {t(`customers.type.${type}`)}
            </Button>
          ))}
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          {field("phone", {
            required: !customer,
            type: "tel",
            dir: "ltr",
            disabled: Boolean(customer),
            placeholder: "+90 555 123 45 67",
            hint: customer
              ? t("customers.form.phone_locked")
              : t("customers.form.phone_hint"),
          })}
          {field("email", { type: "email", dir: "ltr" })}
          {field("name", { required: true, maxLength: NAME_MAX })}
          {field("surname", { maxLength: NAME_MAX })}
          {values.type === "corporate" ? (
            <>
              {field("company_name", {
                required: true,
                maxLength: COMPANY_MAX,
              })}
              {field("tax_office", { maxLength: TAX_OFFICE_MAX })}
              {field("tax_no", {
                dir: "ltr",
                maxLength: 24,
                hint: masked(customer?.tax_no_last4),
              })}
            </>
          ) : null}
          {field("national_id", {
            dir: "ltr",
            maxLength: 24,
            hint: masked(customer?.national_id_last4),
          })}
        </div>
        <p className="text-muted-foreground text-xs">
          {t("customers.form.identity_hint")}
        </p>
        <div className="flex justify-end gap-2">
          <Button
            type="button"
            variant="ghost"
            onClick={() =>
              router.push(
                customer
                  ? routes.tenant.customers.detail(slug, customer.uuid)
                  : routes.tenant.customers.list(slug),
              )
            }
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            data-testid="customer-submit"
            disabled={save.isPending}
            onClick={submit}
          >
            {customer ? t("customers.form.save") : t("customers.form.create")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
