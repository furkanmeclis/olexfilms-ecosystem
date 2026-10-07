"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  Check,
  Send,
  ShieldAlert,
  Sparkles,
  Upload,
  X,
} from "lucide-react";
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
import { isKnownPart } from "@/features/services/lib/car-parts";
import { ClaimPhotoPicker } from "@/features/warranty-claims/components/claim-photo-picker";
import {
  claimActions,
  claimStatusTone,
  type WarrantyClaim,
  type WarrantyClaimStatus,
} from "@/features/warranty-claims/lib/claims";
import {
  claimKeys,
  claimPhotoUrl,
  claimsService,
} from "@/features/warranty-claims/services/claims.service";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const REJECTION_REASON_MAX = 5000;

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm">{children}</dd>
    </div>
  );
}

/** Warning band of a failed automatic coverage check (never auto-rejects). */
export function CoverageWarning({ claim }: { claim: WarrantyClaim }) {
  const { t } = useLocale();
  if (claim.coverage_check.ok) return null;
  return (
    <div
      role="alert"
      data-testid="coverage-warning"
      className="flex gap-3 rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm"
    >
      <AlertTriangle className="size-5 shrink-0 text-amber-600" />
      <div className="space-y-1">
        <p className="font-medium">{t("warranty.claims.coverage.title")}</p>
        <ul className="list-disc ps-5">
          {claim.coverage_check.reasons.map((reason) => (
            <li key={reason}>
              {t(`warranty.claims.coverage.reasons.${reason}`)}
            </li>
          ))}
        </ul>
        <p className="text-muted-foreground text-xs">
          {t("warranty.claims.coverage.hint")}
        </p>
      </div>
    </div>
  );
}

function RejectDialog({
  open,
  onOpenChange,
  pending,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  pending: boolean;
  onConfirm: (reason: string) => void;
}) {
  const { t } = useLocale();
  const [reason, setReason] = useState("");
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("warranty.claims.reject.title")}</DialogTitle>
          <DialogDescription>
            {t("warranty.claims.reject.description")}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-1.5">
          <Label htmlFor="claim-reject-reason">
            {t("warranty.claims.reject.reason")}
          </Label>
          <Textarea
            id="claim-reject-reason"
            value={reason}
            rows={4}
            maxLength={REJECTION_REASON_MAX}
            onChange={(e) => setReason(e.target.value)}
          />
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("warranty.claims.cancel")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            data-testid="claim-reject-confirm"
            disabled={reason.trim() === "" || pending}
            onClick={() => onConfirm(reason.trim())}
          >
            {t("warranty.claims.actions.reject")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Tenant > Warranty claims > detail (TEC-339): coverage warning band,
 * description and parts, photo gallery, event timeline, AI triage fields
 * (when filled), the linked re-application service, the center's cost
 * summary (only when the API returns it) and the flow actions: submit for
 * review (owner, needs a photo), forward to the center (distributor) and
 * approve / reject with a reason (center).
 */
export function WarrantyClaimDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const qc = useQueryClient();
  const canRead = can(Permission.WarrantyClaimsRead);
  const [rejectOpen, setRejectOpen] = useState(false);
  const [newPhotos, setNewPhotos] = useState<File[]>([]);

  const detail = useQuery({
    queryKey: claimKeys.detail(uuid),
    queryFn: () => claimsService.get(uuid),
    enabled: canRead && uuid !== "",
    retry: (count, err) =>
      !(isApiError(err) && (err.status === 404 || err.status === 403)) &&
      count < 2,
  });

  const onUpdated = (updated: WarrantyClaim) => {
    qc.setQueryData(claimKeys.detail(uuid), updated);
    void qc.invalidateQueries({ queryKey: claimKeys.all });
  };
  const transition = useMutation({
    mutationFn: (v: { status: WarrantyClaimStatus; reason?: string }) =>
      claimsService.transition(uuid, v.status, v.reason),
    onSuccess: (updated) => {
      onUpdated(updated);
      setRejectOpen(false);
      toast.success(t("warranty.claims.toast.updated"));
    },
    onError: (err) => {
      toast.error(
        isApiError(err) && err.code === "CLAIM_PHOTO_REQUIRED"
          ? t("warranty.claims.toast.photo_required")
          : t("warranty.claims.toast.error"),
      );
    },
  });
  const upload = useMutation({
    mutationFn: async (files: File[]) => {
      for (const file of files) await claimsService.addPhoto(uuid, file);
    },
    onSuccess: () => {
      setNewPhotos([]);
      void qc.invalidateQueries({ queryKey: claimKeys.detail(uuid) });
      toast.success(t("warranty.claims.toast.photos_uploaded"));
    },
    onError: () => {
      void qc.invalidateQueries({ queryKey: claimKeys.detail(uuid) });
      toast.error(t("warranty.claims.toast.photo_error"));
    },
  });

  const c = detail.data;
  const listTitle = t("warranty.claims.title");
  const title = c
    ? t("warranty.claims.detail.title", { no: c.claim_no })
    : listTitle;
  const actions = c
    ? claimActions(
        c.status,
        {
          write: can(Permission.WarrantyClaimsWrite),
          review: can(Permission.WarrantyClaimsReview),
          decide: can(Permission.WarrantyClaimsDecide),
        },
        org?.type,
      )
    : null;
  const busy = transition.isPending || upload.isPending;
  const photoCount = c?.photos?.length ?? 0;

  const header = (
    <PageHeader
      title={title}
      icon={<ShieldAlert className="size-6" />}
      description={c ? c.product_name : undefined}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: listTitle, href: routes.tenant.warrantyClaims.list(slug) },
        { label: title },
      ]}
      actions={
        c && actions ? (
          <div className="flex flex-wrap gap-2">
            {actions.submitReview ? (
              <Button
                type="button"
                data-testid="claim-submit-review"
                disabled={busy || photoCount === 0}
                onClick={() => transition.mutate({ status: "dealer_review" })}
              >
                <Send className="size-4 rtl:rotate-180" />
                {t("warranty.claims.actions.submit_review")}
              </Button>
            ) : null}
            {actions.forward ? (
              <Button
                type="button"
                data-testid="claim-forward"
                disabled={busy}
                onClick={() => transition.mutate({ status: "center_review" })}
              >
                <Send className="size-4 rtl:rotate-180" />
                {t("warranty.claims.actions.forward")}
              </Button>
            ) : null}
            {actions.decide ? (
              <>
                <Button
                  type="button"
                  data-testid="claim-approve"
                  disabled={busy}
                  onClick={() => transition.mutate({ status: "approved" })}
                >
                  <Check className="size-4" />
                  {t("warranty.claims.actions.approve")}
                </Button>
                <Button
                  type="button"
                  variant="destructive"
                  data-testid="claim-reject"
                  disabled={busy}
                  onClick={() => setRejectOpen(true)}
                >
                  <X className="size-4" />
                  {t("warranty.claims.actions.reject")}
                </Button>
              </>
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
          description={t("warranty.claims.forbidden")}
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
              ? t("warranty.claims.detail.not_found")
              : t("common.error_generic")
          }
          onRetry={notFound ? undefined : () => void detail.refetch()}
          retryLabel={t("common.retry")}
        />
      </div>
    );
  }
  if (!c) {
    return (
      <div className="space-y-6">
        {header}
        <p className="text-muted-foreground text-sm">
          {t("warranty.claims.loading")}
        </p>
      </div>
    );
  }

  const partName = (part: string) =>
    isKnownPart(part) ? t(`services.parts.names.${part}`) : part;
  const hasAI =
    c.ai_damage_type != null || c.ai_summary != null || c.ai_confidence != null;
  const statusLabel = (s: string) => t(`warranty.claims.status.${s}`);

  return (
    <div className="space-y-6" data-testid="claim-detail">
      {header}
      <CoverageWarning claim={c} />
      {c.status === "rejected" && c.rejection_reason ? (
        <div
          className="border-destructive/30 bg-destructive/5 rounded-md border p-3 text-sm"
          data-testid="claim-rejection"
        >
          <p className="font-medium">
            {t("warranty.claims.detail.rejection_reason")}
          </p>
          <p className="text-muted-foreground whitespace-pre-wrap">
            {c.rejection_reason}
          </p>
        </div>
      ) : null}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-2">
          <CardTitle>{t("warranty.claims.detail.summary")}</CardTitle>
          <StatusChip
            label={statusLabel(c.status)}
            tone={claimStatusTone(c.status)}
          />
        </CardHeader>
        <CardContent className="space-y-4">
          <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <Field label={t("warranty.claims.columns.warranty_no")}>
              <Link
                href={routes.tenant.warranties.detail(slug, c.warranty_uuid)}
                className="font-mono hover:underline"
                dir="ltr"
              >
                {c.warranty_no || "—"}
              </Link>
            </Field>
            <Field label={t("warranty.claims.columns.product")}>
              {c.product_name || "—"}
            </Field>
            <Field label={t("warranty.claims.columns.organization")}>
              {c.organization_name || "—"}
            </Field>
            <Field label={t("warranty.claims.columns.service_no")}>
              {c.service_uuid && can(Permission.ServicesRead) ? (
                <Link
                  href={routes.tenant.services.detail(slug, c.service_uuid)}
                  className="font-mono hover:underline"
                  dir="ltr"
                >
                  {c.service_no}
                </Link>
              ) : (
                <span className="font-mono" dir="ltr">
                  {c.service_no || "—"}
                </span>
              )}
              {c.plate ? (
                <span
                  className="text-muted-foreground ms-2 font-mono text-xs"
                  dir="ltr"
                >
                  {c.plate}
                </span>
              ) : null}
            </Field>
            <Field label={t("warranty.claims.columns.created_at")}>
              {format.dateTime(c.created_at)}
            </Field>
            {c.reapply_service ? (
              <Field label={t("warranty.claims.detail.reapply_service")}>
                <Link
                  href={routes.tenant.services.detail(
                    slug,
                    c.reapply_service.uuid,
                  )}
                  className="font-mono hover:underline"
                  dir="ltr"
                  data-testid="claim-reapply-link"
                >
                  {c.reapply_service.service_no}
                </Link>
              </Field>
            ) : null}
          </dl>
          <div className="space-y-1">
            <p className="text-muted-foreground text-xs">
              {t("warranty.claims.detail.description")}
            </p>
            <p className="text-sm whitespace-pre-wrap">{c.description}</p>
          </div>
          {c.parts && c.parts.length > 0 ? (
            <div className="space-y-1">
              <p className="text-muted-foreground text-xs">
                {t("warranty.claims.detail.parts")}
              </p>
              <ul className="flex flex-wrap gap-2" data-testid="claim-parts">
                {c.parts.map((p) => (
                  <li
                    key={p.uuid}
                    className="bg-muted rounded px-2 py-0.5 text-xs"
                  >
                    {partName(p.part_key)}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
        </CardContent>
      </Card>

      {c.cost_summary ? (
        <Card data-testid="claim-cost-summary">
          <CardHeader>
            <CardTitle>{t("warranty.claims.cost.title")}</CardTitle>
          </CardHeader>
          <CardContent>
            <dl className="grid gap-4 sm:grid-cols-3">
              <Field label={t("warranty.claims.cost.product")}>
                {format.currency(
                  Number(c.cost_summary.product_cost),
                  c.cost_summary.currency,
                )}
              </Field>
              <Field label={t("warranty.claims.cost.labor")}>
                {format.currency(
                  Number(c.cost_summary.labor),
                  c.cost_summary.currency,
                )}
              </Field>
              <Field label={t("warranty.claims.cost.total")}>
                {format.currency(
                  Number(c.cost_summary.product_cost) +
                    Number(c.cost_summary.labor),
                  c.cost_summary.currency,
                )}
              </Field>
            </dl>
          </CardContent>
        </Card>
      ) : null}

      {hasAI ? (
        <Card data-testid="claim-ai">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Sparkles className="size-4" />
              {t("warranty.claims.ai.title")}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <dl className="grid gap-4 sm:grid-cols-3">
              {c.ai_damage_type != null ? (
                <Field label={t("warranty.claims.ai.damage_type")}>
                  {c.ai_damage_type}
                </Field>
              ) : null}
              {c.ai_confidence != null ? (
                <Field label={t("warranty.claims.ai.confidence")}>
                  {format.number(Math.round(Number(c.ai_confidence) * 100))}%
                </Field>
              ) : null}
              {c.ai_summary != null ? (
                <Field label={t("warranty.claims.ai.summary")}>
                  {c.ai_summary}
                </Field>
              ) : null}
            </dl>
          </CardContent>
        </Card>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>
            {t("warranty.claims.photos.title", { count: photoCount })}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {photoCount === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("warranty.claims.photos.empty")}
            </p>
          ) : (
            <ul
              className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4"
              data-testid="claim-gallery"
            >
              {(c.photos ?? []).map((p) => {
                const src = claimPhotoUrl(c.uuid, p.uuid);
                return (
                  <li key={p.uuid}>
                    <a href={src} target="_blank" rel="noreferrer">
                      {/* eslint-disable-next-line @next/next/no-img-element -- authenticated BFF stream */}
                      <img
                        src={src}
                        alt={t("warranty.claims.photos.alt", {
                          date: format.dateTime(p.created_at),
                        })}
                        loading="lazy"
                        className="aspect-square w-full rounded-md border object-cover"
                      />
                    </a>
                  </li>
                );
              })}
            </ul>
          )}
          {c.status === "open" && can(Permission.WarrantyClaimsWrite) ? (
            <div className="space-y-2">
              <ClaimPhotoPicker
                files={newPhotos}
                onChange={setNewPhotos}
                disabled={busy}
              />
              <Button
                type="button"
                variant="outline"
                data-testid="claim-photo-upload"
                disabled={newPhotos.length === 0 || busy}
                onClick={() => upload.mutate(newPhotos)}
              >
                <Upload className="size-4" />
                {t("warranty.claims.photos.upload")}
              </Button>
            </div>
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("warranty.claims.timeline.title")}</CardTitle>
        </CardHeader>
        <CardContent>
          <ol
            className="border-border space-y-4 border-s ps-4"
            data-testid="claim-timeline"
          >
            {(c.events ?? []).map((e) => (
              <li key={e.uuid} className="relative">
                <span className="bg-primary absolute -start-[1.3rem] top-1.5 size-2 rounded-full" />
                <p className="text-sm font-medium">
                  {e.event_type === "status_changed" && e.from_status
                    ? t("warranty.claims.timeline.status_changed", {
                        from: statusLabel(e.from_status),
                        to: statusLabel(e.to_status ?? ""),
                      })
                    : t(`warranty.claims.timeline.events.${e.event_type}`)}
                </p>
                {e.note ? (
                  <p className="text-muted-foreground text-sm whitespace-pre-wrap">
                    {e.note}
                  </p>
                ) : null}
                <p className="text-muted-foreground text-xs">
                  {format.dateTime(e.created_at)}
                </p>
              </li>
            ))}
          </ol>
        </CardContent>
      </Card>

      {actions?.decide ? (
        <RejectDialog
          open={rejectOpen}
          onOpenChange={setRejectOpen}
          pending={transition.isPending}
          onConfirm={(reason) =>
            transition.mutate({ status: "rejected", reason })
          }
        />
      ) : null}
    </div>
  );
}
