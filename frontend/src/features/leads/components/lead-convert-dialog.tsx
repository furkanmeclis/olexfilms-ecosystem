"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { useState, type ReactNode } from "react";
import { toast } from "sonner";

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
import { AsyncCombobox } from "@/components/ui/async-combobox";
import { Checkbox } from "@/components/ui/checkbox";
import { routes } from "@/config/routes";
import { ratesService } from "@/features/exchange-rates/services/rates.service";
import {
  convertBody,
  convertKinds,
  validateConvert,
  type ConvertContext,
  type ConvertForm,
} from "@/features/leads/lib/quotes";
import { leadKeys, type Lead } from "@/features/leads/services/leads.service";
import {
  quotesService,
  type LeadExistingUser,
} from "@/features/leads/services/quotes.service";
import {
  serviceCatalogKeys,
  serviceCatalogService,
} from "@/features/service-catalog/services/service-catalog.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid grid-cols-[140px_1fr] gap-2 text-sm">
      <dt className="text-muted-foreground">{label}</dt>
      <dd>{children}</dd>
    </div>
  );
}

function existingUserOf(err: unknown): LeadExistingUser | null {
  if (!isApiError(err) || err.code !== "LEAD_USER_CONFLICT") return null;
  const body = err.body as
    { data?: { existing_user?: LeadExistingUser } } | undefined;
  return body?.data?.existing_user ?? null;
}

/**
 * "Kazanıldı → dönüştür" (TEC-316/319). The conversion target is the
 * lead's target type; the fields follow it: a customer becomes a draft
 * service (the wizard opens next), a dealer candidate a dealer org (the
 * center picks the distributor), a distributor candidate a distributor org
 * with currency and warehouse preset (center super_admin only).
 */
export function LeadConvertDialog({
  lead,
  slug,
  ctx,
  open,
  onOpenChange,
}: {
  lead: Lead;
  slug: string;
  ctx: ConvertContext;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const center = ctx.orgType === "center";
  const kind = lead.target_type;
  const allowed = convertKinds(ctx).includes(kind);
  const [form, setForm] = useState<ConvertForm>({
    distributor_uuid: "",
    currency: "",
    register_as_warehouse: true,
  });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [existing, setExisting] = useState<LeadExistingUser | null>(null);

  const distributors = useQuery({
    queryKey: serviceCatalogKeys.distributors(),
    queryFn: () => serviceCatalogService.listDistributors(),
    enabled: open && allowed && kind === "dealer_candidate" && center,
  });
  const currencies = useQuery({
    queryKey: ["rates", "currencies"],
    queryFn: () => ratesService.currencies(),
    enabled: open && allowed && kind === "distributor_candidate",
    staleTime: 30 * 60 * 1000,
  });

  const convert = useMutation({
    mutationFn: () =>
      quotesService.convert(lead.uuid, convertBody(kind, form, center)),
    onSuccess: async (out) => {
      qc.setQueryData(leadKeys.detail(lead.uuid), out.lead);
      await qc.invalidateQueries({ queryKey: leadKeys.events(lead.uuid) });
      await qc.invalidateQueries({ queryKey: leadKeys.lists });
      onOpenChange(false);
      toast.success(t("leads.convert.done"));
      if (out.service_draft?.uuid) {
        router.push(
          routes.tenant.services.wizard(slug, out.service_draft.uuid),
        );
      }
    },
    onError: (err) => {
      const user = existingUserOf(err);
      setExisting(user);
      if (!user) {
        toast.error(
          isApiError(err) && err.message
            ? err.message
            : t("leads.convert.error"),
        );
      }
    },
  });

  const submit = () => {
    const local = validateConvert(kind, form, center);
    setErrors(local);
    if (Object.keys(local).length === 0) convert.mutate();
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="lead-convert-dialog" data-kind={kind}>
        <DialogHeader>
          <DialogTitle>{t("leads.convert.title")}</DialogTitle>
          <DialogDescription>
            {t(`leads.convert.description.${kind}`)}
          </DialogDescription>
        </DialogHeader>
        <dl className="space-y-1">
          <Row label={t("leads.convert.target")}>
            <span data-testid="lead-convert-kind">
              {t(`leads.target_type.${kind}`)}
            </span>
          </Row>
        </dl>
        {!allowed ? (
          <p
            className="text-destructive text-sm"
            data-testid="lead-convert-forbidden"
          >
            {t("leads.convert.forbidden")}
          </p>
        ) : null}

        {allowed && kind === "customer" ? (
          <dl className="space-y-1" data-testid="lead-convert-customer">
            <Row label={t("leads.form.customer_user_id")}>
              {lead.customer_user_id ? `#${lead.customer_user_id}` : "—"}
            </Row>
            <Row label={t("leads.form.vehicle_id")}>
              {lead.vehicle_id ? (
                `#${lead.vehicle_id}`
              ) : (
                <span className="text-destructive">
                  {t("leads.convert.vehicle_missing")}
                </span>
              )}
            </Row>
            <p className="text-muted-foreground pt-2 text-xs">
              {t("leads.convert.customer_hint")}
            </p>
          </dl>
        ) : null}

        {allowed && kind !== "customer" ? (
          <dl className="space-y-1" data-testid="lead-convert-org">
            <Row label={t("leads.form.company")}>
              {lead.candidate_company_name ?? "—"}
            </Row>
            <Row label={t("leads.form.contact")}>
              {lead.candidate_contact_name ?? "—"}
            </Row>
            <Row label={t("leads.form.phone")}>
              <span dir="ltr">{lead.candidate_phone_e164 ?? "—"}</span>
            </Row>
            <Row label={t("leads.form.email")}>
              {lead.candidate_email ?? "—"}
            </Row>
          </dl>
        ) : null}

        {allowed && kind === "dealer_candidate" && center ? (
          <div className="space-y-1.5">
            <Label htmlFor="lead-convert-distributor">
              {t("leads.convert.distributor")}
            </Label>
            <AsyncCombobox
              id="lead-convert-distributor"
              data-testid="lead-convert-distributor"
              className="h-10"
              value={form.distributor_uuid}
              aria-invalid={errors.distributor_uuid ? true : undefined}
              placeholder={t("leads.convert.select")}
              options={(distributors.data ?? []).map((d) => ({
                value: d.uuid,
                label: d.name,
              }))}
              onValueChange={(value) =>
                setForm((f) => ({ ...f, distributor_uuid: value }))
              }
            />
            {errors.distributor_uuid ? (
              <p className="text-destructive text-xs">
                {t(errors.distributor_uuid)}
              </p>
            ) : null}
          </div>
        ) : null}

        {allowed && kind === "distributor_candidate" ? (
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label htmlFor="lead-convert-currency">
                {t("leads.convert.currency")}
              </Label>
              <AsyncCombobox
                id="lead-convert-currency"
                data-testid="lead-convert-currency"
                className="h-10"
                value={form.currency}
                aria-invalid={errors.currency ? true : undefined}
                placeholder={t("leads.convert.select")}
                options={(currencies.data?.items ?? []).map((c) => ({
                  value: c.code,
                  label: `${c.code} · ${c.name}`,
                }))}
                onValueChange={(value) =>
                  setForm((f) => ({ ...f, currency: value }))
                }
              />
              {errors.currency ? (
                <p className="text-destructive text-xs">{t(errors.currency)}</p>
              ) : null}
            </div>
            <div className="flex items-center gap-2">
              <Checkbox
                id="lead-convert-warehouse"
                data-testid="lead-convert-warehouse"
                checked={form.register_as_warehouse}
                onCheckedChange={(checked) =>
                  setForm((f) => ({
                    ...f,
                    register_as_warehouse: checked === true,
                  }))
                }
              />
              <Label htmlFor="lead-convert-warehouse" className="font-normal">
                {t("leads.convert.warehouse_preset")}
              </Label>
            </div>
          </div>
        ) : null}

        {existing ? (
          <div
            className="border-destructive/40 bg-destructive/5 space-y-1 rounded-md border p-3 text-sm"
            data-testid="lead-convert-conflict"
          >
            <p className="font-medium">{t("leads.convert.user_conflict")}</p>
            <p>
              {existing.name} {existing.surname}
              {existing.phone_masked ? (
                <span dir="ltr"> · {existing.phone_masked}</span>
              ) : null}
              {existing.email_masked ? ` · ${existing.email_masked}` : null}
            </p>
          </div>
        ) : null}

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("leads.form.cancel")}
          </Button>
          {allowed ? (
            <Button
              type="button"
              data-testid="lead-convert-submit"
              disabled={convert.isPending}
              onClick={submit}
            >
              {t("leads.convert.submit")}
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
