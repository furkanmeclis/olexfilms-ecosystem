"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { useRouter } from "next/navigation";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { createColumn } from "@/components/tables";
import { entityHref, type EntityKind } from "@/config/routes";
import { useOptionalTenant } from "@/features/organizations/providers/tenant-provider";
import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api";
import type { Formatter } from "@/lib/i18n/format";
import { useLocale } from "@/providers/locale-provider";

import { canOpenEntity } from "./entity-registry";
import { EntityLink, useEntityPermissionCheck } from "./entity-link";
import { EntityTable } from "./entity-table";
import { refColumn } from "./ref-column";
import { useServerListState } from "./use-server-list-state";

type Schemas = components["schemas"];
type Translate = (key: string, params?: Record<string, unknown>) => string;

/** Query value of a reverse filter (e.g. `{ customer_uuid: "…" }`). */
export type RelatedFilter = Record<string, string | number>;

type Row = { uuid: string };

type ColumnCtx = { t: Translate; format: Formatter };

/**
 * One reverse-relation list: the existing list endpoint, its columns and
 * default sort. Columns follow the endpoint's sort / filter whitelist
 * (docs/list-contract.md); money columns are left out on purpose (TEC-1050,
 * TEC-1214).
 */
export type RelatedListSpec<T extends Row = Row> = {
  path: string;
  defaultSort: string;
  /** Column ids a caller may hide (e.g. `customer` on the customer page). */
  columns: (ctx: ColumnCtx) => ColumnDef<T, unknown>[];
  emptyKey: string;
};

function dateColumn<T extends Row>(
  id: string,
  labelKey: string,
  get: (row: T) => string | null | undefined,
  format: Formatter,
  sortable = true,
): ColumnDef<T, unknown> {
  return createColumn<T, unknown>({
    id,
    labelKey,
    accessorFn: get,
    enableSorting: sortable,
    cell: ({ row }) => {
      const value = get(row.original);
      return value ? format.date(value) : "—";
    },
  });
}

const serviceSpec: RelatedListSpec<Schemas["Service"]> = {
  path: "/v1/services",
  defaultSort: "-created_at",
  emptyKey: "entity.related.empty.service",
  columns: ({ format }) => [
    createColumn<Schemas["Service"], unknown>({
      accessorKey: "service_no",
      labelKey: "entity.related.columns.number",
      enableSorting: true,
      gridPrimary: true,
      cell: ({ row }) => (
        <EntityLink
          kind="service"
          entity={{
            uuid: row.original.uuid,
            name: row.original.service_no || row.original.uuid.slice(0, 8),
          }}
          className="font-mono"
        />
      ),
    }),
    createColumn<Schemas["Service"], unknown>({
      accessorKey: "status",
      labelKey: "entity.related.columns.status",
      enableSorting: true,
      cell: ({ row }) => <StatusChip label={row.original.status_label} />,
    }),
    createColumn<Schemas["Service"], unknown>({
      id: "plate",
      accessorFn: (row) => row.plate ?? "",
      labelKey: "entity.related.columns.vehicle",
      enableSorting: true,
      cell: ({ row }) => (
        <EntityLink
          kind="vehicle"
          entity={{
            uuid: row.original.vehicle_uuid,
            name:
              [
                row.original.plate,
                row.original.car_brand?.name,
                row.original.car_model?.name,
              ]
                .filter(Boolean)
                .join(" · ") || null,
          }}
        />
      ),
    }),
    refColumn<Schemas["Service"]>({
      id: "customer",
      kind: "customer",
      labelKey: "entity.related.columns.customer",
      getRef: (row) =>
        row.customer
          ? {
              uuid: row.customer.uuid,
              name: [row.customer.name, row.customer.surname]
                .filter(Boolean)
                .join(" "),
            }
          : null,
    }) as ColumnDef<Schemas["Service"], unknown>,
    refColumn<Schemas["Service"]>({
      id: "organization",
      kind: "organization",
      labelKey: "entity.related.columns.organization",
      enableSorting: true,
      getRef: (row) => row.organization,
    }) as ColumnDef<Schemas["Service"], unknown>,
    dateColumn<Schemas["Service"]>(
      "created_at",
      "entity.related.columns.created_at",
      (row) => row.created_at,
      format,
    ),
  ],
};

const warrantySpec: RelatedListSpec<Schemas["Warranty"]> = {
  path: "/v1/warranties",
  defaultSort: "expiry",
  emptyKey: "entity.related.empty.warranty",
  columns: ({ t, format }) => [
    createColumn<Schemas["Warranty"], unknown>({
      accessorKey: "public_code",
      labelKey: "entity.related.columns.number",
      enableSorting: true,
      gridPrimary: true,
      cell: ({ row }) => (
        <EntityLink
          kind="warranty"
          entity={{ uuid: row.original.uuid, name: row.original.public_code }}
          className="font-mono"
        />
      ),
    }),
    createColumn<Schemas["Warranty"], unknown>({
      accessorKey: "status",
      labelKey: "entity.related.columns.status",
      enableSorting: true,
      cell: ({ row }) => (
        <StatusChip label={t(`warranty.status.${row.original.status}`)} />
      ),
    }),
    refColumn<Schemas["Warranty"]>({
      id: "product",
      kind: "product",
      labelKey: "entity.related.columns.product",
      enableSorting: true,
      getRef: (row) => row.product,
    }) as ColumnDef<Schemas["Warranty"], unknown>,
    refColumn<Schemas["Warranty"]>({
      id: "vehicle",
      kind: "vehicle",
      labelKey: "entity.related.columns.vehicle",
      getRef: (row) =>
        row.vehicle
          ? {
              uuid: row.vehicle.uuid,
              name: row.vehicle.plate || null,
            }
          : null,
    }) as ColumnDef<Schemas["Warranty"], unknown>,
    refColumn<Schemas["Warranty"]>({
      id: "organization",
      kind: "organization",
      labelKey: "entity.related.columns.organization",
      enableSorting: true,
      getRef: (row) => row.organization,
    }) as ColumnDef<Schemas["Warranty"], unknown>,
    dateColumn<Schemas["Warranty"]>(
      "end_at",
      "entity.related.columns.end_at",
      (row) => row.end_at,
      format,
    ),
  ],
};

const orderSpec: RelatedListSpec<Schemas["Order"]> = {
  path: "/v1/orders",
  defaultSort: "-created_at",
  emptyKey: "entity.related.empty.order",
  columns: ({ format }) => [
    createColumn<Schemas["Order"], unknown>({
      accessorKey: "order_no",
      labelKey: "entity.related.columns.number",
      enableSorting: false,
      gridPrimary: true,
      cell: ({ row }) => (
        <EntityLink
          kind="order"
          entity={{ uuid: row.original.uuid, name: row.original.order_no }}
          className="font-mono"
        />
      ),
    }),
    createColumn<Schemas["Order"], unknown>({
      accessorKey: "status",
      labelKey: "entity.related.columns.status",
      enableSorting: false,
      cell: ({ row }) => <StatusChip label={row.original.status_label} />,
    }),
    refColumn<Schemas["Order"]>({
      id: "seller",
      kind: "organization",
      labelKey: "entity.related.columns.seller",
      getRef: (row) => row.seller,
    }) as ColumnDef<Schemas["Order"], unknown>,
    refColumn<Schemas["Order"]>({
      id: "buyer",
      kind: "organization",
      labelKey: "entity.related.columns.buyer",
      getRef: (row) => row.buyer,
    }) as ColumnDef<Schemas["Order"], unknown>,
    dateColumn<Schemas["Order"]>(
      "created_at",
      "entity.related.columns.created_at",
      (row) => row.created_at,
      format,
    ),
  ],
};

const taskSpec: RelatedListSpec<Schemas["Task"]> = {
  path: "/v1/tasks",
  defaultSort: "-created_at",
  emptyKey: "entity.related.empty.task",
  columns: ({ t, format }) => [
    createColumn<Schemas["Task"], unknown>({
      accessorKey: "title",
      labelKey: "entity.related.columns.title",
      enableSorting: true,
      gridPrimary: true,
      cell: ({ row }) => (
        <EntityLink
          kind="task"
          entity={{ uuid: row.original.uuid, name: row.original.title }}
        />
      ),
    }),
    createColumn<Schemas["Task"], unknown>({
      accessorKey: "status",
      labelKey: "entity.related.columns.status",
      enableSorting: true,
      cell: ({ row }) => (
        <StatusChip label={t(`tasks.status.${row.original.status}`)} />
      ),
    }),
    refColumn<Schemas["Task"]>({
      id: "assignee",
      kind: "user",
      labelKey: "entity.related.columns.assignee",
      getRef: (row) => row.assignee,
    }) as ColumnDef<Schemas["Task"], unknown>,
    refColumn<Schemas["Task"]>({
      id: "subject",
      kind: "organization",
      labelKey: "entity.related.columns.organization",
      getRef: (row) => row.subject_organization,
    }) as ColumnDef<Schemas["Task"], unknown>,
    dateColumn<Schemas["Task"]>(
      "created_at",
      "entity.related.columns.created_at",
      (row) => row.created_at,
      format,
    ),
  ],
};

/**
 * TEC-1217: kinds `RelatedList` can show. Add a kind here when its list
 * endpoint gains the reverse filter you need (TEC-1216).
 */
export const RELATED_LIST_SPECS: Partial<Record<EntityKind, RelatedListSpec>> =
  {
    service: serviceSpec as unknown as RelatedListSpec,
    warranty: warrantySpec as unknown as RelatedListSpec,
    order: orderSpec as unknown as RelatedListSpec,
    task: taskSpec as unknown as RelatedListSpec,
  };

export type RelatedListProps = {
  kind: EntityKind;
  /** Reverse filter sent with every request (e.g. `{ customer_uuid }`). */
  filter: RelatedFilter;
  /** Unique layout key (`<page>-<kind>-v1`). */
  persistKey: string;
  /** Column ids to leave out (the record the list hangs off). */
  hideColumns?: readonly string[];
  /** Disables the query (e.g. tab not open yet). */
  enabled?: boolean;
};

type ListPage = { items: Row[]; total: number };

/**
 * TEC-1217: reverse-relation list ("services of this customer"). Calls the
 * kind's existing list endpoint with `filter`, keeps the list contract
 * (sort / `q` / paging, EntityTable layout persistence) and opens the
 * record's detail page on row click. Without the kind's read permission it
 * renders nothing.
 */
export function RelatedList({
  kind,
  filter,
  persistKey,
  hideColumns,
  enabled = true,
}: RelatedListProps) {
  const { t, format } = useLocale();
  const router = useRouter();
  const tenant = useOptionalTenant();
  const can = useEntityPermissionCheck();
  const spec = RELATED_LIST_SPECS[kind];
  const allowed = Boolean(spec && can && canOpenEntity(kind, can));

  const columns = useMemo(() => {
    if (!spec) return [];
    const hidden = new Set(hideColumns ?? []);
    return spec
      .columns({ t: t as Translate, format })
      .filter(
        (c) =>
          !hidden.has(
            String(c.id ?? (c as { accessorKey?: string }).accessorKey),
          ),
      );
  }, [spec, hideColumns, t, format]);

  const list = useServerListState({
    columns,
    initialSort: spec?.defaultSort ?? null,
    initialPageSize: 10,
    persistKey,
  });
  const query = useMemo(
    () => ({ ...list.params, ...filter }),
    [list.params, filter],
  );
  const result = useQuery({
    queryKey: ["related", kind, query],
    queryFn: () => platformRequest<ListPage>("GET", spec!.path, { query }),
    enabled: enabled && allowed,
  });

  if (!spec || !allowed) return null;

  return (
    <EntityTable
      columns={columns as ColumnDef<Row, unknown>[]}
      data={result.data?.items ?? []}
      getRowId={(row) => row.uuid}
      onRowClick={(row) => {
        const href = entityHref(kind, row.uuid, tenant?.slug);
        if (href) router.push(href);
      }}
      isLoading={result.isLoading}
      isError={result.isError}
      onRetry={() => void result.refetch()}
      emptyTitle={t(spec.emptyKey)}
      rowCount={result.data?.total ?? 0}
      state={list.tableState}
      features={{ persistKey }}
    />
  );
}
