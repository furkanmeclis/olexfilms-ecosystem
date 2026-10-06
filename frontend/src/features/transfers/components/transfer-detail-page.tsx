"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeftRight } from "lucide-react";
import { useState, type ReactNode } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { TransferItemsTable } from "@/features/transfers/components/transfer-items-table";
import { ReasonDialog } from "@/features/transfers/components/transfer-reason-dialog";
import {
  transferErrorMessage,
  transferStatusTone,
  transitionAsksReason,
  transitionButtons,
  transitionIsDestructive,
} from "@/features/transfers/lib/transfers";
import {
  transferKeys,
  transfersService,
  type StockTransfer,
  type StockTransferStatus,
} from "@/features/transfers/services/transfers.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm">{children}</dd>
    </div>
  );
}

/**
 * Tenant > Transfers > detail (TEC-197): parties, status, units and the
 * actions the server allows the active organization (available_transitions:
 * approve/reject, ship, receive, cancel). Reject and cancel ask for an
 * optional reason.
 */
export function TransferDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const qc = useQueryClient();
  const canRead =
    can(Permission.TransfersRequest) || can(Permission.TransfersApprove);
  const [reasonFor, setReasonFor] = useState<StockTransferStatus | null>(null);
  const detail = useQuery({
    queryKey: transferKeys.detail(uuid),
    queryFn: () => transfersService.get(uuid),
    enabled: canRead && uuid !== "",
    retry: (count, err) =>
      !(isApiError(err) && (err.status === 404 || err.status === 403)) &&
      count < 2,
  });
  const move = useMutation({
    mutationFn: (v: { status: StockTransferStatus; reason?: string }) =>
      transfersService.transition(uuid, v.status, v.reason),
    onSuccess: (updated: StockTransfer) => {
      qc.setQueryData(transferKeys.detail(uuid), updated);
      void qc.invalidateQueries({ queryKey: transferKeys.all });
      toast.success(t(`transfers.action_done.${updated.status}`));
      setReasonFor(null);
    },
    onError: (err) => {
      toast.error(transferErrorMessage(err, t, t("transfers.detail.error")));
    },
  });

  const r = detail.data;
  const listTitle = t("transfers.list.title");
  const header = (
    <PageHeader
      title={r ? r.transfer_no : t("transfers.detail.title")}
      icon={<ArrowLeftRight className="size-6" />}
      description={r ? `${r.sender.name} → ${r.receiver.name}` : undefined}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: listTitle, href: routes.tenant.transfers.list(slug) },
        { label: r ? r.transfer_no : t("transfers.detail.title") },
      ]}
      actions={
        r ? (
          <div className="flex flex-wrap gap-2" data-testid="transfer-actions">
            {transitionButtons(r).map((status) => (
              <Button
                key={status}
                type="button"
                variant={
                  transitionIsDestructive(status) ? "destructive" : "default"
                }
                data-transition={status}
                disabled={move.isPending}
                onClick={() =>
                  transitionAsksReason(status)
                    ? setReasonFor(status)
                    : move.mutate({ status })
                }
              >
                {t(`transfers.action.${status}`)}
              </Button>
            ))}
          </div>
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
          description={t("transfers.list.forbidden")}
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
          title={
            notFound
              ? t("transfers.detail.not_found")
              : t("common.error_generic")
          }
          onRetry={notFound ? undefined : () => void detail.refetch()}
          retryLabel={t("common.retry")}
        />
      </div>
    );
  }
  if (!r) {
    return (
      <div className="space-y-6">
        {header}
        <p className="text-muted-foreground text-sm">
          {t("transfers.list.loading")}
        </p>
      </div>
    );
  }

  const when = (v: string | null | undefined) => (v ? format.dateTime(v) : "—");

  return (
    <div className="space-y-6" data-testid="transfer-detail">
      {header}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-2">
          <CardTitle>{t("transfers.detail.summary")}</CardTitle>
          <StatusChip
            label={t(`transfers.status.${r.status}`)}
            tone={transferStatusTone(r.status)}
          />
        </CardHeader>
        <CardContent>
          <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <Field label={t("transfers.kind.label")}>
              <span data-testid="transfer-kind">
                {t(`transfers.kind.${r.kind}`)}
              </span>
            </Field>
            <Field label={t("transfers.columns.sender")}>{r.sender.name}</Field>
            <Field label={t("transfers.columns.receiver")}>
              {r.receiver.name}
            </Field>
            <Field label={t("transfers.detail.parent")}>{r.parent.name}</Field>
            <Field label={t("transfers.detail.role")}>
              {t(`transfers.role.${r.role}`)}
            </Field>
            <Field label={t("transfers.detail.total")}>
              {r.total ? (
                <span dir="ltr">
                  {r.total} {r.currency}
                </span>
              ) : (
                "—"
              )}
            </Field>
            <Field label={t("transfers.columns.created")}>
              {when(r.created_at)}
            </Field>
            <Field label={t("transfers.detail.decided_at")}>
              {when(r.decided_at)}
            </Field>
            <Field label={t("transfers.detail.shipped_at")}>
              {when(r.shipped_at)}
            </Field>
            <Field label={t("transfers.detail.received_at")}>
              {when(r.received_at)}
            </Field>
          </dl>
          {r.note || r.decision_note || r.cancel_reason ? (
            <dl className="mt-4 grid gap-4 sm:grid-cols-3">
              {r.note ? (
                <Field label={t("transfers.form.note")}>{r.note}</Field>
              ) : null}
              {r.decision_note ? (
                <Field label={t("transfers.detail.decision_note")}>
                  {r.decision_note}
                </Field>
              ) : null}
              {r.cancel_reason ? (
                <Field label={t("transfers.detail.cancel_reason")}>
                  {r.cancel_reason}
                </Field>
              ) : null}
            </dl>
          ) : null}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{t("transfers.form.units")}</CardTitle>
        </CardHeader>
        <CardContent>
          <TransferItemsTable items={r.items ?? []} currency={r.currency} />
        </CardContent>
      </Card>
      <ReasonDialog
        key={reasonFor ?? "closed"}
        target={reasonFor}
        pending={move.isPending}
        onClose={() => setReasonFor(null)}
        onConfirm={(reason) =>
          reasonFor &&
          move.mutate({ status: reasonFor, reason: reason || undefined })
        }
      />
    </div>
  );
}
