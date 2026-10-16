"use client";

import { useQuery } from "@tanstack/react-query";
import { Ban, History, Replace, ShieldCheck, XCircle } from "lucide-react";
import Link from "next/link";
import type { ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useReclassDecisions } from "@/features/stock/components/use-reclass-decisions";
import { reclassStatusTone } from "@/features/stock/lib/stock-ops";
import {
  stockOpsKeys,
  stockOpsService,
} from "@/features/stock/services/stock-ops.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[10rem_1fr] gap-2 text-sm">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="min-w-0 break-words">{children}</dd>
    </div>
  );
}

/**
 * Stock > Reclassification request (TEC-157, TEC-763): the unit, source
 * and target products, reason, requester and decision, with approve
 * (stock.reclassify + step-up; never for the requester, who sees why),
 * reject (with a reason) and cancel (the requesting organization).
 */
export function ReclassificationDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const canRead = can(Permission.StockRead);
  const { actionsFor, decide, pending } = useReclassDecisions(slug);

  const detail = useQuery({
    queryKey: stockOpsKeys.reclassification(uuid),
    queryFn: () => stockOpsService.getReclassification(uuid),
    enabled: canRead && uuid !== "",
  });

  const title = t("stock.reclass.detail_title");
  const header = (
    <PageHeader
      title={title}
      icon={<Replace className="size-6" />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("stock.reclass.list_title"),
          href: routes.tenant.stock.reclassifications(slug),
        },
        { label: detail.data?.unit.barcode ?? title },
      ]}
    />
  );

  if (!canRead) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("stock.page.forbidden")}
        />
      </div>
    );
  }
  if (detail.isLoading) {
    return (
      <div className="space-y-6">
        {header}
        <Skeleton className="h-48 w-full" />
      </div>
    );
  }
  const r = detail.data;
  if (detail.isError || !r) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("stock.reclass.not_found")}
          onRetry={() => void detail.refetch()}
        />
      </div>
    );
  }

  const allowed = actionsFor(r);

  return (
    <div className="space-y-6">
      {header}
      <Card data-testid="reclass-detail" data-status={r.status}>
        <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
          <CardTitle className="flex items-center gap-2">
            <span className="font-mono" dir="ltr">
              {r.unit.barcode}
            </span>
            <StatusChip
              label={t(`stock.reclass.status.${r.status}`)}
              tone={reclassStatusTone(r.status)}
            />
          </CardTitle>
          <div className="flex flex-wrap gap-2">
            <Button asChild size="sm" variant="outline">
              <Link href={routes.tenant.stock.unit(slug, r.unit.barcode)}>
                <History className="size-4" />
                {t("stock.unit.history")}
              </Link>
            </Button>
            {allowed.approve ? (
              <Button
                type="button"
                size="sm"
                disabled={pending}
                onClick={() => void decide(r, "approve")}
                data-testid="reclass-approve"
              >
                <ShieldCheck className="size-4" />
                {t("stock.reclass.approve")}
              </Button>
            ) : null}
            {allowed.reject ? (
              <Button
                type="button"
                size="sm"
                variant="destructive"
                disabled={pending}
                onClick={() => void decide(r, "reject")}
                data-testid="reclass-reject"
              >
                <XCircle className="size-4" />
                {t("stock.reclass.reject")}
              </Button>
            ) : null}
            {allowed.cancel ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={pending}
                onClick={() => void decide(r, "cancel")}
                data-testid="reclass-cancel"
              >
                <Ban className="size-4" />
                {t("stock.reclass.cancel")}
              </Button>
            ) : null}
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {allowed.selfRequested ? (
            <p
              className="text-muted-foreground text-sm"
              data-testid="reclass-self-hint"
            >
              {t("stock.reclass.self_hint")}
            </p>
          ) : null}
          <dl className="space-y-2">
            <Row label={t("stock.reclass.columns.from")}>
              {r.from_product.name}{" "}
              <span className="text-muted-foreground font-mono text-xs">
                {r.from_product.sku}
              </span>
            </Row>
            <Row label={t("stock.reclass.columns.to")}>
              {r.to_product.name}{" "}
              <span className="text-muted-foreground font-mono text-xs">
                {r.to_product.sku}
              </span>
            </Row>
            <Row label={t("stock.reclass.reason")}>{r.reason}</Row>
            <Row label={t("stock.reclass.columns.organization")}>
              {r.organization.name}
            </Row>
            <Row label={t("stock.reclass.columns.requested_by")}>
              {r.requested_by?.name ?? "—"}
            </Row>
            <Row label={t("stock.reclass.columns.created")}>
              {format.dateTime(r.created_at)}
            </Row>
            {r.decided_at ? (
              <>
                <Row label={t("stock.reclass.decided_by")}>
                  {r.decided_by?.name ?? "—"}
                </Row>
                <Row label={t("stock.reclass.columns.decided")}>
                  {format.dateTime(r.decided_at)}
                </Row>
              </>
            ) : null}
            {r.decision_note ? (
              <Row label={t("stock.reclass.decision_note")}>
                {r.decision_note}
              </Row>
            ) : null}
          </dl>
        </CardContent>
      </Card>
    </div>
  );
}
