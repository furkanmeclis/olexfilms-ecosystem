"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Camera,
  CheckCircle2,
  FilePenLine,
  Send,
  Trash2,
  XCircle,
} from "lucide-react";
import { useEffect, useRef, useState, type FormEvent } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  useWarehouseAccess,
  WarehouseShell,
} from "@/features/warehouse/components/warehouse-shell";
import {
  adjustmentErrorMessage,
  adjustmentStatusTone,
  canCancelAdjustment,
  isDraft,
  needsPhoto,
  normalizeMeters,
} from "@/features/warehouse/lib/adjustments";
import {
  adjustmentKeys,
  adjustmentsService,
  type StockAdjustment,
} from "@/features/warehouse/services/adjustments.service";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/lib/toast";

/**
 * One stock adjustment document (TEC-1226): header, lines (scan a barcode;
 * meters for part of a roll, quantity for a fixed barcode), photos, and the
 * draft -> pending approval -> posted flow. Above the threshold another
 * person approves; `can_approve` comes from the server.
 */
export function AdjustmentDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format } = useLocale();
  const qc = useQueryClient();
  const { confirm } = useDialogs();
  const access = useWarehouseAccess(slug);
  const canWrite = access.can(Permission.WarehouseWrite);
  const detail = useQuery({
    queryKey: adjustmentKeys.detail(uuid),
    queryFn: () => adjustmentsService.get(uuid),
    enabled: access.allowed,
  });
  const a = detail.data;
  const update = async (next: StockAdjustment) => {
    qc.setQueryData(adjustmentKeys.detail(uuid), next);
    await qc.invalidateQueries({
      queryKey: ["warehouse", "adjustments", "list"],
    });
  };
  const onError = (err: unknown) =>
    appToast.error(adjustmentErrorMessage(err, t, t("warehouse.form.error")));

  const action = useMutation({
    mutationFn: (v: "submit" | "approve" | "cancel") =>
      adjustmentsService.action(uuid, v),
    onSuccess: async (next, v) => {
      await update(next);
      appToast.success(t(`stock_adjustments.done.${v}`));
    },
    onError,
  });
  const deleteLine = useMutation({
    mutationFn: (line: string) => adjustmentsService.deleteLine(uuid, line),
    onSuccess: update,
    onError,
  });
  const deletePhoto = useMutation({
    mutationFn: (photo: string) => adjustmentsService.deletePhoto(uuid, photo),
    onSuccess: update,
    onError,
  });
  const upload = useMutation({
    mutationFn: (file: File) => adjustmentsService.addPhoto(uuid, file),
    onSuccess: update,
    onError,
  });
  const fileRef = useRef<HTMLInputElement>(null);

  const askCancel = async () => {
    const ok = await confirm({
      title: t("stock_adjustments.cancel_title"),
      description: t("stock_adjustments.cancel_description"),
      confirmLabel: t("stock_adjustments.actions.cancel"),
      variant: "destructive",
    });
    if (ok) action.mutate("cancel");
  };

  return (
    <WarehouseShell
      slug={slug}
      access={access}
      title={a ? a.adjustment_no : t("stock_adjustments.title")}
      description={a ? t(`stock_adjustments.type.${a.type}`) : undefined}
      icon={<FilePenLine className="size-6" />}
      crumbs={[
        {
          label: t("stock_adjustments.title"),
          href: routes.tenant.warehouse.adjustments(slug),
        },
      ]}
      actions={
        a && canWrite ? (
          <div className="flex flex-wrap gap-2">
            {isDraft(a) ? (
              <Button
                type="button"
                onClick={() => action.mutate("submit")}
                disabled={action.isPending || (a.lines?.length ?? 0) === 0}
                data-testid="adjustment-submit"
              >
                <Send className="size-4" />
                {t("stock_adjustments.actions.submit")}
              </Button>
            ) : null}
            {a.status === "pending_approval" && a.can_approve ? (
              <Button
                type="button"
                onClick={() => action.mutate("approve")}
                disabled={action.isPending}
                data-testid="adjustment-approve"
              >
                <CheckCircle2 className="size-4" />
                {t("stock_adjustments.actions.approve")}
              </Button>
            ) : null}
            {canCancelAdjustment(a) ? (
              <Button
                type="button"
                variant="outline"
                onClick={() => void askCancel()}
                disabled={action.isPending}
              >
                <XCircle className="size-4" />
                {t("stock_adjustments.actions.cancel")}
              </Button>
            ) : null}
          </div>
        ) : null
      }
    >
      {detail.isError ? (
        <p role="alert" className="text-destructive text-sm">
          {t("stock_adjustments.load_error")}
        </p>
      ) : null}
      {a ? (
        <div className="space-y-4">
          <Card>
            <CardContent className="grid gap-3 pt-6 text-sm sm:grid-cols-2 lg:grid-cols-4">
              <Info label={t("stock_adjustments.columns.status")}>
                <StatusChip
                  label={t(`stock_adjustments.status.${a.status}`)}
                  tone={adjustmentStatusTone(a.status)}
                />
              </Info>
              <Info label={t("stock_adjustments.columns.source")}>
                {t(`stock_adjustments.source.${a.source}`)}
              </Info>
              <Info label={t("stock_adjustments.columns.warehouse")}>
                {a.warehouse
                  ? `${a.warehouse.code} · ${a.warehouse.name}`
                  : "—"}
              </Info>
              <Info label={t("stock_adjustments.fields.reason_code")}>
                {a.reason_code ?? "—"}
              </Info>
              <Info label={t("stock_adjustments.columns.pieces")}>
                {format.number(a.total_quantity)}
              </Info>
              <Info label={t("stock_adjustments.columns.meters")}>
                <span dir="ltr">{a.total_meters}</span>
              </Info>
              <Info label={t("stock_adjustments.fields.created_by")}>
                {a.created_by?.name ?? "—"}
              </Info>
              <Info label={t("stock_adjustments.fields.approved_by")}>
                {a.approved_by?.name ?? "—"}
              </Info>
              {a.note ? (
                <div className="sm:col-span-2 lg:col-span-4">
                  <Info label={t("stock_adjustments.fields.note")}>
                    {a.note}
                  </Info>
                </div>
              ) : null}
            </CardContent>
          </Card>

          {a.status === "pending_approval" ? (
            <p
              className="bg-muted rounded-md p-3 text-sm"
              data-testid="adjustment-approval-hint"
            >
              {a.requires_second_approval
                ? t("stock_adjustments.hint.second_approval")
                : t("stock_adjustments.hint.single_approval")}
            </p>
          ) : null}
          {isDraft(a) && needsPhoto(a) ? (
            <p className="text-destructive text-sm" role="status">
              {t("stock_adjustments.hint.photo_required")}
            </p>
          ) : null}

          <Card>
            <CardHeader>
              <CardTitle>{t("stock_adjustments.lines.title")}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              {isDraft(a) && canWrite ? (
                <AddLineForm
                  adjustment={a}
                  onAdded={(next) => void update(next)}
                />
              ) : null}
              {(a.lines?.length ?? 0) === 0 ? (
                <p className="text-muted-foreground text-sm">
                  {t("stock_adjustments.lines.empty")}
                </p>
              ) : (
                <ul className="divide-y" data-testid="adjustment-lines">
                  {a.lines?.map((l) => (
                    <li
                      key={l.uuid}
                      className="flex flex-wrap items-center justify-between gap-2 py-2"
                    >
                      <div className="min-w-0">
                        <div className="font-medium" dir="ltr">
                          {l.unit.barcode}
                        </div>
                        <div className="text-muted-foreground text-xs">
                          {l.product.sku} · {l.product.name}
                        </div>
                      </div>
                      <div className="flex items-center gap-3 text-sm">
                        <span>
                          {l.kind === "meters"
                            ? t("stock_adjustments.lines.meters", {
                                meters: l.meters ?? "",
                              })
                            : l.kind === "quantity"
                              ? t("stock_adjustments.lines.quantity", {
                                  quantity: l.quantity ?? 0,
                                })
                              : l.unit.is_roll
                                ? t("stock_adjustments.lines.whole_roll", {
                                    meters: l.roll_meters ?? "",
                                  })
                                : t("stock_adjustments.lines.whole")}
                        </span>
                        {l.location ? (
                          <span className="text-muted-foreground" dir="ltr">
                            {l.location.full_code}
                          </span>
                        ) : null}
                        {isDraft(a) && canWrite ? (
                          <Button
                            type="button"
                            size="icon"
                            variant="ghost"
                            aria-label={t("common.delete")}
                            onClick={() => deleteLine.mutate(l.uuid)}
                            disabled={deleteLine.isPending}
                          >
                            <Trash2 className="size-4" />
                          </Button>
                        ) : null}
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between gap-2">
              <CardTitle>{t("stock_adjustments.photos.title")}</CardTitle>
              {canWrite && canCancelAdjustment(a) ? (
                <>
                  <input
                    ref={fileRef}
                    type="file"
                    accept="image/jpeg,image/png,image/webp"
                    className="hidden"
                    data-testid="adjustment-photo-input"
                    onChange={(e) => {
                      const f = e.target.files?.[0];
                      if (f) upload.mutate(f);
                      e.target.value = "";
                    }}
                  />
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => fileRef.current?.click()}
                    disabled={upload.isPending}
                  >
                    <Camera className="size-4" />
                    {t("stock_adjustments.photos.add")}
                  </Button>
                </>
              ) : null}
            </CardHeader>
            <CardContent>
              {(a.photos?.length ?? 0) === 0 ? (
                <p className="text-muted-foreground text-sm">
                  {t("stock_adjustments.photos.empty")}
                </p>
              ) : (
                <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                  {a.photos?.map((p) => (
                    <div key={p.uuid} className="relative">
                      <AdjustmentPhoto adjustment={uuid} photo={p.uuid} />
                      {isDraft(a) && canWrite ? (
                        <Button
                          type="button"
                          size="icon"
                          variant="secondary"
                          className="absolute end-1 top-1"
                          aria-label={t("common.delete")}
                          onClick={() => deletePhoto.mutate(p.uuid)}
                        >
                          <Trash2 className="size-4" />
                        </Button>
                      ) : null}
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      ) : null}
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
      <div className="text-muted-foreground text-xs">{label}</div>
      <div>{children}</div>
    </div>
  );
}

/** A photo loaded through the BFF (the API needs the session). */
function AdjustmentPhoto({
  adjustment,
  photo,
}: {
  adjustment: string;
  photo: string;
}) {
  const { t } = useLocale();
  const [src, setSrc] = useState<string | null>(null);
  useEffect(() => {
    let url: string | null = null;
    let alive = true;
    adjustmentsService
      .photoBlob(adjustment, photo)
      .then((blob) => {
        if (!alive) return;
        url = URL.createObjectURL(blob);
        setSrc(url);
      })
      .catch(() => undefined);
    return () => {
      alive = false;
      if (url) URL.revokeObjectURL(url);
    };
  }, [adjustment, photo]);
  return src ? (
    // eslint-disable-next-line @next/next/no-img-element -- blob URL
    <img
      src={src}
      alt={t("stock_adjustments.photos.alt")}
      className="aspect-square w-full rounded-md object-cover"
    />
  ) : (
    <div className="bg-muted aspect-square w-full animate-pulse rounded-md" />
  );
}

/** Scan or type a barcode; meters (roll part) or quantity (fixed barcode). */
function AddLineForm({
  adjustment,
  onAdded,
}: {
  adjustment: StockAdjustment;
  onAdded: (next: StockAdjustment) => void;
}) {
  const { t } = useLocale();
  const [barcode, setBarcode] = useState("");
  const [meters, setMeters] = useState("");
  const [quantity, setQuantity] = useState("");
  const [error, setError] = useState<string | null>(null);
  const add = useMutation({
    mutationFn: () =>
      adjustmentsService.addLine(adjustment.uuid, {
        barcode: barcode.trim(),
        meters: normalizeMeters(meters) ?? null,
        quantity: quantity.trim() ? Number(quantity) : null,
      }),
    onSuccess: (next) => {
      setBarcode("");
      setMeters("");
      setQuantity("");
      onAdded(next);
    },
    onError: (err) =>
      setError(adjustmentErrorMessage(err, t, t("warehouse.form.error"))),
  });
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    if (!barcode.trim()) {
      setError(t("stock_adjustments.lines.barcode_required"));
      return;
    }
    if (meters.trim() && !normalizeMeters(meters)) {
      setError(t("stock_adjustments.lines.meters_invalid"));
      return;
    }
    add.mutate();
  };
  return (
    <form
      className="grid gap-3 sm:grid-cols-[2fr_1fr_1fr_auto] sm:items-end"
      onSubmit={onSubmit}
      noValidate
      data-testid="adjustment-line-form"
    >
      <div className="space-y-1.5">
        <Label htmlFor="adj-barcode">
          {t("stock_adjustments.lines.barcode")}
        </Label>
        <Input
          id="adj-barcode"
          autoFocus
          dir="ltr"
          value={barcode}
          onChange={(e) => setBarcode(e.target.value)}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="adj-meters">
          {t("stock_adjustments.lines.meters_label")}
        </Label>
        <Input
          id="adj-meters"
          inputMode="decimal"
          value={meters}
          onChange={(e) => setMeters(e.target.value)}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="adj-qty">
          {t("stock_adjustments.lines.quantity_label")}
        </Label>
        <Input
          id="adj-qty"
          inputMode="numeric"
          value={quantity}
          onChange={(e) => setQuantity(e.target.value)}
        />
      </div>
      <Button type="submit" disabled={add.isPending}>
        {t("stock_adjustments.lines.add")}
      </Button>
      {error ? (
        <p role="alert" className="text-destructive text-sm sm:col-span-4">
          {error}
        </p>
      ) : null}
      <p className="text-muted-foreground text-xs sm:col-span-4">
        {adjustment.type === "found"
          ? t("stock_adjustments.lines.help_found")
          : t("stock_adjustments.lines.help")}
      </p>
    </form>
  );
}
