"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Eye, Layers, Undo2 } from "lucide-react";
import { useCallback, useMemo, useState } from "react";

import { FilePickButton } from "@/components/common/file-pick-button";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { AsyncCombobox } from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Progress } from "@/components/ui/progress";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { Permission } from "@/config/permissions";
import { catalogService } from "@/features/catalog/services/catalog.service";
import {
  ACTIVE_JOB_STATUSES,
  parseBarcodeLines,
  STOCK_BULK_ACTIONS,
  stockBulkKeys,
  stockBulkService,
  type StockBulkAction,
  type StockBulkItem,
  type StockBulkJob,
  type StockBulkParams,
  type StockBulkSelection,
} from "@/features/stock-bulk/services/stock-bulk.service";
import { LocationPicker } from "@/features/warehouse/components/location-picker";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import {
  warehouseKeys,
  warehouseService,
} from "@/features/warehouse/services/warehouse.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { useDialogs } from "@/providers/dialog-provider";
import { appToast } from "@/providers/toast-provider";

export const STOCK_BULK_PERSIST_KEY = "tenant-stock-bulk-v1";
const UNIT_STATUSES = ["available", "placed", "reserved", "printed"] as const;

const statusTone = (s: string) =>
  s === "completed"
    ? "success"
    : s === "failed"
      ? "danger"
      : s === "undone" || s === "discarded"
        ? "default"
        : "warning";

/**
 * Warehouse > Bulk barcode operations (TEC-1008, super_admin): pick units
 * (filters and/or a pasted / uploaded barcode list, "BARCODE;12.5" sets a
 * roll length), preview what the action touches and what it leaves out
 * (with the reason), run it in the background after a step-up, follow its
 * progress, undo it. Every stock change is a forward ledger movement; units
 * that went to a dealer or were used in a service are never touched.
 */
export function StockBulkPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const access = useWarehouseAccess(slug, Permission.StockBulk);
  const allowed = { ...access, allowed: access.allowed && access.isCenter };
  const qc = useQueryClient();
  const { confirm } = useDialogs();

  const [action, setAction] = useState<StockBulkAction>("convert_to_roll");
  const [meters, setMeters] = useState("");
  const [targetProduct, setTargetProduct] = useState("");
  const [targetWarehouse, setTargetWarehouse] = useState("");
  const [targetLocation, setTargetLocation] = useState("");
  const [lot, setLot] = useState("");
  const [note, setNote] = useState("");
  const [filterProduct, setFilterProduct] = useState("");
  const [filterWarehouse, setFilterWarehouse] = useState("");
  const [statuses, setStatuses] = useState<string[]>([]);
  const [filterLot, setFilterLot] = useState("");
  const [barcodes, setBarcodes] = useState("");
  const [jobUuid, setJobUuid] = useState<string | null>(null);

  const warehouses = useQuery({
    queryKey: warehouseKeys.warehouses,
    queryFn: () => warehouseService.listWarehouses(),
    enabled: allowed.allowed,
  });
  const loadProducts = useCallback(async (term: string) => {
    const res = await catalogService.listProducts({ q: term, limit: 20 });
    return res.items.map((p) => ({
      value: p.uuid,
      label: p.name,
      description: p.sku,
    }));
  }, []);

  const job = useQuery({
    queryKey: stockBulkKeys.job(jobUuid ?? ""),
    queryFn: () => stockBulkService.get(jobUuid ?? ""),
    enabled: Boolean(jobUuid),
    refetchInterval: (q) =>
      q.state.data && ACTIVE_JOB_STATUSES.includes(q.state.data.status)
        ? 2000
        : false,
  });

  const errorToast = (e: unknown) =>
    appToast.error(isApiError(e) ? e.message : t("stock.bulk.failed"));

  const preview = useMutation({
    mutationFn: () => {
      const params: StockBulkParams = {};
      if (action === "convert_to_roll" && meters.trim()) params.meters = meters;
      if (action === "change_product") params.product_uuid = targetProduct;
      if (action === "move_location") params.location_uuid = targetLocation;
      if (action === "set_lot") params.lot = lot;
      if (action === "set_note") params.note = note;
      const selection: StockBulkSelection = {
        ...(filterProduct ? { product_uuids: [filterProduct] } : {}),
        ...(filterWarehouse ? { warehouse_uuid: filterWarehouse } : {}),
        ...(statuses.length
          ? { statuses: statuses as StockBulkSelection["statuses"] }
          : {}),
        ...(filterLot.trim() ? { lot: filterLot.trim() } : {}),
        ...(barcodes.trim() ? { barcodes: parseBarcodeLines(barcodes) } : {}),
      };
      return stockBulkService.preview({ action, params, selection });
    },
    onSuccess: (j) => {
      setJobUuid(j.uuid);
      qc.setQueryData(stockBulkKeys.job(j.uuid), j);
      void qc.invalidateQueries({ queryKey: ["stock-bulk", "list"] });
    },
    onError: errorToast,
  });

  const act = useMutation({
    mutationFn: ({
      kind,
      uuid,
    }: {
      kind: "run" | "undo" | "discard";
      uuid: string;
    }) =>
      kind === "run"
        ? stockBulkService.run(uuid)
        : kind === "undo"
          ? stockBulkService.undo(uuid)
          : stockBulkService.discard(uuid).then(() => null),
    onSuccess: (j, v) => {
      if (j) qc.setQueryData(stockBulkKeys.job(j.uuid), j);
      if (v.kind === "discard") setJobUuid(null);
      else setJobUuid(v.uuid);
      void qc.invalidateQueries({ queryKey: ["stock-bulk"] });
    },
    onError: errorToast,
  });

  const runJob = async (j: StockBulkJob) => {
    const ok = await confirm({
      title: t("stock.bulk.run_title"),
      description: t(
        j.reversible
          ? "stock.bulk.run_description"
          : "stock.bulk.run_irreversible",
        { count: j.eligible },
      ),
      confirmLabel: t("stock.bulk.run"),
      variant: j.reversible ? "default" : "destructive",
    });
    if (ok) act.mutate({ kind: "run", uuid: j.uuid });
  };
  const undoJob = async (j: StockBulkJob) => {
    const ok = await confirm({
      title: t("stock.bulk.undo_title"),
      description: t("stock.bulk.undo_description", { count: j.succeeded }),
      confirmLabel: t("stock.bulk.undo"),
      variant: "destructive",
    });
    if (ok) act.mutate({ kind: "undo", uuid: j.uuid });
  };

  const columns = useMemo(
    () =>
      [
        createColumn<StockBulkJob>({
          accessorKey: "created_at",
          labelKey: "stock.bulk.columns.created_at",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => format.dateTime(row.original.created_at),
        }),
        createColumn<StockBulkJob>({
          accessorKey: "action",
          labelKey: "stock.bulk.columns.action",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: STOCK_BULK_ACTIONS.map((value) => ({
            value,
            label: value,
            labelKey: `stock.bulk.action.${value}`,
          })),
          param: "action",
          cell: ({ row }) => t(`stock.bulk.action.${row.original.action}`),
        }),
        createColumn<StockBulkJob>({
          accessorKey: "status",
          labelKey: "stock.bulk.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: [
            "preview",
            "queued",
            "running",
            "completed",
            "failed",
            "undoing",
            "undone",
            "discarded",
          ].map((value) => ({
            value,
            label: value,
            labelKey: `stock.bulk.status.${value}`,
          })),
          param: "status",
          cell: ({ row }) => (
            <StatusChip
              label={t(`stock.bulk.status.${row.original.status}`)}
              tone={statusTone(row.original.status)}
            />
          ),
        }),
        createColumn<StockBulkJob>({
          accessorKey: "eligible",
          labelKey: "stock.bulk.columns.units",
          enableSorting: true,
          cell: ({ row }) => {
            const j = row.original;
            return (
              <span className="tabular-nums">
                {format.number(j.succeeded)} / {format.number(j.eligible)}
                {j.failed
                  ? ` · ${t("stock.bulk.failed_count", { count: j.failed })}`
                  : ""}
                {j.reverted
                  ? ` · ${t("stock.bulk.reverted_count", { count: j.reverted })}`
                  : ""}
              </span>
            );
          },
        }),
        createColumn<StockBulkJob>({
          accessorKey: "created_by",
          labelKey: "stock.bulk.columns.created_by",
          enableSorting: false,
          cell: ({ row }) => row.original.created_by,
        }),
        createColumn<StockBulkJob>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) => {
            const j = row.original;
            const items: EntityRowAction[] = [
              {
                id: "view",
                label: t("common.view"),
                icon: Eye,
                onSelect: () => setJobUuid(j.uuid),
              },
            ];
            if (j.reversible && j.status === "completed") {
              items.push({
                id: "undo",
                label: t("stock.bulk.undo"),
                icon: Undo2,
                onSelect: () => void undoJob(j),
              });
            }
            return <EntityRowActions actions={items} />;
          },
        }),
      ] as ColumnDef<StockBulkJob, unknown>[],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [format, t],
  );
  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: 10,
    persistKey: STOCK_BULK_PERSIST_KEY,
  });
  const history = useQuery({
    queryKey: stockBulkKeys.list(listState.params),
    queryFn: () => stockBulkService.list(listState.params),
    enabled: allowed.allowed,
  });

  const whOptions = warehouses.data?.items ?? [];
  const current = job.data;
  const canPreview =
    !preview.isPending &&
    (action !== "change_product" || targetProduct !== "") &&
    (action !== "move_location" || targetLocation !== "");

  return (
    <WarehouseShell
      slug={slug}
      access={allowed}
      title={t("stock.bulk.title")}
      description={t("stock.bulk.description")}
      icon={<Layers className="size-6" />}
      forbiddenKey="stock.bulk.forbidden"
    >
      <Card>
        <CardHeader>
          <CardTitle>{t("stock.bulk.action_title")}</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="bulk-action">{t("stock.bulk.action_label")}</Label>
            <Select
              value={action}
              onValueChange={(v) => setAction(v as StockBulkAction)}
            >
              <SelectTrigger id="bulk-action" data-testid="bulk-action">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {STOCK_BULK_ACTIONS.map((a) => (
                  <SelectItem key={a} value={a}>
                    {t(`stock.bulk.action.${a}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-muted-foreground text-xs">
              {t(`stock.bulk.action_hint.${action}`)}
            </p>
          </div>
          <div className="space-y-1.5">
            {action === "convert_to_roll" ? (
              <>
                <Label htmlFor="bulk-meters">{t("stock.bulk.meters")}</Label>
                <Input
                  id="bulk-meters"
                  inputMode="decimal"
                  dir="ltr"
                  value={meters}
                  placeholder={t("stock.bulk.meters_placeholder")}
                  onChange={(e) => setMeters(e.target.value)}
                />
              </>
            ) : null}
            {action === "change_product" ? (
              <>
                <Label htmlFor="bulk-product">
                  {t("stock.bulk.target_product")}
                </Label>
                <AsyncCombobox
                  id="bulk-product"
                  value={targetProduct}
                  onValueChange={setTargetProduct}
                  loadOptions={loadProducts}
                  placeholder={t("stock.bulk.pick_product")}
                />
              </>
            ) : null}
            {action === "move_location" ? (
              <>
                <Label htmlFor="bulk-target-wh">
                  {t("stock.bulk.target_location")}
                </Label>
                <Select
                  value={targetWarehouse}
                  onValueChange={setTargetWarehouse}
                >
                  <SelectTrigger id="bulk-target-wh">
                    <SelectValue placeholder={t("stock.bulk.pick_warehouse")} />
                  </SelectTrigger>
                  <SelectContent>
                    {whOptions.map((w) => (
                      <SelectItem key={w.uuid} value={w.uuid}>
                        {w.code} · {w.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {targetWarehouse ? (
                  <LocationPicker
                    warehouseUuid={targetWarehouse}
                    value={targetLocation}
                    onChange={setTargetLocation}
                    testId="bulk-target"
                  />
                ) : null}
              </>
            ) : null}
            {action === "set_lot" ? (
              <>
                <Label htmlFor="bulk-lot">{t("stock.bulk.lot")}</Label>
                <Input
                  id="bulk-lot"
                  maxLength={64}
                  value={lot}
                  onChange={(e) => setLot(e.target.value)}
                />
              </>
            ) : null}
            {action === "set_note" ? (
              <>
                <Label htmlFor="bulk-note">{t("stock.bulk.note")}</Label>
                <Textarea
                  id="bulk-note"
                  maxLength={2000}
                  value={note}
                  onChange={(e) => setNote(e.target.value)}
                />
              </>
            ) : null}
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("stock.bulk.selection_title")}</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="bulk-filter-product">
              {t("stock.bulk.filter_product")}
            </Label>
            <AsyncCombobox
              id="bulk-filter-product"
              value={filterProduct}
              onValueChange={setFilterProduct}
              loadOptions={loadProducts}
              clearable
              placeholder={t("stock.bulk.any")}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="bulk-filter-wh">
              {t("stock.bulk.filter_warehouse")}
            </Label>
            <Select
              value={filterWarehouse || "any"}
              onValueChange={(v) => setFilterWarehouse(v === "any" ? "" : v)}
            >
              <SelectTrigger id="bulk-filter-wh">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="any">{t("stock.bulk.any")}</SelectItem>
                {whOptions.map((w) => (
                  <SelectItem key={w.uuid} value={w.uuid}>
                    {w.code} · {w.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5">
            <Label>{t("stock.bulk.filter_status")}</Label>
            <div className="flex flex-wrap gap-3">
              {UNIT_STATUSES.map((s) => (
                <label key={s} className="flex items-center gap-1.5 text-sm">
                  <Checkbox
                    checked={statuses.includes(s)}
                    onCheckedChange={(on) =>
                      setStatuses((prev) =>
                        on ? [...prev, s] : prev.filter((x) => x !== s),
                      )
                    }
                  />
                  {t(`stock.status.${s}`)}
                </label>
              ))}
            </div>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="bulk-filter-lot">
              {t("stock.bulk.filter_lot")}
            </Label>
            <Input
              id="bulk-filter-lot"
              value={filterLot}
              onChange={(e) => setFilterLot(e.target.value)}
            />
          </div>
          <div className="space-y-1.5 md:col-span-2">
            <Label htmlFor="bulk-barcodes">{t("stock.bulk.barcodes")}</Label>
            <Textarea
              id="bulk-barcodes"
              rows={6}
              dir="ltr"
              className="font-mono text-xs"
              value={barcodes}
              placeholder={t("stock.bulk.barcodes_placeholder")}
              onChange={(e) => setBarcodes(e.target.value)}
              data-testid="bulk-barcodes"
            />
            <div className="flex flex-wrap items-center gap-2">
              <FilePickButton
                id="bulk-barcode-file"
                accept=".txt,.csv,text/plain,text/csv"
                hint={t("stock.bulk.barcodes_file_hint")}
                onFile={(file) =>
                  void file
                    .text()
                    .then((text) =>
                      setBarcodes((prev) => (prev ? `${prev}\n${text}` : text)),
                    )
                }
              />
              <span className="text-muted-foreground text-xs">
                {t("stock.bulk.barcodes_count", {
                  count: parseBarcodeLines(barcodes).length,
                })}
              </span>
            </div>
          </div>
          <div className="md:col-span-2">
            <Button
              type="button"
              disabled={!canPreview}
              onClick={() => preview.mutate()}
              data-testid="bulk-preview"
            >
              {t("stock.bulk.preview")}
            </Button>
          </div>
        </CardContent>
      </Card>

      {current ? (
        <JobPanel
          job={current}
          busy={act.isPending}
          onRun={() => void runJob(current)}
          onUndo={() => void undoJob(current)}
          onDiscard={() => act.mutate({ kind: "discard", uuid: current.uuid })}
        />
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>{t("stock.bulk.history")}</CardTitle>
        </CardHeader>
        <CardContent>
          <EntityTable
            columns={columns}
            data={history.data?.items ?? []}
            getRowId={(row) => row.uuid}
            isLoading={history.isLoading}
            isError={history.isError}
            onRetry={() => void history.refetch()}
            emptyTitle={t("stock.bulk.history_empty")}
            emptyDescription=""
            rowCount={history.data?.total ?? 0}
            state={listState.tableState}
            features={{ persistKey: STOCK_BULK_PERSIST_KEY }}
            toolbarExtra={
              <EntityToolbar
                onRefresh={() => void history.refetch()}
                refreshDisabled={history.isFetching}
              />
            }
          />
        </CardContent>
      </Card>
    </WarehouseShell>
  );
}

function JobPanel({
  job,
  busy,
  onRun,
  onUndo,
  onDiscard,
}: {
  job: StockBulkJob;
  busy: boolean;
  onRun: () => void;
  onUndo: () => void;
  onDiscard: () => void;
}) {
  const { t, format } = useLocale();
  const active = ACTIVE_JOB_STATUSES.includes(job.status);
  const done = job.succeeded + job.failed;
  const missing = ((job.selection as { missing?: string[] } | null)?.missing ??
    []) as string[];
  return (
    <Card data-testid="bulk-job">
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
        <CardTitle>
          {t(`stock.bulk.action.${job.action}`)} ·{" "}
          {format.dateTime(job.created_at)}
        </CardTitle>
        <StatusChip
          label={t(`stock.bulk.status.${job.status}`)}
          tone={statusTone(job.status)}
        />
      </CardHeader>
      <CardContent className="space-y-4 text-sm">
        <p data-testid="bulk-counts">
          {t("stock.bulk.counts", {
            total: job.total,
            eligible: job.eligible,
            excluded: job.excluded,
          })}
        </p>
        {job.status === "running" || job.status === "completed" ? (
          <Progress value={job.eligible ? (done / job.eligible) * 100 : 0} />
        ) : null}
        {job.breakdown?.length ? (
          <ul className="space-y-1">
            {job.breakdown.map((b) => (
              <li key={`${b.status}-${b.reason}`}>
                {t(`stock.bulk.item_status.${b.status}`)}
                {b.reason
                  ? ` · ${t(`stock.bulk.reason.${b.reason}`)}`
                  : ""}:{" "}
                <span className="tabular-nums">{format.number(b.count)}</span>
              </li>
            ))}
          </ul>
        ) : null}
        {missing.length ? (
          <p className="text-destructive">
            {t("stock.bulk.missing", { count: missing.length })}{" "}
            <span className="font-mono text-xs" dir="ltr">
              {missing.slice(0, 20).join(", ")}
            </span>
          </p>
        ) : null}
        {job.sample_eligible?.length ? (
          <SampleList
            title={t("stock.bulk.sample_eligible")}
            items={job.sample_eligible}
          />
        ) : null}
        {job.sample_excluded?.length ? (
          <SampleList
            title={t("stock.bulk.sample_excluded")}
            items={job.sample_excluded}
          />
        ) : null}
        {job.error ? <p className="text-destructive">{job.error}</p> : null}
        <div className="flex flex-wrap gap-2">
          {job.status === "preview" ? (
            <>
              <Button
                type="button"
                disabled={busy || job.eligible === 0}
                variant={job.reversible ? "default" : "destructive"}
                onClick={onRun}
                data-testid="bulk-run"
              >
                {t("stock.bulk.run")}
              </Button>
              <Button
                type="button"
                variant="outline"
                disabled={busy}
                onClick={onDiscard}
              >
                {t("stock.bulk.discard")}
              </Button>
            </>
          ) : null}
          {job.status === "completed" && job.reversible ? (
            <Button
              type="button"
              variant="outline"
              disabled={busy}
              onClick={onUndo}
            >
              {t("stock.bulk.undo")}
            </Button>
          ) : null}
          {active ? (
            <span className="text-muted-foreground">
              {t("stock.bulk.working")}
            </span>
          ) : null}
        </div>
      </CardContent>
    </Card>
  );
}

function SampleList({
  title,
  items,
}: {
  title: string;
  items: StockBulkItem[];
}) {
  const { t } = useLocale();
  return (
    <section className="space-y-1">
      <h4 className="font-medium">{title}</h4>
      <ul className="space-y-0.5">
        {items.map((i) => (
          <li key={i.unit_uuid} className="flex flex-wrap gap-x-2">
            <span className="font-mono text-xs" dir="ltr">
              {i.barcode}
            </span>
            <span>{i.product.name}</span>
            {i.meters ? <span>→ {i.meters} m</span> : null}
            {i.reason ? (
              <span className="text-muted-foreground">
                {t(`stock.bulk.reason.${i.reason}`)}
              </span>
            ) : null}
          </li>
        ))}
      </ul>
    </section>
  );
}
