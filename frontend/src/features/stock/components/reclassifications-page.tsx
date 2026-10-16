"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Ban, Eye, History, Replace, ShieldCheck, XCircle } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useMemo, useRef, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { catalogService } from "@/features/catalog/services/catalog.service";
import { useReclassDecisions } from "@/features/stock/components/use-reclass-decisions";
import {
  RECLASS_STATUSES,
  reclassStatusTone,
} from "@/features/stock/lib/stock-ops";
import {
  stockOpsKeys,
  stockOpsService,
  type StockReclassification,
} from "@/features/stock/services/stock-ops.service";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const RECLASS_PAGE_SIZE = 20;
export const RECLASS_PERSIST_KEY = "tenant-stock-reclassifications-v1";

/** Filter-only columns fed by the product pickers. */
const FROM_FILTER = "from_product_filter";
const TO_FILTER = "to_product_filter";

type Row = StockReclassification;

/**
 * Stock > Reclassification requests (TEC-157, TEC-763): the requests of
 * the organizations in reach over a server DataTable — sort (created,
 * decided, status), search (barcode, product, reason), status /
 * requester / created filters, source and target product pickers, row
 * actions (open, unit history, approve with step-up, reject with a
 * reason, cancel) and mobile cards. The requester gets no approve action.
 */
export function ReclassificationsPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const router = useRouter();
  const { can } = usePermission();
  const { user } = useAuth();
  const canRead = can(Permission.StockRead);
  const { actionsFor, decide, pending } = useReclassDecisions(slug);

  const [fromProduct, setFromProduct] = useState<ComboboxOption | null>(null);
  const [toProduct, setToProduct] = useState<ComboboxOption | null>(null);
  const seen = useRef(new Map<string, ComboboxOption>());
  const loadProducts = useCallback(async (q: string) => {
    const page = await catalogService.listProducts({
      q: q.trim() || undefined,
      limit: 20,
    });
    const options = page.items.map((p) => ({
      value: p.uuid,
      label: p.name,
      description: p.sku,
    }));
    for (const option of options) seen.current.set(option.value, option);
    return options;
  }, []);

  const requesterOptions = useMemo(
    () =>
      user
        ? [
            {
              value: user.uuid,
              label: t("stock.reclass.mine"),
              labelKey: "stock.reclass.mine",
            },
          ]
        : [],
    [t, user],
  );

  const columns = useMemo(() => {
    const productCell = (p: Row["from_product"]) => (
      <div>
        <div className="font-medium">{p.name}</div>
        <div className="text-muted-foreground font-mono text-xs" dir="ltr">
          {p.sku}
        </div>
      </div>
    );
    return [
      createColumn<Row>({
        id: "barcode",
        accessorFn: (r) => r.unit.barcode,
        labelKey: "stock.columns.unit",
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <Link
            href={routes.tenant.stock.reclassification(slug, row.original.uuid)}
            className="font-mono text-xs font-medium hover:underline"
            dir="ltr"
            data-testid="reclass-row"
            data-uuid={row.original.uuid}
            onClick={(event) => event.stopPropagation()}
          >
            {row.original.unit.barcode}
          </Link>
        ),
      }),
      createColumn<Row>({
        id: "from_product",
        accessorFn: (r) => r.from_product.name,
        labelKey: "stock.reclass.columns.from",
        enableSorting: false,
        cell: ({ row }) => productCell(row.original.from_product),
      }),
      createColumn<Row>({
        id: "to_product",
        accessorFn: (r) => r.to_product.name,
        labelKey: "stock.reclass.columns.to",
        enableSorting: false,
        gridSecondary: true,
        cell: ({ row }) => productCell(row.original.to_product),
      }),
      // Filter only: the product pickers above the table set them.
      createColumn<Row>({
        id: FROM_FILTER,
        accessorFn: (r) => r.from_product.uuid,
        labelKey: "stock.reclass.columns.from",
        enableSorting: false,
        enableHiding: false,
        enableColumnFilter: false,
        defaultHidden: true,
        param: "from_product_uuid",
        paramFormat: "string",
        cell: () => null,
      }),
      createColumn<Row>({
        id: TO_FILTER,
        accessorFn: (r) => r.to_product.uuid,
        labelKey: "stock.reclass.columns.to",
        enableSorting: false,
        enableHiding: false,
        enableColumnFilter: false,
        defaultHidden: true,
        param: "to_product_uuid",
        paramFormat: "string",
        cell: () => null,
      }),
      createColumn<Row>({
        id: "organization",
        accessorFn: (r) => r.organization.name,
        labelKey: "stock.reclass.columns.organization",
        enableSorting: false,
        cell: ({ row }) => row.original.organization.name,
      }),
      createColumn<Row>({
        id: "requested_by",
        accessorFn: (r) => r.requested_by?.name ?? "",
        labelKey: "stock.reclass.columns.requested_by",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: requesterOptions,
        enableColumnFilter: requesterOptions.length > 0,
        param: "requested_by",
        cell: ({ row }) => row.original.requested_by?.name ?? "—",
      }),
      createColumn<Row>({
        accessorKey: "status",
        labelKey: "stock.columns.status",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: RECLASS_STATUSES.map((value) => ({
          value,
          label: value,
          labelKey: `stock.reclass.status.${value}`,
        })),
        param: "status",
        cell: ({ row }) => (
          <StatusChip
            label={t(`stock.reclass.status.${row.original.status}`)}
            tone={reclassStatusTone(row.original.status)}
          />
        ),
      }),
      createColumn<Row>({
        accessorKey: "created_at",
        labelKey: "stock.reclass.columns.created",
        enableSorting: true,
        filterVariant: "date-range",
        param: "created",
        cell: ({ row }) => (
          <span className="text-muted-foreground text-xs whitespace-nowrap">
            {format.dateTime(row.original.created_at)}
          </span>
        ),
      }),
      createColumn<Row>({
        id: "decided_at",
        accessorFn: (r) => r.decided_at ?? "",
        labelKey: "stock.reclass.columns.decided",
        enableSorting: true,
        cell: ({ row }) => (
          <span className="text-muted-foreground text-xs whitespace-nowrap">
            {row.original.decided_at
              ? format.dateTime(row.original.decided_at)
              : "—"}
          </span>
        ),
      }),
      createColumn<Row>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => {
          const r = row.original;
          const allowed = actionsFor(r);
          const items: EntityRowAction[] = [
            {
              id: "view",
              label: t("common.view"),
              icon: Eye,
              onSelect: () =>
                router.push(routes.tenant.stock.reclassification(slug, r.uuid)),
            },
            {
              id: "history",
              label: t("stock.unit.history"),
              icon: History,
              onSelect: () =>
                router.push(routes.tenant.stock.unit(slug, r.unit.barcode)),
            },
          ];
          if (allowed.approve) {
            items.push({
              id: "approve",
              label: t("stock.reclass.approve"),
              icon: ShieldCheck,
              disabled: pending,
              onSelect: () => void decide(r, "approve"),
            });
          }
          if (allowed.reject) {
            items.push({
              id: "reject",
              label: t("stock.reclass.reject"),
              icon: XCircle,
              variant: "destructive",
              disabled: pending,
              onSelect: () => void decide(r, "reject"),
            });
          }
          if (allowed.cancel) {
            items.push({
              id: "cancel",
              label: t("stock.reclass.cancel"),
              icon: Ban,
              variant: "destructive",
              disabled: pending,
              onSelect: () => void decide(r, "cancel"),
            });
          }
          return <EntityRowActions actions={items} />;
        },
      }),
    ] as ColumnDef<Row, unknown>[];
  }, [actionsFor, decide, format, pending, requesterOptions, router, slug, t]);

  // Column meta drives the params: status / requested_by (CSV), created
  // (created_from/_to), from/to product.
  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: RECLASS_PAGE_SIZE,
    persistKey: RECLASS_PERSIST_KEY,
  });
  const query = listState.params;
  const list = useQuery({
    queryKey: stockOpsKeys.reclassifications(query),
    queryFn: () => stockOpsService.listReclassifications(query),
    enabled: canRead,
  });

  const { onColumnFiltersChange, columnFilters } = listState;
  const pickProduct = (
    id: string,
    set: (o: ComboboxOption | null) => void,
    value: string,
  ) => {
    const option = value
      ? (seen.current.get(value) ?? { value, label: value })
      : null;
    set(option);
    onColumnFiltersChange((prev) => [
      ...prev.filter((f) => f.id !== id),
      ...(option ? [{ id, value: option.value }] : []),
    ]);
  };

  const title = t("stock.reclass.list_title");
  const header = (
    <PageHeader
      title={title}
      icon={<Replace className="size-6" />}
      description={t("stock.reclass.list_description")}
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

  const filtered = columnFilters.length > 0 || Boolean(query.q);

  return (
    <div className="space-y-6">
      {header}
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.tenant.stock.reclassification(slug, row.uuid))
        }
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("stock.reclass.empty_title")}
        emptyDescription={t(
          filtered
            ? "stock.reclass.empty_filtered"
            : "stock.reclass.empty_description",
        )}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: RECLASS_PERSIST_KEY,
          rowSelection: false,
          viewMode: true,
        }}
        renderGridItem={(r) => (
          <div className="space-y-2">
            <div className="flex items-start justify-between gap-2">
              <span className="font-mono text-sm font-semibold" dir="ltr">
                {r.unit.barcode}
              </span>
              <StatusChip
                label={t(`stock.reclass.status.${r.status}`)}
                tone={reclassStatusTone(r.status)}
              />
            </div>
            <p className="text-sm">
              {r.from_product.name} → {r.to_product.name}
            </p>
            <div className="text-muted-foreground flex justify-between gap-2 text-xs">
              <span>{r.requested_by?.name ?? r.organization.name}</span>
              <span>{format.dateTime(r.created_at)}</span>
            </div>
          </div>
        )}
        toolbarExtra={
          <>
            <div className="w-full sm:w-56" data-testid="reclass-from-filter">
              <AsyncCombobox
                id="reclass-from-filter"
                value={fromProduct?.value ?? ""}
                options={fromProduct ? [fromProduct] : undefined}
                clearable
                loadOptions={loadProducts}
                onValueChange={(value) =>
                  pickProduct(FROM_FILTER, setFromProduct, value)
                }
                placeholder={t("stock.reclass.filters.from")}
                searchPlaceholder={t("stock.filters.product_search")}
                emptyText={t("stock.reclass.target_none")}
              />
            </div>
            <div className="w-full sm:w-56" data-testid="reclass-to-filter">
              <AsyncCombobox
                id="reclass-to-filter"
                value={toProduct?.value ?? ""}
                options={toProduct ? [toProduct] : undefined}
                clearable
                loadOptions={loadProducts}
                onValueChange={(value) =>
                  pickProduct(TO_FILTER, setToProduct, value)
                }
                placeholder={t("stock.reclass.filters.to")}
                searchPlaceholder={t("stock.filters.product_search")}
                emptyText={t("stock.reclass.target_none")}
              />
            </div>
            <EntityToolbar
              onRefresh={() => void list.refetch()}
              refreshDisabled={list.isFetching}
            />
          </>
        }
      />
    </div>
  );
}
