"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Download,
  FileText,
  Paperclip,
  Plus,
  RotateCcw,
  Trash2,
  TriangleAlert,
} from "lucide-react";
import { useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { useStepUp } from "@/features/step-up-engine";
import { LabelButton } from "@/features/warehouse/components/label-button";
import { warehouseErrorMessage } from "@/features/warehouse/lib/errors";
import {
  arrivalChecked,
  buildDecisions,
  movementLabel,
  unknownExtras,
  type DamagedChoice,
  type ExtraChoice,
} from "@/features/warehouse/lib/receiving";
import {
  warehouseKeys,
  warehouseService,
  type StockEntry,
  type StockEntryAttachment,
  type StockEntryAttachmentKind,
} from "@/features/warehouse/services/warehouse.service";
import {
  platformDownloadFile,
  triggerBrowserDownload,
} from "@/lib/api/platform-form-request";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/lib/toast";

const ATTACHMENT_KINDS: StockEntryAttachmentKind[] = [
  "waybill",
  "invoice",
  "photo",
  "other",
];
const ACCEPT = "image/jpeg,image/png,image/webp,application/pdf";

async function downloadAttachment(a: StockEntryAttachment) {
  const { blob, filename } = await platformDownloadFile(a.url);
  triggerBrowserDownload(blob, filename ?? a.filename);
}

/**
 * TEC-1225: confirm with the discrepancy record. Missing lines are left
 * out, extras are added or rejected, damaged lines are taken into stock or
 * not (reason and photo required).
 */
export function ReceivingConfirmDialog({
  entry,
  open,
  onOpenChange,
  onConfirmed,
}: {
  entry: StockEntry;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirmed: (next: StockEntry) => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const report = useQuery({
    queryKey: warehouseKeys.arrivalReport(entry.uuid),
    queryFn: () => warehouseService.arrivalReport(entry.uuid),
    enabled: open,
  });
  const [extras, setExtras] = useState<Record<string, ExtraChoice>>({});
  const [damaged, setDamaged] = useState<DamagedChoice[]>([]);
  const [error, setError] = useState<string | null>(null);
  const checked = arrivalChecked(entry, report.data);
  const unknown = unknownExtras(report.data);
  const photos = (entry.attachments ?? []).filter((a) =>
    a.mime_type.startsWith("image/"),
  );
  const { decisions, pending } = buildDecisions(
    report.data,
    checked,
    extras,
    damaged,
  );
  const missing = checked
    ? (report.data?.missing ?? []).filter(
        (l) =>
          !damaged.some(
            (d) => d.barcode.toUpperCase() === l.barcode.toUpperCase(),
          ),
      )
    : [];

  const confirm = useMutation({
    mutationFn: () => warehouseService.confirm(entry.uuid, decisions),
    onSuccess: (next) => {
      setError(null);
      onOpenChange(false);
      onConfirmed(next);
    },
    onError: (err) => {
      void qc.invalidateQueries({
        queryKey: warehouseKeys.arrivalReport(entry.uuid),
      });
      setError(warehouseErrorMessage(err, t, t("warehouse.form.error")));
    },
  });

  const upload = useMutation({
    mutationFn: (file: File) =>
      warehouseService.addEntryAttachment(entry.uuid, file, "photo"),
    onSuccess: (att) => {
      void qc.invalidateQueries({ queryKey: warehouseKeys.entry(entry.uuid) });
      return att;
    },
    onError: (err) =>
      appToast.error(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });

  const lineOptions = (entry.lines ?? []).filter(
    (l) => !damaged.some((d) => d.barcode === l.barcode),
  );
  const patchDamaged = (i: number, next: Partial<DamagedChoice>) =>
    setDamaged((all) => all.map((d, j) => (j === i ? { ...d, ...next } : d)));

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-h-[90vh] overflow-y-auto sm:max-w-2xl"
        data-testid="receiving-dialog"
      >
        <DialogHeader>
          <DialogTitle>{t("warehouse.receiving.confirm_title")}</DialogTitle>
          <DialogDescription>
            {t("warehouse.receiving.confirm_description")}
          </DialogDescription>
        </DialogHeader>

        {checked ? (
          <section className="space-y-2" data-testid="receiving-missing">
            <h3 className="text-sm font-semibold">
              {t("warehouse.receiving.missing_title", {
                count: missing.length,
              })}
            </h3>
            {missing.length === 0 ? (
              <p className="text-muted-foreground text-sm">
                {t("warehouse.receiving.none")}
              </p>
            ) : (
              <>
                <p className="text-muted-foreground text-xs">
                  {t("warehouse.receiving.missing_hint")}
                </p>
                <ul className="divide-y rounded-md border text-sm">
                  {missing.map((l) => (
                    <li
                      key={l.uuid}
                      className="flex flex-wrap justify-between gap-2 p-2"
                    >
                      <span className="font-mono">{l.barcode}</span>
                      <span className="text-muted-foreground">
                        {l.product.name}
                      </span>
                    </li>
                  ))}
                </ul>
              </>
            )}
          </section>
        ) : null}

        <section className="space-y-2" data-testid="receiving-extras">
          <h3 className="text-sm font-semibold">
            {t("warehouse.receiving.extras_title", {
              count: report.data?.extras.length ?? 0,
            })}
          </h3>
          {(report.data?.extras ?? []).length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("warehouse.receiving.none")}
            </p>
          ) : (
            <ul className="divide-y rounded-md border text-sm">
              {(report.data?.extras ?? []).map((x) => (
                <li
                  key={x.barcode}
                  className="flex flex-wrap items-center justify-between gap-2 p-2"
                >
                  <span>
                    <span className="font-mono">{x.barcode}</span>{" "}
                    <span className="text-muted-foreground">
                      {x.product?.name ?? t("warehouse.receiving.unknown")}
                    </span>
                  </span>
                  <Select
                    value={extras[x.barcode] ?? ""}
                    onValueChange={(v) =>
                      setExtras((all) => ({
                        ...all,
                        [x.barcode]: v as ExtraChoice,
                      }))
                    }
                  >
                    <SelectTrigger
                      className="w-44"
                      data-testid="receiving-extra-choice"
                    >
                      <SelectValue
                        placeholder={t("warehouse.receiving.choose")}
                      />
                    </SelectTrigger>
                    <SelectContent>
                      {!unknown.has(x.barcode) ? (
                        <SelectItem value="added">
                          {t("warehouse.receiving.decision.added")}
                        </SelectItem>
                      ) : null}
                      <SelectItem value="rejected">
                        {t("warehouse.receiving.decision.rejected")}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </li>
              ))}
            </ul>
          )}
        </section>

        <section className="space-y-2" data-testid="receiving-damaged">
          <div className="flex items-center justify-between gap-2">
            <h3 className="text-sm font-semibold">
              {t("warehouse.receiving.damaged_title")}
            </h3>
            <Select
              value=""
              onValueChange={(barcode) =>
                setDamaged((all) => [
                  ...all,
                  {
                    barcode,
                    decision: "accepted",
                    reason: "",
                    attachmentUuids: [],
                  },
                ])
              }
              disabled={lineOptions.length === 0}
            >
              <SelectTrigger
                className="w-56"
                data-testid="receiving-add-damaged"
              >
                <Plus className="size-4" />
                <SelectValue
                  placeholder={t("warehouse.receiving.add_damaged")}
                />
              </SelectTrigger>
              <SelectContent>
                {lineOptions.map((l) => (
                  <SelectItem key={l.uuid} value={l.barcode}>
                    {l.barcode} · {l.product.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {damaged.map((d, i) => (
            <div
              key={d.barcode}
              className="space-y-2 rounded-md border p-3"
              data-testid="receiving-damaged-row"
            >
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="font-mono text-sm">{d.barcode}</span>
                <div className="flex gap-2">
                  <Select
                    value={d.decision}
                    onValueChange={(v) =>
                      patchDamaged(i, {
                        decision: v as DamagedChoice["decision"],
                      })
                    }
                  >
                    <SelectTrigger className="w-48">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="accepted">
                        {t("warehouse.receiving.decision.accepted")}
                      </SelectItem>
                      <SelectItem value="excluded">
                        {t("warehouse.receiving.decision.excluded")}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    aria-label={t("common.delete")}
                    onClick={() =>
                      setDamaged((all) => all.filter((_, j) => j !== i))
                    }
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
              </div>
              <Textarea
                value={d.reason}
                maxLength={500}
                placeholder={t("warehouse.receiving.reason")}
                aria-label={t("warehouse.receiving.reason")}
                onChange={(e) => patchDamaged(i, { reason: e.target.value })}
                data-testid="receiving-damaged-reason"
              />
              <div className="space-y-1">
                <p className="text-muted-foreground text-xs">
                  {t("warehouse.receiving.photos_required")}
                </p>
                <div className="flex flex-wrap gap-3">
                  {photos.map((p) => (
                    <label
                      key={p.uuid}
                      className="flex items-center gap-1.5 text-sm"
                    >
                      <Checkbox
                        checked={d.attachmentUuids.includes(p.uuid)}
                        onCheckedChange={(on) =>
                          patchDamaged(i, {
                            attachmentUuids: on
                              ? [...d.attachmentUuids, p.uuid]
                              : d.attachmentUuids.filter((u) => u !== p.uuid),
                          })
                        }
                      />
                      {p.filename}
                    </label>
                  ))}
                  <PhotoUpload
                    busy={upload.isPending}
                    onFile={async (file) => {
                      const att = await upload.mutateAsync(file);
                      patchDamaged(i, {
                        attachmentUuids: [...d.attachmentUuids, att.uuid],
                      });
                    }}
                  />
                </div>
              </div>
            </div>
          ))}
        </section>

        {pending.length > 0 ? (
          <p className="text-sm text-amber-700" data-testid="receiving-pending">
            {t("warehouse.receiving.pending", { count: pending.length })}
          </p>
        ) : null}
        {error ? (
          <p role="alert" className="text-destructive text-sm">
            {error}
          </p>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            disabled={
              pending.length > 0 || confirm.isPending || report.isLoading
            }
            onClick={() => confirm.mutate()}
            data-testid="receiving-confirm"
          >
            {t("warehouse.receiving.confirm_with_record")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function PhotoUpload({
  busy,
  onFile,
}: {
  busy: boolean;
  onFile: (file: File) => Promise<void>;
}) {
  const { t } = useLocale();
  const ref = useRef<HTMLInputElement>(null);
  return (
    <>
      <input
        ref={ref}
        type="file"
        accept="image/jpeg,image/png,image/webp"
        capture="environment"
        className="hidden"
        onChange={(e) => {
          const file = e.target.files?.[0];
          e.target.value = "";
          if (file) void onFile(file).catch(() => undefined);
        }}
      />
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={busy}
        onClick={() => ref.current?.click()}
      >
        <Plus className="size-4" />
        {t("warehouse.receiving.add_photo")}
      </Button>
    </>
  );
}

/** TEC-1225: waybill, invoice, photo and other files of an entry. */
export function EntryAttachmentsCard({
  entry,
  canWrite,
  onChanged,
}: {
  entry: StockEntry;
  canWrite: boolean;
  onChanged: () => void;
}) {
  const { t, format } = useLocale();
  const [kind, setKind] = useState<StockEntryAttachmentKind>("waybill");
  const ref = useRef<HTMLInputElement>(null);
  const writable = canWrite && entry.status !== "cancelled";
  const evidence = new Set(
    (entry.discrepancies ?? []).flatMap((d) =>
      d.attachments.map((a) => a.uuid),
    ),
  );
  const upload = useMutation({
    mutationFn: (file: File) =>
      warehouseService.addEntryAttachment(entry.uuid, file, kind),
    onSuccess: () => {
      appToast.success(t("warehouse.receiving.attachment_added"));
      onChanged();
    },
    onError: (err) =>
      appToast.error(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });
  const remove = useMutation({
    mutationFn: (att: string) =>
      warehouseService.deleteEntryAttachment(entry.uuid, att),
    onSuccess: onChanged,
    onError: (err) =>
      appToast.error(warehouseErrorMessage(err, t, t("warehouse.form.error"))),
  });
  const items = entry.attachments ?? [];
  return (
    <Card data-testid="entry-attachments">
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
        <CardTitle className="flex items-center gap-2">
          <Paperclip className="size-4" />
          {t("warehouse.receiving.attachments")}
        </CardTitle>
        {writable ? (
          <div className="flex flex-wrap items-center gap-2">
            <Select
              value={kind}
              onValueChange={(v) => setKind(v as StockEntryAttachmentKind)}
            >
              <SelectTrigger
                className="w-36"
                aria-label={t("warehouse.receiving.attachment_kind_label")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {ATTACHMENT_KINDS.map((k) => (
                  <SelectItem key={k} value={k}>
                    {t(`warehouse.receiving.attachment_kind.${k}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <input
              ref={ref}
              type="file"
              accept={ACCEPT}
              className="hidden"
              data-testid="entry-attachment-input"
              onChange={(e) => {
                const file = e.target.files?.[0];
                e.target.value = "";
                if (file) upload.mutate(file);
              }}
            />
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={upload.isPending}
              onClick={() => ref.current?.click()}
            >
              <Plus className="size-4" />
              {t("warehouse.receiving.add_attachment")}
            </Button>
          </div>
        ) : null}
      </CardHeader>
      <CardContent>
        {items.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("warehouse.receiving.no_attachments")}
          </p>
        ) : (
          <ul className="divide-y text-sm">
            {items.map((a) => (
              <li
                key={a.uuid}
                className="flex flex-wrap items-center justify-between gap-2 py-2"
                data-testid="entry-attachment"
              >
                <span className="min-w-0 break-all">
                  <span className="font-medium">{a.filename}</span>{" "}
                  <span className="text-muted-foreground">
                    · {t(`warehouse.receiving.attachment_kind.${a.kind}`)} ·{" "}
                    {format.dateTime(a.created_at)}
                    {a.uploaded_by ? ` · ${a.uploaded_by.name}` : ""}
                  </span>
                </span>
                <span className="flex gap-1">
                  <Button
                    type="button"
                    size="icon-sm"
                    variant="ghost"
                    aria-label={t("warehouse.receiving.download")}
                    onClick={() =>
                      void downloadAttachment(a).catch(() =>
                        appToast.error(t("warehouse.form.error")),
                      )
                    }
                  >
                    <Download className="size-4" />
                  </Button>
                  {writable && !evidence.has(a.uuid) ? (
                    <Button
                      type="button"
                      size="icon-sm"
                      variant="ghost"
                      aria-label={t("common.delete")}
                      disabled={remove.isPending}
                      onClick={() => remove.mutate(a.uuid)}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  ) : null}
                </span>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

/** TEC-1225: the discrepancy record ("kabul tutanağı") and its PDF. */
export function EntryReceiptCard({ entry }: { entry: StockEntry }) {
  const { t } = useLocale();
  const items = entry.discrepancies ?? [];
  if (entry.status === "draft" || entry.mode === "import") return null;
  return (
    <Card data-testid="entry-receipt">
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
        <CardTitle className="flex items-center gap-2">
          <FileText className="size-4" />
          {t("warehouse.receiving.record_title")}
        </CardTitle>
        <LabelButton
          path={warehouseService.entryReceiptPdfPath(entry.uuid)}
          filename={`tutanak-${entry.uuid.slice(0, 8)}.pdf`}
          label={t("warehouse.receiving.record_pdf")}
          testId="entry-receipt-pdf"
        />
      </CardHeader>
      <CardContent>
        {items.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("warehouse.receiving.record_clean")}
          </p>
        ) : (
          <ul className="divide-y text-sm">
            {items.map((d) => (
              <li
                key={d.uuid}
                className="space-y-1 py-2"
                data-testid="entry-receipt-item"
              >
                <div className="flex flex-wrap items-center gap-2">
                  <span className="rounded bg-amber-500/10 px-1.5 py-0.5 text-xs font-medium text-amber-700">
                    {t(`warehouse.receiving.kind.${d.kind}`)}
                  </span>
                  <span className="font-mono">{d.barcode}</span>
                  <span className="text-muted-foreground">
                    {d.product?.name ?? t("warehouse.receiving.unknown")}
                  </span>
                  <span className="ms-auto font-medium">
                    {t(`warehouse.receiving.decision.${d.decision}`)}
                  </span>
                </div>
                {d.reason ? (
                  <p className="text-muted-foreground">{d.reason}</p>
                ) : null}
                {d.attachments.length > 0 ? (
                  <div className="flex flex-wrap gap-2">
                    {d.attachments.map((a) => (
                      <Button
                        key={a.uuid}
                        type="button"
                        size="sm"
                        variant="outline"
                        onClick={() =>
                          void downloadAttachment(a).catch(() =>
                            appToast.error(t("warehouse.form.error")),
                          )
                        }
                      >
                        <Paperclip className="size-3.5" />
                        {a.filename}
                      </Button>
                    ))}
                  </div>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

/**
 * TEC-1225: undo a confirmed manual entry (stock.adjust + step-up). Units
 * that moved after the entry block the undo and are listed.
 */
export function EntryUndoButton({
  entry,
  onUndone,
}: {
  entry: StockEntry;
  onUndone: (next: StockEntry) => void;
}) {
  const { t, format } = useLocale();
  const { ensure } = useStepUp();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [error, setError] = useState<string | null>(null);
  const check = useQuery({
    queryKey: warehouseKeys.entryUndoCheck(entry.uuid),
    queryFn: () => warehouseService.entryUndoCheck(entry.uuid),
    enabled: open,
  });
  const undo = useMutation({
    mutationFn: () => warehouseService.undoEntry(entry.uuid, reason.trim()),
    onSuccess: (next) => {
      setOpen(false);
      setReason("");
      onUndone(next);
      appToast.success(t("warehouse.receiving.undone"));
    },
    onError: (err) => {
      void check.refetch();
      setError(warehouseErrorMessage(err, t, t("warehouse.form.error")));
    },
  });
  const blockers = check.data?.blockers ?? [];
  return (
    <>
      <Button
        type="button"
        variant="outline"
        onClick={() => {
          setError(null);
          setOpen(true);
        }}
        data-testid="entry-undo"
      >
        <RotateCcw className="size-4" />
        {t("warehouse.receiving.undo")}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent data-testid="entry-undo-dialog">
          <DialogHeader>
            <DialogTitle>{t("warehouse.receiving.undo_title")}</DialogTitle>
            <DialogDescription>
              {t("warehouse.receiving.undo_description", {
                count: check.data?.lines ?? entry.line_count,
              })}
            </DialogDescription>
          </DialogHeader>
          {blockers.length > 0 ? (
            <div
              className="space-y-2 rounded-md border border-amber-500/40 bg-amber-500/5 p-3 text-sm"
              data-testid="entry-undo-blockers"
            >
              <p className="flex items-center gap-2 font-medium">
                <TriangleAlert className="size-4 text-amber-600" />
                {t("warehouse.receiving.undo_blocked", {
                  count: blockers.length,
                })}
              </p>
              <ul className="space-y-1">
                {blockers.map((b) => (
                  <li key={b.barcode} className="flex flex-wrap gap-2">
                    <span className="font-mono">{b.barcode}</span>
                    <span className="text-muted-foreground">
                      {b.product.name}
                    </span>
                    <span className="ms-auto">
                      {movementLabel(b.movement_type, t)} ·{" "}
                      {format.dateTime(b.at)}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
          <div className="space-y-1.5">
            <Label htmlFor="entry-undo-reason">
              {t("warehouse.receiving.undo_reason")}
            </Label>
            <Input
              id="entry-undo-reason"
              value={reason}
              maxLength={500}
              onChange={(e) => setReason(e.target.value)}
              data-testid="entry-undo-reason"
            />
          </div>
          {error ? (
            <p role="alert" className="text-destructive text-sm">
              {error}
            </p>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setOpen(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={
                !check.data?.allowed || !reason.trim() || undo.isPending
              }
              onClick={async () => {
                if (await ensure()) undo.mutate();
              }}
              data-testid="entry-undo-confirm"
            >
              {t("warehouse.receiving.undo_confirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
