"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, PackagePlus, Plus } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { AppForm, AppInput } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { NativeSelectField } from "@/features/warehouse/components/native-select-field";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import {
  entryStatusTone,
  ENTRY_STATUSES,
} from "@/features/warehouse/lib/entries";
import {
  pageCount,
  warehouseErrorMessage,
} from "@/features/warehouse/lib/errors";
import {
  entryFormSchema,
  type EntryFormValues,
  type EntryMode,
} from "@/features/warehouse/lib/forms";
import {
  warehouseKeys,
  warehouseService,
  type StockEntryListQuery,
  type StockEntryStatus,
  type Warehouse,
} from "@/features/warehouse/services/warehouse.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export const ENTRY_PAGE_SIZE = 20;
const ALL = "all";

/**
 * Warehouse > Stock entries (TEC-204): the entry documents of the active
 * organization and a new draft at one of its warehouses. Only the center
 * reserves new barcodes in an entry (generate_new, K14); a distributor
 * takes in printed labels (with_existing).
 */
export function StockEntriesPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const access = useWarehouseAccess(slug);
  const canWrite = access.can(Permission.WarehouseWrite);
  const [status, setStatus] = useState<StockEntryStatus | typeof ALL>(ALL);
  const [page, setPage] = useState(0);
  const [creating, setCreating] = useState(false);

  const query = useMemo<StockEntryListQuery>(
    () => ({
      ...(status === ALL ? {} : { status }),
      limit: ENTRY_PAGE_SIZE,
      offset: page * ENTRY_PAGE_SIZE,
    }),
    [status, page],
  );
  const list = useQuery({
    queryKey: warehouseKeys.entries(query),
    queryFn: () => warehouseService.listEntries(query),
    enabled: access.allowed,
    placeholderData: keepPreviousData,
  });
  const rows = list.data?.items ?? [];
  const total = list.data?.total ?? 0;
  const pages = pageCount(total, ENTRY_PAGE_SIZE);

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

      <Card>
        <CardContent className="space-y-4 pt-6">
          <div
            className="flex flex-wrap gap-2"
            role="group"
            aria-label={t("warehouse.entries.status_filter")}
          >
            {[ALL, ...ENTRY_STATUSES].map((s) => (
              <Button
                key={s}
                type="button"
                size="sm"
                variant={status === s ? "secondary" : "ghost"}
                aria-pressed={status === s}
                onClick={() => {
                  setStatus(s as StockEntryStatus | typeof ALL);
                  setPage(0);
                }}
              >
                {s === ALL
                  ? t("warehouse.entries.all_statuses")
                  : t(`warehouse.entry_status.${s}`)}
              </Button>
            ))}
          </div>

          {list.isError ? (
            <ErrorState
              title={t("common.error_generic")}
              onRetry={() => void list.refetch()}
              retryLabel={t("common.retry")}
            />
          ) : list.isLoading ? (
            <p className="text-muted-foreground text-sm">
              {t("warehouse.list.loading")}
            </p>
          ) : rows.length === 0 ? (
            <div className="py-8 text-center" data-testid="entries-empty">
              <p className="font-medium">
                {t("warehouse.entries.empty_title")}
              </p>
              <p className="text-muted-foreground text-sm">
                {t("warehouse.entries.empty_description")}
              </p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm" data-testid="entries-table">
                <thead>
                  <tr className="text-muted-foreground border-b text-xs">
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.entries.columns.warehouse")}
                    </th>
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.entries.columns.status")}
                    </th>
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.entries.columns.mode")}
                    </th>
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.entries.columns.lines")}
                    </th>
                    <th className="p-2 text-start font-medium">
                      {t("warehouse.entries.columns.created")}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((e) => (
                    <tr
                      key={e.uuid}
                      className="hover:bg-accent/50 border-b last:border-0"
                      data-testid="entry-row"
                    >
                      <td className="p-2">
                        <Link
                          href={routes.tenant.warehouse.entry(slug, e.uuid)}
                          className="font-medium hover:underline"
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
                      </td>
                      <td className="p-2">
                        <StatusChip
                          label={t(`warehouse.entry_status.${e.status}`)}
                          tone={entryStatusTone(e.status)}
                        />
                      </td>
                      <td className="p-2">
                        {t(`warehouse.entry_mode.${e.mode}`)}
                      </td>
                      <td className="p-2">
                        {e.placed_count !== undefined
                          ? t("warehouse.entries.placed_of", {
                              placed: e.placed_count,
                              total: e.line_count,
                            })
                          : format.number(e.line_count)}
                      </td>
                      <td className="text-muted-foreground p-2 text-xs whitespace-nowrap">
                        {format.dateTime(e.created_at)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          <div
            className={cn(
              "flex flex-wrap items-center justify-between gap-2",
              rows.length === 0 && page === 0 && "hidden",
            )}
          >
            <p className="text-muted-foreground text-sm">
              {t("warehouse.list.page", { page: page + 1, pages, total })}
            </p>
            <div className="flex gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={page === 0 || list.isFetching}
                onClick={() => setPage((p) => Math.max(0, p - 1))}
              >
                <ChevronLeft className="size-4 rtl:rotate-180" />
                {t("warehouse.list.prev")}
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={page + 1 >= pages || list.isFetching}
                onClick={() => setPage((p) => p + 1)}
              >
                {t("warehouse.list.next")}
                <ChevronRight className="size-4 rtl:rotate-180" />
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>
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
          <NativeSelectField
            name="warehouse_uuid"
            label={t("warehouse.fields.warehouse")}
            placeholder={t("warehouse.entries.pick_warehouse")}
            options={active.map((w) => ({
              value: w.uuid,
              label: `${w.code} · ${w.name}`,
            }))}
            testId="entry-warehouse"
          />
          <NativeSelectField
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
