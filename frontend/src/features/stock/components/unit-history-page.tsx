"use client";

import { useInfiniteQuery } from "@tanstack/react-query";
import { ArrowRight, Barcode, FileText } from "lucide-react";
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
import { UnitOperations } from "@/features/stock/components/unit-operations";
import { unitStatusTone } from "@/features/stock/lib/stock";
import { movementDocumentHref } from "@/features/stock/lib/stock-ops";
import {
  HISTORY_PAGE_SIZE,
  stockOpsKeys,
  stockOpsService,
  unitLabelPath,
  type StockMovement,
  type StockOwner,
  type StockUnitHistory,
} from "@/features/stock/services/stock-ops.service";
import type { StockUnitStatus } from "@/features/stock/services/stock.service";
import { LabelButton } from "@/features/warehouse/components/label-button";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type T = ReturnType<typeof useLocale>["t"];

/** An owner as text: location code, organization, service or masked. */
export function ownerText(owner: StockOwner | null | undefined, t: T): string {
  if (!owner) return "—";
  if (owner.masked) return t("stock.history.masked");
  const org = owner.organization?.name;
  switch (owner.type) {
    case "warehouse_location":
      return owner.location
        ? `${owner.location.code}${org ? ` · ${org}` : ""}`
        : (org ?? "—");
    case "service":
      return t("stock.history.owner_service");
    case "trash":
      return t("stock.history.owner_trash");
    default:
      return org ?? "—";
  }
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[9rem_1fr] gap-2 text-sm">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="min-w-0 break-words">{children}</dd>
    </div>
  );
}

/**
 * Stock > Unit (TEC-763): one barcode's product, status, holder and
 * location, meters or holdings, its operations (label, product fix, roll
 * split) and the movement timeline in ledger order (TEC-155; rows of
 * organizations outside the viewer's reach come masked). The timeline is
 * a DataTable exception (AGENTS.md §6).
 */
export function UnitHistoryPage({
  slug,
  barcode,
}: {
  slug: string;
  barcode: string;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canRead = can(Permission.StockRead);

  const history = useInfiniteQuery({
    queryKey: stockOpsKeys.unit(barcode),
    queryFn: ({ pageParam }) => stockOpsService.unitHistory(barcode, pageParam),
    initialPageParam: 0,
    getNextPageParam: (last) => {
      const next = last.movements.offset + last.movements.items.length;
      return next < last.movements.total && last.movements.items.length > 0
        ? next
        : undefined;
    },
    enabled: canRead && barcode !== "",
  });

  const title = barcode || t("stock.unit.title");
  const header = (
    <PageHeader
      title={title}
      icon={<Barcode className="size-6" />}
      description={t("stock.unit.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("stock.nav"), href: routes.tenant.stock.root(slug) },
        { label: title },
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

  const first = history.data?.pages[0];
  const movements = history.data?.pages.flatMap((p) => p.movements.items) ?? [];

  return (
    <div className="space-y-6">
      {header}
      {history.isLoading ? (
        <Skeleton className="h-48 w-full" />
      ) : history.isError || !first ? (
        <ErrorState
          title={
            isApiError(history.error) && history.error.status === 404
              ? t("stock.unit.not_found")
              : t("stock.unit.load_failed")
          }
          description={t("stock.unit.not_found_hint")}
          onRetry={() => void history.refetch()}
        />
      ) : (
        <>
          <UnitSummary slug={slug} data={first} />
          <Card>
            <CardHeader>
              <CardTitle>
                {t("stock.history.title", {
                  total: first.movements.total,
                })}
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              {movements.length === 0 ? (
                <p className="text-muted-foreground text-sm">
                  {t("stock.history.empty")}
                </p>
              ) : (
                <ol className="space-y-3" data-testid="unit-timeline">
                  {movements.map((m) => (
                    <MovementItem key={m.uuid} slug={slug} movement={m} />
                  ))}
                </ol>
              )}
              {history.hasNextPage ? (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={history.isFetchingNextPage}
                  onClick={() => void history.fetchNextPage()}
                >
                  {t("stock.history.more", { size: HISTORY_PAGE_SIZE })}
                </Button>
              ) : null}
            </CardContent>
          </Card>
        </>
      )}
    </div>
  );
}

function UnitSummary({ slug, data }: { slug: string; data: StockUnitHistory }) {
  const { t, format } = useLocale();
  const { unit, current, holdings } = data;
  const status = (current?.status ?? unit.status) as StockUnitStatus;
  const roll = unit.product.unit_type === "roll_meter";

  return (
    <Card data-testid="unit-summary">
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
        <CardTitle className="flex items-center gap-2">
          <span className="font-mono" dir="ltr">
            {unit.barcode}
          </span>
          <StatusChip
            label={t(`stock.status.${status}`)}
            tone={unitStatusTone(status)}
          />
        </CardTitle>
        <LabelButton
          path={unitLabelPath(unit.barcode)}
          filename={`${unit.barcode}.pdf`}
        />
      </CardHeader>
      <CardContent className="space-y-4">
        <dl className="space-y-2">
          <Row label={t("stock.columns.product")}>
            {unit.product.name}{" "}
            <span className="text-muted-foreground font-mono text-xs" dir="ltr">
              {unit.product.sku}
            </span>
          </Row>
          <Row label={t("stock.unit.kind")}>
            {t(`stock.unit_kind.${unit.unit_kind}`)}
          </Row>
          {current ? (
            <>
              <Row label={t("stock.unit.holder")}>{current.holder.name}</Row>
              <Row label={t("stock.unit.owner")}>
                <span data-testid="unit-owner">
                  {ownerText(current.owner, t)}
                </span>
              </Row>
              <Row label={t("stock.columns.updated")}>
                {format.dateTime(current.updated_at)}
              </Row>
            </>
          ) : null}
          {roll ? (
            <Row label={t("stock.columns.meters")}>
              <span data-testid="unit-meters">
                {unit.remaining_meters
                  ? t("stock.list.meters_of", {
                      remaining: unit.remaining_meters,
                      initial: unit.initial_meters ?? unit.remaining_meters,
                    })
                  : "—"}
              </span>
            </Row>
          ) : null}
          {holdings.length > 0 ? (
            <Row label={t("stock.unit.holdings")}>
              <ul className="space-y-1">
                {holdings.map((h, i) => (
                  <li key={`${h.holder.uuid}-${i}`}>
                    {h.holder.name} · {ownerText(h.owner, t)} ·{" "}
                    {format.number(h.quantity)}
                  </li>
                ))}
              </ul>
            </Row>
          ) : null}
        </dl>
        <UnitOperations
          slug={slug}
          unit={{
            barcode: unit.barcode,
            status,
            remaining_meters: unit.remaining_meters,
            product: unit.product,
          }}
        />
      </CardContent>
    </Card>
  );
}

function MovementItem({
  slug,
  movement: m,
}: {
  slug: string;
  movement: StockMovement;
}) {
  const { t, format } = useLocale();
  const href = m.document ? movementDocumentHref(slug, m.document) : null;
  const meters = Number(m.meters_delta);
  return (
    <li
      className="border-s-2 ps-4"
      data-testid="unit-movement"
      data-type={m.type}
    >
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <span className="font-medium">{t(`stock.movement.${m.type}`)}</span>
        <span className="text-muted-foreground text-xs whitespace-nowrap">
          {format.dateTime(m.created_at)}
        </span>
      </div>
      <div className="text-muted-foreground flex flex-wrap items-center gap-1 text-sm">
        <span>{ownerText(m.from_owner, t)}</span>
        <ArrowRight className="size-3 shrink-0 rtl:rotate-180" aria-hidden />
        <span>{ownerText(m.to_owner, t)}</span>
        {m.to_status ? <span>· {t(`stock.status.${m.to_status}`)}</span> : null}
        {meters !== 0 ? (
          <span dir="ltr">
            · {meters > 0 ? "+" : ""}
            {m.meters_delta} m
          </span>
        ) : m.quantity_delta !== 0 ? (
          <span dir="ltr">
            · {m.quantity_delta > 0 ? "+" : ""}
            {m.quantity_delta}
          </span>
        ) : null}
      </div>
      <div className="text-muted-foreground flex flex-wrap gap-x-3 text-xs">
        {m.organization_masked ? (
          <span>{t("stock.history.masked")}</span>
        ) : m.organization ? (
          <span>{m.organization.name}</span>
        ) : null}
        {m.actor ? (
          <span>{t("stock.history.by", { name: m.actor.name })}</span>
        ) : null}
        {m.document && href ? (
          <Link
            href={href}
            className="inline-flex items-center gap-1 hover:underline"
            data-testid="unit-movement-document"
          >
            <FileText className="size-3" />
            {t(`stock.history.document.${m.document.type}`)}
          </Link>
        ) : null}
      </div>
      {m.reason ? <p className="text-sm">{m.reason}</p> : null}
    </li>
  );
}
