"use client";

import { useQuery } from "@tanstack/react-query";
import { Ban, FileDown, Repeat } from "lucide-react";
import { useState, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { contractSigningService } from "@/features/contracts/services/contract-signing.service";
import { CancelRequestDialog } from "@/features/service-subscriptions/components/cancel-request-dialog";
import { SubscriptionStatusChip } from "@/features/service-subscriptions/components/subscription-status";
import {
  amountNumber,
  canRequestCancel,
} from "@/features/service-subscriptions/lib/subscriptions";
import {
  serviceSubscriptionKeys,
  serviceSubscriptionsService,
  type ServiceSubscription,
} from "@/features/service-subscriptions/services/service-subscriptions.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { isApiError } from "@/lib/api";
import { triggerBrowserDownload } from "@/lib/api/platform-form-request";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-1">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm">{children}</dd>
    </div>
  );
}

const CONTRACT_TONE = {
  draft: "default",
  pending: "warning",
  executed: "success",
  voided: "danger",
} as const;

function ContractCard({
  contract,
}: {
  contract: NonNullable<ServiceSubscription["contract"]>;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const [busy, setBusy] = useState(false);
  const canDownload = contract.pdf_ready && can(permissions.contracts.read);

  const download = async () => {
    setBusy(true);
    try {
      const { blob, filename } = await contractSigningService.downloadPdf(
        contract.uuid,
      );
      triggerBrowserDownload(
        blob,
        filename ?? `contract-${contract.contract_no}.pdf`,
      );
    } catch {
      appToast.error(t("services.contract.errors.pdf_failed"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card data-testid="subscription-contract">
      <CardHeader>
        <CardTitle className="text-base">
          {t("catalog.subscriptions.detail.contract")}
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-wrap items-center gap-3">
        <span className="font-mono" dir="ltr">
          #{contract.contract_no}
        </span>
        <StatusChip
          label={t(`services.contract.status.${contract.status}`)}
          tone={CONTRACT_TONE[contract.status]}
        />
        {canDownload ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={busy}
            data-testid="subscription-contract-pdf"
            onClick={() => void download()}
          >
            <FileDown className="size-4" />
            {t("catalog.subscriptions.detail.contract_pdf")}
          </Button>
        ) : (
          <span className="text-muted-foreground text-sm">
            {t(
              contract.status === "pending"
                ? "catalog.subscriptions.detail.contract_pending"
                : "catalog.subscriptions.detail.contract_pdf_not_ready",
            )}
          </span>
        )}
      </CardContent>
    </Card>
  );
}

/**
 * Subscription detail (TEC-311): terms frozen at assignment, status, the
 * subscription contract (status + PDF) and the early cancellation request.
 * The period list waits for the period accounting API (F3-08d, TEC-308).
 */
export function SubscriptionDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const canRead = can(permissions.serviceSubscriptions.read);
  const [cancelOpen, setCancelOpen] = useState(false);

  const detail = useQuery({
    queryKey: serviceSubscriptionKeys.detail(uuid),
    queryFn: () => serviceSubscriptionsService.get(uuid),
    enabled: canRead,
  });
  const sub = detail.data;

  const listTitle = t("catalog.subscriptions.title");
  const header = (
    <PageHeader
      title={sub?.item_name ?? t("catalog.subscriptions.detail.title")}
      icon={<Repeat className="size-6" />}
      description={sub?.organization_name}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: listTitle,
          href: routes.tenant.serviceSubscriptions.list(slug),
        },
        { label: sub?.item_name ?? t("catalog.subscriptions.detail.title") },
      ]}
      actions={
        sub &&
        canRequestCancel(
          sub,
          org?.uuid,
          can(permissions.serviceSubscriptions.cancelRequest),
        ) ? (
          <Button
            type="button"
            variant="destructive"
            data-testid="open-cancel-request"
            onClick={() => setCancelOpen(true)}
          >
            <Ban className="size-4" />
            {t("catalog.subscriptions.cancel.action")}
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
          description={t("catalog.subscriptions.forbidden")}
        />
      </div>
    );
  }
  if (detail.isLoading) {
    return (
      <div className="space-y-6">
        {header}
        <Loading />
      </div>
    );
  }
  if (!sub) {
    const notFound = isApiError(detail.error) && detail.error.status === 404;
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t(
            notFound
              ? "catalog.subscriptions.detail.not_found"
              : "catalog.subscriptions.detail.load_failed",
          )}
          onRetry={notFound ? undefined : () => void detail.refetch()}
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-3 text-base">
            {t("catalog.subscriptions.detail.terms")}
            <SubscriptionStatusChip status={sub.status} />
          </CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <Field label={t("catalog.subscriptions.fields.organization")}>
              {sub.organization_name}
            </Field>
            <Field label={t("catalog.subscriptions.fields.item")}>
              {sub.item_name}
            </Field>
            <Field label={t("catalog.subscriptions.fields.recurrence")}>
              {t(`catalog.services.recurrences.${sub.recurrence}`)}
            </Field>
            <Field label={t("catalog.subscriptions.fields.starts_on")}>
              {format.date(sub.starts_on)}
            </Field>
            <Field label={t("catalog.subscriptions.fields.ends_on")}>
              {format.date(sub.ends_on)}
            </Field>
            <Field label={t("catalog.subscriptions.fields.price")}>
              <span className="tabular-nums">
                {format.currency(amountNumber(sub.price), sub.currency)}
              </span>
            </Field>
            <Field label={t("catalog.subscriptions.fields.cancellation_fee")}>
              <span className="tabular-nums">
                {format.currency(
                  amountNumber(sub.cancellation_fee),
                  sub.currency,
                )}
              </span>
            </Field>
            <Field label={t("catalog.fields.created_at")}>
              {format.dateTime(sub.created_at)}
            </Field>
          </dl>
        </CardContent>
      </Card>
      {sub.contract ? <ContractCard contract={sub.contract} /> : null}
      <CancelRequestDialog
        key={cancelOpen ? "open" : "closed"}
        subscription={cancelOpen ? sub : null}
        onClose={() => setCancelOpen(false)}
      />
    </div>
  );
}
