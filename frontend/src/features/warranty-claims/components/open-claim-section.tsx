"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ShieldAlert } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import { toast } from "sonner";

import { StatusChip } from "@/components/common/status-chip";
import { Button } from "@/components/ui/button";
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
import { useFeature } from "@/features/modules/hooks/use-features";
import { CarPartPicker } from "@/features/services/components/car-part-picker";
import {
  serviceWizardKeys,
  serviceWizardService,
} from "@/features/services/services/service-wizard.service";
import { ClaimPhotoPicker } from "@/features/warranty-claims/components/claim-photo-picker";
import {
  canSubmitClaim,
  claimStatusTone,
  isLiveClaim,
  type WarrantyClaim,
} from "@/features/warranty-claims/lib/claims";
import {
  claimKeys,
  claimsService,
} from "@/features/warranty-claims/services/claims.service";
import type { Warranty } from "@/features/warranty/lib/warranty-list";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const CLAIM_DESCRIPTION_MAX = 20000;

/**
 * Warranted parts of the warranty's service item: the service detail lists
 * its warranties (service_item_uuid) and items (applied_parts).
 */
function useWarrantedParts(warranty: Warranty, enabled: boolean) {
  const service = useQuery({
    queryKey: serviceWizardKeys.service(warranty.service.uuid),
    queryFn: () => serviceWizardService.getService(warranty.service.uuid),
    enabled,
  });
  return useMemo(() => {
    const svc = service.data;
    const itemUuid = svc?.warranties?.find(
      (w) => w.uuid === warranty.uuid,
    )?.service_item_uuid;
    const item = svc?.items?.find((i) => i.uuid === itemUuid);
    return {
      itemUuid: item?.uuid ?? null,
      parts: item?.applied_parts ?? [],
      isLoading: service.isLoading,
      isError: service.isError || (Boolean(svc) && !item),
    };
  }, [service.data, service.isError, service.isLoading, warranty.uuid]);
}

/**
 * "Open a claim" dialog (TEC-339): warranted parts on the F1 part picker
 * (only the parts of the warranted item can be picked), a description and
 * photos (several, ≤ 12 MB each). "Submit for review" creates the claim,
 * uploads the photos and moves it to dealer_review; it stays disabled
 * without a photo. A failure after the create leaves an open claim, so the
 * user lands on its detail to finish there.
 */
export function OpenClaimDialog({
  slug,
  warranty,
  open,
  onOpenChange,
}: {
  slug: string;
  warranty: Warranty;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const warranted = useWarrantedParts(warranty, open);
  const [parts, setParts] = useState<string[]>([]);
  const [description, setDescription] = useState("");
  const [photos, setPhotos] = useState<File[]>([]);

  const submit = useMutation({
    mutationFn: async () => {
      const claim = await claimsService.create({
        warranty_uuid: warranty.uuid,
        description: description.trim(),
        parts: parts.map((part_key) => ({
          part_key,
          service_item_uuid: warranted.itemUuid,
        })),
      });
      try {
        for (const file of photos)
          await claimsService.addPhoto(claim.uuid, file);
        await claimsService.transition(claim.uuid, "dealer_review");
      } catch {
        return { claim, complete: false };
      }
      return { claim, complete: true };
    },
    onSuccess: ({ claim, complete }) => {
      void qc.invalidateQueries({ queryKey: claimKeys.all });
      if (complete) toast.success(t("warranty.claims.open.success"));
      else toast.error(t("warranty.claims.open.partial"));
      onOpenChange(false);
      router.push(routes.tenant.warrantyClaims.detail(slug, claim.uuid));
    },
    onError: (err) => {
      toast.error(
        isApiError(err) && err.code === "WARRANTY_CLAIM_EXISTS"
          ? t("warranty.claims.open.exists")
          : t("warranty.claims.open.error"),
      );
    },
  });

  const ready = canSubmitClaim({
    parts,
    description,
    photos: photos.length,
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t("warranty.claims.open.title")}</DialogTitle>
          <DialogDescription>
            {t("warranty.claims.open.description", {
              code: warranty.public_code,
            })}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-5">
          <div className="space-y-2">
            <Label>{t("warranty.claims.open.parts")}</Label>
            {warranted.isLoading ? (
              <p className="text-muted-foreground text-sm">
                {t("warranty.claims.loading")}
              </p>
            ) : warranted.isError || warranted.parts.length === 0 ? (
              <p className="text-destructive text-sm" role="alert">
                {t("warranty.claims.open.no_parts")}
              </p>
            ) : (
              <CarPartPicker
                available={warranted.parts}
                selected={parts}
                onChange={setParts}
                disabled={submit.isPending}
              />
            )}
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="claim-description">
              {t("warranty.claims.open.text")}
            </Label>
            <Textarea
              id="claim-description"
              data-testid="claim-description"
              rows={4}
              maxLength={CLAIM_DESCRIPTION_MAX}
              value={description}
              placeholder={t("warranty.claims.open.text_placeholder")}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label>{t("warranty.claims.photos.label")}</Label>
            <ClaimPhotoPicker
              files={photos}
              onChange={setPhotos}
              disabled={submit.isPending}
            />
          </div>
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
            data-testid="claim-open-submit"
            disabled={!ready || submit.isPending}
            onClick={() => submit.mutate()}
          >
            {t("warranty.claims.actions.submit_review")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Warranty detail block (TEC-339): the live claim of the warranty as a
 * link, or "Open a claim" for users with warranty_claims.write while the
 * warranty_claims module is on.
 */
export function WarrantyClaimSection({
  slug,
  warranty,
}: {
  slug: string;
  warranty: Warranty;
}) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const feature = useFeature(slug, "warranty_claims");
  const canRead = can(Permission.WarrantyClaimsRead);
  const canWrite = can(Permission.WarrantyClaimsWrite);
  const [open, setOpen] = useState(false);
  const claims = useQuery({
    queryKey: claimKeys.byWarranty(warranty.uuid),
    queryFn: () =>
      claimsService.list({
        warranty_uuid: warranty.uuid,
        limit: 5,
        offset: 0,
      }),
    enabled: feature.enabled && canRead,
  });
  if (!feature.enabled || !canRead) return null;
  const live: WarrantyClaim | undefined = claims.data?.items.find((c) =>
    isLiveClaim(c.status),
  );
  return (
    <div
      className="flex flex-wrap items-center gap-3"
      data-testid="warranty-claim-section"
    >
      {live ? (
        <Link
          href={routes.tenant.warrantyClaims.detail(slug, live.uuid)}
          className="inline-flex items-center gap-2 text-sm hover:underline"
          data-testid="warranty-live-claim"
        >
          <ShieldAlert className="size-4" />
          {t("warranty.claims.live", {
            no: live.claim_no,
            date: format.date(live.created_at),
          })}
          <StatusChip
            label={t(`warranty.claims.status.${live.status}`)}
            tone={claimStatusTone(live.status)}
          />
        </Link>
      ) : canWrite && warranty.status !== "void" && claims.isSuccess ? (
        <>
          <Button
            type="button"
            variant="outline"
            data-testid="claim-open"
            onClick={() => setOpen(true)}
          >
            <ShieldAlert className="size-4" />
            {t("warranty.claims.open.button")}
          </Button>
          {open ? (
            <OpenClaimDialog
              slug={slug}
              warranty={warranty}
              open
              onOpenChange={setOpen}
            />
          ) : null}
        </>
      ) : null}
    </div>
  );
}
