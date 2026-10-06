"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, MapPin, PackagePlus, XCircle } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { EntryLinesTable } from "@/features/warehouse/components/entry-lines-table";
import { GenerateForm } from "@/features/warehouse/components/generate-form";
import { LabelButton } from "@/features/warehouse/components/label-button";
import { nativeSelectClass } from "@/features/warehouse/components/native-select-field";
import { ScanInput } from "@/features/warehouse/components/scan-input";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import {
  canConfirm,
  entryStatusTone,
  linesToPlace,
  unplacedLines,
} from "@/features/warehouse/lib/entries";
import { warehouseErrorMessage } from "@/features/warehouse/lib/errors";
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
  const { t } = useLocale();
  const access = useWarehouseAccess(slug);
  const canWrite = access.can(Permission.WarehouseWrite);
  const qc = useQueryClient();
  const { confirm } = useDialogs();
  const [selected, setSelected] = useState<string[]>([]);
  const [roomUuid, setRoomUuid] = useState("");
  const [locationUuid, setLocationUuid] = useState("");
  const [lineError, setLineError] = useState<string | null>(null);
  const [placeError, setPlaceError] = useState<string | null>(null);

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

  const onBarcode = async (code: string) => {
    if (scanKind(code) === "location") {
      setLineError(t("warehouse.entry.scan_location_here"));
      return;
    }
    try {
      await addLines.mutateAsync({ barcodes: [unitBarcode(code)] });
    } catch (err) {
      setLineError(
        warehouseErrorMessage(err, t, t("warehouse.entry.add_failed")),
      );
    }
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
              {entry.note ? (
                <div className="sm:col-span-4">
                  <Info label={t("warehouse.fields.note")}>{entry.note}</Info>
                </div>
              ) : null}
            </CardContent>
          </Card>

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
                      id="entry-barcode"
                      label={t("warehouse.entry.scan_barcode")}
                      hint={t("warehouse.entry.scan_barcode_hint")}
                      busy={addLines.isPending}
                      onScan={onBarcode}
                    />
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
                      <select
                        id="entry-room"
                        className={nativeSelectClass}
                        value={roomUuid}
                        data-testid="entry-room"
                        onChange={(e) => {
                          setRoomUuid(e.target.value);
                          setLocationUuid("");
                        }}
                      >
                        <option value="">
                          {t("warehouse.entry.pick_room")}
                        </option>
                        {(rooms.data?.items ?? []).map((r) => (
                          <option key={r.uuid} value={r.uuid}>
                            {r.code} · {r.name}
                          </option>
                        ))}
                      </select>
                    </div>
                    <div className="space-y-1.5">
                      <Label htmlFor="entry-location-pick">
                        {t("warehouse.fields.location")}
                      </Label>
                      <select
                        id="entry-location-pick"
                        className={nativeSelectClass}
                        value={locationUuid}
                        disabled={!roomUuid}
                        data-testid="entry-location-pick"
                        onChange={(e) => setLocationUuid(e.target.value)}
                      >
                        <option value="">
                          {t("warehouse.entry.pick_location")}
                        </option>
                        {locOptions.map((o) => (
                          <option key={o.value} value={o.value}>
                            {o.label}
                          </option>
                        ))}
                      </select>
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
        </>
      )}
    </WarehouseShell>
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
