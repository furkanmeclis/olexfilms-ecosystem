"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ClipboardCheck,
  Pencil,
  Save,
  Ship,
  Trash2,
  Undo2,
} from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { AsyncCombobox } from "@/components/ui/async-combobox";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { ImportWizardMappingTools } from "@/features/io/components/import-wizard";
import { LocationPicker } from "@/features/warehouse/components/location-picker";
import { ScanInput } from "@/features/warehouse/components/scan-input";
import { warehouseErrorMessage } from "@/features/warehouse/lib/errors";
import { scanKind, unitBarcode } from "@/features/warehouse/lib/scan";
import {
  warehouseKeys,
  warehouseService,
  type EntryImportMappingTemplate,
  type StockEntry,
  type StockEntryArrivalScan,
  type StockEntryHeaderInput,
} from "@/features/warehouse/services/warehouse.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/** The header fields of a delivery form, from an entry. */
export function deliveryForm(entry: StockEntry) {
  return {
    supplier_uuid: entry.supplier?.uuid ?? "",
    container_no: entry.container_no ?? "",
    waybill_no: entry.waybill_no ?? "",
    arrival_date: entry.arrival_date ?? "",
    default_location_uuid: entry.default_location?.uuid ?? "",
  };
}

type DeliveryForm = ReturnType<typeof deliveryForm>;

/** Only the changed fields; an emptied field is sent as "" (clears it). */
export function deliveryPatch(
  before: DeliveryForm,
  after: DeliveryForm,
): StockEntryHeaderInput {
  const out: Record<string, string> = {};
  for (const key of Object.keys(after) as (keyof DeliveryForm)[]) {
    if (after[key].trim() !== before[key]) out[key] = after[key].trim();
  }
  return out as StockEntryHeaderInput;
}

/**
 * The delivery of a container entry (TEC-1007): supplier, container and
 * waybill number, arrival date and the default bin new lines land on.
 */
export function ContainerDeliveryCard({
  entry,
  editable,
  onSaved,
}: {
  entry: StockEntry;
  editable: boolean;
  onSaved: (next: StockEntry) => void;
}) {
  const { t, format } = useLocale();
  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState<DeliveryForm>(() => deliveryForm(entry));
  const [supplierLabel, setSupplierLabel] = useState(
    entry.supplier?.name ?? "",
  );
  const [newSupplier, setNewSupplier] = useState("");

  const save = useMutation({
    mutationFn: () =>
      warehouseService.updateEntryHeader(
        entry.uuid,
        deliveryPatch(deliveryForm(entry), form),
      ),
    onSuccess: (next) => {
      onSaved(next);
      setEditing(false);
      appToast.success(t("warehouse.delivery.saved"));
    },
    onError: (err) =>
      appToast.error(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });
  const addSupplier = useMutation({
    mutationFn: () => warehouseService.createSupplier(newSupplier.trim()),
    onSuccess: (s) => {
      setForm((prev) => ({ ...prev, supplier_uuid: s.uuid }));
      setSupplierLabel(s.name);
      setNewSupplier("");
    },
    onError: (err) =>
      appToast.error(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });

  const set = (key: keyof DeliveryForm) => (value: string) =>
    setForm((prev) => ({ ...prev, [key]: value }));

  const startEdit = () => {
    setForm(deliveryForm(entry));
    setSupplierLabel(entry.supplier?.name ?? "");
    setEditing(true);
  };

  return (
    <Card data-testid="entry-delivery">
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
        <CardTitle className="flex items-center gap-2">
          <Ship className="size-4" />
          {t("warehouse.delivery.title")}
        </CardTitle>
        {editable && !editing ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={startEdit}
            data-testid="entry-delivery-edit"
          >
            <Pencil className="size-4" />
            {t("warehouse.delivery.edit")}
          </Button>
        ) : null}
      </CardHeader>
      <CardContent>
        {editing ? (
          <form
            className="grid gap-4 sm:grid-cols-2"
            onSubmit={(e) => {
              e.preventDefault();
              save.mutate();
            }}
          >
            <div className="space-y-1.5 sm:col-span-2">
              <Label htmlFor="delivery-supplier">
                {t("warehouse.delivery.supplier")}
              </Label>
              <AsyncCombobox
                id="delivery-supplier"
                data-testid="delivery-supplier"
                value={form.supplier_uuid}
                onValueChange={set("supplier_uuid")}
                placeholder={t("warehouse.delivery.pick_supplier")}
                initialOptions={
                  form.supplier_uuid
                    ? [{ value: form.supplier_uuid, label: supplierLabel }]
                    : []
                }
                loadOptions={async (q) =>
                  (await warehouseService.listSuppliers(q)).items.map((s) => ({
                    value: s.uuid,
                    label: s.name,
                  }))
                }
              />
              <div className="flex gap-2">
                <Input
                  value={newSupplier}
                  onChange={(e) => setNewSupplier(e.target.value)}
                  placeholder={t("warehouse.delivery.new_supplier")}
                  aria-label={t("warehouse.delivery.new_supplier")}
                  data-testid="delivery-new-supplier"
                />
                <Button
                  type="button"
                  variant="outline"
                  disabled={!newSupplier.trim() || addSupplier.isPending}
                  onClick={() => addSupplier.mutate()}
                >
                  {t("warehouse.delivery.add_supplier")}
                </Button>
              </div>
            </div>
            <Field
              id="delivery-container"
              label={t("warehouse.delivery.container_no")}
              value={form.container_no}
              onChange={set("container_no")}
            />
            <Field
              id="delivery-waybill"
              label={t("warehouse.delivery.waybill_no")}
              value={form.waybill_no}
              onChange={set("waybill_no")}
            />
            <div className="space-y-1.5">
              <Label htmlFor="delivery-arrival">
                {t("warehouse.delivery.arrival_date")}
              </Label>
              <DatePicker
                id="delivery-arrival"
                value={form.arrival_date}
                onChange={set("arrival_date")}
              />
            </div>
            {entry.warehouse ? (
              <div className="space-y-1.5 sm:col-span-2">
                <p className="text-sm font-medium">
                  {t("warehouse.delivery.default_location")}
                </p>
                <p className="text-muted-foreground text-xs">
                  {t("warehouse.delivery.default_location_hint")}
                </p>
                <LocationPicker
                  warehouseUuid={entry.warehouse.uuid}
                  value={form.default_location_uuid}
                  onChange={set("default_location_uuid")}
                  testId="delivery-location"
                />
              </div>
            ) : null}
            <div className="flex gap-2 sm:col-span-2">
              <Button type="submit" disabled={save.isPending}>
                <Save className="size-4" />
                {t("warehouse.delivery.save")}
              </Button>
              <Button
                type="button"
                variant="ghost"
                onClick={() => setEditing(false)}
              >
                {t("common.cancel")}
              </Button>
            </div>
          </form>
        ) : (
          <div className="grid gap-4 sm:grid-cols-5">
            <Info label={t("warehouse.delivery.supplier")}>
              {entry.supplier?.name ?? "—"}
            </Info>
            <Info label={t("warehouse.delivery.container_no")}>
              <span dir="ltr">{entry.container_no ?? "—"}</span>
            </Info>
            <Info label={t("warehouse.delivery.waybill_no")}>
              <span dir="ltr">{entry.waybill_no ?? "—"}</span>
            </Info>
            <Info label={t("warehouse.delivery.arrival_date")}>
              {entry.arrival_date
                ? format.date(`${entry.arrival_date}T12:00:00Z`)
                : "—"}
            </Info>
            <Info label={t("warehouse.delivery.default_location")}>
              <span className="font-mono text-xs" dir="ltr">
                {entry.default_location
                  ? (entry.default_location.full_code ??
                    entry.default_location.code)
                  : "—"}
              </span>
            </Info>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

/**
 * Arrival check of a container entry (TEC-1007): scan what arrived; lines
 * of the entry turn received, other barcodes are extras. The report lists
 * what is missing and what was extra.
 */
export function ArrivalScanCard({
  entry,
  onChanged,
}: {
  entry: StockEntry;
  onChanged: () => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [last, setLast] = useState<StockEntryArrivalScan | null>(null);
  const [error, setError] = useState<string | null>(null);
  const reportKey = warehouseKeys.arrivalReport(entry.uuid);
  const report = useQuery({
    queryKey: reportKey,
    queryFn: () => warehouseService.arrivalReport(entry.uuid),
  });
  const refresh = () => {
    void qc.invalidateQueries({ queryKey: reportKey });
    onChanged();
  };
  const scan = useMutation({
    mutationFn: (code: string) =>
      warehouseService.arrivalScan(entry.uuid, code),
    onSuccess: (res) => {
      setError(null);
      setLast(res);
      refresh();
    },
    onError: (err) =>
      setError(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });
  const unscan = useMutation({
    mutationFn: (code: string) =>
      warehouseService.arrivalUnscan(entry.uuid, code),
    onSuccess: (res) => {
      setLast(res);
      refresh();
    },
    onError: (err) =>
      appToast.error(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });

  const data = report.data;
  const expected = last?.expected ?? data?.expected ?? entry.line_count;
  const scanned = last?.scanned ?? data?.scanned ?? entry.scanned_count ?? 0;
  const extras = data?.extras ?? [];
  const missing = data?.missing ?? [];

  return (
    <Card data-testid="entry-arrival">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ClipboardCheck className="size-4" />
          {t("warehouse.arrival.title")}
        </CardTitle>
        <p className="text-muted-foreground text-sm">
          {t("warehouse.arrival.hint")}
        </p>
      </CardHeader>
      <CardContent className="space-y-4">
        <ScanInput
          expect="unit"
          warehouseUuid={entry.warehouse?.uuid}
          id="entry-arrival-scan"
          autoFocus={false}
          label={t("warehouse.arrival.scan")}
          busy={scan.isPending}
          onScan={(code) => {
            if (scanKind(code) === "location") {
              setError(t("warehouse.entry.scan_location_here"));
              return;
            }
            scan.mutate(unitBarcode(code));
          }}
        />
        {error ? (
          <p role="alert" className="text-destructive text-sm">
            {error}
          </p>
        ) : null}
        {last ? (
          <p
            className="text-sm"
            data-testid="entry-arrival-last"
            data-result={last.result}
          >
            <span className="font-mono" dir="ltr">
              {last.barcode}
            </span>{" "}
            · {t(`warehouse.arrival.result.${last.result}`)}
          </p>
        ) : null}
        <div
          className="grid gap-3 sm:grid-cols-3"
          data-testid="entry-arrival-counts"
        >
          <Count label={t("warehouse.arrival.expected")} value={expected} />
          <Count label={t("warehouse.arrival.received")} value={scanned} />
          <Count
            label={t("warehouse.arrival.extras")}
            value={last?.extras ?? extras.length}
          />
        </div>
        {missing.length > 0 && scanned > 0 ? (
          <details className="text-sm">
            <summary className="cursor-pointer font-medium">
              {t("warehouse.arrival.missing", { count: missing.length })}
            </summary>
            <ul className="mt-2 space-y-1" data-testid="entry-arrival-missing">
              {missing.slice(0, 200).map((l) => (
                <li key={l.uuid} className="flex flex-wrap gap-2">
                  <span className="font-mono text-xs" dir="ltr">
                    {l.barcode}
                  </span>
                  <span className="text-muted-foreground">
                    {l.product.name}
                    {l.box_no
                      ? ` · ${t("warehouse.fields.box_no")} ${l.box_no}`
                      : ""}
                  </span>
                </li>
              ))}
            </ul>
          </details>
        ) : null}
        {extras.length > 0 ? (
          <div className="space-y-1 text-sm" data-testid="entry-arrival-extras">
            <p className="font-medium">{t("warehouse.arrival.extras_title")}</p>
            <ul className="space-y-1">
              {extras.map((x) => (
                <li key={x.barcode} className="flex items-center gap-2">
                  <span className="font-mono text-xs" dir="ltr">
                    {x.barcode}
                  </span>
                  <span className="text-muted-foreground">
                    {x.product?.name ?? t("warehouse.arrival.unknown_barcode")}
                  </span>
                  <Button
                    type="button"
                    size="icon-sm"
                    variant="ghost"
                    aria-label={t("warehouse.arrival.unscan", {
                      barcode: x.barcode,
                    })}
                    onClick={() => unscan.mutate(x.barcode)}
                    disabled={unscan.isPending}
                  >
                    <Undo2 className="size-4" />
                  </Button>
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}

/** The saved template of a supplier, else none. */
export function supplierTemplate(
  templates: EntryImportMappingTemplate[],
  supplierUuid: string | undefined,
): EntryImportMappingTemplate | undefined {
  if (!supplierUuid) return undefined;
  return templates.find((tpl) => tpl.supplier?.uuid === supplierUuid);
}

/** Saved column mappings of the entry import (TEC-1007). */
export function useEntryImportTemplates(enabled: boolean) {
  return useQuery({
    queryKey: warehouseKeys.entryImportTemplates,
    queryFn: () => warehouseService.listEntryImportTemplates(),
    enabled,
  });
}

/**
 * Apply, save or delete a saved column mapping on the columns step. Saving
 * links the template to the entry's supplier so the next list of the same
 * supplier is mapped at once.
 */
export function MappingTemplateBar({
  tools,
  templates,
  supplier,
}: {
  tools: ImportWizardMappingTools;
  templates: EntryImportMappingTemplate[];
  supplier: StockEntry["supplier"];
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [picked, setPicked] = useState(
    () => supplierTemplate(templates, supplier?.uuid)?.uuid ?? "",
  );
  const [name, setName] = useState(supplier?.name ?? "");
  const invalidate = () =>
    void qc.invalidateQueries({ queryKey: warehouseKeys.entryImportTemplates });
  const save = useMutation({
    mutationFn: () =>
      warehouseService.saveEntryImportTemplate({
        name: name.trim(),
        supplier_uuid: supplier?.uuid ?? null,
        mapping: tools.mapping,
      }),
    onSuccess: (tpl) => {
      setPicked(tpl.uuid);
      invalidate();
      appToast.success(t("warehouse.entry_import.template_saved"));
    },
    onError: (err) =>
      appToast.error(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });
  const remove = useMutation({
    mutationFn: (uuid: string) =>
      warehouseService.deleteEntryImportTemplate(uuid),
    onSuccess: () => {
      setPicked("");
      invalidate();
    },
  });
  const current = templates.find((tpl) => tpl.uuid === picked);

  return (
    <div
      className="bg-muted/40 grid gap-3 rounded-lg border p-3 lg:grid-cols-2"
      data-testid="entry-import-templates"
    >
      <div className="space-y-1.5">
        <Label htmlFor="entry-import-template">
          {t("warehouse.entry_import.template")}
        </Label>
        <div className="flex gap-2">
          <Select value={picked} onValueChange={setPicked}>
            <SelectTrigger
              id="entry-import-template"
              className="min-w-0 flex-1"
            >
              <SelectValue
                placeholder={t("warehouse.entry_import.pick_template")}
              />
            </SelectTrigger>
            <SelectContent>
              {templates.map((tpl) => (
                <SelectItem key={tpl.uuid} value={tpl.uuid}>
                  {tpl.name}
                  {tpl.supplier ? ` · ${tpl.supplier.name}` : ""}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button
            type="button"
            variant="outline"
            disabled={!current}
            onClick={() => current && tools.applyMapping(current.mapping)}
          >
            {t("warehouse.entry_import.apply_template")}
          </Button>
          <Button
            type="button"
            size="icon"
            variant="ghost"
            disabled={!current || remove.isPending}
            aria-label={t("warehouse.entry_import.delete_template")}
            onClick={() => current && remove.mutate(current.uuid)}
          >
            <Trash2 className="size-4" />
          </Button>
        </div>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="entry-import-template-name">
          {t("warehouse.entry_import.save_template")}
        </Label>
        <div className="flex gap-2">
          <Input
            id="entry-import-template-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t("warehouse.entry_import.template_name")}
          />
          <Button
            type="button"
            variant="outline"
            disabled={
              !name.trim() ||
              Object.keys(tools.mapping).length === 0 ||
              save.isPending
            }
            onClick={() => save.mutate()}
          >
            <Save className="size-4" />
            {t("warehouse.entry_import.save")}
          </Button>
        </div>
      </div>
    </div>
  );
}

function Field({
  id,
  label,
  value,
  onChange,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        value={value}
        maxLength={64}
        onChange={(e) => onChange(e.target.value)}
        data-testid={id}
      />
    </div>
  );
}

function Count({ label, value }: { label: string; value: number }) {
  const { format } = useLocale();
  return (
    <div className="rounded-md border p-3">
      <p className="text-muted-foreground text-xs">{label}</p>
      <p className="text-lg font-semibold tabular-nums">
        {format.number(value)}
      </p>
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
