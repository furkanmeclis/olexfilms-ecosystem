"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { AlertTriangle, History, Pencil, Plus, Trash2 } from "lucide-react";
import Link from "next/link";
import { useCallback, useMemo, useRef, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  refColumn,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { catalogService } from "@/features/catalog/services/catalog.service";
import { enumFilterOptions } from "@/features/warehouse/components/table-options";
import {
  listExportQuery,
  WarehouseExportMenu,
} from "@/features/warehouse/components/warehouse-export";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import { warehouseErrorMessage } from "@/features/warehouse/lib/errors";
import {
  MOVEMENT_TYPES,
  reorderLevelTone,
  reorderInput,
  type ReorderForm,
} from "@/features/warehouse/lib/stock-view";
import {
  warehouseExportPaths,
  warehouseKeys,
  warehouseService,
  type StockMovementEnd,
  type StockMovementRow,
  type StockReorderRule,
  type StockWarehouseProduct,
} from "@/features/warehouse/services/warehouse.service";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const WAREHOUSE_PRODUCTS_KEY = "tenant-warehouse-detail-products-v1";
export const STOCK_MOVEMENTS_KEY = "tenant-warehouse-movements-v1";
export const REORDER_RULES_KEY = "tenant-warehouse-reorder-v1";
const PAGE_SIZE = 20;
const ALL = "__all__";

/**
 * Product totals of a warehouse (TEC-1229): on hand, reserved for orders,
 * available (on hand - reserved - quarantine), incoming (in-transit
 * transfers and shipped purchase orders) and quarantine; pieces and roll
 * meters apart. No prices.
 */
export function WarehouseProductsTab({ uuid }: { uuid: string }) {
  const { t, format } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<StockWarehouseProduct>({
          id: "product",
          accessorFn: (p) => p.product.name,
          labelKey: "warehouse.fields.product",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div data-testid="warehouse-product-row">
              {row.original.product.name}
              <div
                className="text-muted-foreground font-mono text-xs"
                dir="ltr"
              >
                {row.original.product.sku}
              </div>
            </div>
          ),
        }),
        createColumn<StockWarehouseProduct>({
          id: "quantity",
          accessorFn: (p) => p.quantity,
          labelKey: "warehouse.stock.on_hand",
          enableSorting: true,
          cell: ({ row }) => format.number(row.original.quantity),
        }),
        createColumn<StockWarehouseProduct>({
          id: "reserved",
          accessorFn: (p) => p.balance?.reserved_quantity ?? 0,
          labelKey: "warehouse.stock.reserved",
          enableSorting: false,
          cell: ({ row }) =>
            format.number(row.original.balance?.reserved_quantity ?? 0),
        }),
        createColumn<StockWarehouseProduct>({
          id: "available",
          accessorFn: (p) => p.balance?.available_quantity ?? p.quantity,
          labelKey: "warehouse.stock.available",
          enableSorting: false,
          cell: ({ row }) => (
            <strong data-testid="warehouse-product-available">
              {format.number(
                row.original.balance?.available_quantity ??
                  row.original.quantity,
              )}
            </strong>
          ),
        }),
        createColumn<StockWarehouseProduct>({
          id: "incoming",
          accessorFn: (p) => p.balance?.incoming_quantity ?? 0,
          labelKey: "warehouse.stock.incoming",
          enableSorting: false,
          cell: ({ row }) =>
            format.number(row.original.balance?.incoming_quantity ?? 0),
        }),
        createColumn<StockWarehouseProduct>({
          id: "quarantine",
          accessorFn: (p) => p.balance?.quarantine_quantity ?? 0,
          labelKey: "warehouse.stock.quarantine",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            format.number(row.original.balance?.quarantine_quantity ?? 0),
        }),
        createColumn<StockWarehouseProduct>({
          id: "meters",
          accessorFn: (p) => Number(p.meters),
          labelKey: "warehouse.stock.meters",
          enableSorting: true,
          cell: ({ row }) => (
            <span dir="ltr">
              {row.original.meters}
              {row.original.balance &&
              row.original.balance.reserved_meters !== "0.00" ? (
                <span className="text-muted-foreground ms-1 text-xs">
                  ({t("warehouse.stock.available")}:{" "}
                  {row.original.balance.available_meters})
                </span>
              ) : null}
            </span>
          ),
        }),
        createColumn<StockWarehouseProduct>({
          accessorKey: "roll_count",
          labelKey: "warehouse.stock.rolls",
          enableSorting: false,
          cell: ({ row }) => format.number(row.original.roll_count),
        }),
      ] as ColumnDef<StockWarehouseProduct, unknown>[],
    [format, t],
  );
  const listState = useServerListState({
    columns,
    initialSort: "product",
    initialPageSize: PAGE_SIZE,
    persistKey: WAREHOUSE_PRODUCTS_KEY,
  });
  const list = useQuery({
    queryKey: warehouseKeys.products(uuid, listState.params),
    queryFn: () =>
      warehouseService.listWarehouseProducts(uuid, listState.params),
  });
  return (
    <EntityTable
      columns={columns}
      data={list.data?.items ?? []}
      getRowId={(row) => row.product.uuid}
      isLoading={list.isLoading}
      isError={list.isError}
      onRetry={() => void list.refetch()}
      emptyTitle={t("warehouse.stock.empty")}
      emptyDescription=""
      rowCount={list.data?.total ?? 0}
      state={listState.tableState}
      features={{ persistKey: WAREHOUSE_PRODUCTS_KEY }}
      toolbarExtra={
        <EntityToolbar
          onRefresh={() => void list.refetch()}
          refreshDisabled={list.isFetching}
        />
      }
    />
  );
}

/** Warehouse picker of the stock pages ("all" allowed). */
function WarehouseSelect({
  value,
  onChange,
  allowAll,
  testId,
}: {
  value: string;
  onChange: (uuid: string) => void;
  allowAll?: boolean;
  testId: string;
}) {
  const { t } = useLocale();
  const warehouses = useQuery({
    queryKey: warehouseKeys.warehouses,
    queryFn: () => warehouseService.listWarehouses(),
  });
  return (
    <Select
      value={value || (allowAll ? ALL : "")}
      onValueChange={(v) => onChange(v === ALL ? "" : v)}
    >
      <SelectTrigger
        className="w-full sm:w-72"
        aria-label={t("warehouse.units.warehouse")}
        data-testid={testId}
      >
        <SelectValue placeholder={t("warehouse.units.warehouse")} />
      </SelectTrigger>
      <SelectContent>
        {allowAll ? (
          <SelectItem value={ALL}>
            {t("warehouse.stock.all_warehouses")}
          </SelectItem>
        ) : null}
        {(warehouses.data?.items ?? []).map((w) => (
          <SelectItem key={w.uuid} value={w.uuid}>
            {w.code} · {w.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function endText(e: StockMovementEnd | null): string {
  if (!e) return "—";
  return (
    [e.organization?.name, e.warehouse?.name, e.location?.name]
      .filter(Boolean)
      .join(" · ") || e.owner_type
  );
}

/**
 * Warehouse > Movements (TEC-1229): every stock ledger movement of the
 * organization or of one warehouse; type, product, date and text filters,
 * export (XLSX / CSV / PDF). No prices.
 */
export function StockMovementsPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const access = useWarehouseAccess(slug, Permission.StockRead);
  const [warehouse, setWarehouse] = useState("");
  const columns = useMemo(
    () =>
      [
        createColumn<StockMovementRow>({
          accessorKey: "created_at",
          labelKey: "warehouse.movements.columns.date",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          filterVariant: "date-range",
          param: "created",
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.dateTime(row.original.created_at)}
            </span>
          ),
        }),
        createColumn<StockMovementRow>({
          accessorKey: "type",
          labelKey: "warehouse.movements.columns.type",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: enumFilterOptions(
            MOVEMENT_TYPES,
            "warehouse.movement_type",
          ),
          param: "type",
          cell: ({ row }) => (
            <span data-testid="movement-row">
              {t(`warehouse.movement_type.${row.original.type}`)}
            </span>
          ),
        }),
        refColumn<StockMovementRow>({
          id: "product",
          kind: "product",
          labelKey: "warehouse.fields.product",
          enableSorting: true,
          getRef: (m) => ({
            uuid: m.product.uuid,
            name: m.product.name,
            sub: m.product.sku,
          }),
        }),
        createColumn<StockMovementRow>({
          id: "barcode",
          accessorFn: (m) => m.unit.barcode,
          labelKey: "warehouse.units.columns.barcode",
          enableSorting: false,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.stock.unit(slug, row.original.unit.barcode)}
              className="font-mono hover:underline"
              dir="ltr"
            >
              {row.original.unit.barcode}
            </Link>
          ),
        }),
        createColumn<StockMovementRow>({
          id: "quantity_delta",
          accessorFn: (m) => m.quantity_delta,
          labelKey: "warehouse.movements.columns.quantity",
          enableSorting: false,
          cell: ({ row }) => (
            <span dir="ltr">
              {format.number(row.original.quantity_delta, {
                signDisplay: "exceptZero",
              })}
              {row.original.meters_delta !== "0.00"
                ? ` · ${row.original.meters_delta} m`
                : ""}
            </span>
          ),
        }),
        createColumn<StockMovementRow>({
          id: "from",
          accessorFn: (m) => endText(m.from),
          labelKey: "warehouse.movements.columns.from",
          enableSorting: false,
          cell: ({ row }) => endText(row.original.from),
        }),
        createColumn<StockMovementRow>({
          id: "to",
          accessorFn: (m) => endText(m.to),
          labelKey: "warehouse.movements.columns.to",
          enableSorting: false,
          cell: ({ row }) => endText(row.original.to),
        }),
        createColumn<StockMovementRow>({
          id: "actor",
          accessorFn: (m) => m.actor?.name ?? "",
          labelKey: "warehouse.movements.columns.actor",
          enableSorting: false,
          cell: ({ row }) => row.original.actor?.name || "—",
        }),
        createColumn<StockMovementRow>({
          id: "reason",
          accessorFn: (m) => m.reason ?? "",
          labelKey: "warehouse.movements.columns.reason",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.reason ?? "—",
        }),
      ] as ColumnDef<StockMovementRow, unknown>[],
    [format, slug, t],
  );
  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: PAGE_SIZE,
    persistKey: STOCK_MOVEMENTS_KEY,
  });
  const query = {
    ...listState.params,
    warehouse_uuid: warehouse || undefined,
  };
  const exportQuery = {
    ...listExportQuery(listState.filterParams, listState.params),
    warehouse_uuid: warehouse || undefined,
  };
  const list = useQuery({
    queryKey: warehouseKeys.movements(query),
    queryFn: () => warehouseService.listMovements(query),
    enabled: access.allowed,
  });
  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={t("warehouse.movements.title")}
      description={t("warehouse.movements.description")}
      icon={<History className="size-6" />}
    >
      <Card>
        <CardContent className="space-y-4 pt-6">
          <WarehouseSelect
            value={warehouse}
            onChange={setWarehouse}
            allowAll
            testId="movements-warehouse"
          />
          <EntityTable
            columns={columns}
            data={list.data?.items ?? []}
            getRowId={(row) => row.uuid}
            isLoading={list.isLoading}
            isError={list.isError}
            onRetry={() => void list.refetch()}
            emptyTitle={t("warehouse.movements.empty")}
            emptyDescription=""
            rowCount={list.data?.total ?? 0}
            state={listState.tableState}
            features={{ persistKey: STOCK_MOVEMENTS_KEY }}
            toolbarExtra={
              <>
                <WarehouseExportMenu
                  slug={slug}
                  exportPath={warehouseExportPaths.movements}
                  query={exportQuery}
                  testId="movements-export"
                />
                <EntityToolbar
                  onRefresh={() => void list.refetch()}
                  refreshDisabled={list.isFetching}
                />
              </>
            }
          />
        </CardContent>
      </Card>
    </WarehouseShell>
  );
}

/**
 * Warehouse > Min / max (TEC-1229): quantity based minimum, reorder point
 * and maximum per warehouse and product, with the live level (below
 * minimum, reorder, few days left) and the suggested order quantity.
 * "Critical only" lists the rules that are not ok. stock.write edits.
 */
export function ReorderRulesPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const qc = useQueryClient();
  const { confirm } = useDialogs();
  const access = useWarehouseAccess(slug, Permission.StockRead);
  const canWrite = access.can(Permission.StockWrite);
  const [warehouse, setWarehouse] = useState("");
  const [critical, setCritical] = useState(true);
  const [editing, setEditing] = useState<StockReorderRule | "new" | null>(null);

  const remove = useMutation({
    mutationFn: (uuid: string) => warehouseService.deleteReorderRule(uuid),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["warehouse", "reorder"] });
      appToast.success(t("warehouse.reorder.deleted"));
    },
    onError: (err) =>
      appToast.error(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });

  const columns = useMemo(
    () =>
      [
        createColumn<StockReorderRule>({
          id: "product",
          accessorFn: (r) => r.product.name,
          labelKey: "warehouse.fields.product",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div data-testid="reorder-row" data-level={row.original.level}>
              {row.original.product.name}
              <div
                className="text-muted-foreground font-mono text-xs"
                dir="ltr"
              >
                {row.original.product.sku}
              </div>
            </div>
          ),
        }),
        createColumn<StockReorderRule>({
          id: "warehouse",
          accessorFn: (r) => r.warehouse.name,
          labelKey: "warehouse.units.warehouse",
          enableSorting: true,
          cell: ({ row }) =>
            `${row.original.warehouse.code} · ${row.original.warehouse.name}`,
        }),
        createColumn<StockReorderRule>({
          id: "level",
          accessorFn: (r) => r.level,
          labelKey: "warehouse.reorder.columns.level",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <StatusChip
              label={t(`warehouse.reorder.level.${row.original.level}`)}
              tone={reorderLevelTone(row.original.level)}
            />
          ),
        }),
        createColumn<StockReorderRule>({
          id: "available",
          accessorFn: (r) => r.balance.available_quantity,
          labelKey: "warehouse.stock.available",
          enableSorting: true,
          cell: ({ row }) => (
            <span>
              <strong>
                {format.number(row.original.balance.available_quantity)}
              </strong>
              <span className="text-muted-foreground ms-1 text-xs">
                / {format.number(row.original.balance.on_hand_quantity)}
              </span>
            </span>
          ),
        }),
        createColumn<StockReorderRule>({
          id: "incoming",
          accessorFn: (r) => r.balance.incoming_quantity,
          labelKey: "warehouse.stock.incoming",
          enableSorting: false,
          cell: ({ row }) =>
            format.number(row.original.balance.incoming_quantity),
        }),
        createColumn<StockReorderRule>({
          id: "thresholds",
          accessorFn: (r) => r.min_quantity,
          labelKey: "warehouse.reorder.columns.thresholds",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="text-xs" dir="ltr">
              {row.original.min_quantity} / {row.original.reorder_point ?? "—"}{" "}
              / {row.original.max_quantity ?? "—"}
            </span>
          ),
        }),
        createColumn<StockReorderRule>({
          id: "days_left",
          accessorFn: (r) => r.days_left ?? "",
          labelKey: "warehouse.reorder.columns.days_left",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.days_left
              ? `${row.original.days_left}${
                  row.original.warning_days
                    ? ` / ${row.original.warning_days}`
                    : ""
                }`
              : "—",
        }),
        createColumn<StockReorderRule>({
          id: "suggested",
          accessorFn: (r) => r.suggested_quantity,
          labelKey: "warehouse.reorder.columns.suggested",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.suggested_quantity > 0
              ? format.number(row.original.suggested_quantity)
              : "—",
        }),
        ...(canWrite
          ? [
              createColumn<StockReorderRule>({
                id: "actions",
                labelKey: "common.actions",
                enableSorting: false,
                enableHiding: false,
                enableResizing: false,
                cell: ({ row }) => {
                  const items: EntityRowAction[] = [
                    {
                      id: "edit",
                      label: t("common.edit"),
                      icon: Pencil,
                      onSelect: () => setEditing(row.original),
                    },
                    {
                      id: "delete",
                      label: t("common.delete"),
                      icon: Trash2,
                      variant: "destructive",
                      onSelect: () =>
                        void confirm({
                          title: t("warehouse.reorder.delete_title"),
                          description: row.original.product.name,
                          confirmLabel: t("common.delete"),
                        }).then((ok) => ok && remove.mutate(row.original.uuid)),
                    },
                  ];
                  return <EntityRowActions actions={items} />;
                },
              }),
            ]
          : []),
      ] as ColumnDef<StockReorderRule, unknown>[],
    [canWrite, confirm, format, remove, t],
  );
  const listState = useServerListState({
    columns,
    initialSort: "level",
    initialPageSize: PAGE_SIZE,
    persistKey: REORDER_RULES_KEY,
  });
  const query = {
    ...listState.params,
    warehouse_uuid: warehouse || undefined,
  };
  const list = useQuery({
    queryKey: warehouseKeys.reorder(query, critical),
    queryFn: () => warehouseService.listReorderRules(query, critical),
    enabled: access.allowed,
  });
  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={t("warehouse.reorder.title")}
      description={t("warehouse.reorder.description")}
      icon={<AlertTriangle className="size-6" />}
      actions={
        canWrite ? (
          <Button
            type="button"
            onClick={() => setEditing("new")}
            data-testid="reorder-new"
          >
            <Plus className="size-4" />
            {t("warehouse.reorder.new")}
          </Button>
        ) : null
      }
    >
      <Card>
        <CardContent className="space-y-4 pt-6">
          <div className="flex flex-wrap items-center gap-3">
            <WarehouseSelect
              value={warehouse}
              onChange={setWarehouse}
              allowAll
              testId="reorder-warehouse"
            />
            <Select
              value={critical ? "critical" : "all"}
              onValueChange={(v) => setCritical(v === "critical")}
            >
              <SelectTrigger
                className="w-full sm:w-56"
                aria-label={t("warehouse.reorder.show")}
                data-testid="reorder-critical"
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="critical">
                  {t("warehouse.reorder.critical_only")}
                </SelectItem>
                <SelectItem value="all">
                  {t("warehouse.reorder.all")}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
          <EntityTable
            columns={columns}
            data={list.data?.items ?? []}
            getRowId={(row) => row.uuid}
            isLoading={list.isLoading}
            isError={list.isError}
            onRetry={() => void list.refetch()}
            emptyTitle={
              critical
                ? t("warehouse.reorder.empty_critical")
                : t("warehouse.reorder.empty")
            }
            emptyDescription=""
            rowCount={list.data?.total ?? 0}
            state={listState.tableState}
            features={{ persistKey: REORDER_RULES_KEY }}
            toolbarExtra={
              <EntityToolbar
                onRefresh={() => void list.refetch()}
                refreshDisabled={list.isFetching}
              />
            }
          />
        </CardContent>
      </Card>
      {editing ? (
        <ReorderRuleDialog
          rule={editing === "new" ? null : editing}
          warehouse={warehouse}
          onClose={() => setEditing(null)}
        />
      ) : null}
    </WarehouseShell>
  );
}

/** Create or edit the rule of a warehouse and product (stock.write). */
export function ReorderRuleDialog({
  rule,
  warehouse,
  onClose,
}: {
  rule: StockReorderRule | null;
  warehouse: string;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [form, setForm] = useState<ReorderForm>({
    warehouse_uuid: rule?.warehouse.uuid ?? warehouse,
    product_uuid: rule?.product.uuid ?? "",
    min_quantity: rule ? String(rule.min_quantity) : "",
    reorder_point: rule?.reorder_point?.toString() ?? "",
    max_quantity: rule?.max_quantity?.toString() ?? "",
    warning_days: rule?.warning_days?.toString() ?? "",
    cover_days: rule?.cover_days?.toString() ?? "",
  });
  const [product, setProduct] = useState<ComboboxOption | null>(
    rule
      ? {
          value: rule.product.uuid,
          label: rule.product.name,
          description: rule.product.sku,
        }
      : null,
  );
  const [error, setError] = useState<string | null>(null);
  const seen = useRef(new Map<string, ComboboxOption>());
  const loadProducts = useCallback(async (q: string) => {
    const page = await catalogService.listProducts({
      ...(q.trim() ? { q: q.trim() } : {}),
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
  const parsed = reorderInput(form);
  const save = useMutation({
    mutationFn: () => {
      if (!parsed.ok) throw new Error(parsed.error);
      return warehouseService.putReorderRule(parsed.value);
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["warehouse", "reorder"] });
      appToast.success(t("warehouse.reorder.saved"));
      onClose();
    },
    onError: (err) =>
      setError(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });
  const set = (key: keyof ReorderForm) => (v: string) =>
    setForm((prev) => ({ ...prev, [key]: v }));
  const field = (key: keyof ReorderForm, labelKey: string) => (
    <div className="space-y-1">
      <Label htmlFor={`reorder-${key}`}>{t(labelKey)}</Label>
      <Input
        id={`reorder-${key}`}
        inputMode="numeric"
        value={form[key]}
        onChange={(e) => set(key)(e.target.value)}
        data-testid={`reorder-${key}`}
      />
    </div>
  );
  return (
    <Dialog open onOpenChange={(open) => (!open ? onClose() : undefined)}>
      <DialogContent data-testid="reorder-dialog">
        <DialogHeader>
          <DialogTitle>
            {rule ? t("warehouse.reorder.edit") : t("warehouse.reorder.new")}
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          <WarehouseSelect
            value={form.warehouse_uuid}
            onChange={set("warehouse_uuid")}
            testId="reorder-form-warehouse"
          />
          <AsyncCombobox
            id="reorder-product"
            value={product?.value ?? ""}
            options={product ? [product] : undefined}
            loadOptions={loadProducts}
            onValueChange={(value) => {
              const next = value
                ? (seen.current.get(value) ?? { value, label: value })
                : null;
              setProduct(next);
              set("product_uuid")(next?.value ?? "");
            }}
            placeholder={t("warehouse.reorder.product")}
            searchPlaceholder={t("warehouse.export.summary_product_search")}
            emptyText={t("warehouse.export.summary_product_none")}
          />
          <div className="grid gap-3 sm:grid-cols-3">
            {field("min_quantity", "warehouse.reorder.min")}
            {field("reorder_point", "warehouse.reorder.point")}
            {field("max_quantity", "warehouse.reorder.max")}
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            {field("warning_days", "warehouse.reorder.warning_days")}
            {field("cover_days", "warehouse.reorder.cover_days")}
          </div>
          <p className="text-muted-foreground text-xs">
            {t("warehouse.reorder.hint")}
          </p>
          {!parsed.ok && parsed.error !== "required" ? (
            <p className="text-destructive text-sm">
              {t(`warehouse.reorder.error.${parsed.error}`)}
            </p>
          ) : null}
          {error ? (
            <p role="alert" className="text-destructive text-sm">
              {error}
            </p>
          ) : null}
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            disabled={!parsed.ok || save.isPending}
            onClick={() => save.mutate()}
            data-testid="reorder-save"
          >
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
