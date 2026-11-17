"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { FileDown, FileSignature, Loader2 } from "lucide-react";
import { useMemo, useState, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  CLIENT_SIDE_MANUAL,
  EntityLink,
  EntityTable,
  entityKindLabelKey,
} from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { localeDir, normalizeLocale } from "@/config/i18n";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  contractSigningKeys,
  contractSigningService,
  type ContractSigner,
  type ContractStatus,
} from "@/features/contracts/services/contract-signing.service";
import { isApiError } from "@/lib/api";
import { triggerBrowserDownload } from "@/lib/api/platform-form-request";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

const D = "contract_templates.contract_detail";
export const CONTRACT_SIGNERS_PERSIST_KEY = "tenant-contract-signers-v1";

function statusTone(status: ContractStatus) {
  return status === "executed"
    ? "success"
    : status === "voided"
      ? "danger"
      : status === "pending"
        ? "warning"
        : "default";
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm">{children}</dd>
    </div>
  );
}

/**
 * Tenant > contract detail (TEC-1218): number, kind, status and dates, the
 * subject record (service or subscription) linked through its ref, the
 * customer (the customer signer's account), the signers and the executed
 * PDF. Needs `contracts.read`.
 */
export function ContractDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const canRead = can(Permission.ContractsRead);
  const [busy, setBusy] = useState(false);

  const detail = useQuery({
    queryKey: contractSigningKeys.detail(uuid),
    queryFn: () => contractSigningService.get(uuid),
    enabled: canRead && uuid !== "",
    retry: (count, err) =>
      !(isApiError(err) && (err.status === 404 || err.status === 403)) &&
      count < 2,
  });
  const c = detail.data;

  const signerColumns = useMemo(
    () =>
      [
        createColumn<ContractSigner>({
          accessorKey: "role",
          labelKey: `${D}.signer_role`,
          enableSorting: false,
          cell: ({ row }) =>
            t(
              row.original.role === "customer"
                ? "services.contract.customer"
                : "services.contract.staff",
            ),
        }),
        createColumn<ContractSigner>({
          accessorKey: "name",
          labelKey: `${D}.signer_name`,
          enableSorting: false,
          gridPrimary: true,
        }),
        createColumn<ContractSigner>({
          id: "account",
          accessorFn: (s) => s.user?.name ?? "",
          labelKey: `${D}.signer_account`,
          enableSorting: false,
          cell: ({ row }) => {
            const ref = row.original.user;
            return ref ? (
              <EntityLink kind={ref.kind} entity={ref} slug={slug} />
            ) : (
              "—"
            );
          },
        }),
        createColumn<ContractSigner>({
          id: "otp_verified_at",
          accessorFn: (s) => s.otp_verified_at ?? "",
          labelKey: `${D}.otp_verified_at`,
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.otp_verified_at
              ? format.dateTime(row.original.otp_verified_at)
              : "—",
        }),
        createColumn<ContractSigner>({
          id: "signed_at",
          accessorFn: (s) => s.signed_at ?? "",
          labelKey: `${D}.signed_at`,
          enableSorting: false,
          cell: ({ row }) =>
            row.original.signed_at ? (
              format.dateTime(row.original.signed_at)
            ) : (
              <span className="text-muted-foreground">
                {t(`${D}.not_signed`)}
              </span>
            ),
        }),
      ] as ColumnDef<ContractSigner, unknown>[],
    [format, slug, t],
  );

  const download = async () => {
    if (!c) return;
    setBusy(true);
    try {
      const { blob, filename } = await contractSigningService.downloadPdf(
        c.uuid,
      );
      triggerBrowserDownload(blob, filename ?? `contract-${c.contract_no}.pdf`);
    } catch {
      appToast.error(t("services.contract.errors.pdf_failed"));
    } finally {
      setBusy(false);
    }
  };

  const title = c ? t(`${D}.title_no`, { no: c.contract_no }) : t(`${D}.title`);
  const executed = c?.status === "executed";
  const preparing = executed && !c?.pdf_ready;
  const header = (
    <PageHeader
      title={title}
      icon={<FileSignature className="size-6" />}
      description={c ? t(`contract_templates.kinds.${c.kind}`) : undefined}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
      actions={
        c && executed ? (
          <Button
            type="button"
            variant="outline"
            disabled={preparing || busy}
            aria-busy={preparing || busy}
            onClick={() => void download()}
            data-testid="contract-detail-pdf"
          >
            {preparing || busy ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <FileDown className="size-4" />
            )}
            {preparing
              ? t("services.contract.pdf_preparing")
              : t("services.contract.pdf")}
          </Button>
        ) : null
      }
    />
  );

  if (!canRead) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("services.contract.forbidden")}
        />
      </div>
    );
  }
  if (detail.isError) {
    const notFound = isApiError(detail.error) && detail.error.status === 404;
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={notFound ? t(`${D}.not_found`) : t("common.error_generic")}
          onRetry={notFound ? undefined : () => void detail.refetch()}
          retryLabel={t("common.retry")}
        />
      </div>
    );
  }
  if (!c) {
    return (
      <div className="space-y-6">
        {header}
        <p className="text-muted-foreground text-sm">{t("common.loading")}</p>
      </div>
    );
  }

  const when = (v: string | null | undefined) => (v ? format.dateTime(v) : "—");
  const customer = c.signers.find((s) => s.role === "customer")?.user;
  const docLocale = normalizeLocale(c.locale) ?? "tr";

  return (
    <div className="space-y-6" data-testid="contract-detail">
      {header}
      <Card>
        <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
          <CardTitle>{t(`${D}.summary`)}</CardTitle>
          <span data-testid="contract-detail-status" data-status={c.status}>
            <StatusChip
              label={t(`services.contract.status.${c.status}`)}
              tone={statusTone(c.status)}
            />
          </span>
        </CardHeader>
        <CardContent>
          <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <Field label={t(`${D}.contract_no`)}>
              <span dir="ltr">#{c.contract_no}</span>
            </Field>
            <Field label={t("contract_templates.fields.kind")}>
              {t(`contract_templates.kinds.${c.kind}`)}
            </Field>
            <Field label={t(`${D}.subject`)}>
              {c.subject ? (
                <span data-testid="contract-detail-subject">
                  <EntityLink
                    kind={c.subject.kind}
                    entity={c.subject}
                    slug={slug}
                  />
                  <span className="text-muted-foreground ms-1 text-xs">
                    {t(entityKindLabelKey(c.subject.kind))}
                  </span>
                </span>
              ) : (
                "—"
              )}
            </Field>
            <Field label={t("services.contract.customer")}>
              <span data-testid="contract-detail-customer">
                {customer ? (
                  <EntityLink
                    kind={customer.kind}
                    entity={customer}
                    slug={slug}
                  />
                ) : (
                  (c.signers.find((s) => s.role === "customer")?.name ?? "—")
                )}
              </span>
            </Field>
            <Field label={t(`${D}.locale`)}>{c.locale}</Field>
            <Field label={t(`${D}.template_version`)}>
              {format.number(c.template_version)}
            </Field>
            <Field label={t(`${D}.created_at`)}>{when(c.created_at)}</Field>
            <Field label={t(`${D}.executed_at`)}>{when(c.executed_at)}</Field>
            {c.voided_at ? (
              <>
                <Field label={t(`${D}.voided_at`)}>{when(c.voided_at)}</Field>
                <Field label={t(`${D}.void_reason`)}>
                  {c.void_reason || "—"}
                </Field>
              </>
            ) : null}
          </dl>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t(`${D}.signers`)}</CardTitle>
        </CardHeader>
        <CardContent>
          <EntityTable
            columns={signerColumns}
            data={c.signers}
            getRowId={(row) => row.uuid}
            manual={CLIENT_SIDE_MANUAL}
            emptyTitle={t(`${D}.no_signers`)}
            emptyDescription=""
            features={{ persistKey: CONTRACT_SIGNERS_PERSIST_KEY }}
          />
        </CardContent>
      </Card>

      {c.rendered_html ? (
        <Card>
          <CardHeader>
            <CardTitle>{t("services.contract.preview")}</CardTitle>
          </CardHeader>
          <CardContent>
            <iframe
              title={t("services.contract.preview")}
              sandbox=""
              className="h-96 w-full rounded-md border bg-white"
              data-testid="contract-detail-preview"
              srcDoc={`<!doctype html><html dir="${localeDir(docLocale)}" lang="${docLocale}"><head><meta charset="utf-8"></head><body style="font-family:system-ui,sans-serif;padding:16px">${c.rendered_html}</body></html>`}
            />
          </CardContent>
        </Card>
      ) : null}
    </div>
  );
}
