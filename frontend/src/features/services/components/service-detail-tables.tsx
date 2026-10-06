"use client";

import type { ColumnDef, FilterFn } from "@tanstack/react-table";
import { Eye } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  CLIENT_SIDE_MANUAL,
  EntityRowActions,
  EntityTable,
  clientDateRangeFilter,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { routes } from "@/config/routes";
import { isKnownPart } from "@/features/services/lib/car-parts";
import { itemAmount, warrantyTone } from "@/features/services/lib/detail";
import type {
  ServiceItem,
  ServiceWarranty,
} from "@/features/services/services/service-wizard.service";
import { useLocale } from "@/providers/locale-provider";

export const SERVICE_ITEMS_PERSIST_KEY = "tenant-service-items-v1";
export const SERVICE_WARRANTIES_PERSIST_KEY = "tenant-service-warranties-v1";

const WARRANTY_STATUSES = ["active", "expired", "void"] as const;

/** Amount for sorting: meters of a roll cut, else pieces, else one unit. */
function amountValue(item: ServiceItem): number {
  if (item.kind === "partial" && item.meters) return Number(item.meters) || 0;
  return item.quantity ?? 1;
}

/** Faceted filter over an array cell (any selected value matches). */
const anyOfFilter: FilterFn<ServiceItem> = (row, columnId, value) => {
  const selected = value as string[] | undefined;
  if (!selected?.length) return true;
  const cell = row.getValue<string[]>(columnId) ?? [];
  return cell.some((v) => selected.includes(v));
};

function uniqueOptions(values: string[], label: (v: string) => string) {
  return [...new Set(values)]
    .map((value) => ({ value, label: label(value) }))
    .sort((a, b) => a.label.localeCompare(b.label));
}

/**
 * Products of a service (TEC-183, TEC-378) as a nested client-side
 * DataTable: the detail response embeds every item, so sort, search, the
 * product / part facets and paging run in the browser.
 */
export function ServiceItemsTable({ items }: { items: ServiceItem[] }) {
  const { t, format } = useLocale();
  const partLabel = useMemo(
    () => (p: string) => (isKnownPart(p) ? t(`services.parts.names.${p}`) : p),
    [t],
  );

  const productOptions = useMemo(
    () =>
      uniqueOptions(
        items.map((i) => i.product.name),
        (v) => v,
      ),
    [items],
  );
  const partOptions = useMemo(
    () =>
      uniqueOptions(
        items.flatMap((i) => i.applied_parts),
        partLabel,
      ),
    [items, partLabel],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<ServiceItem>({
          id: "product",
          accessorFn: (row) => row.product.name,
          labelKey: "services.detail.items.product",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          filterVariant: "faceted",
          filterOptions: productOptions,
          enableColumnFilter: productOptions.length > 1,
          cell: ({ row }) => (
            <div className="min-w-0" data-testid="detail-item">
              <p className="font-medium">{row.original.product.name}</p>
              <p className="text-muted-foreground font-mono text-xs" dir="ltr">
                {row.original.product.sku}
              </p>
            </div>
          ),
        }),
        createColumn<ServiceItem>({
          accessorKey: "barcode",
          labelKey: "services.detail.items.barcode",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs" dir="ltr">
              {row.original.barcode}
            </span>
          ),
        }),
        createColumn<ServiceItem>({
          id: "amount",
          accessorFn: amountValue,
          labelKey: "services.detail.items.amount",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => {
            const amount = itemAmount(row.original);
            return (
              <span className="whitespace-nowrap">
                {t(amount.key, amount.params)}
              </span>
            );
          },
        }),
        createColumn<ServiceItem>({
          id: "parts",
          accessorFn: (row) => row.applied_parts,
          labelKey: "services.detail.items.parts",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: partOptions,
          filterFn: anyOfFilter,
          enableColumnFilter: partOptions.length > 1,
          cell: ({ row }) =>
            row.original.applied_parts.length > 0 ? (
              <div className="flex flex-wrap gap-1">
                {row.original.applied_parts.map((p) => (
                  <Badge key={p} variant="outline" className="text-xs">
                    {partLabel(p)}
                  </Badge>
                ))}
              </div>
            ) : (
              "—"
            ),
        }),
        createColumn<ServiceItem>({
          accessorKey: "notes",
          labelKey: "services.detail.notes",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.notes ? (
              <span className="text-muted-foreground text-xs">
                {row.original.notes}
              </span>
            ) : (
              "—"
            ),
        }),
        createColumn<ServiceItem>({
          accessorKey: "created_at",
          labelKey: "services.detail.items.added_at",
          enableSorting: true,
          defaultHidden: true,
          filterVariant: "date-range",
          filterFn: clientDateRangeFilter as FilterFn<ServiceItem>,
          cell: ({ row }) => format.dateTime(row.original.created_at),
        }),
      ] as ColumnDef<ServiceItem, unknown>[],
    [format, partLabel, partOptions, productOptions, t],
  );

  return (
    <EntityTable
      columns={columns}
      data={items}
      getRowId={(row) => row.uuid}
      manual={CLIENT_SIDE_MANUAL}
      initialState={{
        pagination: { pageIndex: 0, pageSize: 10 },
        sorting: [{ id: "created_at", desc: false }],
      }}
      emptyTitle={t("services.stock.items_empty")}
      emptyDescription=""
      features={{ persistKey: SERVICE_ITEMS_PERSIST_KEY }}
    />
  );
}

/**
 * Warranties issued by a service (TEC-183, TEC-378) as a nested client-side
 * DataTable with a status facet and date sorting. With warranties.read a
 * row opens the warranty page.
 */
export function ServiceWarrantiesTable({
  slug,
  warranties,
  canOpen,
  emptyTitle,
}: {
  slug: string;
  warranties: ServiceWarranty[];
  canOpen: boolean;
  emptyTitle: string;
}) {
  const { t, format } = useLocale();
  const router = useRouter();

  const columns = useMemo(
    () =>
      [
        createColumn<ServiceWarranty>({
          accessorKey: "public_code",
          labelKey: "warranty.list.columns.code",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => {
            const w = row.original;
            return (
              <span data-testid="detail-warranty" data-uuid={w.uuid}>
                {canOpen ? (
                  <Link
                    href={routes.tenant.warranties.detail(slug, w.uuid)}
                    className="font-mono text-xs font-medium hover:underline"
                    dir="ltr"
                    onClick={(event) => event.stopPropagation()}
                  >
                    {w.public_code}
                  </Link>
                ) : (
                  <span className="font-mono text-xs" dir="ltr">
                    {w.public_code}
                  </span>
                )}
              </span>
            );
          },
        }),
        createColumn<ServiceWarranty>({
          accessorKey: "product_name",
          labelKey: "warranty.list.columns.product",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => row.original.product_name || "—",
        }),
        createColumn<ServiceWarranty>({
          accessorKey: "status",
          labelKey: "warranty.list.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: WARRANTY_STATUSES.map((value) => ({
            value,
            label: value,
            labelKey: `services.detail.warranty_status.${value}`,
          })),
          enableColumnFilter: warranties.length > 1,
          cell: ({ row }) => (
            <StatusChip
              label={t(
                `services.detail.warranty_status.${row.original.status}`,
              )}
              tone={warrantyTone(row.original.status)}
            />
          ),
        }),
        createColumn<ServiceWarranty>({
          accessorKey: "start_at",
          labelKey: "warranty.detail.start",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => format.date(row.original.start_at),
        }),
        createColumn<ServiceWarranty>({
          accessorKey: "end_at",
          labelKey: "warranty.detail.end",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.date(row.original.end_at)}
            </span>
          ),
        }),
        ...(canOpen
          ? [
              createColumn<ServiceWarranty>({
                id: "actions",
                labelKey: "common.actions",
                enableSorting: false,
                enableHiding: false,
                enableResizing: false,
                cell: ({ row }) => (
                  <EntityRowActions
                    actions={[
                      {
                        id: "view",
                        label: t("common.view"),
                        icon: Eye,
                        onSelect: () =>
                          router.push(
                            routes.tenant.warranties.detail(
                              slug,
                              row.original.uuid,
                            ),
                          ),
                      },
                    ]}
                  />
                ),
              }),
            ]
          : []),
      ] as ColumnDef<ServiceWarranty, unknown>[],
    [canOpen, format, router, slug, t, warranties.length],
  );

  return (
    <EntityTable
      columns={columns}
      data={warranties}
      getRowId={(row) => row.uuid}
      manual={CLIENT_SIDE_MANUAL}
      onRowClick={
        canOpen
          ? (w) => router.push(routes.tenant.warranties.detail(slug, w.uuid))
          : undefined
      }
      initialState={{
        pagination: { pageIndex: 0, pageSize: 10 },
        sorting: [{ id: "end_at", desc: false }],
      }}
      emptyTitle={emptyTitle}
      emptyDescription=""
      features={{ persistKey: SERVICE_WARRANTIES_PERSIST_KEY }}
    />
  );
}
