"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ExternalLink, ShieldCheck, ShieldOff } from "lucide-react";
import Link from "next/link";
import { useState, type ReactNode } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout/page-header";
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
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { WarrantyCertificateButton } from "@/features/warranty/components/warranty-certificate-button";
import { WarrantyProgressBar } from "@/features/warranty/components/warranty-progress";
import {
  warrantyHolderName,
  warrantyStatusTone,
  warrantyVehicleTitle,
  type Warranty,
} from "@/features/warranty/lib/warranty-list";
import { panelCertificateClient } from "@/features/warranty/services/certificate.service";
import {
  warrantyKeys,
  warrantyService,
} from "@/features/warranty/services/warranty.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const VOID_REASON_MIN = 3;
export const VOID_REASON_MAX = 500;

/** Trimmed reason length is within the API bounds (3–500). */
export function validVoidReason(reason: string): boolean {
  const n = [...reason.trim()].length;
  return n >= VOID_REASON_MIN && n <= VOID_REASON_MAX;
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm">{children}</dd>
    </div>
  );
}

function VoidDialog({
  warranty,
  open,
  onOpenChange,
}: {
  warranty: Warranty;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [reason, setReason] = useState("");
  const mutation = useMutation({
    mutationFn: () => warrantyService.void(warranty.uuid, reason.trim()),
    onSuccess: (updated) => {
      qc.setQueryData(warrantyKeys.detail(warranty.uuid), updated);
      void qc.invalidateQueries({ queryKey: warrantyKeys.all });
      toast.success(t("warranty.void.success"));
      setReason("");
      onOpenChange(false);
    },
    onError: (err) => {
      toast.error(
        isApiError(err) && err.code === "WARRANTY_ALREADY_VOID"
          ? t("warranty.void.already_void")
          : t("warranty.void.error"),
      );
    },
  });
  const ok = validVoidReason(reason);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("warranty.void.title")}</DialogTitle>
          <DialogDescription>
            {t("warranty.void.description")}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-1.5">
          <Label htmlFor="void-reason">{t("warranty.void.reason")}</Label>
          <Textarea
            id="void-reason"
            value={reason}
            maxLength={VOID_REASON_MAX}
            rows={4}
            placeholder={t("warranty.void.reason_placeholder")}
            onChange={(e) => setReason(e.target.value)}
          />
          <p className="text-muted-foreground text-xs">
            {t("warranty.void.reason_hint")}
          </p>
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("warranty.void.cancel")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            data-testid="void-confirm"
            disabled={!ok || mutation.isPending}
            onClick={() => mutation.mutate()}
          >
            {t("warranty.void.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Tenant > Warranties > detail (TEC-191): period with days left and the
 * elapsed bar, covered product, vehicle, holder, service and the center
 * void (warranties.void + step-up, reason required).
 */
export function WarrantyDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format, locale } = useLocale();
  const { can } = usePermission();
  const canRead = can(Permission.WarrantiesRead);
  const [voidOpen, setVoidOpen] = useState(false);
  const detail = useQuery({
    queryKey: warrantyKeys.detail(uuid),
    queryFn: () => warrantyService.get(uuid),
    enabled: canRead && uuid !== "",
    retry: (count, err) =>
      !(isApiError(err) && (err.status === 404 || err.status === 403)) &&
      count < 2,
  });

  const listTitle = t("warranty.list.title");
  const w = detail.data;
  const header = (
    <PageHeader
      title={w ? w.public_code : t("warranty.detail.title")}
      icon={<ShieldCheck className="size-6" />}
      description={w ? w.product.name : undefined}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: listTitle, href: routes.tenant.warranties.list(slug) },
        { label: w ? w.public_code : t("warranty.detail.title") },
      ]}
      actions={
        w ? (
          <div className="flex flex-wrap gap-2">
            {/* TEC-188: the certificate PDF of the service (active only). */}
            {w.status === "active" ? (
              <WarrantyCertificateButton
                client={panelCertificateClient(w.service.uuid)}
                locale={locale}
              />
            ) : null}
            {w.can_void && can(Permission.WarrantiesVoid) ? (
              <Button
                type="button"
                variant="destructive"
                data-testid="void-open"
                onClick={() => setVoidOpen(true)}
              >
                <ShieldOff className="size-4" />
                {t("warranty.void.open")}
              </Button>
            ) : null}
          </div>
        ) : null
      }
    />
  );

  if (!canRead) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("warranty.list.forbidden")}
        />
      </div>
    );
  }
  if (detail.isError) {
    const notFound = isApiError(detail.error) && detail.error.status === 404;
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={
            notFound
              ? t("warranty.detail.not_found")
              : t("common.error_generic")
          }
          onRetry={notFound ? undefined : () => void detail.refetch()}
          retryLabel={t("common.retry")}
        />
      </div>
    );
  }
  if (!w) {
    return (
      <div className="space-y-6">
        {header}
        <p className="text-muted-foreground text-sm">
          {t("warranty.list.loading")}
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-6" data-testid="warranty-detail">
      {header}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-2">
          <CardTitle>{t("warranty.detail.period")}</CardTitle>
          <StatusChip
            label={t(`warranty.status.${w.status}`)}
            tone={warrantyStatusTone(w.status)}
          />
        </CardHeader>
        <CardContent className="space-y-4">
          <WarrantyProgressBar warranty={w} />
          <dl className="grid gap-4 sm:grid-cols-3">
            <Field label={t("warranty.detail.start")}>
              {format.date(w.start_at)}
            </Field>
            <Field label={t("warranty.detail.end")}>
              {format.date(w.end_at)}
            </Field>
            <Field label={t("warranty.detail.kind")}>
              {t(`warranty.kind.${w.item_kind}`)}
            </Field>
          </dl>
          {w.status === "void" ? (
            <div
              className="border-destructive/30 bg-destructive/5 rounded-md border p-3 text-sm"
              data-testid="void-info"
            >
              <p className="font-medium">
                {t("warranty.detail.voided_at", {
                  date: w.voided_at ? format.date(w.voided_at) : "—",
                })}
              </p>
              {w.void_reason ? (
                <p className="text-muted-foreground">{w.void_reason}</p>
              ) : null}
            </div>
          ) : null}
        </CardContent>
      </Card>
      <Card>
        <CardContent className="pt-6">
          <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <Field label={t("warranty.detail.code")}>
              <span className="font-mono" dir="ltr">
                {w.public_code}
              </span>
              <a
                href={`/garanti/${encodeURIComponent(w.public_code)}`}
                target="_blank"
                rel="noreferrer"
                className="text-primary ms-2 inline-flex items-center gap-1 text-xs hover:underline"
              >
                <ExternalLink className="size-3" />
                {t("warranty.detail.public_page")}
              </a>
            </Field>
            <Field label={t("warranty.detail.product")}>
              {w.product.name}{" "}
              <span
                className="text-muted-foreground font-mono text-xs"
                dir="ltr"
              >
                {w.product.sku}
              </span>
            </Field>
            <Field label={t("warranty.detail.vehicle")}>
              {warrantyVehicleTitle(w)}
              {w.vehicle.plate ? (
                <span
                  className="text-muted-foreground ms-2 font-mono text-xs"
                  dir="ltr"
                >
                  {w.vehicle.plate}
                </span>
              ) : null}
            </Field>
            <Field label={t("warranty.detail.holder")}>
              {warrantyHolderName(w) || "—"}
            </Field>
            <Field label={t("warranty.detail.organization")}>
              {w.organization.name}
            </Field>
            <Field label={t("warranty.detail.service")}>
              {can(Permission.ServicesRead) ? (
                <Link
                  href={routes.tenant.services.detail(slug, w.service.uuid)}
                  className="font-mono hover:underline"
                  dir="ltr"
                >
                  {w.service.service_no}
                </Link>
              ) : (
                <span className="font-mono" dir="ltr">
                  {w.service.service_no}
                </span>
              )}
            </Field>
          </dl>
        </CardContent>
      </Card>
      {w.can_void ? (
        <VoidDialog warranty={w} open={voidOpen} onOpenChange={setVoidOpen} />
      ) : null}
    </div>
  );
}
