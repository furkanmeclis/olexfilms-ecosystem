"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import {
  ClipboardCheck,
  Eye,
  FileDown,
  Play,
  Plus,
  ShieldCheck,
  XCircle,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState, type FormEvent } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { catalogService } from "@/features/catalog/services/catalog.service";
import { LocationPicker } from "@/features/warehouse/components/location-picker";
import { nativeSelectClass } from "@/features/warehouse/components/native-select-field";
import {
  enumFilterOptions,
  useWarehouseFilterOptions,
} from "@/features/warehouse/components/table-options";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import {
  canCancelCount,
  canStart,
  COUNT_METHODS,
  COUNT_SCOPES,
  COUNT_STATUSES,
  COUNT_VISIBILITIES,
  countBody,
  countFormErrors,
  countStatusTone,
  EMPTY_COUNT_FORM,
  needsStartApproval,
  scopesFor,
  type CountFormState,
} from "@/features/warehouse/lib/counts";
import { warehouseErrorMessage } from "@/features/warehouse/lib/errors";
import {
  countExportPath,
  warehouseKeys,
  warehouseService,
  type CountListQuery,
  type StockCount,
} from "@/features/warehouse/services/warehouse.service";
import { useDebounce } from "@/hooks/use-debounce";
import {
  platformDownloadFile,
  triggerBrowserDownload,
} from "@/lib/api/platform-form-request";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const COUNT_PAGE_SIZE = 20;
export const COUNTS_PERSIST_KEY = "tenant-warehouse-counts-v1";

type CountRowAction = "approve-start" | "start" | "cancel";

/** Scope summary of a count: warehouse, room, location or product. */
export function countScopeText(
  c: StockCount,
  t: (k: string) => string,
): string {
  switch (c.scope_type) {
    case "room":
      return c.scope_room ? `${c.scope_room.code} · ${c.scope_room.name}` : "";
    case "location":
      return c.scope_location?.full_code ?? "";
    case "product":
      return c.scope_product
        ? `${c.scope_product.sku} · ${c.scope_product.name}`
        : "";
    default:
      return t("warehouse.count_scope.warehouse");
  }
}

/** The CSV of a count exists once it is completed (review or approved). */
function hasReport(c: StockCount): boolean {
  return c.status === "pending_review" || c.status === "approved";
}

/**
 * Warehouse > Stock counts (TEC-206, TEC-376): the counts of the
 * organization over a server DataTable — sort (created, status,
 * warehouse), search (note, warehouse), status / method / visibility /
 * scope / warehouse / created filters, row actions (open, approve the
 * start, start, cancel, CSV) and mobile cards — and a new draft
 * (warehouse, method, blind / guided, scope).
 */
export function CountsPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const { confirm } = useDialogs();
  const access = useWarehouseAccess(slug);
  const canWrite = access.can(Permission.WarehouseWrite);
  const [creating, setCreating] = useState(false);
  const warehouseOptions = useWarehouseFilterOptions(access.allowed);

  const action = useMutation({
    mutationFn: (v: { uuid: string; action: CountRowAction }) =>
      warehouseService.countAction(v.uuid, v.action),
    onSuccess: async (next, v) => {
      qc.setQueryData(warehouseKeys.count(next.uuid), next);
      await qc.invalidateQueries({ queryKey: ["warehouse", "counts"] });
      appToast.success(t(`warehouse.count.${v.action.replace("-", "_")}_done`));
    },
    onError: (err) =>
      appToast.error(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });
  const runAction = action.mutate;
  const actionPending = action.isPending;

  const columns = useMemo(() => {
    const askCancel = async (count: StockCount) => {
      const ok = await confirm({
        title: t("warehouse.count.cancel_title"),
        description: t("warehouse.count.cancel_description"),
        confirmLabel: t("warehouse.count.cancel"),
        variant: "destructive",
      });
      if (ok) runAction({ uuid: count.uuid, action: "cancel" });
    };
    const downloadCsv = async (count: StockCount) => {
      try {
        const { blob, filename } = await platformDownloadFile(
          countExportPath(count.uuid),
        );
        triggerBrowserDownload(blob, filename ?? `count-${count.uuid}.csv`);
      } catch {
        appToast.error(t("warehouse.count.export_failed"));
      }
    };
    return [
      createColumn<StockCount>({
        id: "warehouse",
        accessorFn: (c) => c.warehouse.name,
        labelKey: "warehouse.entries.columns.warehouse",
        enableSorting: true,
        gridPrimary: true,
        filterVariant: "faceted",
        filterOptions: warehouseOptions,
        enableColumnFilter: warehouseOptions.length > 0,
        param: "warehouse_uuid",
        cell: ({ row }) => (
          <Link
            href={routes.tenant.warehouse.count(slug, row.original.uuid)}
            className="font-medium hover:underline"
            data-testid="count-row"
            data-uuid={row.original.uuid}
            onClick={(event) => event.stopPropagation()}
          >
            {row.original.warehouse.code} · {row.original.warehouse.name}
          </Link>
        ),
      }),
      createColumn<StockCount>({
        accessorKey: "method",
        labelKey: "warehouse.fields.method",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: enumFilterOptions(
          COUNT_METHODS,
          "warehouse.count_method",
        ),
        param: "method",
        gridSecondary: true,
        cell: ({ row }) => t(`warehouse.count_method.${row.original.method}`),
      }),
      createColumn<StockCount>({
        accessorKey: "visibility",
        labelKey: "warehouse.fields.visibility",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: enumFilterOptions(
          COUNT_VISIBILITIES,
          "warehouse.count_visibility",
        ),
        param: "visibility",
        cell: ({ row }) =>
          t(`warehouse.count_visibility.${row.original.visibility}`),
      }),
      createColumn<StockCount>({
        accessorKey: "scope_type",
        labelKey: "warehouse.fields.scope",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: enumFilterOptions(COUNT_SCOPES, "warehouse.count_scope"),
        param: "scope_type",
        cell: ({ row }) => (
          <div>
            {t(`warehouse.count_scope.${row.original.scope_type}`)}
            {row.original.scope_type !== "warehouse" ? (
              <div className="text-muted-foreground text-xs" dir="ltr">
                {countScopeText(row.original, t)}
              </div>
            ) : null}
          </div>
        ),
      }),
      createColumn<StockCount>({
        accessorKey: "status",
        labelKey: "warehouse.entries.columns.status",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: enumFilterOptions(
          COUNT_STATUSES,
          "warehouse.count_status",
        ),
        param: "status",
        cell: ({ row }) => (
          <StatusChip
            label={t(`warehouse.count_status.${row.original.status}`)}
            tone={countStatusTone(row.original.status)}
          />
        ),
      }),
      createColumn<StockCount>({
        accessorKey: "created_at",
        labelKey: "warehouse.entries.columns.created",
        enableSorting: true,
        filterVariant: "date-range",
        param: "created",
        cell: ({ row }) => (
          <span className="text-muted-foreground text-xs whitespace-nowrap">
            {format.dateTime(row.original.created_at)}
          </span>
        ),
      }),
      createColumn<StockCount>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => {
          const count = row.original;
          const items: EntityRowAction[] = [
            {
              id: "view",
              label: t("common.view"),
              icon: Eye,
              onSelect: () =>
                router.push(routes.tenant.warehouse.count(slug, count.uuid)),
            },
          ];
          if (canWrite && needsStartApproval(count)) {
            items.push({
              id: "approve-start",
              label: t("warehouse.count.approve_start"),
              icon: ShieldCheck,
              disabled: actionPending,
              onSelect: () =>
                runAction({ uuid: count.uuid, action: "approve-start" }),
            });
          }
          if (canWrite && canStart(count)) {
            items.push({
              id: "start",
              label: t("warehouse.count.start"),
              icon: Play,
              disabled: actionPending,
              onSelect: () => runAction({ uuid: count.uuid, action: "start" }),
            });
          }
          if (hasReport(count)) {
            items.push({
              id: "export",
              label: t("warehouse.count.export"),
              icon: FileDown,
              onSelect: () => void downloadCsv(count),
            });
          }
          if (canWrite && canCancelCount(count)) {
            items.push({
              id: "cancel",
              label: t("warehouse.count.cancel"),
              icon: XCircle,
              variant: "destructive",
              disabled: actionPending,
              onSelect: () => void askCancel(count),
            });
          }
          return <EntityRowActions actions={items} />;
        },
      }),
    ] as ColumnDef<StockCount, unknown>[];
  }, [
    actionPending,
    canWrite,
    confirm,
    format,
    router,
    runAction,
    slug,
    t,
    warehouseOptions,
  ]);

  // Column meta drives the params: warehouse_uuid / method / visibility /
  // scope_type / status (CSV), created (created_from/_to).
  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: COUNT_PAGE_SIZE,
    persistKey: COUNTS_PERSIST_KEY,
  });
  const query: CountListQuery = listState.params;
  const list = useQuery({
    queryKey: warehouseKeys.counts(query),
    queryFn: () => warehouseService.listCounts(query),
    enabled: access.allowed,
  });

  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={t("warehouse.counts.title")}
      description={t("warehouse.counts.description")}
      icon={<ClipboardCheck className="size-6" />}
      actions={
        canWrite && !creating ? (
          <Button
            type="button"
            onClick={() => setCreating(true)}
            data-testid="count-new"
          >
            <Plus className="size-4" />
            {t("warehouse.counts.new")}
          </Button>
        ) : null
      }
    >
      {creating ? (
        <NewCountForm slug={slug} onCancel={() => setCreating(false)} />
      ) : null}

      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.tenant.warehouse.count(slug, row.uuid))
        }
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("warehouse.counts.empty_title")}
        emptyDescription={t("warehouse.counts.empty_description")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: COUNTS_PERSIST_KEY,
          rowSelection: false,
          viewMode: true,
        }}
        renderGridItem={(c) => (
          <div className="space-y-2">
            <div className="flex items-start justify-between gap-2">
              <span className="font-medium">
                {c.warehouse.code} · {c.warehouse.name}
              </span>
              <StatusChip
                label={t(`warehouse.count_status.${c.status}`)}
                tone={countStatusTone(c.status)}
              />
            </div>
            <p className="text-sm">
              {t(`warehouse.count_method.${c.method}`)} ·{" "}
              {t(`warehouse.count_visibility.${c.visibility}`)}
            </p>
            <div className="text-muted-foreground flex justify-between gap-2 text-xs">
              <span>{countScopeText(c, t)}</span>
              <span>{format.dateTime(c.created_at)}</span>
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
    </WarehouseShell>
  );
}

function FieldError({ id, text }: { id: string; text?: string }) {
  return text ? (
    <p id={id} role="alert" className="text-destructive text-xs">
      {text}
    </p>
  ) : null;
}

/**
 * New stock count: warehouse, one of the four methods, blind or guided and
 * a scope (warehouse, room, location subtree, product). initial_placement
 * has no product scope; its start needs an approval on the detail page.
 */
export function NewCountForm({
  slug,
  onCancel,
}: {
  slug: string;
  onCancel: () => void;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const [v, setV] = useState<CountFormState>(EMPTY_COUNT_FORM);
  const [errors, setErrors] = useState<
    Partial<Record<keyof CountFormState, string>>
  >({});
  const [error, setError] = useState<string | null>(null);
  const [q, setQ] = useState("");
  const productQuery = useDebounce(q, 300);

  const warehouses = useQuery({
    queryKey: warehouseKeys.warehouses,
    queryFn: () => warehouseService.listWarehouses(),
  });
  const active = (warehouses.data?.items ?? []).filter((w) => w.active);
  const products = useQuery({
    queryKey: ["warehouse", "catalog-products", productQuery],
    queryFn: () =>
      catalogService.listProducts({
        q: productQuery || undefined,
        active: true,
        limit: 50,
      }),
    enabled: v.scope_type === "product",
  });

  const set = <K extends keyof CountFormState>(
    key: K,
    value: CountFormState[K],
  ) => setV((prev) => ({ ...prev, [key]: value }));

  const create = useMutation({
    mutationFn: () => warehouseService.createCount(countBody(v)),
    onSuccess: async (count) => {
      await qc.invalidateQueries({ queryKey: ["warehouse", "counts"] });
      router.push(routes.tenant.warehouse.count(slug, count.uuid));
    },
    onError: (err) =>
      setError(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    const next = countFormErrors(v, t);
    setErrors(next);
    setError(null);
    if (Object.keys(next).length === 0) create.mutate();
  };

  const scopes = scopesFor(v.method);

  return (
    <Card data-testid="count-form">
      <CardHeader>
        <CardTitle>{t("warehouse.counts.new_title")}</CardTitle>
      </CardHeader>
      <CardContent>
        <form className="space-y-4" onSubmit={onSubmit} noValidate>
          <div className="grid gap-4 sm:grid-cols-3">
            <div className="space-y-1.5">
              <Label htmlFor="count-warehouse">
                {t("warehouse.fields.warehouse")}
              </Label>
              <select
                id="count-warehouse"
                className={nativeSelectClass}
                value={v.warehouse_uuid}
                aria-invalid={Boolean(errors.warehouse_uuid)}
                data-testid="count-warehouse"
                onChange={(e) =>
                  setV((prev) => ({
                    ...prev,
                    warehouse_uuid: e.target.value,
                    room_uuid: "",
                    location_uuid: "",
                  }))
                }
              >
                <option value="">
                  {t("warehouse.entries.pick_warehouse")}
                </option>
                {active.map((w) => (
                  <option key={w.uuid} value={w.uuid}>
                    {w.code} · {w.name}
                  </option>
                ))}
              </select>
              <FieldError
                id="count-warehouse-error"
                text={errors.warehouse_uuid}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="count-method">
                {t("warehouse.fields.method")}
              </Label>
              <select
                id="count-method"
                className={nativeSelectClass}
                value={v.method}
                data-testid="count-method"
                onChange={(e) => {
                  const method = e.target.value as CountFormState["method"];
                  setV((prev) => ({
                    ...prev,
                    method,
                    scope_type: scopesFor(method).includes(prev.scope_type)
                      ? prev.scope_type
                      : "warehouse",
                  }));
                }}
              >
                {COUNT_METHODS.map((m) => (
                  <option key={m} value={m}>
                    {t(`warehouse.count_method.${m}`)}
                  </option>
                ))}
              </select>
              <p className="text-muted-foreground text-xs">
                {t(`warehouse.count_method_hint.${v.method}`)}
              </p>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="count-visibility">
                {t("warehouse.fields.visibility")}
              </Label>
              <select
                id="count-visibility"
                className={nativeSelectClass}
                value={v.visibility}
                data-testid="count-visibility"
                onChange={(e) =>
                  set(
                    "visibility",
                    e.target.value as CountFormState["visibility"],
                  )
                }
              >
                {COUNT_VISIBILITIES.map((m) => (
                  <option key={m} value={m}>
                    {t(`warehouse.count_visibility.${m}`)}
                  </option>
                ))}
              </select>
              <p className="text-muted-foreground text-xs">
                {t(`warehouse.count_visibility_hint.${v.visibility}`)}
              </p>
            </div>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="count-scope">{t("warehouse.fields.scope")}</Label>
            <select
              id="count-scope"
              className={nativeSelectClass}
              value={v.scope_type}
              data-testid="count-scope"
              onChange={(e) =>
                setV((prev) => ({
                  ...prev,
                  scope_type: e.target.value as CountFormState["scope_type"],
                  room_uuid: "",
                  location_uuid: "",
                  product_uuid: "",
                }))
              }
            >
              {scopes.map((s) => (
                <option key={s} value={s}>
                  {t(`warehouse.count_scope.${s}`)}
                </option>
              ))}
            </select>
            <FieldError id="count-scope-error" text={errors.scope_type} />
          </div>

          {v.scope_type === "room" || v.scope_type === "location" ? (
            <div className="space-y-1">
              <LocationPicker
                key={`${v.warehouse_uuid}-${v.scope_type}`}
                warehouseUuid={v.warehouse_uuid}
                value={v.location_uuid}
                onChange={(uuid) => set("location_uuid", uuid)}
                onRoom={(uuid) => set("room_uuid", uuid)}
                roomOnly={v.scope_type === "room"}
                testId="count-pick"
              />
              <FieldError
                id="count-target-error"
                text={errors.room_uuid ?? errors.location_uuid}
              />
            </div>
          ) : null}

          {v.scope_type === "product" ? (
            <div className="grid gap-2 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="count-product-search">
                  {t("warehouse.generate.product_search")}
                </Label>
                <Input
                  id="count-product-search"
                  value={q}
                  onChange={(e) => setQ(e.target.value)}
                  data-testid="count-product-search"
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="count-product">
                  {t("warehouse.fields.product")}
                </Label>
                <select
                  id="count-product"
                  className={nativeSelectClass}
                  value={v.product_uuid}
                  data-testid="count-product"
                  onChange={(e) => set("product_uuid", e.target.value)}
                >
                  <option value="">
                    {t("warehouse.generate.pick_product")}
                  </option>
                  {(products.data?.items ?? []).map((p) => (
                    <option key={p.uuid} value={p.uuid}>
                      {p.sku} · {p.name}
                    </option>
                  ))}
                </select>
                <FieldError
                  id="count-product-error"
                  text={errors.product_uuid}
                />
              </div>
            </div>
          ) : null}

          <div className="space-y-1.5">
            <Label htmlFor="count-note">{t("warehouse.fields.note")}</Label>
            <Input
              id="count-note"
              value={v.note}
              onChange={(e) => set("note", e.target.value)}
              data-testid="count-note"
            />
            <FieldError id="count-note-error" text={errors.note} />
          </div>

          {error ? (
            <p role="alert" className="text-destructive text-sm">
              {error}
            </p>
          ) : null}
          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" onClick={onCancel}>
              {t("warehouse.form.cancel")}
            </Button>
            <Button
              type="submit"
              disabled={create.isPending}
              data-testid="count-create"
            >
              {t("warehouse.counts.create")}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}
