"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CheckCircle2,
  FileSpreadsheet,
  MapPin,
  PackagePlus,
  TriangleAlert,
  XCircle,
} from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { ActivityTimeline } from "@/components/common/activity-timeline";
import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { AsyncCombobox } from "@/components/ui/async-combobox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ImportWizard } from "@/features/io/components/import-wizard";
import {
  MAX_UNIT_LABELS,
  unitLabelsPath,
} from "@/features/stock/services/stock.service";
import {
  ArrivalScanCard,
  ContainerDeliveryCard,
  MappingTemplateBar,
  supplierTemplate,
  useEntryImportTemplates,
} from "@/features/warehouse/components/container-entry";
import { EntryLinesTable } from "@/features/warehouse/components/entry-lines-table";
import { GenerateForm } from "@/features/warehouse/components/generate-form";
import { LabelButton } from "@/features/warehouse/components/label-button";
import { ScanInput } from "@/features/warehouse/components/scan-input";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import {
  canConfirm,
  entryStatusTone,
  importLinesKept,
  linesToPlace,
  unplacedLines,
} from "@/features/warehouse/lib/entries";
import { warehouseErrorMessage } from "@/features/warehouse/lib/errors";
import { entryImportConfirmCopy } from "@/features/warehouse/lib/entry-import";
import { scanKind, unitBarcode } from "@/features/warehouse/lib/scan";
import {
  buildLocationTree,
  type LocationNode,
} from "@/features/warehouse/lib/tree";
import {
  warehouseKeys,
  warehouseService,
  type StockEntry,
} from "@/features/warehouse/services/warehouse.service";
import { voidHeldWarnings } from "@/features/warehouse/lib/void-result";
import { useLocale } from "@/providers/locale-provider";
import { useDialogs } from "@/providers/dialog-provider";
import { appToast } from "@/providers/toast-provider";

/** Flattened tree as indented select options. */
function locationOptions(
  nodes: LocationNode[],
  depth = 0,
): { value: string; label: string }[] {
  return nodes.flatMap((n) => [
    {
      value: n.uuid,
      label: `${"  ".repeat(depth)}${n.full_code}${n.active ? "" : " ×"}`,
    },
    ...locationOptions(n.children, depth + 1),
  ]);
}

/**
 * One stock entry (TEC-204): add lines (scan printed labels, or reserve new
 * barcodes at the center), place them on locations (pick or scan a
 * location QR), print labels, then confirm (ledger entry + placement) or
 * cancel.
 */
export function StockEntryDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format } = useLocale();
  const access = useWarehouseAccess(slug);
  const canWrite = access.can(Permission.WarehouseWrite);
  const qc = useQueryClient();
  const { confirm } = useDialogs();
  const [selected, setSelected] = useState<string[]>([]);
  const [roomUuid, setRoomUuid] = useState("");
  const [locationUuid, setLocationUuid] = useState("");
  const [lineError, setLineError] = useState<string | null>(null);
  const [placeError, setPlaceError] = useState<string | null>(null);
  const [importOpen, setImportOpen] = useState(false);
  const [fixedPending, setFixedPending] = useState<{
    barcode: string;
    product: string;
  } | null>(null);

  const key = warehouseKeys.entry(uuid);
  const entryQuery = useQuery({
    queryKey: key,
    queryFn: () => warehouseService.getEntry(uuid),
    enabled: access.allowed,
  });
  const entry = entryQuery.data;
  const draft = entry?.status === "draft";
  const editable = canWrite && draft;
  const warehouseUuid = entry?.warehouse?.uuid ?? "";
  const templates = useEntryImportTemplates(
    Boolean(editable && entry?.mode === "with_existing"),
  );

  const rooms = useQuery({
    queryKey: warehouseKeys.rooms(warehouseUuid),
    queryFn: () => warehouseService.listRooms(warehouseUuid),
    enabled: Boolean(editable && warehouseUuid),
  });
  const locations = useQuery({
    queryKey: warehouseKeys.locations(roomUuid),
    queryFn: () => warehouseService.listLocations(roomUuid),
    enabled: Boolean(editable && roomUuid),
  });
  const locOptions = useMemo(
    () => locationOptions(buildLocationTree(locations.data?.items ?? [])),
    [locations.data],
  );

  const apply = (next: StockEntry) => {
    qc.setQueryData(key, next);
    void qc.invalidateQueries({ queryKey: ["warehouse", "entries"] });
  };

  const addLines = useMutation({
    mutationFn: (body: Parameters<typeof warehouseService.addLines>[1]) =>
      warehouseService.addLines(uuid, body),
    onSuccess: (next) => {
      setLineError(null);
      apply(next);
    },
  });
  const place = useMutation({
    mutationFn: (body: Parameters<typeof warehouseService.place>[1]) =>
      warehouseService.place(uuid, body),
    onSuccess: (next) => {
      setPlaceError(null);
      setSelected([]);
      apply(next);
      appToast.success(t("warehouse.entry.placed"));
    },
    onError: (err) =>
      setPlaceError(
        warehouseErrorMessage(err, t, t("warehouse.entry.place_failed")),
      ),
  });
  const removeLine = useMutation({
    mutationFn: (lineUuid: string) =>
      warehouseService.deleteLine(uuid, lineUuid),
    onSuccess: apply,
    onError: (err) =>
      appToast.error(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });
  const voidSelected = useMutation({
    mutationFn: async () => {
      const selectedLines = (entry?.lines ?? []).filter((line) =>
        selected.includes(line.uuid),
      );
      const result = await warehouseService.voidBarcodes({
        unit_uuids: selectedLines.map((line) => line.unit_uuid),
      });
      for (const item of result.items) {
        if (!item.voided) continue;
        const line = selectedLines.find(
          (candidate) => candidate.unit_uuid === item.unit_uuid,
        );
        if (line) await warehouseService.deleteLine(uuid, line.uuid);
      }
      return result;
    },
    onSuccess: (result) => {
      setSelected([]);
      void qc.invalidateQueries({ queryKey: key });
      appToast.success(
        t("warehouse.barcodes.void_result", {
          voided: result.items.filter((item) => item.voided).length,
          rejected: result.items.filter((item) => !item.voided).length,
        }),
      );
      for (const message of voidHeldWarnings(result.items, t))
        appToast.warning(message);
    },
    onError: (err) =>
      appToast.error(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });
  const askVoidSelected = async () => {
    const ok = await confirm({
      title: t("warehouse.barcodes.void_title"),
      description: t("warehouse.barcodes.void_description", {
        count: selected.length,
      }),
      confirmLabel: t("warehouse.barcodes.void_selected"),
      variant: "destructive",
    });
    if (ok) voidSelected.mutate();
  };
  const finish = useMutation({
    mutationFn: (action: "confirm" | "cancel") =>
      action === "confirm"
        ? warehouseService.confirm(uuid)
        : warehouseService.cancel(uuid),
    onSuccess: (next, action) => {
      apply(next);
      appToast.success(
        action === "confirm"
          ? t("warehouse.entry.confirmed")
          : t("warehouse.entry.cancelled"),
      );
    },
    onError: (err) =>
      appToast.error(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });

  const addBarcode = async (barcode: string, fixedQuantity?: number) => {
    try {
      await addLines.mutateAsync({
        barcodes: [barcode],
        ...(fixedQuantity ? { fixed_quantity: fixedQuantity } : {}),
      });
    } catch (err) {
      setLineError(
        warehouseErrorMessage(err, t, t("warehouse.entry.add_failed")),
      );
    }
  };
  const onBarcode = async (code: string) => {
    if (scanKind(code) === "location") {
      setLineError(t("warehouse.entry.scan_location_here"));
      return;
    }
    const barcode = unitBarcode(code);
    // TEC-1032: a fixed barcode stands for N boxes; ask how many arrived
    // (the API takes fixed_quantity, default 1).
    const scan = await warehouseService.scan(barcode).catch(() => null);
    if (scan?.unit?.unit_kind === "fixed") {
      setFixedPending({ barcode, product: scan.product?.name ?? "" });
      return;
    }
    await addBarcode(barcode);
  };

  const placeBody = (
    target: { location_uuid: string } | { location_code: string },
  ) =>
    entry ? { ...target, line_uuids: linesToPlace(entry, selected) } : target;

  const onLocationScan = (code: string) => {
    if (scanKind(code) === "unit") {
      setPlaceError(t("warehouse.entry.scan_unit_here"));
      return;
    }
    place.mutate(placeBody({ location_code: code }));
  };

  const askFinish = async (action: "confirm" | "cancel") => {
    const ok = await confirm({
      title: t(`warehouse.entry.${action}_title`),
      description: t(`warehouse.entry.${action}_description`),
      confirmLabel: t(`warehouse.entry.${action}`),
      variant: action === "cancel" ? "destructive" : "default",
    });
    if (ok) finish.mutate(action);
  };

  const title = t("warehouse.entry.title");
  const lines = entry?.lines ?? [];
  const unplaced = entry ? unplacedLines(entry).length : 0;

  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={title}
      icon={<PackagePlus className="size-6" />}
      crumbs={[
        {
          label: t("warehouse.entries.title"),
          href: routes.tenant.warehouse.entries(slug),
        },
      ]}
      actions={
        entry && editable ? (
          <div className="flex flex-wrap gap-2">
            {selected.length > 0 ? (
              <Button
                type="button"
                variant="destructive"
                disabled={voidSelected.isPending}
                onClick={() => void askVoidSelected()}
              >
                {t("warehouse.barcodes.void_selected")}
              </Button>
            ) : null}
            <Button
              type="button"
              variant="outline"
              onClick={() => void askFinish("cancel")}
              disabled={finish.isPending}
              data-testid="entry-cancel"
            >
              <XCircle className="size-4" />
              {t("warehouse.entry.cancel")}
            </Button>
            <Button
              type="button"
              onClick={() => void askFinish("confirm")}
              disabled={finish.isPending || !canConfirm(entry)}
              title={
                canConfirm(entry)
                  ? undefined
                  : t("warehouse.entry.confirm_blocked")
              }
              data-testid="entry-confirm"
            >
              <CheckCircle2 className="size-4" />
              {t("warehouse.entry.confirm")}
            </Button>
          </div>
        ) : null
      }
    >
      {entryQuery.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void entryQuery.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : !entry ? (
        <p className="text-muted-foreground text-sm">
          {t("warehouse.list.loading")}
        </p>
      ) : (
        <>
          {editable && entry.mode === "with_existing" ? (
            <ImportWizard
              resource="tenant.warehouse.entry_barcodes"
              scope="tenant"
              paths={{
                upload: `/v1/warehouse/stock-entries/${uuid}/import`,
                sample: `/v1/warehouse/stock-entries/${uuid}/import/sample`,
              }}
              open={importOpen}
              onOpenChange={setImportOpen}
              onComplete={() => void qc.invalidateQueries({ queryKey: key })}
              resultDetailHref={routes.tenant.warehouse.entry(slug, uuid)}
              jobsHref=""
              title={t("warehouse.entry_import.title")}
              initialOptions={{ existing_policy: "none" }}
              extraOptions={({ values, setValue }) => (
                <fieldset className="space-y-2">
                  <legend className="text-sm font-medium">
                    {t("warehouse.entry_import.policy_title")}
                  </legend>
                  <div className="grid gap-2 sm:grid-cols-3">
                    {(["none", "all_existing", "conflicts_only"] as const).map(
                      (policy) => (
                        <button
                          key={policy}
                          type="button"
                          aria-pressed={values.existing_policy === policy}
                          onClick={() => setValue("existing_policy", policy)}
                          className={`rounded-lg border p-3 text-start text-sm ${values.existing_policy === policy ? "border-primary bg-primary/5" : "border-border"}`}
                        >
                          <span className="block font-medium">
                            {t(`warehouse.entry_import.policy.${policy}.title`)}
                          </span>
                          <span className="text-muted-foreground mt-1 block">
                            {t(
                              `warehouse.entry_import.policy.${policy}.description`,
                            )}
                          </span>
                        </button>
                      ),
                    )}
                  </div>
                </fieldset>
              )}
              confirmCopy={entryImportConfirmCopy}
              presetMapping={() =>
                supplierTemplate(
                  templates.data?.items ?? [],
                  entry.supplier?.uuid,
                )?.mapping ?? null
              }
              mappingTools={(tools) => (
                <MappingTemplateBar
                  tools={tools}
                  templates={templates.data?.items ?? []}
                  supplier={entry.supplier}
                />
              )}
            />
          ) : null}
          <Card>
            <CardContent className="grid gap-4 pt-6 sm:grid-cols-4">
              <Info label={t("warehouse.fields.warehouse")}>
                {entry.warehouse
                  ? `${entry.warehouse.code} · ${entry.warehouse.name}`
                  : "—"}
              </Info>
              <Info label={t("warehouse.entries.columns.status")}>
                <span data-testid="entry-status" data-status={entry.status}>
                  <StatusChip
                    label={t(`warehouse.entry_status.${entry.status}`)}
                    tone={entryStatusTone(entry.status)}
                  />
                </span>
              </Info>
              <Info label={t("warehouse.fields.mode")}>
                {t(`warehouse.entry_mode.${entry.mode}`)}
              </Info>
              <Info label={t("warehouse.entries.columns.lines")}>
                <span data-testid="entry-counts">
                  {t("warehouse.entries.placed_of", {
                    placed: entry.placed_count ?? lines.length - unplaced,
                    total: entry.line_count,
                  })}
                </span>
              </Info>
              <Info label={t("warehouse.entry.created_at")}>
                <span data-testid="entry-created-at">
                  {format.dateTime(entry.created_at)}
                </span>
              </Info>
              {entry.confirmed_at ? (
                <Info label={t("warehouse.entry.confirmed_at")}>
                  <span data-testid="entry-confirmed-at">
                    {format.dateTime(entry.confirmed_at)}
                  </span>
                </Info>
              ) : null}
              {entry.cancelled_at ? (
                <Info label={t("warehouse.entry.cancelled_at")}>
                  {format.dateTime(entry.cancelled_at)}
                </Info>
              ) : null}
              {entry.note ? (
                <div className="sm:col-span-4">
                  <Info label={t("warehouse.fields.note")}>{entry.note}</Info>
                </div>
              ) : null}
              {entry.mode === "import" ? (
                <div className="sm:col-span-4">
                  <ImportSource slug={slug} entry={entry} />
                </div>
              ) : null}
            </CardContent>
          </Card>

          {entry.mode !== "import" ? (
            <ContainerDeliveryCard
              key={entry.uuid}
              entry={entry}
              editable={editable}
              onSaved={apply}
            />
          ) : null}

          {editable && lines.length > 0 ? (
            <ArrivalScanCard
              entry={entry}
              onChanged={() => void qc.invalidateQueries({ queryKey: key })}
            />
          ) : null}

          {editable ? (
            <Card>
              <CardHeader>
                <CardTitle>{t("warehouse.entry.add_title")}</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                {entry.mode === "generate_new" ? (
                  <GenerateForm
                    submitLabel={t("warehouse.entry.generate")}
                    pending={addLines.isPending}
                    onSubmit={(body) => addLines.mutateAsync(body)}
                  />
                ) : (
                  <>
                    <ScanInput
                      expect="unit"
                      warehouseUuid={entry.warehouse?.uuid}
                      id="entry-barcode"
                      label={t("warehouse.entry.scan_barcode")}
                      hint={t("warehouse.entry.scan_barcode_hint")}
                      busy={addLines.isPending}
                      onScan={onBarcode}
                    />
                    {entry.mode === "with_existing" ? (
                      <Button
                        type="button"
                        variant="outline"
                        onClick={() => setImportOpen(true)}
                      >
                        {t("warehouse.entry_import.open")}
                      </Button>
                    ) : null}
                    {lineError ? (
                      <p
                        role="alert"
                        className="text-destructive text-sm"
                        data-testid="entry-line-error"
                      >
                        {lineError}
                      </p>
                    ) : null}
                  </>
                )}
              </CardContent>
            </Card>
          ) : null}

          {editable && lines.length > 0 ? (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <MapPin className="size-4" />
                  {t("warehouse.entry.place_title")}
                </CardTitle>
                <p className="text-muted-foreground text-sm">
                  {selected.length > 0
                    ? t("warehouse.entry.place_selected", {
                        count: selected.length,
                      })
                    : t("warehouse.entry.place_unplaced", { count: unplaced })}
                </p>
              </CardHeader>
              <CardContent className="grid gap-6 lg:grid-cols-2">
                <ScanInput
                  expect="bin"
                  warehouseUuid={entry.warehouse?.uuid}
                  id="entry-location"
                  autoFocus={false}
                  label={t("warehouse.entry.scan_location")}
                  hint={t("warehouse.entry.scan_location_hint")}
                  busy={place.isPending}
                  onScan={onLocationScan}
                />
                <div className="space-y-2">
                  <div className="grid gap-2 sm:grid-cols-2">
                    <div className="space-y-1.5">
                      <Label htmlFor="entry-room">
                        {t("warehouse.fields.room")}
                      </Label>
                      <Select
                        value={roomUuid}
                        onValueChange={(value) => {
                          setRoomUuid(value);
                          setLocationUuid("");
                        }}
                      >
                        <SelectTrigger id="entry-room" data-testid="entry-room">
                          <SelectValue
                            placeholder={t("warehouse.entry.pick_room")}
                          />
                        </SelectTrigger>
                        <SelectContent>
                          {(rooms.data?.items ?? []).map((r) => (
                            <SelectItem key={r.uuid} value={r.uuid}>
                              {r.code} · {r.name}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="space-y-1.5">
                      <Label htmlFor="entry-location-pick">
                        {t("warehouse.fields.location")}
                      </Label>
                      <AsyncCombobox
                        id="entry-location-pick"
                        data-testid="entry-location-pick"
                        value={locationUuid}
                        disabled={!roomUuid}
                        placeholder={t("warehouse.entry.pick_location")}
                        options={locOptions}
                        onValueChange={setLocationUuid}
                      />
                    </div>
                  </div>
                  <Button
                    type="button"
                    variant="outline"
                    disabled={!locationUuid || place.isPending}
                    onClick={() =>
                      place.mutate(placeBody({ location_uuid: locationUuid }))
                    }
                    data-testid="entry-place"
                  >
                    {t("warehouse.entry.place")}
                  </Button>
                </div>
                {placeError ? (
                  <p
                    role="alert"
                    className="text-destructive text-sm lg:col-span-2"
                    data-testid="entry-place-error"
                  >
                    {placeError}
                  </p>
                ) : null}
              </CardContent>
            </Card>
          ) : null}

          <Card>
            <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
              <CardTitle>{t("warehouse.entry.lines")}</CardTitle>
              <div className="flex flex-wrap gap-2">
                {(entry.label_batches ?? []).map((b, i) => (
                  <LabelButton
                    key={b.batch_uuid}
                    path={b.labels_url}
                    filename={`entry-labels-${i + 1}.pdf`}
                    label={t("warehouse.labels.print_batch", { n: i + 1 })}
                    testId="entry-batch-labels"
                  />
                ))}
                {/* TEC-1032: print every line (chunks of MAX_UNIT_LABELS) or the selection, in any mode. */}
                {editable && selected.length > 0 ? (
                  <LabelButton
                    path={unitLabelsPath(
                      lines
                        .filter((l) => selected.includes(l.uuid))
                        .map((l) => l.barcode),
                    )}
                    filename="entry-labels-selected.pdf"
                    label={t("warehouse.labels.print_selected", {
                      count: selected.length,
                    })}
                    testId="entry-selected-labels"
                  />
                ) : null}
                {lines.length > 0 && !(entry.label_batches ?? []).length
                  ? labelChunks(lines.map((l) => l.barcode)).map(
                      (chunk, i, all) => (
                        <LabelButton
                          key={`all-${i}`}
                          path={unitLabelsPath(chunk)}
                          filename={`entry-labels-${i + 1}.pdf`}
                          label={
                            all.length === 1
                              ? t("warehouse.labels.print_all")
                              : t("warehouse.labels.print_range", {
                                  from: i * MAX_UNIT_LABELS + 1,
                                  to: i * MAX_UNIT_LABELS + chunk.length,
                                })
                          }
                          testId="entry-all-labels"
                        />
                      ),
                    )
                  : null}
              </div>
            </CardHeader>
            <CardContent>
              {lines.length === 0 ? (
                <p
                  className="text-muted-foreground text-sm"
                  data-testid="entry-lines-empty"
                >
                  {t("warehouse.entry.no_lines")}
                </p>
              ) : (
                <EntryLinesTable
                  lines={lines}
                  editable={editable}
                  selected={selected}
                  onSelectedChange={setSelected}
                  onRemove={removeLine.mutate}
                  removing={removeLine.isPending}
                />
              )}
            </CardContent>
          </Card>
          {/* TEC-1060: who took each step. */}
          <Card>
            <CardHeader>
              <CardTitle>{t("warehouse.entry.timeline")}</CardTitle>
            </CardHeader>
            <CardContent>
              <ActivityTimeline
                testId="entry-timeline"
                steps={(entry.timeline ?? []).map((s, i) => ({
                  key: `${s.action}-${s.at}-${i}`,
                  at: s.at,
                  user: s.user?.name ?? null,
                  label: t(`warehouse.entry.activity.${s.action}`),
                }))}
              />
            </CardContent>
          </Card>
          <EntryFixedQuantityDialog
            key={fixedPending?.barcode ?? "closed"}
            pending={fixedPending}
            busy={addLines.isPending}
            onCancel={() => setFixedPending(null)}
            onConfirm={(quantity) => {
              const barcode = fixedPending?.barcode;
              setFixedPending(null);
              if (barcode) void addBarcode(barcode, quantity);
            }}
          />
        </>
      )}
    </WarehouseShell>
  );
}

/** Line barcodes in chunks the label endpoint accepts (≤ MAX_UNIT_LABELS). */
function labelChunks(barcodes: string[]): string[][] {
  const chunks: string[][] = [];
  for (let i = 0; i < barcodes.length; i += MAX_UNIT_LABELS)
    chunks.push(barcodes.slice(i, i + MAX_UNIT_LABELS));
  return chunks;
}

/**
 * TEC-1032: how many pieces of a fixed-barcode product arrived. A fixed
 * barcode is shared by every box of the product, so one scan may stand for
 * N boxes (`fixed_quantity`).
 */
function EntryFixedQuantityDialog({
  pending,
  busy,
  onConfirm,
  onCancel,
}: {
  pending: { barcode: string; product: string } | null;
  busy: boolean;
  onConfirm: (quantity: number) => void;
  onCancel: () => void;
}) {
  const { t } = useLocale();
  const [quantity, setQuantity] = useState("1");
  const count = Number(quantity);
  const valid = Number.isInteger(count) && count >= 1;
  return (
    <Dialog open={pending !== null} onOpenChange={(o) => !o && onCancel()}>
      <DialogContent data-testid="entry-fixed-dialog">
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (valid) onConfirm(count);
          }}
          className="space-y-4"
        >
          <DialogHeader>
            <DialogTitle>
              {t("warehouse.entry.fixed_quantity_title")}
            </DialogTitle>
            <DialogDescription>
              {t("warehouse.entry.fixed_quantity_description", {
                barcode: pending?.barcode ?? "",
                product: pending?.product ?? "",
              })}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="entry-fixed-quantity">
              {t("warehouse.fixed.quantity_label")}
            </Label>
            <Input
              id="entry-fixed-quantity"
              type="number"
              min={1}
              step={1}
              inputMode="numeric"
              autoFocus
              value={quantity}
              onChange={(e) => setQuantity(e.target.value)}
              aria-invalid={!valid}
              data-testid="entry-fixed-quantity"
            />
            {!valid ? (
              <p className="text-destructive text-sm">
                {t("warehouse.fixed.quantity_invalid")}
              </p>
            ) : null}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onCancel}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={!valid || busy}>
              {t("warehouse.fixed.add")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Import entries (TEC-759): the source file, the import job and, after a
 * partial undo, why some units stayed in stock.
 */
function ImportSource({ slug, entry }: { slug: string; entry: StockEntry }) {
  const { t } = useLocale();
  const kept = importLinesKept(entry);
  return (
    <div className="space-y-3" data-testid="entry-import-source">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <Info label={t("warehouse.entry.import_source")}>
          <span className="flex items-center gap-1.5 break-all">
            <FileSpreadsheet className="text-muted-foreground size-4 shrink-0" />
            {entry.import_source_filename || "—"}
          </span>
        </Info>
        {entry.import_job_uuid ? (
          <Button asChild variant="outline" size="sm">
            <Link
              href={routes.tenant.imports.detail(slug, entry.import_job_uuid)}
              data-testid="entry-import-job"
            >
              {t("warehouse.entry.import_job")}
            </Link>
          </Button>
        ) : null}
      </div>
      {kept !== null ? (
        <div
          className="flex items-start gap-2 rounded-md border border-amber-500/40 bg-amber-500/5 p-3 text-sm"
          data-testid="entry-import-partial"
        >
          <TriangleAlert className="size-4 shrink-0 text-amber-600" />
          <div className="space-y-0.5">
            <p className="font-medium">
              {t("warehouse.entry.import_partial_title")}
            </p>
            <p className="text-muted-foreground">
              {t("warehouse.entry.import_partial", { count: kept })}
            </p>
          </div>
        </div>
      ) : null}
    </div>
  );
}

function Info({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1">
      <p className="text-muted-foreground text-xs">{label}</p>
      <div className="text-sm font-medium">{children}</div>
    </div>
  );
}
