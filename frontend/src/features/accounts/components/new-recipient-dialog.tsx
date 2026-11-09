"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  parsePhoneNumberFromString,
  type CountryCode,
} from "libphonenumber-js";
import { useEffect, useRef } from "react";
import { useFormContext, useWatch } from "react-hook-form";
import { toast } from "sonner";
import { z } from "zod";

import {
  AppForm,
  AppInput,
  AppRadioGroup,
  AppTextarea,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  pendingRecipientKeys,
  pendingRecipientsService,
  type PendingRecipient,
  type PendingRecipientCreateRequest,
} from "@/features/accounts/services/pending-recipients.service";
import { AddressFields, addressIds, useCountries } from "@/features/geo";
import { orderErrorMessage } from "@/features/orders/lib/errors";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "new-recipient-form";
const TAX_RE = /^[0-9]{10,11}$/;

type Translate = (key: string) => string;

/** E.164 of a typed phone (the address country, TR by default) or null. */
export function recipientPhoneE164(
  raw: string,
  countryIso2?: string | null,
): string | null {
  const v = raw.trim();
  if (!v) return null;
  const parsed = parsePhoneNumberFromString(
    v,
    (countryIso2 || "TR").toUpperCase() as CountryCode,
  );
  return parsed?.isValid() ? parsed.number : null;
}

const recipientShape = (t: Translate) =>
  z.object({
    name: z
      .string()
      .trim()
      .min(1, t("organizations.recipients.errors.name_required")),
    phone: z
      .string()
      .trim()
      .min(1, t("organizations.recipients.errors.phone_required")),
    kind: z.enum(["dealer_candidate", "retail_point"]),
    country_id: z.string().optional(),
    province_id: z.string().optional(),
    district_id: z.string().optional(),
    address: z.string().trim(),
    tax_no: z
      .string()
      .trim()
      .refine(
        (v) => v === "" || TAX_RE.test(v),
        t("organizations.recipients.errors.tax_invalid"),
      ),
    tax_office: z.string().trim(),
  });

type RecipientValues = z.infer<ReturnType<typeof recipientShape>>;

/** K29: the phone must parse (libphonenumber) in the picked country. */
export function recipientSchema(
  t: Translate,
  regionOf: (countryId?: string) => string,
) {
  return recipientShape(t).superRefine((v, ctx) => {
    if (v.phone && !recipientPhoneE164(v.phone, regionOf(v.country_id))) {
      ctx.addIssue({
        code: "custom",
        path: ["phone"],
        message: t("organizations.recipients.errors.phone_invalid"),
      });
    }
  });
}

/** Preselects Türkiye (or the first country) while no country is picked. */
function DefaultCountry() {
  const form = useFormContext<RecipientValues>();
  const countries = useCountries();
  const value = useWatch({ control: form.control, name: "country_id" });
  const done = useRef(false);
  useEffect(() => {
    if (done.current || value || !countries.data?.length) return;
    done.current = true;
    const tr = countries.data.find((c) => c.iso2 === "TR");
    if (tr) form.setValue("country_id", String(tr.id));
  }, [countries.data, form, value]);
  return null;
}

/** The phone in E.164 with the picked country as the default region. */
function usePhoneRegion() {
  const countries = useCountries();
  return (countryId?: string) =>
    countries.data?.find((c) => String(c.id) === countryId)?.iso2 ?? "TR";
}

/**
 * TEC-1062: opens a new order recipient (no dealer record yet) from the
 * center's order form. It falls into the waiting pool as a dealer
 * candidate or a retail delivery point; the order can be opened for it at
 * once and the center turns it into a dealer later from the pool.
 */
export function NewRecipientDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: (recipient: PendingRecipient) => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const region = usePhoneRegion();
  const create = useMutation({
    mutationFn: (values: RecipientValues) => {
      const ids = addressIds(values);
      const phone =
        recipientPhoneE164(values.phone, region(values.country_id)) ??
        values.phone.trim();
      const body: PendingRecipientCreateRequest = {
        name: values.name.trim(),
        phone,
        kind: values.kind,
        ...(ids.country_id ? { country_id: ids.country_id } : {}),
        ...(ids.province_id ? { province_id: ids.province_id } : {}),
        ...(ids.district_id ? { district_id: ids.district_id } : {}),
        ...(values.address ? { address: values.address } : {}),
        ...(values.tax_no ? { tax_no: values.tax_no } : {}),
        ...(values.tax_office ? { tax_office: values.tax_office } : {}),
      };
      return pendingRecipientsService.create(body);
    },
    onSuccess: (r) => {
      void qc.invalidateQueries({ queryKey: pendingRecipientKeys.all });
      void qc.invalidateQueries({ queryKey: ["orders"] });
      void qc.invalidateQueries({ queryKey: ["tenant", "organizations"] });
      toast.success(t("organizations.recipients.created", { name: r.name }));
      onCreated(r);
      onOpenChange(false);
    },
    onError: (err) =>
      toast.error(
        orderErrorMessage(err, t, t("organizations.recipients.save_failed")),
      ),
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-h-[90vh] overflow-y-auto sm:max-w-2xl"
        data-testid="new-recipient-dialog"
      >
        <DialogHeader>
          <DialogTitle>{t("organizations.recipients.new_title")}</DialogTitle>
          <DialogDescription>
            {t("organizations.recipients.new_description")}
          </DialogDescription>
        </DialogHeader>
        {open ? (
          <AppForm
            id={FORM_ID}
            schema={recipientSchema(t, region)}
            defaultValues={{
              name: "",
              phone: "",
              kind: "dealer_candidate",
              country_id: "",
              province_id: "",
              district_id: "",
              address: "",
              tax_no: "",
              tax_office: "",
            }}
            onSubmit={async (values) => {
              await create.mutateAsync(values).catch(() => undefined);
            }}
          >
            <DefaultCountry />
            <div className="grid gap-4 sm:grid-cols-2">
              <AppRadioGroup
                name="kind"
                label={t("organizations.recipients.fields.kind")}
                className="sm:col-span-2"
                options={[
                  {
                    value: "dealer_candidate",
                    label: t("organizations.recipients.kinds.dealer_candidate"),
                    description: t(
                      "organizations.recipients.kinds.dealer_candidate_hint",
                    ),
                  },
                  {
                    value: "retail_point",
                    label: t("organizations.recipients.kinds.retail_point"),
                    description: t(
                      "organizations.recipients.kinds.retail_point_hint",
                    ),
                  },
                ]}
              />
              <AppInput
                name="name"
                label={t("organizations.recipients.fields.name")}
                className="sm:col-span-2"
                data-testid="new-recipient-name"
              />
              <AppInput
                name="phone"
                type="tel"
                inputMode="tel"
                dir="ltr"
                label={t("organizations.recipients.fields.phone")}
                description={t("organizations.recipients.fields.phone_hint")}
                placeholder="+905551112233"
                data-testid="new-recipient-phone"
              />
              <div className="hidden sm:block" />
              <AddressFields />
              <AppTextarea
                name="address"
                label={t("organizations.recipients.fields.address")}
                rows={2}
                className="sm:col-span-2"
              />
              <AppInput
                name="tax_no"
                inputMode="numeric"
                dir="ltr"
                label={t("organizations.recipients.fields.tax_no")}
                description={t("organizations.recipients.fields.tax_no_hint")}
              />
              <AppInput
                name="tax_office"
                label={t("organizations.recipients.fields.tax_office")}
              />
            </div>
          </AppForm>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={FORM_ID}
            disabled={create.isPending}
            data-testid="new-recipient-save"
          >
            {t("organizations.recipients.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
