"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage } from "@/components/entity";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { DisputeStatusChip } from "@/features/accounting/components/disputes-page";
import {
  FieldError,
  Money,
  useCategoryLabels,
} from "@/features/accounting/components/shared";
import {
  accountingKeys,
  useAccountingAccess,
} from "@/features/accounting/hooks/use-accounting-access";
import {
  canResolveDispute,
  DISPUTE_RESOLUTIONS,
  DISPUTE_TEXT_MAX,
  resolveFormSchema,
  resolveInput,
  type ResolveFormValues,
} from "@/features/accounting/lib/disputes";
import { fieldErrors } from "@/features/accounting/lib/form";
import {
  accountingService,
  type AccountingDispute,
  type AccountingDisputeResolveInput,
} from "@/features/accounting/services/accounting.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * Resolution of an open dispute by the parent the row came from (TEC-174):
 * reversal (reverse every open row of the source), revision (reverse and
 * repost the corrected amount in the entry's original currency at the
 * frozen rate) or reject (a note is required, nothing is posted).
 */
export function ResolveForm({
  dispute,
  pending,
  onSubmit,
}: {
  dispute: AccountingDispute;
  pending?: boolean;
  onSubmit: (input: AccountingDisputeResolveInput) => Promise<unknown>;
}) {
  const { t } = useLocale();
  const [values, setValues] = useState<ResolveFormValues>({
    resolution: "reversal",
    corrected_amount: "",
    note: "",
  });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const set = <K extends keyof ResolveFormValues>(
    key: K,
    value: ResolveFormValues[K],
  ) => setValues((v) => ({ ...v, [key]: value }));

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    const parsed = resolveFormSchema(t).safeParse(values);
    if (!parsed.success) {
      setErrors(fieldErrors(parsed.error));
      return;
    }
    setErrors({});
    try {
      await onSubmit(resolveInput(values));
    } catch (error) {
      if (isApiError(error) && error.isValidation) {
        setErrors(error.fieldErrors());
      }
    }
  };

  return (
    <form
      onSubmit={submit}
      noValidate
      className="space-y-4"
      data-testid="resolve-form"
      data-resolution={values.resolution}
    >
      <fieldset className="grid gap-2">
        <legend className="mb-1 text-sm font-medium">
          {t("accounting.disputes.resolve.resolution")}
        </legend>
        <RadioGroup
          name="resolution"
          value={values.resolution}
          onValueChange={(r) => {
            set("resolution", r as (typeof DISPUTE_RESOLUTIONS)[number]);
            setErrors({});
          }}
          className="grid gap-2"
        >
          {DISPUTE_RESOLUTIONS.map((r) => (
            <label
              key={r}
              className="has-data-[state=checked]:border-primary flex cursor-pointer items-start gap-2 rounded-md border p-3"
            >
              <RadioGroupItem
                value={r}
                className="mt-1"
                data-testid={`resolve-${r}`}
              />
              <span>
                <span className="block text-sm font-medium">
                  {t(`accounting.disputes.resolve.${r}`)}
                </span>
                <span className="text-muted-foreground block text-xs">
                  {t(`accounting.disputes.resolve.${r}_hint`)}
                </span>
              </span>
            </label>
          ))}
        </RadioGroup>
      </fieldset>
      {values.resolution === "revision" ? (
        <div className="grid gap-1.5">
          <Label htmlFor="resolve-amount">
            {t("accounting.disputes.resolve.corrected_amount", {
              currency: dispute.entry.orig_currency,
            })}
          </Label>
          <Input
            id="resolve-amount"
            name="corrected_amount"
            inputMode="decimal"
            dir="ltr"
            autoComplete="off"
            className="text-end tabular-nums sm:max-w-48"
            value={values.corrected_amount}
            aria-invalid={errors.corrected_amount ? true : undefined}
            aria-describedby={
              errors.corrected_amount ? "resolve-amount-error" : undefined
            }
            onChange={(e) => set("corrected_amount", e.target.value)}
          />
          <FieldError
            id="resolve-amount-error"
            message={errors.corrected_amount}
          />
        </div>
      ) : null}
      <div className="grid gap-1.5">
        <Label htmlFor="resolve-note">
          {values.resolution === "reject"
            ? t("accounting.disputes.resolve.reject_note")
            : t("accounting.disputes.resolve.note")}
        </Label>
        <Textarea
          id="resolve-note"
          name="note"
          rows={3}
          maxLength={DISPUTE_TEXT_MAX}
          value={values.note}
          aria-invalid={errors.note ? true : undefined}
          aria-describedby={errors.note ? "resolve-note-error" : undefined}
          onChange={(e) => set("note", e.target.value)}
        />
        <FieldError id="resolve-note-error" message={errors.note} />
      </div>
      <div className="flex justify-end">
        <Button
          type="submit"
          disabled={pending}
          variant={values.resolution === "reject" ? "destructive" : "default"}
          data-testid="resolve-submit"
        >
          {t(`accounting.disputes.resolve.submit_${values.resolution}`)}
        </Button>
      </div>
    </form>
  );
}

function Row({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="grid gap-1 sm:grid-cols-[12rem_1fr]">
      <dt className="text-muted-foreground text-sm">{label}</dt>
      <dd className="text-sm">{children}</dd>
    </div>
  );
}

/** Tenant > Accounting > Disputes > detail; the parent resolves here. */
export function DisputeDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format } = useLocale();
  const queryClient = useQueryClient();
  const access = useAccountingAccess(slug);
  const enabled = access.canRead && Boolean(access.orgUuid);
  const categories = useCategoryLabels(access.orgUuid, enabled);

  const dispute = useQuery({
    queryKey: accountingKeys.dispute(access.orgUuid, uuid),
    queryFn: () => accountingService.getDispute(uuid),
    enabled,
  });

  const resolve = useMutation({
    mutationFn: (input: AccountingDisputeResolveInput) =>
      accountingService.resolveDispute(uuid, input),
    onSuccess: async (resolved) => {
      queryClient.setQueryData(
        accountingKeys.dispute(access.orgUuid, uuid),
        resolved,
      );
      await queryClient.invalidateQueries({
        queryKey: accountingKeys.all(access.orgUuid),
      });
      appToast.success(t(`accounting.disputes.resolved.${resolved.status}`));
    },
    onError: (error: unknown) => {
      appToast.error(
        isApiError(error) ? error.message : t("accounting.toast.failed"),
      );
    },
  });

  const d = dispute.data;
  const resolvable = d
    ? canResolveDispute(d, access.orgUuid, access.canResolve)
    : false;

  return (
    <EntityPage
      title={t("accounting.disputes.detail_title")}
      description={d ? d.organization.name : undefined}
      permission={permissions.accounting.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("accounting.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("accounting.disputes.title"),
          href: routes.tenant.accounting.disputes(slug),
        },
        { label: t("accounting.disputes.detail_title") },
      ]}
      actions={d ? <DisputeStatusChip status={d.status} /> : null}
    >
      {dispute.isLoading ? (
        <Loading />
      ) : dispute.isError || !d ? (
        <ErrorState
          title={
            isApiError(dispute.error) && dispute.error.status === 404
              ? t("accounting.disputes.not_found")
              : t("accounting.load_failed")
          }
          onRetry={() => void dispute.refetch()}
        />
      ) : (
        <div className="grid gap-6 lg:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>{t("accounting.disputes.entry_title")}</CardTitle>
            </CardHeader>
            <CardContent>
              <dl className="space-y-3" data-testid="dispute-detail">
                <Row label={t("accounting.disputes.fields.organization")}>
                  {d.organization.name}
                </Row>
                <Row label={t("accounting.disputes.fields.counterparty")}>
                  {d.counterparty_organization.name}
                </Row>
                <Row label={t("accounting.fields.date")}>
                  {format.dateTime(d.entry.created_at)}
                </Row>
                <Row label={t("accounting.fields.direction")}>
                  {t(`accounting.directions.${d.entry.direction}`)}
                </Row>
                <Row label={t("accounting.fields.category")}>
                  {categories.get(d.entry.category) ?? d.entry.category}
                </Row>
                <Row label={t("accounting.fields.source")}>
                  {(
                    ["manual", "order", "service", "transfer"] as string[]
                  ).includes(d.source_type)
                    ? t(`accounting.sources.${d.source_type}`)
                    : d.source_type}
                </Row>
                <Row label={t("accounting.fields.amount")}>
                  <Money
                    amount={d.entry.orig_amount}
                    currency={d.entry.orig_currency}
                  />
                </Row>
                <Row label={t("accounting.disputes.fields.reason")}>
                  <span className="whitespace-pre-wrap">{d.reason}</span>
                </Row>
                <Row label={t("accounting.disputes.fields.opened_at")}>
                  {format.dateTime(d.created_at)}
                </Row>
              </dl>
            </CardContent>
          </Card>
          {d.status !== "open" ? (
            <Card>
              <CardHeader>
                <CardTitle>
                  {t("accounting.disputes.resolution_title")}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <dl className="space-y-3" data-testid="dispute-resolution">
                  <Row label={t("accounting.fields.status")}>
                    <DisputeStatusChip status={d.status} />
                  </Row>
                  {d.corrected_amount ? (
                    <Row
                      label={t("accounting.disputes.fields.corrected_amount")}
                    >
                      <Money
                        amount={d.corrected_amount}
                        currency={d.entry.orig_currency}
                      />
                    </Row>
                  ) : null}
                  {d.resolution_note ? (
                    <Row
                      label={t("accounting.disputes.fields.resolution_note")}
                    >
                      <span className="whitespace-pre-wrap">
                        {d.resolution_note}
                      </span>
                    </Row>
                  ) : null}
                  {d.resolved_at ? (
                    <Row label={t("accounting.disputes.fields.resolved_at")}>
                      {format.dateTime(d.resolved_at)}
                    </Row>
                  ) : null}
                </dl>
              </CardContent>
            </Card>
          ) : resolvable ? (
            <Card>
              <CardHeader>
                <CardTitle>{t("accounting.disputes.resolve.title")}</CardTitle>
                <CardDescription>
                  {t("accounting.disputes.resolve.description")}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <ResolveForm
                  dispute={d}
                  pending={resolve.isPending}
                  onSubmit={(input) => resolve.mutateAsync(input)}
                />
              </CardContent>
            </Card>
          ) : (
            <Card>
              <CardContent className="text-muted-foreground pt-6 text-sm">
                {t("accounting.disputes.waiting")}
              </CardContent>
            </Card>
          )}
        </div>
      )}
    </EntityPage>
  );
}
