"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link2, Loader2, Search } from "lucide-react";
import { useState, type FormEvent } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
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
import { LOCALE_NAMES, SUPPORTED_LOCALES } from "@/config/i18n";
import {
  fleetKeys,
  fleetMatchFromError,
  fleetsService,
  type FleetMatch,
  type FleetOpenInput,
} from "@/features/fleets/services/fleets.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type Step = "tax" | "found" | "form";

type FormState = {
  legal_name: string;
  name: string;
  tax_office: string;
  contact_name: string;
  contact_phone: string;
  billing_email: string;
  report_frequency: "monthly" | "quarterly" | "off";
  report_locale: string;
  user_email: string;
  user_name: string;
  user_surname: string;
};

const EMPTY_FORM: FormState = {
  legal_name: "",
  name: "",
  tax_office: "",
  contact_name: "",
  contact_phone: "",
  billing_email: "",
  report_frequency: "monthly",
  report_locale: "tr",
  user_email: "",
  user_name: "",
  user_surname: "",
};

/** VKN (10 digits) or TCKN (11 digits); the server checks the checksum. */
export function isTaxNumberShape(value: string) {
  return /^\d{10,11}$/.test(value.trim());
}

function errorText(error: unknown, fallback: string) {
  return isApiError(error) ? error.message : fallback;
}

/**
 * New fleet (TEC-477): the VKN/TCKN is looked up first. A fleet of the
 * brand with that number gets a link request (the fleet accepts it in the
 * portal); otherwise the opening form (profile + primary user e-mail)
 * opens the fleet with an active link and invites its first user.
 */
export function NewFleetDialog({
  open,
  onOpenChange,
  onOpened,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Called with the new fleet uuid (the page opens its card). */
  onOpened?: (uuid: string) => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [step, setStep] = useState<Step>("tax");
  const [taxNumber, setTaxNumber] = useState("");
  const [match, setMatch] = useState<FleetMatch | null>(null);
  const [form, setForm] = useState<FormState>(EMPTY_FORM);
  const [error, setError] = useState<string | null>(null);

  const reset = () => {
    setStep("tax");
    setTaxNumber("");
    setMatch(null);
    setForm(EMPTY_FORM);
    setError(null);
  };

  const close = (next: boolean) => {
    if (!next) reset();
    onOpenChange(next);
  };

  const lookup = useMutation({
    mutationFn: () => fleetsService.lookup(taxNumber.trim()),
    onSuccess: (found) => {
      setError(null);
      if (found) {
        setMatch(found);
        setStep("found");
      } else {
        setStep("form");
      }
    },
    onError: (e) => setError(errorText(e, t("fleets.new.lookup_failed"))),
  });

  const link = useMutation({
    mutationFn: (uuid: string) => fleetsService.requestLink(uuid),
    onSuccess: async () => {
      appToast.success(t("fleets.new.link_requested"));
      await qc.invalidateQueries({ queryKey: fleetKeys.all });
      close(false);
    },
    onError: (e) => setError(errorText(e, t("fleets.new.link_failed"))),
  });

  const create = useMutation({
    mutationFn: async () => {
      const body: FleetOpenInput = {
        legal_name: form.legal_name.trim(),
        tax_number: taxNumber.trim(),
        report_frequency: form.report_frequency,
        report_locale: form.report_locale,
      };
      const optional = [
        "name",
        "tax_office",
        "contact_name",
        "contact_phone",
        "billing_email",
      ] as const;
      for (const key of optional) {
        const v = form[key].trim();
        if (v) body[key] = v;
      }
      const fleet = await fleetsService.open(body);
      let inviteError: unknown = null;
      if (form.user_email.trim()) {
        try {
          await fleetsService.inviteUser(fleet.uuid, {
            email: form.user_email.trim(),
            name: form.user_name.trim() || form.legal_name.trim(),
            surname: form.user_surname.trim() || undefined,
          });
        } catch (e) {
          inviteError = e;
        }
      }
      return { fleet, inviteError };
    },
    onSuccess: async ({ fleet, inviteError }) => {
      await qc.invalidateQueries({ queryKey: fleetKeys.all });
      if (inviteError) {
        appToast.warning(errorText(inviteError, t("fleets.new.invite_failed")));
      } else {
        appToast.success(t("fleets.new.created"));
      }
      close(false);
      onOpened?.(fleet.uuid);
    },
    onError: (e) => {
      // Opened by another dealer in the meantime: offer the link request.
      const existing = fleetMatchFromError(e);
      if (existing) {
        setMatch(existing);
        setStep("found");
        setError(null);
        return;
      }
      setError(errorText(e, t("fleets.new.create_failed")));
    },
  });

  const onLookup = (event: FormEvent) => {
    event.preventDefault();
    if (!isTaxNumberShape(taxNumber)) {
      setError(t("fleets.new.tax_invalid"));
      return;
    }
    lookup.mutate();
  };

  const onCreate = (event: FormEvent) => {
    event.preventDefault();
    if (!form.legal_name.trim()) {
      setError(t("fleets.new.legal_name_required"));
      return;
    }
    create.mutate();
  };

  const set = <K extends keyof FormState>(key: K, value: FormState[K]) =>
    setForm((prev) => ({ ...prev, [key]: value }));

  const text = (key: keyof FormState, labelKey: string, type = "text") => (
    <div className="space-y-1.5">
      <Label htmlFor={`fleet-${key}`}>{t(labelKey)}</Label>
      <Input
        id={`fleet-${key}`}
        type={type}
        value={form[key]}
        data-testid={`fleet-form-${key}`}
        onChange={(e) => set(key, e.target.value as never)}
      />
    </div>
  );

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="max-h-[90vh] max-w-lg overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{t("fleets.new.title")}</DialogTitle>
          <DialogDescription>{t("fleets.new.description")}</DialogDescription>
        </DialogHeader>

        {error ? (
          <Alert variant="destructive" data-testid="fleet-new-error">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}

        {step === "tax" ? (
          <form className="space-y-4" onSubmit={onLookup}>
            <div className="space-y-1.5">
              <Label htmlFor="fleet-tax-number">
                {t("fleets.fields.tax_number")}
              </Label>
              <Input
                id="fleet-tax-number"
                inputMode="numeric"
                dir="ltr"
                autoFocus
                value={taxNumber}
                data-testid="fleet-tax-number"
                onChange={(e) => setTaxNumber(e.target.value)}
              />
              <p className="text-muted-foreground text-xs">
                {t("fleets.new.tax_hint")}
              </p>
            </div>
            <DialogFooter>
              <Button
                type="submit"
                disabled={lookup.isPending}
                data-testid="fleet-lookup"
              >
                {lookup.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <Search className="size-4" />
                )}
                {t("fleets.new.lookup")}
              </Button>
            </DialogFooter>
          </form>
        ) : null}

        {step === "found" && match ? (
          <div className="space-y-4" data-testid="fleet-found">
            <Alert>
              <AlertTitle>{t("fleets.new.found_title")}</AlertTitle>
              <AlertDescription>
                <span className="font-medium">{match.legal_name}</span>
                {match.name && match.name !== match.legal_name
                  ? ` (${match.name})`
                  : null}
              </AlertDescription>
            </Alert>
            {match.link_status ? (
              <p
                className="text-muted-foreground text-sm"
                data-testid="fleet-found-status"
              >
                {t(`fleets.new.found_link_${match.link_status}`)}
              </p>
            ) : (
              <p className="text-muted-foreground text-sm">
                {t("fleets.new.found_hint")}
              </p>
            )}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  setStep("tax");
                  setMatch(null);
                  setError(null);
                }}
              >
                {t("common.back")}
              </Button>
              {match.link_status === "active" ? (
                <Button
                  type="button"
                  onClick={() => {
                    close(false);
                    onOpened?.(match.fleet_uuid);
                  }}
                >
                  {t("fleets.new.open_card")}
                </Button>
              ) : (
                <Button
                  type="button"
                  disabled={link.isPending || match.link_status === "pending"}
                  data-testid="fleet-request-link"
                  onClick={() => link.mutate(match.fleet_uuid)}
                >
                  {link.isPending ? (
                    <Loader2 className="size-4 animate-spin" />
                  ) : (
                    <Link2 className="size-4" />
                  )}
                  {t("fleets.new.request_link")}
                </Button>
              )}
            </DialogFooter>
          </div>
        ) : null}

        {step === "form" ? (
          <form
            className="space-y-4"
            onSubmit={onCreate}
            data-testid="fleet-open-form"
          >
            <p className="text-muted-foreground text-sm">
              {t("fleets.new.not_found", { tax: taxNumber.trim() })}
            </p>
            {text("legal_name", "fleets.fields.legal_name")}
            {text("name", "fleets.fields.name")}
            <div className="grid gap-4 sm:grid-cols-2">
              {text("tax_office", "fleets.fields.tax_office")}
              {text("contact_name", "fleets.fields.contact_name")}
              {text("contact_phone", "fleets.fields.contact_phone", "tel")}
              {text("billing_email", "fleets.fields.billing_email", "email")}
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label>{t("fleets.fields.report_frequency")}</Label>
                <Select
                  value={form.report_frequency}
                  onValueChange={(v) =>
                    set("report_frequency", v as FormState["report_frequency"])
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {(["monthly", "quarterly", "off"] as const).map((v) => (
                      <SelectItem key={v} value={v}>
                        {t(`fleets.report_frequency.${v}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1.5">
                <Label>{t("fleets.fields.report_locale")}</Label>
                <Select
                  value={form.report_locale}
                  onValueChange={(v) => set("report_locale", v)}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {SUPPORTED_LOCALES.map((code) => (
                      <SelectItem key={code} value={code}>
                        {LOCALE_NAMES[code]}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
            <fieldset className="space-y-3 rounded-md border p-3">
              <legend className="px-1 text-sm font-medium">
                {t("fleets.new.primary_user")}
              </legend>
              <p className="text-muted-foreground text-xs">
                {t("fleets.new.primary_user_hint")}
              </p>
              {text("user_email", "fleets.fields.email", "email")}
              <div className="grid gap-4 sm:grid-cols-2">
                {text("user_name", "fleets.fields.user_name")}
                {text("user_surname", "fleets.fields.user_surname")}
              </div>
            </fieldset>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  setStep("tax");
                  setError(null);
                }}
              >
                {t("common.back")}
              </Button>
              <Button
                type="submit"
                disabled={create.isPending}
                data-testid="fleet-open-submit"
              >
                {create.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : null}
                {t("fleets.new.create")}
              </Button>
            </DialogFooter>
          </form>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
