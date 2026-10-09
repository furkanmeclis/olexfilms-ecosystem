"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Eye, PackagePlus, Plus, XCircle } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { AppForm, AppInput, AppSelect } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  enumFilterOptions,
  useWarehouseFilterOptions,
} from "@/features/warehouse/components/table-options";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import {
  ENTRY_MODES,
  entryStatusTone,
  ENTRY_STATUSES,
} from "@/features/warehouse/lib/entries";
import { warehouseErrorMessage } from "@/features/warehouse/lib/errors";
import {
  entryFormSchema,
  type EntryFormValues,
  type EntryMode,
} from "@/features/warehouse/lib/forms";
import {
  warehouseKeys,
  warehouseService,
  type StockEntry,
  type StockEntryListQuery,
  type Warehouse,
} from "@/features/warehouse/services/warehouse.service";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const ENTRY_PAGE_SIZE = 20;
export const ENTRIES_PERSIST_KEY = "tenant-warehouse-entries-v1";

/**
 * Warehouse > Stock entries (TEC-204, TEC-376): the entry documents of the
 * active organization over a server DataTable — sort (created, status,
 * warehouse), search (note, warehouse name / code), status / mode /
 * warehouse / created filters, row actions (open, cancel a draft) and
 * mobile cards — and a new draft at one of its warehouses. Only the center
 * reserves new barcodes in an entry (generate_new, K14); a distributor
 * takes in printed labels (with_existing).
 */
export function StockEntriesPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const { confirm } = useDialogs();
  const access = useWarehouseAccess(slug);
  const canWrite = access.can(Permission.WarehouseWrite);
  const [creating, setCreating] = useState(false);
  const warehouseOptions = useWarehouseFilterOptions(access.allowed);

  const cancel = useMutation({
    mutationFn: (uuid: string) => warehouseService.cancel(uuid),
    onSuccess: async (entry) => {
      qc.setQueryData(warehouseKeys.entry(entry.uuid), entry);
      await qc.invalidateQueries({ queryKey: ["warehouse", "entries"] });
      appToast.success(t("warehouse.entry.cancelled"));
    },
    onError: (err) =>
      appToast.error(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });
  const runCancel = cancel.mutate;
  const cancelPending = cancel.isPending;

  const columns = useMemo(() => {
    const askCancel = async (entry: StockEntry) => {
      const ok = await confirm({
        title: t("warehouse.entry.cancel_title"),
        description: t("warehouse.entry.cancel_description"),
        confirmLabel: t("warehouse.entry.cancel"),
        variant: "destructive",
      });
      if (ok) runCancel(entry.uuid);
    };
    return [
      createColumn<StockEntry>({
        id: "warehouse",
        accessorFn: (e) => e.warehouse?.name ?? "",
        labelKey: "warehouse.entries.columns.warehouse",
        enableSorting: true,
        gridPrimary: true,
        filterVariant: "faceted",
        filterOptions: warehouseOptions,
        enableColumnFilter: warehouseOptions.length > 0,
        param: "warehouse_uuid",
        cell: ({ row }) => {
          const e = row.original;
          return (
            <div className="min-w-0">
              <Link
                href={routes.tenant.warehouse.entry(slug, e.uuid)}
                className="font-medium hover:underline"
                data-testid="entry-row"
                data-uuid={e.uuid}
                onClick={(event) => event.stopPropagation()}
              >
                {e.warehouse
                  ? `${e.warehouse.code} · ${e.warehouse.name}`
                  : t("warehouse.entries.no_warehouse")}
              </Link>
              {e.note ? (
                <div className="text-muted-foreground truncate text-xs">
                  {e.note}
                </div>
              ) : null}
            </div>
          );
        },
      }),
      createColumn<StockEntry>({
        accessorKey: "status",
        labelKey: "warehouse.entries.columns.status",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: enumFilterOptions(
          ENTRY_STATUSES,
          "warehouse.entry_status",
        ),
        param: "status",
        cell: ({ row }) => (
          <StatusChip
            label={t(`warehouse.entry_status.${row.original.status}`)}
            tone={entryStatusTone(row.original.status)}
          />
        ),
      }),
      createColumn<StockEntry>({
        accessorKey: "mode",
        labelKey: "warehouse.entries.columns.mode",
        enableSorting: false,
        filterVariant: "faceted",
        filterOptions: enumFilterOptions(ENTRY_MODES, "warehouse.entry_mode"),
        param: "mode",
        gridSecondary: true,
        cell: ({ row }) => t(`warehouse.entry_mode.${row.original.mode}`),
      }),
      createColumn<StockEntry>({
        accessorKey: "line_count",
        labelKey: "warehouse.entries.columns.lines",
        enableSorting: false,
        cell: ({ row }) => format.number(row.original.line_count),
      }),
      createColumn<StockEntry>({
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
      createColumn<StockEntry>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => {
          const entry = row.original;
          const items: EntityRowAction[] = [
            {
              id: "view",
              label: t("common.view"),
              icon: Eye,
              onSelect: () =>
                router.push(routes.tenant.warehouse.entry(slug, entry.uuid)),
            },
          ];
          if (canWrite && entry.status === "draft") {
            items.push({
              id: "cancel",
              label: t("warehouse.entry.cancel"),
              icon: XCircle,
              variant: "destructive",
              disabled: cancelPending,
              onSelect: () => void askCancel(entry),
            });
          }
          return <EntityRowActions actions={items} />;
        },
      }),
    ] as ColumnDef<StockEntry, unknown>[];
  }, [
    canWrite,
    cancelPending,
    confirm,
    format,
    router,
    runCancel,
    slug,
    t,
    warehouseOptions,
  ]);

  // Column meta drives the params: warehouse_uuid / status / mode (CSV),
  // created (created_from/_to); sort created_at | status | warehouse.
  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: ENTRY_PAGE_SIZE,
    persistKey: ENTRIES_PERSIST_KEY,
  });
  const query: StockEntryListQuery = listState.params;
  const list = useQuery({
    queryKey: warehouseKeys.entries(query),
    queryFn: () => warehouseService.listEntries(query),
    enabled: access.allowed,
  });

  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={t("warehouse.entries.title")}
      description={t("warehouse.entries.description")}
      icon={<PackagePlus className="size-6" />}
      actions={
        canWrite && !creating ? (
          <Button
            type="button"
            onClick={() => setCreating(true)}
            data-testid="entry-new"
          >
            <Plus className="size-4" />
            {t("warehouse.entries.new")}
          </Button>
        ) : null
      }
    >
      {creating ? (
        <NewEntryForm
          slug={slug}
          isCenter={access.isCenter}
          onCancel={() => setCreating(false)}
        />
      ) : null}

      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.tenant.warehouse.entry(slug, row.uuid))
        }
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("warehouse.entries.empty_title")}
        emptyDescription={t("warehouse.entries.empty_description")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: ENTRIES_PERSIST_KEY,
          rowSelection: false,
          viewMode: true,
        }}
        renderGridItem={(e) => (
          <div className="space-y-2">
            <div className="flex items-start justify-between gap-2">
              <span className="font-medium">
                {e.warehouse
                  ? `${e.warehouse.code} · ${e.warehouse.name}`
                  : t("warehouse.entries.no_warehouse")}
              </span>
              <StatusChip
                label={t(`warehouse.entry_status.${e.status}`)}
                tone={entryStatusTone(e.status)}
              />
            </div>
            {e.note ? (
              <p className="text-muted-foreground truncate text-xs">{e.note}</p>
            ) : null}
            <div className="text-muted-foreground flex justify-between gap-2 text-xs">
              <span>
                {t(`warehouse.entry_mode.${e.mode}`)} ·{" "}
                {format.number(e.line_count)}
              </span>
              <span>{format.dateTime(e.created_at)}</span>
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

/** Draft entry form: warehouse, mode (generate_new center only), note. */
export function NewEntryForm({
  slug,
  isCenter,
  onCancel,
}: {
  slug: string;
  isCenter: boolean;
  onCancel: () => void;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const modes: EntryMode[] = isCenter
    ? ["generate_new", "with_existing"]
    : ["with_existing"];

  const warehouses = useQuery({
    queryKey: warehouseKeys.warehouses,
    queryFn: () => warehouseService.listWarehouses(),
  });
  const active: Warehouse[] = (warehouses.data?.items ?? []).filter(
    (w) => w.active,
  );

  const create = useMutation({
    mutationFn: (v: EntryFormValues) =>
      warehouseService.createEntry({
        warehouse_uuid: v.warehouse_uuid,
        mode: v.mode as EntryMode,
        note: v.note || null,
      }),
    onSuccess: async (entry) => {
      await qc.invalidateQueries({ queryKey: ["warehouse", "entries"] });
      router.push(routes.tenant.warehouse.entry(slug, entry.uuid));
    },
  });

  return (
    <Card data-testid="entry-form">
      <CardHeader>
        <CardTitle>{t("warehouse.entries.new_title")}</CardTitle>
      </CardHeader>
      <CardContent>
        {warehouses.isSuccess && active.length === 0 ? (
          <p
            className="text-muted-foreground text-sm"
            data-testid="entry-no-warehouse"
          >
            {t("warehouse.entries.no_active_warehouse")}
          </p>
        ) : null}
        <AppForm<EntryFormValues>
          schema={entryFormSchema(t, modes) as never}
          defaultValues={{ warehouse_uuid: "", mode: modes[0], note: "" }}
          onSubmit={async (v) => {
            setError(null);
            try {
              await create.mutateAsync(v);
            } catch (err) {
              setError(
                warehouseErrorMessage(err, t, t("warehouse.form.error")),
              );
            }
          }}
          className="space-y-4"
        >
          <AppSelect
            name="warehouse_uuid"
            label={t("warehouse.fields.warehouse")}
            placeholder={t("warehouse.entries.pick_warehouse")}
            options={active.map((w) => ({
              value: w.uuid,
              label: `${w.code} · ${w.name}`,
            }))}
            testId="entry-warehouse"
          />
          <AppSelect
            name="mode"
            label={t("warehouse.fields.mode")}
            description={
              isCenter
                ? t("warehouse.entries.mode_hint_center")
                : t("warehouse.entries.mode_hint_distributor")
            }
            options={modes.map((m) => ({
              value: m,
              label: t(`warehouse.entry_mode.${m}`),
            }))}
            testId="entry-mode"
          />
          <AppInput
            name="note"
            label={t("warehouse.fields.note")}
            data-testid="entry-note"
          />
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
              data-testid="entry-create"
            >
              {t("warehouse.entries.create")}
            </Button>
          </div>
        </AppForm>
      </CardContent>
    </Card>
  );
}
