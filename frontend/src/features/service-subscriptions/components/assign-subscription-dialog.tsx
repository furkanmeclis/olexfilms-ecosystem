"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";

import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
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
import { serviceCatalogService } from "@/features/service-catalog/services/service-catalog.service";
import {
  amountNumber,
  assignFormReady,
  assignTargets,
  type AssignForm,
} from "@/features/service-subscriptions/lib/subscriptions";
import {
  serviceSubscriptionKeys,
  serviceSubscriptionsService,
} from "@/features/service-subscriptions/services/service-subscriptions.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

function today(): string {
  return new Date().toISOString().slice(0, 10);
}

function assignErrorKey(error: unknown): string {
  if (isApiError(error)) {
    if (error.code === "RATE_NOT_FOUND")
      return "catalog.subscriptions.assign.errors.rate_not_found";
    if (error.code === "MODULE_BLOCKED_BY_PARENT")
      return "catalog.subscriptions.assign.errors.module_blocked";
  }
  return "catalog.subscriptions.assign.errors.failed";
}

/**
 * "Ata" dialog (TEC-311): target organization, catalog item and the
 * required start / end dates. The price is never typed: the API freezes the
 * effective price of the target (distributor override or the center
 * default), shown read-only as a preview.
 */
export function AssignSubscriptionDialog({
  open,
  onOpenChange,
  orgType,
  initial,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  orgType: string | undefined;
  /** Preset values, e.g. a preselected organization. */
  initial?: Partial<AssignForm>;
}) {
  const { t, format } = useLocale();
  const id = useId();
  const queryClient = useQueryClient();
  const [form, setForm] = useState<AssignForm>(() => ({
    organizationUuid: "",
    itemUuid: "",
    startsOn: today(),
    endsOn: "",
    ...initial,
  }));
  const set = (patch: Partial<AssignForm>) =>
    setForm((prev) => ({ ...prev, ...patch }));

  const organizations = useQuery({
    queryKey: serviceSubscriptionKeys.organizations(),
    queryFn: () => serviceSubscriptionsService.listOrganizations(),
    enabled: open,
    staleTime: 5 * 60_000,
  });
  const targets = assignTargets(organizations.data ?? [], orgType);
  const items = useQuery({
    queryKey: serviceSubscriptionKeys.items(),
    queryFn: () => serviceCatalogService.listVisible(),
    enabled: open,
    staleTime: 5 * 60_000,
  });
  const preview = useQuery({
    queryKey: serviceSubscriptionKeys.preview(
      form.itemUuid,
      form.organizationUuid,
    ),
    queryFn: () =>
      serviceSubscriptionsService.previewPrice(
        form.itemUuid,
        form.organizationUuid,
      ),
    enabled: open && Boolean(form.itemUuid && form.organizationUuid),
  });

  const assign = useMutation({
    mutationFn: () =>
      serviceSubscriptionsService.assign({
        organization_uuid: form.organizationUuid,
        item_uuid: form.itemUuid,
        starts_on: form.startsOn,
        ends_on: form.endsOn,
      }),
    onSuccess: () => {
      appToast.success(t("catalog.subscriptions.assign.done"));
      void queryClient.invalidateQueries({
        queryKey: serviceSubscriptionKeys.all,
      });
      onOpenChange(false);
    },
    onError: (error) => appToast.error(t(assignErrorKey(error))),
  });

  const priceText = preview.data
    ? format.currency(amountNumber(preview.data.amount), preview.data.currency)
    : "";
  const datesInvalid =
    Boolean(form.startsOn && form.endsOn) && form.endsOn <= form.startsOn;
  const ready = assignFormReady(form) && !assign.isPending;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="assign-subscription-dialog">
        <DialogHeader>
          <DialogTitle>{t("catalog.subscriptions.assign.title")}</DialogTitle>
          <DialogDescription>
            {t("catalog.subscriptions.assign.description")}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor={`${id}-org`}>
              {t("catalog.subscriptions.fields.organization")}
            </Label>
            <Select
              value={form.organizationUuid}
              onValueChange={(v) => set({ organizationUuid: v })}
            >
              <SelectTrigger id={`${id}-org`} className="w-full">
                <SelectValue
                  placeholder={t("catalog.subscriptions.assign.pick_org")}
                />
              </SelectTrigger>
              <SelectContent>
                {targets.map((o) => (
                  <SelectItem key={o.uuid} value={o.uuid}>
                    {o.name} · {t(`catalog.subscriptions.org_type.${o.type}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor={`${id}-item`}>
              {t("catalog.subscriptions.fields.item")}
            </Label>
            <Select
              value={form.itemUuid}
              onValueChange={(v) => set({ itemUuid: v })}
            >
              <SelectTrigger id={`${id}-item`} className="w-full">
                <SelectValue
                  placeholder={t("catalog.subscriptions.assign.pick_item")}
                />
              </SelectTrigger>
              <SelectContent>
                {(items.data?.items ?? []).map((item) => (
                  <SelectItem key={item.uuid} value={item.uuid}>
                    {item.name} ·{" "}
                    {t(`catalog.services.recurrences.${item.recurrence}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor={`${id}-start`}>
                {t("catalog.subscriptions.fields.starts_on")}
              </Label>
              <DatePicker
                id={`${id}-start`}
                className="w-full"
                value={form.startsOn}
                onChange={(v) => set({ startsOn: v })}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor={`${id}-end`}>
                {t("catalog.subscriptions.fields.ends_on")}
              </Label>
              <DatePicker
                id={`${id}-end`}
                className="w-full"
                value={form.endsOn}
                onChange={(v) => set({ endsOn: v })}
                aria-invalid={datesInvalid}
              />
            </div>
          </div>
          {datesInvalid ? (
            <p className="text-destructive text-sm" role="alert">
              {t("catalog.subscriptions.assign.ends_after_start")}
            </p>
          ) : null}
          <div className="space-y-2">
            <Label htmlFor={`${id}-price`}>
              {t("catalog.subscriptions.fields.price")}
            </Label>
            <Input
              id={`${id}-price`}
              data-testid="assign-price"
              readOnly
              aria-readonly="true"
              tabIndex={-1}
              dir="ltr"
              value={priceText}
              placeholder={t("catalog.subscriptions.assign.price_placeholder")}
            />
            <p className="text-muted-foreground text-xs">
              {orgType === "distributor"
                ? t("catalog.subscriptions.assign.price_locked")
                : t("catalog.subscriptions.assign.price_preview_hint")}
            </p>
          </div>
        </div>
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
            data-testid="assign-submit"
            disabled={!ready}
            onClick={() => assign.mutate()}
          >
            {t("catalog.subscriptions.assign.submit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
