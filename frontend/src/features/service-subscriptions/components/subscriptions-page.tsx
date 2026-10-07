"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Ban, Eye, Inbox, Plus, Repeat } from "lucide-react";
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
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { serviceCatalogService } from "@/features/service-catalog/services/service-catalog.service";
import { AssignSubscriptionDialog } from "@/features/service-subscriptions/components/assign-subscription-dialog";
import { CancelRequestDialog } from "@/features/service-subscriptions/components/cancel-request-dialog";
import { SubscriptionStatusChip } from "@/features/service-subscriptions/components/subscription-status";
import {
  amountNumber,
  canAssignFrom,
  canRequestCancel,
} from "@/features/service-subscriptions/lib/subscriptions";
import {
  SUBSCRIPTION_STATUSES,
  serviceSubscriptionKeys,
  serviceSubscriptionsService,
  type ServiceSubscription,
} from "@/features/service-subscriptions/services/service-subscriptions.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const SUBSCRIPTIONS_PERSIST_KEY = "tenant-service-subscriptions-v1";

/**
 * "Hizmet abonelikleri" (TEC-311). Center: every subscription of the brand;
 * distributor: its own and its dealers'; dealer: its own. Server DataTable
 * (sort, search, status / organization / item / end date filters, row
 * actions, grid cards). Center and distributor assign ("Ata"); the
 * subscriber requests early cancellation of its own active subscription.
 */
export function SubscriptionsPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const org = useActiveOrganization(slug);
  const canRead = can(permissions.serviceSubscriptions.read);
  const canAssign =
    can(permissions.serviceSubscriptions.assign) && canAssignFrom(org?.type);
  const canCancel = can(permissions.serviceSubscriptions.cancelRequest);
  const canQueue =
    org?.type === "center" &&
    can(permissions.serviceSubscriptions.cancelApprove);
  const canFilterOrg =
    org?.type !== "dealer" && can(permissions.organizations.tenantRead);

  const [assignOpen, setAssignOpen] = useState(false);
  const [cancelTarget, setCancelTarget] = useState<ServiceSubscription | null>(
    null,
  );

  const organizations = useQuery({
    queryKey: serviceSubscriptionKeys.organizations(),
    queryFn: () => serviceSubscriptionsService.listOrganizations(),
    enabled: canRead && canFilterOrg,
    staleTime: 5 * 60_000,
  });
  const orgOptions = useMemo(
    () =>
      (organizations.data ?? []).map((o) => ({ value: o.uuid, label: o.name })),
    [organizations.data],
  );
  const catalog = useQuery({
    queryKey: serviceSubscriptionKeys.items(),
    queryFn: () => serviceCatalogService.listVisible(),
    enabled: canRead && can(permissions.serviceCatalog.read),
    staleTime: 5 * 60_000,
  });
  const itemOptions = useMemo(
    () =>
      (catalog.data?.items ?? []).map((i) => ({
        value: i.uuid,
        label: i.name,
      })),
    [catalog.data?.items],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<ServiceSubscription>({
          accessorKey: "item_name",
          labelKey: "catalog.subscriptions.fields.item",
          enableSorting: true,
          gridPrimary: true,
          filterVariant: "faceted",
          filterOptions: itemOptions,
          enableColumnFilter: itemOptions.length > 0,
          param: "item_uuid",
          cell: ({ row }) => (
            <Link
              href={routes.tenant.serviceSubscriptions.detail(
                slug,
                row.original.uuid,
              )}
              className="font-medium hover:underline"
              data-testid="subscription-row"
              data-uuid={row.original.uuid}
              onClick={(event) => event.stopPropagation()}
            >
              {row.original.item_name}
            </Link>
          ),
        }),
        createColumn<ServiceSubscription>({
          accessorKey: "organization_name",
          labelKey: "catalog.subscriptions.fields.organization",
          enableSorting: true,
          gridSecondary: true,
          filterVariant: "faceted",
          filterOptions: orgOptions,
          enableColumnFilter: canFilterOrg && orgOptions.length > 0,
          param: "organization_uuid",
          cell: ({ row }) => row.original.organization_name,
        }),
        createColumn<ServiceSubscription>({
          accessorKey: "status",
          labelKey: "catalog.subscriptions.fields.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: SUBSCRIPTION_STATUSES.map((value) => ({
            value,
            label: value,
            labelKey: `catalog.subscriptions.status.${value}`,
          })),
          param: "status",
          cell: ({ row }) => (
            <SubscriptionStatusChip status={row.original.status} />
          ),
        }),
        createColumn<ServiceSubscription>({
          accessorKey: "starts_on",
          labelKey: "catalog.subscriptions.fields.starts_on",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.date(row.original.starts_on)}
            </span>
          ),
        }),
        createColumn<ServiceSubscription>({
          accessorKey: "ends_on",
          labelKey: "catalog.subscriptions.fields.ends_on",
          enableSorting: true,
          filterVariant: "date-range",
          param: "ends_on",
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.date(row.original.ends_on)}
            </span>
          ),
        }),
        createColumn<ServiceSubscription>({
          id: "price",
          accessorFn: (s) => amountNumber(s.price),
          labelKey: "catalog.subscriptions.fields.price",
          enableSorting: true,
          meta: { cellClassName: "text-end", headerClassName: "text-end" },
          cell: ({ row }) => (
            <span className="whitespace-nowrap tabular-nums">
              {format.currency(
                amountNumber(row.original.price),
                row.original.currency,
              )}
            </span>
          ),
        }),
        createColumn<ServiceSubscription>({
          accessorKey: "recurrence",
          labelKey: "catalog.subscriptions.fields.recurrence",
          enableSorting: false,
          cell: ({ row }) =>
            t(`catalog.services.recurrences.${row.original.recurrence}`),
        }),
        createColumn<ServiceSubscription>({
          id: "contract",
          accessorFn: (s) => s.contract?.status ?? "",
          labelKey: "catalog.subscriptions.fields.contract",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.contract
              ? t(`services.contract.status.${row.original.contract.status}`)
              : "—",
        }),
        createColumn<ServiceSubscription>({
          accessorKey: "created_at",
          labelKey: "catalog.fields.created_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => format.dateTime(row.original.created_at),
        }),
        createColumn<ServiceSubscription>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const sub = row.original;
            const items: EntityRowAction[] = [
              {
                id: "view",
                label: t("common.view"),
                icon: Eye,
                onSelect: () =>
                  router.push(
                    routes.tenant.serviceSubscriptions.detail(slug, sub.uuid),
                  ),
              },
            ];
            if (canRequestCancel(sub, org?.uuid, canCancel)) {
              items.push({
                id: "cancel-request",
                label: t("catalog.subscriptions.cancel.action"),
                icon: Ban,
                variant: "destructive",
                onSelect: () => setCancelTarget(sub),
              });
            }
            return <EntityRowActions actions={items} />;
          },
        }),
      ] as ColumnDef<ServiceSubscription, unknown>[],
    [
      canCancel,
      canFilterOrg,
      format,
      itemOptions,
      org?.uuid,
      orgOptions,
      router,
      slug,
      t,
    ],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: 20,
    persistKey: SUBSCRIPTIONS_PERSIST_KEY,
  });
  const params = listState.params;
  const list = useQuery({
    queryKey: serviceSubscriptionKeys.list(params),
    queryFn: () => serviceSubscriptionsService.list(params),
    enabled: canRead,
  });
  const total = list.data?.total ?? 0;

  const title = t("catalog.subscriptions.title");
  const header = (
    <PageHeader
      title={title}
      icon={<Repeat className="size-6" />}
      description={t(
        org?.type === "dealer"
          ? "catalog.subscriptions.description_dealer"
          : "catalog.subscriptions.description",
      )}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
      actions={
        <div className="flex flex-wrap gap-2">
          {canQueue ? (
            <Button asChild variant="outline">
              <Link
                href={routes.tenant.serviceSubscriptions.cancelRequests(slug)}
              >
                <Inbox className="size-4" />
                {t("catalog.subscriptions.queue.title")}
              </Link>
            </Button>
          ) : null}
          {canAssign ? (
            <Button
              type="button"
              data-testid="open-assign"
              onClick={() => setAssignOpen(true)}
            >
              <Plus className="size-4" />
              {t("catalog.subscriptions.assign.open")}
            </Button>
          ) : null}
        </div>
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

  return (
    <div className="space-y-6">
      {header}
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.tenant.serviceSubscriptions.detail(slug, row.uuid))
        }
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("catalog.subscriptions.empty_title")}
        emptyDescription={
          listState.columnFilters.length > 0 || params.q
            ? t("catalog.subscriptions.empty_filtered")
            : t("catalog.subscriptions.empty_description")
        }
        rowCount={total}
        state={listState.tableState}
        features={{
          persistKey: SUBSCRIPTIONS_PERSIST_KEY,
          rowSelection: false,
          viewMode: true,
        }}
        renderGridItem={(sub) => (
          <div className="space-y-2">
            <div className="flex items-start justify-between gap-2">
              <span className="font-semibold">{sub.item_name}</span>
              <SubscriptionStatusChip status={sub.status} />
            </div>
            <p className="text-sm">{sub.organization_name}</p>
            <div className="text-muted-foreground flex justify-between gap-2 text-xs">
              <span>
                {format.date(sub.starts_on)} – {format.date(sub.ends_on)}
              </span>
              <span className="text-foreground font-medium tabular-nums">
                {format.currency(amountNumber(sub.price), sub.currency)}
              </span>
            </div>
          </div>
        )}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void list.refetch()}
            refreshDisabled={list.isFetching}
          />
        }
      />
      {canAssign ? (
        <AssignSubscriptionDialog
          key={assignOpen ? "open" : "closed"}
          open={assignOpen}
          onOpenChange={setAssignOpen}
          orgType={org?.type}
        />
      ) : null}
      <CancelRequestDialog
        key={cancelTarget?.uuid ?? "none"}
        subscription={cancelTarget}
        onClose={() => setCancelTarget(null)}
      />
    </div>
  );
}
