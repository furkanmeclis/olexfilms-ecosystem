"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Check, Eye, Inbox, X } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  CancelDecisionDialog,
  type CancelDecision,
} from "@/features/service-subscriptions/components/cancel-decision-dialog";
import { CancelRequestStatusChip } from "@/features/service-subscriptions/components/subscription-status";
import { amountNumber } from "@/features/service-subscriptions/lib/subscriptions";
import {
  CANCEL_REQUEST_STATUSES,
  serviceSubscriptionKeys,
  serviceSubscriptionsService,
  type CancelQueueItem,
} from "@/features/service-subscriptions/services/service-subscriptions.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const CANCEL_REQUESTS_PERSIST_KEY = "tenant-subscription-cancels-v1";

/** The queue opens on the requests still waiting for a decision. */
const INITIAL_FILTERS = [{ id: "status", value: ["pending"] }];

/**
 * Center "İptal talepleri" queue (TEC-311): early cancellation requests of
 * the brand over a server DataTable (sort, search, status / created
 * filters); pending rows are approved or rejected (reject needs a reason).
 */
export function CancelRequestsPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const org = useActiveOrganization(slug);
  const allowed =
    org?.type === "center" &&
    can(permissions.serviceSubscriptions.cancelApprove);
  const [decision, setDecision] = useState<{
    request: CancelQueueItem;
    decision: CancelDecision;
  } | null>(null);

  const columns = useMemo(
    () =>
      [
        createColumn<CancelQueueItem>({
          accessorKey: "organization_name",
          labelKey: "catalog.subscriptions.fields.organization",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">
              {row.original.organization_name}
            </span>
          ),
        }),
        createColumn<CancelQueueItem>({
          accessorKey: "item_name",
          labelKey: "catalog.subscriptions.fields.item",
          enableSorting: true,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.serviceSubscriptions.detail(
                slug,
                row.original.subscription_uuid,
              )}
              className="hover:underline"
              onClick={(event) => event.stopPropagation()}
            >
              {row.original.item_name}
            </Link>
          ),
        }),
        createColumn<CancelQueueItem>({
          accessorKey: "reason",
          labelKey: "catalog.subscriptions.cancel.reason",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="line-clamp-2 max-w-md">{row.original.reason}</span>
          ),
        }),
        createColumn<CancelQueueItem>({
          id: "cancellation_fee",
          accessorFn: (r) => amountNumber(r.cancellation_fee),
          labelKey: "catalog.subscriptions.fields.cancellation_fee",
          enableSorting: true,
          meta: { cellClassName: "text-end", headerClassName: "text-end" },
          cell: ({ row }) => (
            <span className="whitespace-nowrap tabular-nums">
              {format.currency(
                amountNumber(row.original.cancellation_fee),
                row.original.currency,
              )}
            </span>
          ),
        }),
        createColumn<CancelQueueItem>({
          accessorKey: "status",
          labelKey: "catalog.subscriptions.fields.status",
          enableSorting: true,
          gridSecondary: true,
          filterVariant: "faceted",
          filterOptions: CANCEL_REQUEST_STATUSES.map((value) => ({
            value,
            label: value,
            labelKey: `catalog.subscriptions.request_status.${value}`,
          })),
          param: "status",
          cell: ({ row }) => (
            <CancelRequestStatusChip status={row.original.status} />
          ),
        }),
        createColumn<CancelQueueItem>({
          accessorKey: "ends_on",
          labelKey: "catalog.subscriptions.fields.ends_on",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => format.date(row.original.ends_on),
        }),
        createColumn<CancelQueueItem>({
          accessorKey: "decision_note",
          labelKey: "catalog.subscriptions.queue.decision_note",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.decision_note ?? "—",
        }),
        createColumn<CancelQueueItem>({
          accessorKey: "created_at",
          labelKey: "catalog.subscriptions.queue.requested_at",
          enableSorting: true,
          filterVariant: "date-range",
          param: "created",
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.dateTime(row.original.created_at)}
            </span>
          ),
        }),
        createColumn<CancelQueueItem>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const request = row.original;
            const items: EntityRowAction[] = [
              {
                id: "view",
                label: t("catalog.subscriptions.queue.view_subscription"),
                icon: Eye,
                onSelect: () =>
                  router.push(
                    routes.tenant.serviceSubscriptions.detail(
                      slug,
                      request.subscription_uuid,
                    ),
                  ),
              },
            ];
            if (request.status === "pending") {
              items.push(
                {
                  id: "approve",
                  label: t("catalog.subscriptions.queue.approve"),
                  icon: Check,
                  onSelect: () => setDecision({ request, decision: "approve" }),
                },
                {
                  id: "reject",
                  label: t("catalog.subscriptions.queue.reject"),
                  icon: X,
                  variant: "destructive",
                  onSelect: () => setDecision({ request, decision: "reject" }),
                },
              );
            }
            return <EntityRowActions actions={items} />;
          },
        }),
      ] as ColumnDef<CancelQueueItem, unknown>[],
    [format, router, slug, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: 20,
    initialColumnFilters: INITIAL_FILTERS,
    persistKey: CANCEL_REQUESTS_PERSIST_KEY,
  });
  const params = listState.params;
  const list = useQuery({
    queryKey: serviceSubscriptionKeys.cancelRequests(params),
    queryFn: () => serviceSubscriptionsService.listCancelRequests(params),
    enabled: allowed,
  });

  const title = t("catalog.subscriptions.queue.title");
  const header = (
    <PageHeader
      title={title}
      icon={<Inbox className="size-6" />}
      description={t("catalog.subscriptions.queue.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("catalog.subscriptions.title"),
          href: routes.tenant.serviceSubscriptions.list(slug),
        },
        { label: title },
      ]}
    />
  );

  if (!allowed) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("catalog.subscriptions.queue.forbidden")}
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("catalog.subscriptions.queue.empty_title")}
        emptyDescription={t("catalog.subscriptions.queue.empty_description")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: CANCEL_REQUESTS_PERSIST_KEY,
          rowSelection: false,
          viewMode: true,
        }}
        renderGridItem={(request) => (
          <div className="space-y-2">
            <div className="flex items-start justify-between gap-2">
              <span className="font-semibold">{request.organization_name}</span>
              <CancelRequestStatusChip status={request.status} />
            </div>
            <p className="text-sm">{request.item_name}</p>
            <p className="text-muted-foreground line-clamp-2 text-xs">
              {request.reason}
            </p>
          </div>
        )}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void list.refetch()}
            refreshDisabled={list.isFetching}
          />
        }
      />
      <CancelDecisionDialog
        key={
          decision ? `${decision.request.uuid}-${decision.decision}` : "none"
        }
        request={decision?.request ?? null}
        decision={decision?.decision ?? null}
        onClose={() => setDecision(null)}
      />
    </div>
  );
}
